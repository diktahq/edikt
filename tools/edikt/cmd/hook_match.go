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

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/injectmatch"
	"github.com/spf13/cobra"
)

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
			// ONE implementation, shared with the Go handler that replaced
			// inject-directives-pre.sh. This used to be the only copy, and the
			// shims reached it by shelling out to the binary they already ran
			// inside; the handler now calls the same function directly. See
			// injectmatch's package comment for why a second copy in the handler
			// would have drifted in the direction nobody watches.
			out := injectmatch.Run(injectmatch.Request{
				Root:            root,
				Path:            path,
				Grade:           grade,
				SessionID:       sessionID,
				Shim:            shim,
				Dedup:           dedup,
				Actor:           actorFlag,
				AgentType:       agentTypeFlag,
				ParentSessionID: parentSessionFlag,
				ContextID:       contextFlag,
			}, os.Stderr)
			res := out.Result

			if jsonOut {
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
