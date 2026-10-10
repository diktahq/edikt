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

// stageExtractorAgent installs an agent file pinning `model` under root.
//
// It also pins prompt_version, which `phasea plan` requires: an UNKNOWN
// extraction contract makes plan refuse outright (ADR-055 §4 by analogy)
// rather than silently baking an unpinned contract into a plan. Without it,
// every test that expects a plan to be emitted fails on the refusal instead
// of on its own subject.
func stageExtractorAgent(t *testing.T, root, model string) {
	t.Helper()
	p := filepath.Join(root, phasea.ExtractorAgentRelPath)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: sidecar-extractor\nmodel: " + model + "\nprompt_version: 4\n---\n\nbody\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	// ADR-085: Phase A now re-renders the installed frontmatter from
	// extractor.model config before every dispatch. Under the config
	// default (inherit, since these fixtures write no .edikt/config.yaml)
	// that render would strip the model: line this helper just staged.
	// Pin extractor.model explicitly so the staged value survives the
	// render and these tests keep testing what they say they test — that
	// the resolved model comes from the agent frontmatter, not a CLI/env
	// pin — rather than accidentally testing the inherit path instead.
	stageExtractorModelConfig(t, root, model)
}

// stageExtractorModelConfig writes .edikt/config.yaml with an explicit
// extractor.model pin.
func stageExtractorModelConfig(t *testing.T, root, model string) {
	t.Helper()
	dir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "extractor:\n  model: " + model + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTwoPhase_RecordsExtractorAgentModel(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageNeverInitialised(t)
	stageExtractorAgent(t, root, "claude-sonnet-5")
	var errBuf, outBuf bytes.Buffer

	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})

	// Phase A emits a plan and refuses, so compile returns an error. The
	// result is still returned, and the model was recorded before the hand-off.
	// (This used to be forced with a fake `claude` that exits 1; there is no
	// dispatch to fail any more, and the refusal is now the ordinary path.)
	if err == nil {
		t.Fatalf("phase A was expected to emit a plan and refuse\nstderr:\n%s", errBuf.String())
	}
	if res.ExtractorModel != "claude-sonnet-5" {
		t.Errorf("ExtractorModel = %q, want the AGENT's model %q — the reported value must name the process that performs extraction",
			res.ExtractorModel, "claude-sonnet-5")
	}
	if want := "Phase A — extractor model: claude-sonnet-5"; !strings.Contains(errBuf.String(), want) {
		t.Errorf("stderr does not announce the extractor's model.\nwant substring: %q\nstderr:\n%s", want, errBuf.String())
	}
}

// ADR-055 §4 / INV-013: no agent file means the model is UNKNOWN. It must not
// silently become a default, or an empty string that reads as "nothing to
// report".
func TestRunTwoPhase_UnknownExtractorModelIsReportedNotSubstituted(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	// F4/F5: ResolveExtractorAgentModel also falls back to the active
	// Claude profile's agents/ dir. Point CLAUDE_HOME at an empty sandbox
	// so this test's "deliberately no agent file" premise holds regardless
	// of what the machine running it actually has installed.
	t.Setenv("CLAUDE_HOME", t.TempDir())

	root := stageNeverInitialised(t) // deliberately no agent file
	var errBuf, outBuf bytes.Buffer

	res, _ := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
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
}

// The extractor model must be reported on the plan-emitting path too.
//
// This replaces TestRunTwoPhase_InjectedRunnerReportsNoExtractorModel, which
// pinned the opposite rule: report a model ONLY when the runner in play was
// ClaudeRunner, on the reasoning that a caller-injected runner dispatches to
// no model and naming one would report a measurement nothing made.
//
// That gate cannot hold any more. Tier-2 no longer dispatches at all
// (SPEC-013), so `runner is ClaudeRunner` is permanently false and the gate
// would have silently made ExtractorModel a permanent empty string — an
// unmeasured value dressed as a measured one, which is the shape the original
// test existed to prevent, arrived at from the other direction.
//
// The rule that survives is ADR-055's: the value comes from the extractor
// AGENT'S frontmatter. A run that emits a plan for tier-1 to fan out is
// exactly a run whose output needs attributing to a model, so it is
// reported here even though this process will not dispatch.
func TestRunTwoPhase_PlanEmittingRunReportsAgentModel(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageNeverInitialised(t)
	stageExtractorAgent(t, root, "claude-sonnet-5")
	var errBuf, outBuf bytes.Buffer

	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})

	// Phase A refuses by design once it has work: it emits a plan and hands
	// off. Guard that it refused for THAT reason — a refusal for any other
	// cause would make the assertions below vacuous, the same guard the test
	// this replaces kept against a run that never dispatched.
	if err == nil {
		t.Fatal("expected Phase A to emit a plan and refuse; it completed instead")
	}
	if !strings.Contains(err.Error(), "await tier-1 extraction") {
		t.Fatalf("refused for the wrong reason, so the model assertions prove nothing: %v\nstderr:\n%s", err, errBuf.String())
	}

	if res.ExtractorModel != "claude-sonnet-5" {
		t.Errorf("ExtractorModel = %q, want %q from the agent frontmatter", res.ExtractorModel, "claude-sonnet-5")
	}
}
