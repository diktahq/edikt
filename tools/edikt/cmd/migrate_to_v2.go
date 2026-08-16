package cmd

import (
	"fmt"
	"os"
	"sort"

	"github.com/spf13/cobra"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

var migrateToV2DryRun bool

var migrateToV2Cmd = &cobra.Command{
	Use:   "to-v2",
	Short: "Convert v1 single-anchor sidecars to the v2 multi-anchor shape",
	Long: `Rewrite each governance sidecar's directives[].source_excerpt and
prohibitions[].source_excerpt into the v2 source_excerpts[] list, and bump
schema_version to 2.

Pure structural cleanup: no LLM, deterministic, idempotent. Existing anchors are
carried verbatim as the single element of the new list — this migration never
invents a second anchor. Richer multi-anchor grounding arrives when an artifact
is next re-extracted, not here.

NEVER writes a parent .md, and edits only the two keys it owns (plus a stale
yaml-language-server $schema comment pointing at the v1 schema, if present),
so human-approved fields (verify:, human_approved_at, approved paths:) survive
byte-intact.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}
		cfg, err := govrun.LoadConfig(projectRoot)
		if err != nil {
			return fmt.Errorf("load .edikt/config.yaml: %w", err)
		}
		// Positional contract per sidecar.dirKinds — decisions, invariants,
		// guidelines, in that order. Guidelines are included deliberately: an
		// excluded dir migrates nothing and reports success, which is the
		// silent-gap shape this migration cannot afford.
		dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}
		pairs, err := sidecar.Discover(projectRoot, dirs)
		if err != nil {
			return err
		}
		var converted, already, absent []string
		for _, p := range pairs {
			if p.SidecarPath == "" {
				continue
			}
			// Discover returns the EXPECTED sidecar path, which may not exist:
			// superseded and skip-marked artifacts legitimately have none. Count
			// them separately rather than erroring (they are not this
			// migration's subject) or silently skipping (they would vanish from
			// the denominator and make coverage look total).
			if _, serr := os.Stat(p.SidecarPath); serr != nil {
				absent = append(absent, p.ArtifactID)
				continue
			}
			if migrateToV2DryRun {
				raw, rerr := os.ReadFile(p.SidecarPath)
				if rerr != nil {
					return rerr
				}
				would, cerr := sidecar.WouldConvertToV2(raw)
				if cerr != nil {
					return cerr
				}
				if would {
					converted = append(converted, p.ArtifactID)
				} else {
					already = append(already, p.ArtifactID)
				}
				continue
			}
			changed, cerr := sidecar.ConvertFileToV2(p.SidecarPath)
			if cerr != nil {
				return cerr
			}
			if changed {
				converted = append(converted, p.ArtifactID)
			} else {
				already = append(already, p.ArtifactID)
			}
		}
		sort.Strings(converted)
		sort.Strings(already)

		verb := "converted"
		if migrateToV2DryRun {
			verb = "would convert"
		}
		// INV-013: report the denominator, not just the hits. "0 converted" out  edikt-guard:allow
		// of 0 discovered is a broken scan; out of 60 it is a finished migration.
		fmt.Fprintf(os.Stderr, "migrate to-v2: %s %d of %d sidecar(s); %d already v2, %d artifact(s) have no sidecar\n",
			verb, len(converted), len(converted)+len(already), len(already), len(absent))
		for _, id := range converted {
			fmt.Fprintf(os.Stderr, "  + %s\n", id)
		}
		if len(pairs) == 0 {
			return fmt.Errorf("no sidecars discovered under %s — nothing was measured, "+
				"which is not the same as nothing to do", projectRoot)
		}
		return nil
	},
}

func init() {
	migrateToV2Cmd.Flags().BoolVar(&migrateToV2DryRun, "dry-run", false, "report what would change without writing")
	migrateCmd.AddCommand(migrateToV2Cmd)
}
