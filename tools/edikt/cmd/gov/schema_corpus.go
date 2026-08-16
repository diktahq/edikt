package gov

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

// D45 call site (b) — THE CORPUS-WIDE CHECK.
//
// The generation boundary covers GENERATED sidecars only. A hand-authored one
// never passes through a dispatch, and `sidecar.Load` stays permissive by a
// recorded decision (v12_test.go:128), so nothing else would catch it.
//
// Same ValidateRawAgainstSchema, same mirrored bytes as the generation gate.
// NOT a third definition.
var schemaCheckCmd = &cobra.Command{
	Use:   "schema-check [root]",
	Short: "Validate every sidecar in the corpus against the authoritative schema",
	Long: `Validate every *.edikt.yaml against templates/schemas/gov-sidecar.v1.schema.json.

Complements the generation-boundary gate, which only sees sidecars an extractor
wrote. This sees hand-authored ones too.

Reports a COUNT and a DENOMINATOR: "N of M valid". A run that found no files
reports that nothing was found rather than a clean bill of health — an empty
corpus and a fully valid one must never look the same.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := "."
		if len(args) > 0 {
			root = args[0]
		}
		var files []string
		err := filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			if d.IsDir() {
				base := d.Name()
				if base == ".git" || base == "node_modules" || base == "fixtures" {
					return filepath.SkipDir
				}
				return nil
			}
			if strings.HasSuffix(p, ".edikt.yaml") {
				files = append(files, p)
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("walk %s: %w", root, err)
		}

		// INV-013: no subject is UNMEASURED, never a pass.  // edikt-guard:allow
		// A corpus check that found nothing must not print a green line —
		// that is indistinguishable from a corpus that is entirely valid.
		if len(files) == 0 {
			fmt.Fprintf(os.Stderr,
				"schema-check: UNMEASURED — no *.edikt.yaml found under %s. Nothing was validated.\n", root)
			return fmt.Errorf("no sidecars found")
		}

		var bad []string
		for _, f := range files {
			raw, rerr := os.ReadFile(f)
			if rerr != nil {
				bad = append(bad, fmt.Sprintf("%s: %v", f, rerr))
				continue
			}
			if verr := sidecar.ValidateRawAgainstDeclaredSchema(raw); verr != nil {
				bad = append(bad, fmt.Sprintf("%s: %v", f, verr))
			}
		}
		fmt.Printf("schema-check: %d of %d sidecars valid\n", len(files)-len(bad), len(files))
		if len(bad) > 0 {
			for _, b := range bad {
				fmt.Fprintf(os.Stderr, "  INVALID %s\n", b)
			}
			return fmt.Errorf("%d of %d sidecars fail the authoritative schema", len(bad), len(files))
		}
		return nil
	},
}

func init() { Cmd.AddCommand(schemaCheckCmd) }
