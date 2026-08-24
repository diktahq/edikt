package gov

// post_flight_render.go — `bin/edikt gov post-flight-render`, replacing the
// inline Markdown renderer in commands/sdlc/post-flight.md (PLAN-v080 Phase 24).
//
// A RENDERER, NOT A DECISION. It reads a composite report and prints the
// human-readable version. Nothing here chooses what goes in the report or what
// any status means — that stays in the command body, which is the line the
// verify-diff invariant draws: a verb may do a well-defined thing the shell
// cannot, and may not absorb a decision.
//
// A MISSING SUMMARY IS RENDERED AS MISSING. The predecessor indexed
// data["l1_summary"] directly and would have raised on an absent one; here it
// prints "(absent)". A report that lost a layer should say so in the document
// people read, rather than failing at render time and leaving no document at
// all — the layer's absence IS the finding.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	var reportPath, outPath string
	cmd := &cobra.Command{
		Use:   "post-flight-render",
		Short: "Render a composite post-flight report as Markdown",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			b, err := os.ReadFile(reportPath) //nolint:gosec // operator-supplied path
			if err != nil {
				return err
			}
			var d map[string]any
			if err := json.Unmarshal(b, &d); err != nil {
				return fmt.Errorf("%s is not valid JSON: %w", reportPath, err)
			}
			var out strings.Builder
			meta, _ := d["meta"].(map[string]any)
			fmt.Fprintf(&out, "# Post-flight report — %v phase %v\n\n", meta["plan"], meta["phase"])
			fmt.Fprintf(&out, "Ran at: %v  (%v)\n\n", meta["ran_at"], meta["dispatch_mode"])

			for _, layer := range []string{"l1", "l2", "l3"} {
				s, ok := d[layer+"_summary"].(map[string]any)
				if !ok {
					fmt.Fprintf(&out, "## %s: (absent)\n", strings.ToUpper(layer))
					continue
				}
				fmt.Fprintf(&out, "## %s: %v\n", strings.ToUpper(layer), s["status"])
				if reason, has := s["reason"]; has {
					fmt.Fprintf(&out, "  %v\n", reason)
				}
			}

			findings, _ := d["findings"].([]any)
			if len(findings) == 0 {
				fmt.Fprint(&out, "\n## Findings: none\n")
			} else {
				fmt.Fprintf(&out, "\n## Findings (%d)\n\n", len(findings))
				for _, raw := range findings {
					f, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					srcs := make([]string, 0)
					if arr, ok := f["sources"].([]any); ok {
						for _, s := range arr {
							srcs = append(srcs, fmt.Sprintf("%v", s))
						}
					}
					fmt.Fprintf(&out, "- **%v** `%v:%v` [%s] %v — %v\n",
						f["severity"], f["file"], f["line_range"],
						strings.Join(srcs, ", "), f["issue_class"], f["description"])
				}
			}

			if outPath == "" {
				_, err = fmt.Fprint(cmd.OutOrStdout(), out.String())
				return err
			}
			return os.WriteFile(outPath, []byte(out.String()), 0o644) //nolint:gosec // report artefact
		},
	}
	cmd.Flags().StringVar(&reportPath, "report", "", "composite report JSON (required)")
	cmd.Flags().StringVar(&outPath, "out", "", "write here instead of stdout")
	_ = cmd.MarkFlagRequired("report")
	Cmd.AddCommand(cmd)
}
