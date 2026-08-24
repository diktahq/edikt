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

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
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
	cmd.AddCommand(newPhaseAPlanCmd(), newPhaseARestoreCmd(), newPhaseAVerifyCmd())
	return cmd
}

func newPhaseAPlanCmd() *cobra.Command {
	var force bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Compute the Phase A work-list and snapshot pinned state",
		Long: `Compute the dispatch plan and write it to .edikt/state/.

--force checks the item count against phasea.force_ceiling instead of
phasea.ceiling (D3). It does NOT widen what gets planned: the stale-sidecar
detector is the same either way, so a forced run plans the same work-list
against a higher budget.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveEdiktRoot()
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			out, err := phasea.Plan(phasea.PlanOptions{ProjectRoot: root, Force: force})
			return emitPhaseAOutcome(cmd, out.ExitCode, out.Message, err)
		},
	}
	cmd.Flags().BoolVar(&force, "force", false, "check the item count against phasea.force_ceiling")
	return cmd
}

func newPhaseARestoreCmd() *cobra.Command {
	var planID string
	cmd := &cobra.Command{
		Use:   "restore",
		Short: "Reinstate human-pinned state onto regenerated sidecars",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveEdiktRoot()
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
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Judge a completed Phase A dispatch (read-only)",
		Long: `Compare regenerated sidecars against the plan and emit a receipt.

Performs no writes to any sidecar. Exit codes: 0 ok, 1 items failed,
2 usage/plan error, 3 receipt unwritable, 5 failed-terminal (D9) — stop and
report rather than retry.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveEdiktRoot()
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			out, err := phasea.Verify(phasea.VerifyOptions{ProjectRoot: root, PlanID: planID})
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
	_ = cmd.MarkFlagRequired("plan")
	return cmd
}

// emitPhaseAOutcome propagates an Outcome's own ExitCode rather than deriving
// a fresh one. A Go-level error with no outcome is a usage/internal failure
// and exits 2; it is never collapsed into 1, because 1 means "items failed"
// and tier-1 retries on it.
func emitPhaseAOutcome(cmd *cobra.Command, code int, msg string, err error) error {
	if err != nil {
		return &exitCodeError{code: 2, msg: err.Error()}
	}
	if msg != "" {
		fmt.Fprintln(cmd.OutOrStdout(), msg)
	}
	if code != 0 {
		return &exitCodeError{code: code, msg: msg}
	}
	return nil
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
