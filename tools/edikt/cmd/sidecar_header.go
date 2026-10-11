package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"gopkg.in/yaml.v3"
)

// The IDE schema hint every PRD/spec/gov sidecar template opens with. It is a
// YAML comment, so any writer that round-trips the document through a plain
// data model (PyYAML safe_dump, a map[string]any) drops it and nothing fails:
// the file is still valid YAML, still validates against the schema, and the
// editor silently loses autocomplete. Two guards close that:
//   - saveSidecar (the one writer the prd/spec verbs share) re-attaches the
//     header the file had before the write;
//   - `util schema-validate --require-schema-header` reports its absence as a
//     finding, so a validation step cannot pass a sidecar that lost it.
//
// These delegate to internal/sidecar so tier-2's Go writers and the command
// bodies' verbs share ONE implementation. A second copy is how the property
// drifts: the writer that keeps the header and the validator that demands it
// have to agree about what a header is.
func schemaHeaderOf(b []byte) string { return sidecar.SchemaHeaderOf(b) }

func schemaHeaderFinding(b []byte) string { return sidecar.SchemaHeaderFinding(b) }

func withSchemaHeader(header string, out []byte) []byte { return sidecar.WithSchemaHeader(header, out) }

// newUtilYAMLRecordCmd — `util yaml record <sidecar>`: append one
// revision_history entry and clear _sync, through the same writer the prd
// verbs use. The review commands (prd-review, spec-review) used to do this by
// hand or through a PyYAML heredoc, which is where headers went missing.
func newUtilYAMLRecordCmd() *cobra.Command {
	var action, note, affectedCSV string
	var audit auditFlags
	cmd := &cobra.Command{
		Use:   "record <sidecar.yaml>",
		Short: "Append a revision_history entry to a PRD/spec sidecar and clear _sync",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if strings.TrimSpace(action) == "" || strings.TrimSpace(note) == "" {
				return fmt.Errorf("util yaml record: --action and --note are required")
			}
			doc, root, err := loadSidecar(args[0])
			if err != nil {
				return err
			}
			var affected []string
			for _, a := range strings.Split(affectedCSV, ",") {
				if a = strings.TrimSpace(a); a != "" {
					affected = append(affected, a)
				}
			}
			appendHistory(root, audit.at, audit.author, action, note, affected)
			clearSync(root)
			return saveSidecar(args[0], doc)
		},
	}
	cmd.Flags().StringVar(&action, "action", "", "revision_history action, e.g. review (required)")
	cmd.Flags().StringVar(&note, "note", "", "revision_history note (required)")
	cmd.Flags().StringVar(&affectedCSV, "affected", "", "comma-separated ids the entry affects")
	audit.bind(cmd)
	return cmd
}

// requireSchemaHeader is the validate step's finding: exit 1 naming what is
// wrong with the header, or nil.
func requireSchemaHeader(path string, b []byte, w *os.File) error {
	if f := schemaHeaderFinding(b); f != "" {
		fmt.Fprintf(w, "SIDECAR HEADER: %s — %s\n", path, f)
		return fmt.Errorf("%s: %s", path, strings.SplitN(f, " —", 2)[0])
	}
	return nil
}

var _ = yaml.Marshal // the helpers above are shared with prd.go's yaml.Node writer
