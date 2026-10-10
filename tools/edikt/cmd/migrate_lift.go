package cmd

// migrate_lift.go — `edikt migrate lift`, the one frozen lifter (ADR-082 (c)).
//
// It turns an unstripped in-body [edikt:directives:start]: # … block into a v2
// sidecar carrying migration_preserved, strips the block, and converts any
// v1-shaped sidecar it finds to the v2 multi-anchor shape in the same pass —
// `migrate sidecars --apply` and `migrate to-v2` collapsed into one verb, so a
// user can no longer land between them (F-085). It reads a dead format and
// never changes again; everything after it is "recompile".
//
// `sidecars` stays as a deprecated alias (D7) so every documented invocation
// keeps working; the logic lives in internal/migrate, not here.

import (
	"fmt"
	"os"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/spf13/cobra"
)

var (
	migrateLiftDryRun    bool
	migrateLiftApply     bool
	migrateLiftForce     bool
	migrateLiftJSON      bool
	migrateLiftStrict    bool
	migrateLiftReport    string
	migrateLiftV12Output string
)

var migrateLiftCmd = &cobra.Command{
	Use:     "lift",
	Aliases: []string{"sidecars"},
	Short:   "Lift in-body sentinel blocks into v2 *.edikt.yaml sidecars (the frozen lifter)",
	Long: `Lift existing in-body [edikt:directives:start]: # … [edikt:directives:end]: #
blocks from ADRs / invariants / guidelines into co-located <artifact>.edikt.yaml
sidecars in the current gov-sidecar.v2 shape, carrying the block's contents
verbatim under migration_preserved so the next gov compile can prove nothing
was dropped. Any existing v1-shaped sidecar is converted to v2 in the same
pass. Mandatory --dry-run before --apply (24-hour gate; --force bypasses it).

'migrate sidecars' is accepted as a deprecated alias of this verb.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		if !migrateLiftDryRun && !migrateLiftApply {
			return fmt.Errorf("must pass --dry-run or --apply")
		}
		if migrateLiftDryRun && migrateLiftApply {
			return fmt.Errorf("--dry-run and --apply are mutually exclusive")
		}
		if migrateLiftReport == "-" {
			return fmt.Errorf("--report-json: use a file path, not \"-\"")
		}
		ediktRoot, _ := resolveEdiktRoot()
		projectRoot, err := os.Getwd()
		if err != nil {
			return err
		}
		return migrate.RunLift(migrate.LiftOptions{
			ProjectRoot: projectRoot,
			EdiktRoot:   ediktRoot,
			DryRun:      migrateLiftDryRun,
			Apply:       migrateLiftApply,
			Force:       migrateLiftForce,
			JSON:        migrateLiftJSON,
			Strict:      migrateLiftStrict,
			ReportJSON:  migrateLiftReport,
			V12Output:   migrateLiftV12Output,
			Emit: func(eventType string, extra map[string]interface{}) {
				emitEvent(ediktRoot, eventType, extra)
			},
		})
	},
}

func init() {
	f := migrateLiftCmd.Flags()
	f.BoolVar(&migrateLiftDryRun, "dry-run", false, "preview the lift; writes the dry-run gate file")
	f.BoolVar(&migrateLiftApply, "apply", false, "apply the lift (requires a prior --dry-run within 24h, or --force)")
	f.BoolVar(&migrateLiftForce, "force", false, "bypass the 24h dry-run gate (test/escape hatch)")
	f.BoolVar(&migrateLiftJSON, "json", false, "emit the plan / summary as JSON to stdout (suppresses prose UI)")
	f.BoolVar(&migrateLiftStrict, "strict", false, "exit 1 on LOST/FACTUAL regressions, exit 2 on DEGRADED, exit 0 if clean")
	f.StringVar(&migrateLiftReport, "report-json", "", "write the regression manifest to this file path")
	f.StringVar(&migrateLiftV12Output, "dry-run-output", "", "write the v1.2 migration summary JSON to this path (also bypasses the dry-run gate)")
	migrateCmd.AddCommand(migrateLiftCmd)
}
