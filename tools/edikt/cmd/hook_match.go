package cmd

// hook_match.go — `bin/edikt hook match`, `hook probe`, `hook report`.
//
// These are the tier-2 verbs the injection shims call. The shims stay thin
// bash (INV-001 keeps tier-1 dependency-free); all matching, normalisation and  edikt-guard:allow
// state live here in Go where they are testable.

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/hookcfg"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
	"github.com/spf13/cobra"
)

// classPriority orders a deny message's entries by structural class when
// several directives match the same write. Lower sorts first. An empty or
// unrecognized class (an index rendered before Class existed, or "unknown")
// sorts last — never ahead of a classified entry, since sorting an unknown
// class first would rank it above a known one for no reason.
func classPriority(class string) int {
	switch class {
	case "invariant":
		return 0
	case "adr":
		return 1
	case "guideline":
		return 2
	default:
		return 3
	}
}

func newHookMatchCmd() *cobra.Command {
	var path, grade, sessionID, shim, root string
	var actorFlag, agentTypeFlag, parentSessionFlag, contextFlag string
	var jsonOut, dedup bool

	cmd := &cobra.Command{
		Use:   "match",
		Short: "Report compiled directives covering a path (write-time injection tier)",
		Long: `Report the compiled directives scoped to a path.

Grades are PINNED in the directive index at render time and are never
re-derived here: a consumer that re-derived grade could silently downgrade an
invariant to advisory with nothing reporting the difference.

Exit codes:
  0  matched, or a clean no-match
  0  every fail-open class (missing binary is the caller's problem; a missing,
     corrupt or empty index returns no directives and lets the write proceed)

Fail-open is deliberate — blocking an editor on a governance bug is worse than
missing an injection. It is never SILENT: every invocation is journaled with
its outcome, and 'edikt hook report' names the classes where governance did
not run.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				root = wd
			}
			res := hookmatch.Match(root, path)

			// Presentation-only: order matched entries by structural class
			// (invariant first, then ADR, then guideline, then unknown),
			// stable within a class. DESIGN-QUESTIONS-2026-08-16.md Q2,
			// option 3 — a human reading a multi-directive deny sees the
			// highest-structural-class rule first. Does not change which
			// entries match, whether the write is denied, or dedup/budget
			// semantics below, all of which key on the entry ID set, not
			// its order.
			sort.SliceStable(res.Entries, func(i, j int) bool {
				return classPriority(res.Entries[i].Class) < classPriority(res.Entries[j].Class)
			})

			if grade != "" {
				res.Entries = hookmatch.FilterGrade(res.Entries, grade)
				// Re-derive the outcome after filtering: entries existed but
				// none of this grade is a NO-MATCH for this caller, not a
				// match with zero results. The pre shim asks "is there a MUST
				// here"; answering "matched" with an empty list would make it
				// bounce on nothing.
				if len(res.Entries) == 0 && res.Outcome == hookmatch.OutcomeMatched {
					res.Outcome = hookmatch.OutcomeNoMatch
				}
			}

			// Dedup AFTER grade filtering, so the key covers exactly the
			// directives this caller would act on.
			suppressedByDedup := false
			budgetExhausted := false
			if dedup && res.Outcome == hookmatch.OutcomeMatched {
				budget := hookmatch.BounceBudget
				if cfg, err := hookcfg.Load(root); err != nil {
					// Fail toward the compiled default rather than toward
					// zero: a budget of 0 is disguised enforcement-off,
					// which hookcfg.Load itself refuses to represent — an
					// unreadable config must not silently reach the same
					// state through this path instead.
					fmt.Fprintf(os.Stderr, "edikt hook match: .edikt/config.yaml unreadable (%v); using default bounce budget %d\n", err, budget)
				} else {
					budget = cfg.BounceBudget
				}
				result, err := hookmatch.AlreadyBounced(sessionID, contextFlag, res.Entries, budget)
				if err != nil {
					// Bounce. Failing toward TELLING the agent about a
					// MUST-grade rule is the safe direction.
					fmt.Fprintf(os.Stderr, "edikt hook match: dedup state unavailable (%v); bouncing anyway\n", err)
				} else if result.BudgetExhausted {
					// Entries stay populated: the caller must still
					// surface them, as advisory rather than a deny —
					// "proceeds LOUDLY", not "proceeds unheard".
					budgetExhausted = true
				} else if !result.Bounce {
					suppressedByDedup = true
					res.Entries = nil
				}
			}

			// Journal every invocation. Its error is ignored on purpose: a
			// journal that cannot be written must not break the write it is
			// observing. `hook report` compensates by reporting staleness.
			ids := make([]string, 0, len(res.Entries))
			for _, e := range res.Entries {
				ids = append(ids, e.ID)
			}
			_ = hookmatch.Append(hookmatch.Record{
				Shim:      shim,
				Outcome:   string(res.Outcome),
				Detail:    res.Detail,
				Path:      res.NormPath,
				Matched:   len(res.Entries),
				SessionID: sessionID,

				Actor:           actorFlag,
				AgentType:       agentTypeFlag,
				ParentSessionID: parentSessionFlag,
				ContextID:       contextFlag,
				DirectiveIDs:    ids,
				// BOUNCED means the deny actually fired. A match suppressed by
				// dedup delivered nothing new: the agent was already told, and
				// counting it as a bounce would inflate the one number that is
				// near-proof a rule changed an outcome. A budget-exhausted
				// match is neither: it delivered something new (advisory),
				// but did not deny.
				Bounced:         shim == "pre" && len(res.Entries) > 0 && !suppressedByDedup && !budgetExhausted,
				DedupSuppressed: suppressedByDedup,
				BudgetExhausted: budgetExhausted,
			})

			if jsonOut {
				out := struct {
					hookmatch.Result
					DedupSuppressed bool `json:"dedup_suppressed"`
					BudgetExhausted bool `json:"budget_exhausted"`
				}{res, suppressedByDedup, budgetExhausted}
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", " ")
				return enc.Encode(out)
			}
			for _, e := range res.Entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\n", e.ID, e.Grade, e.Text)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "file path being written (required)")
	cmd.Flags().StringVar(&grade, "grade", "", "filter to one grade: must | advisory")
	cmd.Flags().StringVar(&sessionID, "session", "", "session id, for once-per-session dedup")
	cmd.Flags().StringVar(&shim, "shim", "probe", "which shim is calling: pre | post | probe")
	cmd.Flags().StringVar(&root, "root", "", "project root (defaults to cwd)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	cmd.Flags().BoolVar(&dedup, "dedup", false, "suppress a repeat bounce for the same session+directives")
	// NEVER INFERRED. A resolver that guessed "parent" when it could not tell
	// would make every subagent look like the session that spawned it, and in
	// this repo one corpus pass is ~70 subagent writes to governed paths.
	cmd.Flags().StringVar(&actorFlag, "actor", "unknown", "who is being governed: parent | subagent | unknown")
	cmd.Flags().StringVar(&agentTypeFlag, "agent-type", "", "subagent slug, when the caller can resolve one")
	cmd.Flags().StringVar(&parentSessionFlag, "parent-session", "", "the spawning session id, for a subagent")
	cmd.Flags().StringVar(&contextFlag, "context", "", "the RECEIVING context id (agent_id, or empty when unresolvable — empty never suppresses)")
	_ = cmd.MarkFlagRequired("path")
	return cmd
}

func newHookProbeCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "probe",
		Short: "Prove the injection chain is live (writes a heartbeat record)",
		Long: `Run one no-op match and journal it.

This exists so 'the chain has never run' is distinguishable from 'the chain ran
and nothing applied'. Without a heartbeat, an unwired hook and a clean project
produce the same empty journal.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				wd, _ := os.Getwd()
				root = wd
			}
			res := hookmatch.Match(root, ".edikt/probe-heartbeat")
			_ = hookmatch.Append(hookmatch.Record{
				Shim:    "probe",
				Outcome: string(res.Outcome),
				Detail:  res.Detail,
				Path:    res.NormPath,
			})
			fmt.Fprintf(cmd.OutOrStdout(), "hook chain probe: outcome=%s", res.Outcome)
			if res.Detail != "" {
				fmt.Fprintf(cmd.OutOrStdout(), " (%s)", res.Detail)
			}
			fmt.Fprintln(cmd.OutOrStdout())
			if res.Outcome.Suppressed() {
				// Exit non-zero: a probe exists to be believed, and a probe
				// that reports a suppressed chain while exiting 0 would be
				// green in any CI that checks only the exit code.
				return fmt.Errorf("injection chain SUPPRESSED: %s — %s", res.Outcome, res.Detail)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "project root (defaults to cwd)")
	return cmd
}

func newHookReportCmd() *cobra.Command {
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Report injection-chain activity and suppression counts",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := hookmatch.ReadSummary()
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", " ")
				return enc.Encode(s)
			}
			fmt.Fprint(cmd.OutOrStdout(), s.Report())
			return nil
		},
	}
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	return cmd
}
