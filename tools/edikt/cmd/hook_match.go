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
	"github.com/spf13/cobra"
)

func newHookMatchCmd() *cobra.Command {
	var path, root string
	var jsonOut bool

	cmd := &cobra.Command{
		Use:   "match",
		Short: "Report the compiled directives that govern a path, and the surface each is delivered on",
		Long: `Report what GOVERNS a path: every compiled directive whose scope covers
it, with the surface that delivers it (topic file or skill package) and the
topic it belongs to.

Grades are PINNED in the delivery map at render time and are never re-derived
here: a consumer that re-derived grade could silently downgrade an invariant
to advisory with nothing reporting the difference.

This verb answers a question about the compiled corpus. It does not inject:
the write-time injection tier that once used this verb (inject-directives-pre)
is retired with the write-time channel, so nothing is delivered per write and there is nothing
to dedup, budget or grade-filter here.

Exit codes:
  0  matched, or a clean no-match
  0  a missing, corrupt or empty delivery map returns no directives; the
     outcome names which, so "nothing governs this path" and "the map is
     absent" are never the same answer`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				root = wd
			}
			gres := hookmatch.MatchGoverning(root, path)
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", " ")
				return enc.Encode(gres)
			}
			for _, e := range gres.Entries {
				fmt.Fprintf(cmd.OutOrStdout(), "%s\t%s\t%s\t%s\n", e.ID, e.Grade, e.Surface, e.Topic)
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&path, "path", "", "file path to resolve (required)")
	cmd.Flags().StringVar(&root, "root", "", "project root (defaults to cwd)")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "emit JSON")
	// --governing is accepted and ignored: it was the flag that selected this
	// (then secondary) behaviour while the injection path was the default.
	// Callers written against it keep working; there is no other behaviour
	// left for the flag to select.
	var governing bool
	cmd.Flags().BoolVar(&governing, "governing", false, "accepted for compatibility; this is the only behaviour now")
	_ = cmd.Flags().MarkHidden("governing")
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
			res := hookmatch.MatchGoverning(root, ".edikt/probe-heartbeat")
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
