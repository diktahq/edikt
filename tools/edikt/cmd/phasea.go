package cmd

// phasea.go — `bin/edikt phasea plan|restore|verify` (PLAN-v080 Phase 4,
// SPEC-013, owner decision D6).
//
// These three verbs are the tier-2 half of the Phase A rewrite. Go computes
// the work-list, reinstates human-pinned state, and judges the result; the
// FAN-OUT ITSELF IS NOT HERE and must never be. Dispatching a sidecar
// extractor means invoking an LLM, which INV-012 forbids a tier-2 binary from
// doing — that is the whole reason ClaudeRunner is being deleted rather than
// moved. Tier-1 markdown owns the dispatch (Phase 5); these verbs are what it
// calls before and after.
//
// Division of labour is D6's, verbatim: `plan` captures the pin-set
// (mandatory, so nothing can dispatch without a snapshot to restore from),
// `restore` performs the mutation, and `verify` stays READ-ONLY and only
// compares. Keeping the mutation out of verify is what lets a caller run
// verify repeatedly — including after a failure — without it changing the
// thing it is judging.
//
// Exit codes are the entire consumable output (ADR-029): tier-1 acts on the
// code, never on parsed prose. Each Outcome carries its own ExitCode and this
// file propagates it unchanged rather than re-deriving one, so there is a
// single place where a code is decided.

import (
	"fmt"
	"os"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/reextract"
	"github.com/spf13/cobra"
)

func newPhaseACmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "phasea",
		Short: "Plan, restore and verify Phase A sidecar extraction",
		Long: `Tier-2 support for Phase A sidecar extraction.

plan     compute the work-list and snapshot every human-pinned field
restore  reinstate pinned state onto regenerated sidecars
verify   judge the result — read-only, safe to re-run

The dispatch itself is not here. Fanning extractors out means invoking an
LLM, which this binary does not do; the slash command that invoked it owns
that step and calls these verbs around it.`,
	}
	cmd.AddCommand(newPhaseAPlanCmd(), newPhaseARestoreCmd(), newPhaseAVerifyCmd(), newPhaseAPruneStaleCmd())
	return cmd
}

func newPhaseAPruneStaleCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "prune-stale",
		Short: "Remove unconsumed plans written by an older schema (D57 (4))",
		Long: `Deletes unconsumed .edikt/state/phasea/*.plan.json files (and their
sibling receipt / restore-receipt / pin-snapshot state) whose schema_version
predates the version this binary writes — dead files a project reconciliation
step (/edikt:upgrade) leaves behind once a binary upgrade lands. A consumed
plan is finished business and is never touched; a plan at the current schema
may still be mid-fan-out and is never touched either.

Prints one line per plan removed. Exit 0 whether or not anything was removed;
non-zero only on an I/O error while deleting.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolvePhaseAProjectRoot()
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			n, err := removeStalePlans(root, cmd.OutOrStdout())
			if err != nil {
				return &exitCodeError{code: 1, msg: err.Error()}
			}
			if n == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "no stale plans found")
			}
			return nil
		},
	}
	return cmd
}

func newPhaseAPlanCmd() *cobra.Command {
	var force bool
	var artifact string
	var bootstrap bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute the Phase A work-list and snapshot pinned state",
		Long: `Compute the dispatch plan and write it to .edikt/state/.

EXIT CODES — one table for the whole phasea group, because the same number
means different things per verb and a caller that assumes otherwise retries a
refusal or accepts a loss:

  plan     0 a plan was written (its path is printed)
           1 nothing to plan — the corpus is current; a NORMAL outcome
           4 refused (ceiling, unresolvable root, extractor definition absent,
             a terminal plan_id on the corpus path, an unclosed plan whose
             sidecar has moved — the message names the plan and the remedy)
  restore  0 pinned state reinstated
           2 the plan was rejected (id mismatch, consumed, FAILED, unreadable)
           3 the plan is malformed
  verify   0 every item passed; the plan is consumed
           1 items are incomplete — retryable, re-dispatch and verify again
           2 the plan was rejected      3 the plan is malformed
           5 an item failed terminally — do NOT re-dispatch against this plan
           6 an item diverged — worse than the previous attempt; stop
           7 no restore receipt for this plan — run restore, then verify again
  (any)    2 also covers a usage or internal error with no outcome

A NORMAL outcome prints on stdout and nothing prefixes it with "error:"; a
failure prints once, on stderr.

A plan_id that is already terminal (consumed, or FAILED) is never written
again: with --artifact the plan is a successor that names the plan it
supersedes; without it the plan is refused.

--force checks the item count against phasea.force_ceiling instead of
phasea.ceiling (D3). It does NOT widen what gets planned: the stale-sidecar
detector is the same either way, so a forced run plans the same work-list
against a higher budget.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolvePhaseAProjectRoot()
			if err != nil {
				return &exitCodeError{code: phasea.ExitRefused, msg: err.Error()}
			}
			out, err := phasea.Plan(phasea.PlanOptions{ProjectRoot: root, Force: force, Artifact: artifact, Bootstrap: bootstrap,
				ContractLead: func(r string) string { return reextract.ReportInstalledContract(r).Summary }})
			if err == nil && out != nil && out.ExitCode == phasea.ExitPlanEmitted && out.Plan != nil && len(out.Plan.Items) > 0 {
				if line := contractStateReport(out.Plan.Items[0].PromptVersion); line != "" {
					fmt.Fprintln(cmd.ErrOrStderr(), line)
				}
			}
			// Nothing to plan is an ANSWER, not a failure: the corpus is
			// current. It keeps its own exit code so tier-1 can branch on it.
			return emitPhaseAOutcomeNormal(cmd, out.ExitCode, out.Message, err, phasea.ExitNothingToPlan)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "check the item count against phasea.force_ceiling")
	cmd.Flags().StringVar(&artifact, "artifact", "", "plan exactly this artifact (path), reason `requested`, whether or not it is stale (SPEC-013 SR-011)")
	cmd.Flags().BoolVar(&bootstrap, "bootstrap", false, "declare that --artifact genuinely has NO prior sidecar. Without it, a requested extraction over an absent sidecar is REFUSED: it would destroy any manual_directives the sidecar carried, and only you can tell a first extraction from a lost one (D414)")
	return cmd
}

// contractStateReport is the one stderr line `phasea plan` prints when the
// extraction contract the plan pins is not recorded as validated in this
// build: "" for a validated contract.
//
// A REPORT, NOT A REFUSAL. Only `gov reextract` consulted the validation
// record, so every per-artifact compile planned against a contract recorded as
// rejected without a word. Refusing here instead would also stop `gov compile`
// (which plans through the same function) and the very validation run that
// could clear the record, so the plan is written, exit 0, and the state is
// named. The full reasoning lives in the record; this line points at it.
func contractStateReport(promptVersion string) string {
	r := reextract.ReportContract(promptVersion)
	if r.State == reextract.ContractStateValidated {
		return ""
	}
	return "phasea plan: " + r.Summary + "; the plan was written anyway (reported, not refused)"
}

func newPhaseARestoreCmd() *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Reinstate human-pinned state onto regenerated sidecars",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolvePhaseAProjectRoot()
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			out, err := phasea.Restore(phasea.RestoreOptions{ProjectRoot: root, PlanID: planID})
			if out != nil {
				// PinsRestored counts work actually done, so it is worth
				// printing even on a non-zero exit: "restored 0" and
				// "restored 12 then failed" are different situations.
				fmt.Fprintf(cmd.OutOrStdout(), "pins restored: %d\n", out.PinsRestored)
				for _, q := range out.Quarantined {
					fmt.Fprintf(cmd.ErrOrStderr(), "quarantined: %s\n", q)
				}
			}
			return emitPhaseARestoreOutcome(cmd, out, err)
		},
	}
	cmd.Flags().StringVar(&planID, "plan", "", "plan_id to restore (required)")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

func newPhaseAVerifyCmd() *cobra.Command {
	var planID string
	var stalled []string
	var onLoss string
	var extractorModel string
	var rejectedPin []string
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Judge a completed Phase A dispatch (read-only)",
		Long: `Compare regenerated sidecars against the plan and emit a receipt.

Performs no writes to any sidecar. Exit codes: 0 ok, 1 items failed,
2 usage/plan error, 3 receipt unwritable, 5 failed-terminal (D9) — stop and
report rather than retry, 6 diverged — worse than the previous attempt, stop,
7 no restore receipt for this plan — run restore, then verify again.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolvePhaseAProjectRoot()
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			rejected := make(map[string]string, len(rejectedPin))
			for _, kv := range rejectedPin {
				id, errText, ok := strings.Cut(kv, "=")
				if !ok || id == "" || errText == "" {
					return &exitCodeError{code: 2, msg: fmt.Sprintf("--rejected-pin %q: expected <item_id>=<host error text>", kv)}
				}
				rejected[id] = errText
			}
			out, err := phasea.Verify(phasea.VerifyOptions{
				ProjectRoot: root, PlanID: planID, Stalled: stalled, OnLoss: onLoss,
				ExtractorModel: extractorModel, RejectedPin: rejected,
			})
			if out != nil {
				// Details is the difference between a report a human can act
				// on and a bare status word; the receipt schema has nowhere
				// to put it, so it is printed rather than persisted.
				for id, detail := range out.Details {
					fmt.Fprintf(cmd.ErrOrStderr(), "%s: %s\n", id, detail)
				}
			}
			return emitPhaseAVerifyOutcome(cmd, out, err)
		},
	}
	cmd.Flags().StringVar(&planID, "plan", "", "plan_id to verify (required)")
	cmd.Flags().StringVar(&onLoss, "on-loss", "abort", "a re-extraction whose output dropped a directive or prohibition the prior sidecar carried (a lifted item's migration_preserved directive, or any prior body whose source prose remains): abort (lossless-loss, retryable) | accept (record the loss; the item passes)")
	cmd.Flags().StringSliceVar(&stalled, "stalled", nil, "item_ids the dispatcher reports as ended without a result; recorded as `stalled`, retryable (SPEC-013 SR-013)")
	cmd.Flags().StringVar(&extractorModel, "extractor-model", "", "self-reported model that actually performed the dispatch, from the calling session's own context (only source that can be right under extractor.model: inherit)")
	cmd.Flags().StringSliceVar(&rejectedPin, "rejected-pin", nil, "<item_id>=<host error text>: the host rejected this item's explicit extractor.model pin; recorded failed-terminal on first observation, never retried")
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

// emitPhaseAOutcome propagates an Outcome's own ExitCode rather than deriving
// a fresh one. A Go-level error with no outcome is a usage/internal failure
// and exits 2; it is never collapsed into 1, because 1 means "items failed"
// and tier-1 retries on it.
func emitPhaseAOutcome(cmd *cobra.Command, code int, msg string, err error) error {
	return emitPhaseAOutcomeNormal(cmd, code, msg, err)
}

// emitPhaseAOutcomeNormal is emitPhaseAOutcome plus the codes that are NORMAL
// outcomes rather than failures for this verb.
//
// A message is printed ONCE. A normal outcome goes to stdout and carries no
// error text, so nothing re-prints it as `error:` — `plan` on a corpus with
// nothing stale used to print "nothing to plan …" on stdout and then again on
// stderr prefixed `error:`, which reads as a failure and is not one (D81 c).
// A failure's message goes to stderr, through the exit-code error, and not to
// stdout at all.
func emitPhaseAOutcomeNormal(cmd *cobra.Command, code int, msg string, err error, normal ...int) error {
	// D119: this used to hardcode code:2 here, discarding the outcome's own
	// ExitCode on every call — and the runner returns a non-nil err
	// alongside EVERY non-OK outcome (verify.go's `return out, nil` only on
	// ExitVerifyOK; plan.go's refusals return `errors.New(msg)` the same
	// way), so this branch fired on every failure and 2 was the only
	// non-zero code the CLI could ever produce. `code` already carries the
	// outcome's real exit code when there IS an outcome; err's presence
	// here means "this is a failure, not a normal outcome" — it does not
	// mean "re-derive the code".
	//
	// code==0 alongside a non-nil err is the one case that still forces 2:
	// it means there was no real Outcome to read a code from at all (e.g.
	// resolvePhaseAProjectRoot failing before any Plan/Restore/Verify ran),
	// so 0 is the zero value, not a claim of success — and it must never
	// surface as 0 (success) or silently fall through to whatever `normal`
	// happens to classify it as.
	if err != nil {
		if code == 0 {
			return &exitCodeError{code: 2, msg: err.Error()}
		}
		return &exitCodeError{code: code, msg: err.Error()}
	}
	isNormal := code == 0
	for _, n := range normal {
		if code == n {
			isNormal = true
		}
	}
	if isNormal {
		if msg != "" {
			fmt.Fprintln(cmd.OutOrStdout(), msg)
		}
		if code == 0 {
			return nil
		}
		// A normal outcome with a non-zero code: the code is the contract
		// (tier-1 reads it), the message is already on stdout.
		return &exitCodeError{code: code, msg: ""}
	}
	return &exitCodeError{code: code, msg: msg}
}

func emitPhaseARestoreOutcome(cmd *cobra.Command, out *phasea.RestoreOutcome, err error) error {
	if out == nil {
		return emitPhaseAOutcome(cmd, 2, "", err)
	}
	return emitPhaseAOutcome(cmd, out.ExitCode, out.Message, err)
}

func emitPhaseAVerifyOutcome(cmd *cobra.Command, out *phasea.VerifyOutcome, err error) error {
	if out == nil {
		return emitPhaseAOutcome(cmd, 2, "", err)
	}
	return emitPhaseAOutcome(cmd, out.ExitCode, out.Message, err)
}

// resolvePhaseAProjectRoot is the PROJECT root: the directory holding
// .edikt/config.yaml, found by walking up from cwd — the same boundary the
// hooks use. It is deliberately NOT resolveEdiktRoot(): that is the STATE
// root (EDIKT_ROOT > EDIKT_HOME > ~/.edikt), and passing it as ProjectRoot
// sent every Plan/Restore/Verify path — the extractor definition under
// .claude/agents/, the corpus, .edikt/state/phasea/ — to ~/.edikt in any
// project where EDIKT_ROOT was not the checkout. Masked in edikt-dev, live for
// every user (D40; bok's verbatim exit 4 on 2026-09-02).
func resolvePhaseAProjectRoot() (string, error) {
	root, err := findProjectRoot()
	if err != nil {
		return "", fmt.Errorf("resolving project root: %w", err)
	}
	if root == "" {
		cwd, _ := os.Getwd()
		return "", fmt.Errorf("no edikt project here: no .edikt/config.yaml in %s or any parent — run from inside the project", cwd)
	}
	return root, nil
}
