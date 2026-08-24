package cmd

// dev_hookcorpus.go — `bin/edikt dev hook-corpus {record,diff,coverage}`.
//
// The operator surface over internal/hookcorpus. Thin on purpose: everything
// with a decision in it — oracle resolution, sandboxing, normalization, the
// comparison — lives in the package, where it is testable without a process.
//
// NO PERMIT ROW, AND THAT IS A DECISION, NOT AN OMISSION. `dev` is developer
// tooling and is never invoked from tier-1 markdown; SPEC-014 spec.md:150 states
// it explicitly, and TestDevVerbNotInvokedFromTier1 keeps it true rather than
// assumed. test/test-permits.sh only scans commands/, so it would pass whether
// or not that stayed true — the Go test is what closes that. Adding a `dev` row
// to templates/permits/tier2-verbs.yaml would PERMIT a call path that must not
// exist, which is the opposite of what a permit list is for.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hookcorpus"
	"github.com/spf13/cobra"
)

var (
	hookCorpusHook       string
	hookCorpusScenario   string
	hookCorpusBin        string
	hookCorpusLive       string
	hookCorpusRequireGo  bool
	hookCorpusCorpusRoot string
	hookCorpusBaseTree   string
	hookCorpusKeep       bool
	hookCorpusJSON       bool
)

func newHookCorpusCmd() *cobra.Command {
	group := &cobra.Command{
		Use:   "hook-corpus",
		Short: "Record and diff the hook characterization corpus",
		Long: `Record what a bash hook does across its curated input matrix, and later
compare a Go replacement against that recording.

The hook is the oracle: nothing here authors an expectation. record executes the
CURRENT implementation and writes down exit code, stdout, stderr and filesystem
effect; diff re-runs each scenario against the implementation registered right
now and compares.

A recording names which implementation it characterizes in its own oracle field,
and that value comes from templates/hooks/registry.yaml rather than from a flag —
recording a hook AFTER its Go handler is registered would compare Go against Go,
so it is refused rather than mislabelled.`,
	}
	group.PersistentFlags().StringVar(&hookCorpusHook, "hook", "", "hook name (required)")
	group.PersistentFlags().StringVar(&hookCorpusCorpusRoot, "corpus-root", "",
		"corpus tree (default test/fixtures/hook-corpus)")
	group.PersistentFlags().StringVar(&hookCorpusBaseTree, "base-tree", "",
		"minimal sandbox fixture tree (default <corpus-root>/_base)")
	group.PersistentFlags().BoolVar(&hookCorpusJSON, "json", false, "emit the report as JSON")

	group.AddCommand(newHookCorpusRecordCmd())
	group.AddCommand(newHookCorpusDiffCmd())
	group.AddCommand(newHookCorpusCoverageCmd())
	return group
}

func newHookCorpusRecordCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:          "record",
		Short:        "Record every scenario in a hook's matrix against the current implementation",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rec, _, err := newCorpusRecorder(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			res, err := rec.Record(hookCorpusHook, hookCorpusScenario)
			if err != nil {
				return err
			}
			if hookCorpusJSON {
				return emitCorpusJSON(cmd.OutOrStdout(), res)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "recorded %d scenario(s) for %s with oracle=%s under %s\n",
				len(res.Scenarios), res.Hook, res.Oracle, res.Dir)
			return nil
		},
	}
	cmd.Flags().StringVar(&hookCorpusScenario, "scenario", "", "record only this scenario id")
	cmd.Flags().BoolVar(&hookCorpusKeep, "keep-content", false,
		"also write the bytes of added/modified sandbox files beside the tree manifests")
	return cmd
}

func newHookCorpusDiffCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare the live implementation against a hook's recorded corpus",
		Long: `Exit 0 only when every scenario's exit code, normalized stdout/stderr hash
and structural fs_diff match the recording. The first failing scenario is named.

Refuses, rather than comparing, when the recording was made under a different
normalizer version, when the scenario's inputs no longer hash to what the
recording was captured against, when no built binary can be resolved, or when
zero scenarios would be compared.`,
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rec, projectRoot, err := newCorpusRecorder(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			started := time.Now()
			report, derr := rec.Diff(hookCorpusHook, hookcorpus.DiffOptions{
				Scenario:  hookCorpusScenario,
				Live:      hookCorpusLive,
				RequireGo: hookCorpusRequireGo,
			}, cmd.OutOrStdout())
			elapsed := time.Since(started)

			// The evidence entry is written for a FAILED gate too, and for one
			// that refused to run. A latency figure that existed only for green
			// runs would be a record of the happy path rather than of the gate.
			writeCorpusEvidence(cmd.ErrOrStderr(), projectRoot, hookCorpusHook, report, elapsed, derr)

			if derr != nil {
				return derr
			}
			if hookCorpusJSON {
				if jerr := emitCorpusJSON(cmd.OutOrStdout(), report); jerr != nil {
					return jerr
				}
			}
			if !report.OK() {
				return &exitCodeError{code: 1, msg: fmt.Sprintf(
					"hook-corpus diff: %d of %d scenario(s) failed for %s; first failure — %s",
					report.Failed, report.Compared, report.Hook, report.FirstFailure())}
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"hook-corpus diff: %d/%d scenarios match for %s (%dms, gate_meaningful=%v)\n",
				report.Passed, report.Compared, report.Hook, elapsed.Milliseconds(), report.GateMeaningful)
			return nil
		},
	}
	cmd.Flags().StringVar(&hookCorpusScenario, "scenario", "", "compare only this scenario id")
	cmd.Flags().StringVar(&hookCorpusBin, "bin", "", "built edikt binary to run hook run with")
	cmd.Flags().StringVar(&hookCorpusLive, "live", "auto",
		"implementation to compare against: auto (follow the registry), go, or bash")
	cmd.Flags().BoolVar(&hookCorpusRequireGo, "require-go", false,
		"fail instead of running a bash-against-bash self-diff")
	return cmd
}

func newHookCorpusCoverageCmd() *cobra.Command {
	return &cobra.Command{
		Use:          "coverage",
		Short:        "Report a hook's coverage across the four characterization axes",
		SilenceUsage: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rec, _, err := newCorpusRecorder(cmd.ErrOrStderr())
			if err != nil {
				return err
			}
			rep, err := rec.Coverage(hookCorpusHook)
			if err != nil {
				return err
			}
			if hookCorpusJSON {
				if jerr := emitCorpusJSON(cmd.OutOrStdout(), rep); jerr != nil {
					return jerr
				}
			} else {
				rep.Render(cmd.OutOrStdout())
			}
			if !rep.OK() {
				return &exitCodeError{code: 1, msg: fmt.Sprintf(
					"hook %s has uncovered applicable axes: %v", rep.Hook, rep.Gaps())}
			}
			return nil
		},
	}
}

// newCorpusRecorder builds the recorder and returns the project root beside it.
func newCorpusRecorder(stderr io.Writer) (*hookcorpus.Recorder, string, error) {
	if hookCorpusHook == "" {
		return nil, "", &exitCodeError{code: 3, msg: "hook-corpus: --hook is required"}
	}
	root, err := findProjectRootForVerify()
	if err != nil {
		return nil, "", err
	}
	rec, err := hookcorpus.NewRecorder(hookcorpus.Options{
		RepoRoot:    root,
		CorpusRoot:  hookCorpusCorpusRoot,
		BaseTree:    hookCorpusBaseTree,
		Binary:      hookCorpusBin,
		KeepContent: hookCorpusKeep,
		Stderr:      stderr,
	})
	if err != nil {
		return nil, "", err
	}
	return rec, root, nil
}

func emitCorpusJSON(w io.Writer, v any) error {
	body, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(append(body, '\n'))
	return err
}

// corpusEvidencePath is where a diff run's wall-clock lands.
//
// AC-7.9 asks for the figure to be RECORDED and deliberately does not ask for a
// threshold: SPEC-014 spec.md:265 declines to set a runtime-path budget for this
// tool ("runs offline, in CI and at authoring time, never in the
// PreToolUse/PostToolUse chain a user waits on"), so a pass/fail gate here would
// cite a number the spec refuses to set. The entry is evidence, never a verdict.
func corpusEvidencePath(projectRoot string) string {
	return filepath.Join(projectRoot, ".edikt", "state", "hook-corpus", "evidence.jsonl")
}

type corpusEvidence struct {
	RanAt          string           `json:"ran_at"`
	Hook           string           `json:"hook"`
	Compared       int              `json:"scenarios_compared"`
	Passed         int              `json:"scenarios_passed"`
	Failed         int              `json:"scenarios_failed"`
	GateMeaningful bool             `json:"gate_meaningful"`
	TotalMS        int64            `json:"total_wall_clock_ms"`
	PerScenarioMS  map[string]int64 `json:"per_scenario_wall_clock_ms"`
	Informational  string           `json:"informational"`
	Refused        string           `json:"refused,omitempty"`
}

// writeCorpusEvidence appends one line per diff run.
//
// A write failure is reported on stderr and NEVER swallowed, but it does not
// fail the gate: the gate's subject is the hook's behaviour, not the evidence
// log's disk. What it must never do is stay silent, because a missing entry
// would then be indistinguishable from a run that never happened (INV-015).
func writeCorpusEvidence(stderr io.Writer, projectRoot, hook string,
	report *hookcorpus.DiffReport, elapsed time.Duration, runErr error) {
	ev := corpusEvidence{
		RanAt:         time.Now().UTC().Format(time.RFC3339),
		Hook:          hook,
		TotalMS:       elapsed.Milliseconds(),
		PerScenarioMS: map[string]int64{},
		Informational: "wall-clock is informational; SPEC-014 spec.md:265 sets no runtime budget " +
			"for the corpus recorder, so no threshold is enforced against these figures",
	}
	if runErr != nil {
		ev.Refused = runErr.Error()
	}
	if report != nil {
		ev.Compared = report.Compared
		ev.Passed = report.Passed
		ev.Failed = report.Failed
		ev.GateMeaningful = report.GateMeaningful
		for _, r := range report.Results {
			ev.PerScenarioMS[r.Scenario] = r.LiveMS
		}
	}
	body, err := json.Marshal(ev)
	if err != nil {
		fmt.Fprintf(stderr, "hook-corpus: evidence NOT RECORDED (encoding: %v)\n", err)
		return
	}
	path := corpusEvidencePath(projectRoot)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintf(stderr, "hook-corpus: evidence NOT RECORDED at %s (%v)\n", path, err)
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644) //nolint:gosec
	if err != nil {
		fmt.Fprintf(stderr, "hook-corpus: evidence NOT RECORDED at %s (%v)\n", path, err)
		return
	}
	defer func() { _ = f.Close() }()
	if _, err := f.Write(append(body, '\n')); err != nil {
		fmt.Fprintf(stderr, "hook-corpus: evidence NOT RECORDED at %s (%v)\n", path, err)
		return
	}
	fmt.Fprintf(stderr, "hook-corpus: wall-clock recorded to %s (%dms)\n", path, ev.TotalMS)
}
