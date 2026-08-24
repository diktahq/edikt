package cmd

// hook_shim.go — `bin/edikt hook shim render` and `bin/edikt hook shim
// check` (PLAN-v080-reliability-core Phase 8, SPEC-014 FR-004).
//
// `render` prints the canonical shim (tools/edikt/internal/hook/shim),
// substituted for one (name, event) pair, to stdout — the same bytes a
// future migration (Phases 12-22) writes verbatim into
// templates/hooks/<file>. `check` is the Go half of AC-8.8's byte-stability
// gate: it renders every non-library templates/hooks/registry.yaml row into
// a temp tree and diffs each `status: migrated` row's shipped file against
// its own fresh render, naming any divergence. Neither subcommand needs a
// new templates/permits/tier2-verbs.yaml row — the `hook` group row already
// covers every sub-verb (ADR-050), the same fact hook_run.go's own comment
// records.
//
// Kept in Go rather than reimplemented as a bash/python registry parse for
// the reason every other hook-tier decision already lives here (INV-001):
// "does templates/hooks/<file> match a fresh render of registry.yaml's row"
// is exactly the comparison shim.Render already owns once; a second version
// of that comparison in the check script is a second place it can drift from
// what actually ships.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
	"github.com/diktahq/edikt/tools/edikt/internal/hook/shim"
	"github.com/spf13/cobra"
)

func newHookShimCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shim",
		Short: "Render or verify the canonical FR-004 hook exec shim",
	}
	cmd.AddCommand(newHookShimRenderCmd(), newHookShimCheckCmd())
	return cmd
}

func newHookShimRenderCmd() *cobra.Command {
	var event string
	var force bool
	cmd := &cobra.Command{
		Use:   "render <name>",
		Short: "Render the canonical shim template for one hook name",
		Long: `Render the canonical FR-004 exec shim, with <name> and --event substituted.

Both substitutions happen here, at generation time only — never at runtime,
and never from anything a hook's own payload carries. The output is the exact
byte sequence a migration (Phases 12-22) writes into templates/hooks/<file>.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			// REFUSE A ROW THAT IS NOT THE CANONICAL SHAPE. This verb used to
			// take a bare name and render, shape-blind — so a bulk loop over
			// "every migrated row" happily regenerated the one row governance
			// admits as a DIFFERENT shape, destroying its dispatch sites. The
			// exception was recorded in the registry and a bulk operation
			// flattened it, because nothing on the write path read it.
			//
			// Missing or unreadable registry is NOT a refusal: this verb is
			// used to bootstrap a shim before a row exists, and failing closed
			// there would break the migration flow it serves. What it refuses
			// is a row that positively declares another shape.
			if rows, rerr := loadRegistryRows(); rerr == nil {
				for _, r := range rows {
					if r.Name == args[0] && r.ShapeName() == hook.ShapeDispatchOrchestrator && !force {
						return &exitCodeError{code: 3, msg: fmt.Sprintf(
							"%s is registered shape: dispatch-orchestrator — it is NOT the canonical "+
								"exec shim and must not be rendered as one. Rendering would replace its "+
								"dispatch sites with a single `hook run` line and drop --step. Edit the "+
								"file directly, or change the registry row first if the shape really changed.",
							args[0])}
					}
				}
			}
			out, err := shim.Render(args[0], event)
			if err != nil {
				return &exitCodeError{code: 3, msg: err.Error()}
			}
			_, werr := fmt.Fprint(cmd.OutOrStdout(), out)
			return werr
		},
	}
	cmd.Flags().StringVar(&event, "event", "", "lifecycle event this shim is generated for (required)")
	// --force exists so the refusal is a SPEED BUMP for a human who means it,
	// not a wall. What it must never be is the default: the flattening that
	// prompted this guard came from a bulk loop, and a bulk loop does not pass
	// --force by accident.
	cmd.Flags().BoolVar(&force, "force", false, "render even when the registry row declares another shape (destroys a dispatch-orchestrating caller)")
	_ = cmd.MarkFlagRequired("event")
	return cmd
}

func newHookShimCheckCmd() *cobra.Command {
	var root string
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Verify no migrated shim diverges from a fresh render (AC-8.8 byte-stability gate)",
		Long: `Render every non-library templates/hooks/registry.yaml row into a temp tree.

Rendering all rows (never "just the ones already migrated") is deliberate:
today zero rows carry status: migrated, and a check whose subject set is
"already-migrated files" would examine zero shims and exit 0 — a vacuous
pass. Every non-library row is a real, always-present subject
regardless of migration progress; any row that IS migrated additionally has
its shipped bytes diffed against this run's fresh render, and a mismatch
(a hand-edit after generation) is reported by name.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if root == "" {
				wd, err := os.Getwd()
				if err != nil {
					return err
				}
				root = wd
			}
			registryPath, _, err := hook.ResolveRegistryPath(root)
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}
			f, err := hook.LoadRegistryFile(registryPath)
			if err != nil {
				return &exitCodeError{code: 2, msg: err.Error()}
			}

			tmpDir, err := os.MkdirTemp("", "edikt-hook-shim-check-*")
			if err != nil {
				return err
			}
			defer os.RemoveAll(tmpDir)

			checked := 0
			migrated := 0
			orchestrating := 0
			var divergent []string
			hooksDir := filepath.Dir(registryPath)
			for _, r := range f.Hooks {
				event, has := r.EventName()
				if !has {
					continue // class: library — never rendered, never a shim
				}
				rendered, rerr := shim.Render(r.Name, event)
				if rerr != nil {
					return &exitCodeError{code: 2, msg: fmt.Sprintf("render %s (event %s): %v", r.Name, event, rerr)}
				}
				if werr := os.WriteFile(filepath.Join(tmpDir, r.File), []byte(rendered), 0o755); werr != nil {
					return werr
				}
				checked++

				if r.Status != hook.StatusMigrated {
					continue
				}
				migrated++
				shipped := filepath.Join(hooksDir, r.File)
				shippedBytes, rerr := os.ReadFile(shipped)
				if rerr != nil {
					divergent = append(divergent, fmt.Sprintf("%s: status: migrated but the shipped file could not be read: %v", r.File, rerr))
					continue
				}

				// The ADR-063 Amendment's second shape is NOT a render of the
				// canonical template and never could be, so diffing it would
				// be a permanent, meaningless failure. It is also not exempt:
				// skipping it would leave the one hand-written body in the
				// tier as the only file this gate has no opinion about, which
				// is precisely where a python3 call or a hand-built JSON
				// literal could reappear unobserved. It gets its OWN
				// assertions instead, and its own line in the summary.
				if r.ShapeName() == hook.ShapeDispatchOrchestrator {
					orchestrating++
					divergent = append(divergent, checkDispatchOrchestrator(r, shippedBytes)...)
					continue
				}
				if string(shippedBytes) != rendered {
					divergent = append(divergent, fmt.Sprintf("%s: shipped bytes differ from a fresh render of the canonical template — hand-edited after generation?", r.File))
				}
			}

			if checked == 0 {
				// INV-013: a registry with rows but zero non-library ones
				// would make every comparison below vacuously true.
				return &exitCodeError{code: 2, msg: "hook shim check: rendered 0 shims from " + registryPath + " — nothing was checked, which is not a pass"}
			}

			fmt.Fprintf(cmd.OutOrStdout(),
				"hook shim check: %d shim(s) rendered into %s from %s, %d migrated "+
					"(%d exec-shim byte-diffed, %d dispatch-orchestrating property-checked), %d divergence(s)\n",
				checked, tmpDir, registryPath, migrated, migrated-orchestrating, orchestrating, len(divergent))
			if len(divergent) > 0 {
				for _, d := range divergent {
					fmt.Fprintf(cmd.ErrOrStderr(), "  - %s\n", d)
				}
				return &exitCodeError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&root, "root", "", "project root (defaults to cwd)")
	return cmd
}

// checkDispatchOrchestrator asserts what the ADR-063 Amendment actually
// requires of a dispatch-orchestrating caller, since a byte-diff against the
// canonical render cannot.
//
// The amendment's rule is that every line beyond the exec shim must be JSON
// construction DELEGATED to Go, or step orchestration. That is not mechanically
// checkable in full, but its two load-bearing consequences are, and both are
// the properties whose loss would silently undo the migration:
//
//	the file still execs `hook run <name>`  — it did not drift into doing the
//	                                          work itself
//	the file invokes no python3             — the whole point of the migration,
//	                                          and the criterion Phase 17 states
//
// A non-empty check is also a check that something was read: an empty or
// truncated body fails here rather than passing three greps vacuously. That
// exact failure — a shipped shim truncated to zero bytes by a redirect that
// ran before its command — is what this plan already hit once.
//
// THE DIRECTION THIS CHECK ORIGINALLY MISSED. Every property above guards the
// hook drifting BACKWARDS — into doing the work itself, or into carrying an
// interpreter again. The failure that actually happened was the opposite: a
// migration over-applied and ERASED the exception, regenerating this caller as
// the canonical shim. Nothing enumerated that direction, so a file with every
// dispatch site removed passed all three properties and the run reported zero
// divergences.
//
// The assertions below close it by describing what the shape IS rather than
// only what it must not become: not a canonical render, more than one dispatch
// site, and --step present.
func checkDispatchOrchestrator(r hook.Row, body []byte) []string {
	var out []string
	if len(body) == 0 {
		return []string{fmt.Sprintf("%s: shape: dispatch-orchestrator but the shipped file is EMPTY — nothing was checked, which is not a pass", r.File)}
	}
	text := string(body)

	// THE CANONICAL-SHIM CHECK. The three properties below are necessary and
	// NOT sufficient: a canonical exec shim for this same hook is non-empty,
	// execs `hook run <name>`, and invokes no python3 — it passes all three
	// while having lost every dispatch site the shape exists for.
	//
	// That is not hypothetical. A bulk re-render walked every migrated row,
	// regenerated this one as a canonical shim, and `shim check` reported
	// "0 divergences" because the file now matched the render it was never
	// meant to be generated from. The row's own comment says this hook is the
	// one that is NOT the canonical shim; nothing enforced it.
	//
	// So the shape asserts what makes it DIFFERENT: it must not be a canonical
	// render, and it must pass --step, which is the argument the dispatch loop
	// round-trips on and which no canonical shim carries.
	// A row with no event cannot be compared against a render, and that is
	// reported rather than skipped: a check that quietly cannot run reports the
	// same "no problem" as one that ran and found none — which is the shape
	// that let the flattening through in the first place.
	if ev := eventOf(r); ev == "" {
		out = append(out, fmt.Sprintf(
			"%s: shape: dispatch-orchestrator but the row carries no event, so it cannot be "+
				"compared against a canonical render — UNCHECKED, not passed", r.File))
	} else if canonical, rerr := shim.Render(r.Name, ev); rerr == nil {
		if strings.TrimSpace(text) == strings.TrimSpace(canonical) {
			out = append(out, fmt.Sprintf(
				"%s: shape: dispatch-orchestrator but the file IS the canonical exec shim — "+
					"the second shape exists precisely because this hook is not one, and a "+
					"render has flattened it", r.File))
		}
	}
	// POSITIVE SHAPE ASSERTION. The shape is admitted because this caller execs
	// the host model CLI at several sites; one site, or none, is not the thing
	// the exception was granted for. Counted on non-comment lines so the doc
	// comment describing the shape cannot satisfy the check that guards it.
	sites := 0
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(strings.TrimLeft(line, " \t"), "#") {
			continue
		}
		if strings.Contains(line, "_TOOL") || strings.Contains(line, "hook run "+r.Name) {
			sites++
		}
	}
	if sites < 2 {
		out = append(out, fmt.Sprintf(
			"%s: shape: dispatch-orchestrator but only %d dispatch site(s) found — the exception "+
				"is granted for a caller that round-trips, and a single site is the canonical "+
				"shape wearing this label", r.File, sites))
	}

	if !strings.Contains(text, "--step") {
		out = append(out, fmt.Sprintf(
			"%s: shape: dispatch-orchestrator but nothing in it passes --step — the dispatch "+
				"loop round-trips on that argument, so a caller without it cannot orchestrate", r.File))
	}

	if !strings.Contains(text, "hook run "+r.Name) {
		out = append(out, fmt.Sprintf("%s: shape: dispatch-orchestrator but nothing in it execs `hook run %s` — the caller is not delegating to the binary", r.File, r.Name))
	}
	for i, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimLeft(line, " \t")
		if strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.Contains(line, "python3") {
			out = append(out, fmt.Sprintf("%s:%d: shape: dispatch-orchestrator but the line invokes python3 — the second shape removes the interpreter, it does not license one", r.File, i+1))
		}
	}
	return out
}

// loadRegistryRows reads the hook registry from the working directory's project
// root. Errors are returned rather than fatal: callers that use this to REFUSE
// something must not turn "no registry here" into a refusal, since this verb is
// also how a shim gets bootstrapped before its row exists.
func loadRegistryRows() ([]hook.Row, error) {
	wd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	registryPath, _, err := hook.ResolveRegistryPath(wd)
	if err != nil {
		return nil, err
	}
	f, err := hook.LoadRegistryFile(registryPath)
	if err != nil {
		return nil, err
	}
	return f.Hooks, nil
}

// eventOf returns a row's event, or "" when it has none. Used only to build the
// canonical render this shape must NOT match.
func eventOf(r hook.Row) string {
	ev, _ := r.EventName()
	return ev
}
