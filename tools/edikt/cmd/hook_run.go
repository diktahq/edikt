package cmd

// hook_run.go — `bin/edikt hook run <name>`.
//
// The single Go entry point every migrated hook shim execs (SPEC-014 FR-005).
// It is a sibling of the existing `hook match` / `probe` / `report` / `ack`
// verbs under the SAME cobra group, and it needs NO new permit row: the `hook`
// group row in templates/permits/tier2-verbs.yaml already covers every
// sub-verb (ADR-050). Adding one would imply the group row does not, which is
// a claim about the permit model that is not true.
//
// This file is deliberately thin. Everything with a decision in it —
// resolution, panic recovery, journaling, the host seam — lives in
// internal/hook, where it is unit-testable without a process.

import (
	"github.com/diktahq/edikt/tools/edikt/internal/hook"
	"github.com/spf13/cobra"
)

func newHookRunCmd() *cobra.Command {
	var event string
	var step int
	var statePath string
	var dispatchOut string
	var dispatchExit int
	var dispatchTool string
	var dispatchAvailable bool

	cmd := &cobra.Command{
		Use:   "run <name>",
		Short: "Run a registered hook handler against a payload on stdin",
		Long: `Run one registered hook handler.

<name> is resolved against the registration surface declared in
templates/hooks/registry.yaml. It is baked into each generated shim at
generation time — it is never runtime input in normal operation.

Exit codes:
  0  the handler ran (or its own exit code, where the hook protocol gives a
     non-zero code a meaning)
  0  unregistered name, undecodable payload, recovered panic

Fail-open is the hook tier's standing posture: a governance bug must not brick
an editor. It is never SILENT. Each of those three classes writes a loud stderr
diagnostic and a distinctly-typed journal record, and both 'edikt hook report'
and 'edikt doctor' count them as invocations where governance DID NOT RUN.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			code := hook.Dispatch(hook.Options{
				Name:   args[0],
				Event:  event,
				Stdin:  cmd.InOrStdin(),
				Stdout: cmd.OutOrStdout(),
				Stderr: cmd.ErrOrStderr(),
				Step: &hook.StepArgs{
					Step:              step,
					StatePath:         statePath,
					DispatchOut:       dispatchOut,
					DispatchExit:      dispatchExit,
					DispatchTool:      dispatchTool,
					DispatchAvailable: dispatchAvailable,
				},
			})
			if code != 0 {
				// exitCodeError with an empty message: the handler's reply has
				// already said whatever there is to say, and cobra's "error: "
				// prefix on top of a protocol reply is noise the host would
				// have to parse around.
				return &exitCodeError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&event, "event", "",
		"lifecycle event this shim was generated for; used only when the payload does not name one")

	// ── the dispatch-orchestrating step protocol (ADR-063 Amendment) ─────
	//
	// Read by exactly one handler today. They live on the shared `hook run`
	// verb rather than on a hook-specific one because SP-007 resolves a hook
	// NAME from data: a second verb would be a second, name-shaped switch,
	// which is the thing the registry exists to avoid. A handler that does
	// not read them cannot be affected by them.
	//
	// --step defaults to 0, which the one handler that reads it treats as
	// step 1 — so the corpus recorder's own `hook run <name>` (no flags)
	// executes the same first leg the caller does, rather than a second
	// entry point the corpus would then be characterizing instead.
	cmd.Flags().IntVar(&step, "step", 0,
		"which leg of the dispatch-orchestrating step protocol to run (default: the first)")
	cmd.Flags().StringVar(&statePath, "state", "",
		"cross-step state file written by the previous step")
	cmd.Flags().StringVar(&dispatchOut, "dispatch-out", "",
		"file the caller redirected the previous dispatch's output into")
	cmd.Flags().IntVar(&dispatchExit, "dispatch-exit", 0,
		"exit status of the previous dispatch (passed explicitly, never inferred from an empty output file)")
	cmd.Flags().StringVar(&dispatchTool, "dispatch-tool", "",
		"name of the CLI the caller dispatches, supplied as data so the binary never names one")
	cmd.Flags().BoolVar(&dispatchAvailable, "dispatch-available", false,
		"whether the caller resolved that tool on PATH")
	return cmd
}
