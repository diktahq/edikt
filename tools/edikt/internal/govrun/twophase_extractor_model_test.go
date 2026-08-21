package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// ADR-054 § Confirmation, AS AMENDED BY ADR-055.
//
// Three items concern TwoPhaseResult rather than the dispatch argv:
//
//   - a run that dispatched through ClaudeRunner records the model taken from
//     the EXTRACTOR AGENT'S frontmatter and announces it on stderr;
//   - that recorded value never tracks the CLI/env pin, which governs a
//     different process entirely;
//   - a run with any other runner leaves the field empty rather than
//     defaulting.
//
// The set is deliberate. The first proves the recording happens at all
// (sensitivity); the second is the D27 regression guard (the CLI pin and the
// extractor model were the same value in the old contract, so the old
// sensitivity test passed while the reported model was wrong for three
// months); the third proves the recording is scoped to the runner that
// actually dispatches (isolation). Deleting the recording block fails the
// first; reverting to phasea.ResolveExtractorModel fails the second; widening
// it to record unconditionally fails the third. No one of them alone catches
// what the other two do.
//
// WHAT CHANGED AND WHY, since this file previously asserted the opposite:
// the old first test required ExtractorModel to equal the EDIKT_EXTRACTOR_MODEL
// override. That override pins the session running the slash command, not the
// forked subagent that performs extraction, so the assertion pinned the defect.
// It is not weakened here — it is pointed at the value that governs.

// fakeClaude writes an executable stub at <dir>/claude that exits non-zero.
// Phase A's dispatch therefore fails — which is fine and is the point: the
// model is resolved and announced BEFORE any task runs, so the recording is
// observable without an LLM, and asserting it on the failure path proves it
// does not depend on extraction succeeding.
func fakeClaude(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "claude")
	script := "#!/bin/sh\nexit 1\n"
	if err := os.WriteFile(bin, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake claude: %v", err)
	}
	return bin
}

// stageExtractorAgent installs an agent file pinning `model` under root.
func stageExtractorAgent(t *testing.T, root, model string) {
	t.Helper()
	p := filepath.Join(root, phasea.ExtractorAgentRelPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: sidecar-extractor\nmodel: " + model + "\n---\n\nbody\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTwoPhase_RecordsExtractorAgentModel(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	// The CLI/env pin is set to a DIFFERENT value than the agent's. This is
	// the whole point: if the report ever tracks this again, D27 is back.
	t.Setenv(phasea.ExtractorModelEnv, "claude-opus-5")

	root := stageNeverInitialised(t)
	stageExtractorAgent(t, root, "claude-sonnet-5")
	var errBuf, outBuf bytes.Buffer

	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      &phasea.ClaudeRunner{Binary: fakeClaude(t)},
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})

	// The stub exits 1, so Phase A fails and compile returns an error. The
	// result is still returned, and the model was recorded before dispatch.
	if err == nil {
		t.Fatalf("fake claude exits 1; phase A was expected to fail\nstderr:\n%s", errBuf.String())
	}
	if res.ExtractorModel != "claude-sonnet-5" {
		t.Errorf("ExtractorModel = %q, want the AGENT's model %q — the reported value must name the process that performs extraction",
			res.ExtractorModel, "claude-sonnet-5")
	}
	if res.ExtractorModel == "claude-opus-5" {
		t.Error("ExtractorModel tracked the CLI/env pin, which governs the dispatching session and not the extractor — this is D27")
	}
	if want := "Phase A — extractor model: claude-sonnet-5"; !strings.Contains(errBuf.String(), want) {
		t.Errorf("stderr does not announce the extractor's model.\nwant substring: %q\nstderr:\n%s", want, errBuf.String())
	}
}

// ADR-055 §4 / INV-013: no agent file means the model is UNKNOWN. It must not
// silently become the CLI pin, the default, or an empty string that reads as
// "nothing to report".
func TestRunTwoPhase_UnknownExtractorModelIsReportedNotSubstituted(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	t.Setenv(phasea.ExtractorModelEnv, "claude-opus-5")
	// F4/F5: ResolveExtractorAgentModel also falls back to the active
	// Claude profile's agents/ dir. Point CLAUDE_HOME at an empty sandbox
	// so this test's "deliberately no agent file" premise holds regardless
	// of what the machine running it actually has installed.
	t.Setenv("CLAUDE_HOME", t.TempDir())

	root := stageNeverInitialised(t) // deliberately no agent file
	var errBuf, outBuf bytes.Buffer

	res, _ := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      &phasea.ClaudeRunner{Binary: fakeClaude(t)},
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})

	if res.ExtractorModel != phasea.ExtractorModelUnknown {
		t.Errorf("ExtractorModel = %q, want %q", res.ExtractorModel, phasea.ExtractorModelUnknown)
	}
	if want := "Phase A — extractor model: " + phasea.ExtractorModelUnknown; !strings.Contains(errBuf.String(), want) {
		t.Errorf("stderr does not report the model as unknown.\nwant substring: %q\nstderr:\n%s", want, errBuf.String())
	}
	if strings.Contains(errBuf.String(), "extractor model: claude-opus-5") {
		t.Error("substituted the CLI/env pin for an unresolvable extractor model — the exact shape ADR-055 §4 forbids")
	}
}

func TestRunTwoPhase_InjectedRunnerReportsNoExtractorModel(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	// Set the override so a failure here cannot be read as "the env was
	// simply unset". If the recording ever widens past ClaudeRunner, this
	// picks up the value and fails.
	t.Setenv(phasea.ExtractorModelEnv, "claude-sonnet-5")

	root := stageNeverInitialised(t)
	runner := &bootstrapRunner{}
	var errBuf, outBuf bytes.Buffer

	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err != nil {
		t.Fatalf("compile failed: %v\nstderr:\n%s", err, errBuf.String())
	}
	if len(runner.dispatched) == 0 {
		t.Fatalf("phase A never dispatched, so this test would pass vacuously\nstderr:\n%s", errBuf.String())
	}

	if res.ExtractorModel != "" {
		t.Errorf("ExtractorModel = %q, want empty — this runner dispatches to no model, and reporting one anyway names a measurement nothing made",
			res.ExtractorModel)
	}
	if strings.Contains(errBuf.String(), "extractor model:") {
		t.Errorf("stderr announces an extractor model for a runner that never used one:\n%s", errBuf.String())
	}
}
