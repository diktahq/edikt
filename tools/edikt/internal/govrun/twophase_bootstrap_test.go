package govrun

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/model"
)

// writeBootstrapSidecar writes the sidecar the extractor would have produced.
//
// This was a bootstrapRunner injected as TwoPhaseOptions.Runner. Phase A no
// longer dispatches from tier-2 (SPEC-013), so there is no dispatch to fake.
func writeBootstrapSidecar(t *testing.T, adrDir, parentBase, artifactID string) {
	t.Helper()
	body := fmt.Sprintf(`schema_version: 1
topic: "testing"
path: "docs/architecture/decisions/%s"
signals:
  - "bootstrap signal"
directives:
  - text: "Test rule MUST hold. (ref: %s)"
    source_excerpt:
      line_start: 9
      line_end: 9
      quote: "Test rule."
`, parentBase, artifactID)
	p := filepath.Join(adrDir, strings.TrimSuffix(parentBase, ".md")+".edikt.yaml")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

func stageNeverInitialised(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Line 9 is "Test rule." — the quote the fake runner anchors to.
	body := "---\nstatus: accepted\n---\n\n# ADR-001 — Test\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write adr: %v", err)
	}
	return root
}

// TestRunTwoPhase_BootstrapsMissingSidecars is the end-to-end regression
// test for the adoption deadlock. A project whose ADRs were never
// edikt-managed has no sidecars and no legacy sentinels; before this,
// every such artifact was reported as a fatal "sidecar missing" load error
// and the project could not reach a compiled state by any documented path.
//
// The subject splits in two once Phase A stops dispatching from tier-2.
// SELECTION — "a missing sidecar is picked up for extraction, same as a stale
// one" — is now answered by the emitted plan, and is covered by
// TestRunTwoPhase_MissingSidecarIsPlannedForExtraction below. What remains
// here is REACHABILITY: once the sidecar exists, Phase B merges it and its
// directive reaches a reader. Both halves survive; neither was dropped to
// reach green.
func TestRunTwoPhase_BootstrapsMissingSidecars(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageNeverInitialised(t)
	// Stage the extractor's output rather than dispatching for it.
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	writeBootstrapSidecar(t, adrDir, "ADR-001-test.md", "ADR-001")
	var errBuf, outBuf bytes.Buffer

	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err != nil {
		t.Fatalf("compile must not fail on a never-initialised project: %v\nstderr:\n%s", err, errBuf.String())
	}

	// The sidecar must exist on disk after the run.
	sidecarPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.edikt.yaml")
	if _, serr := os.Stat(sidecarPath); serr != nil {
		t.Fatalf("no sidecar at %s: %v", sidecarPath, serr)
	}

	// Phase B must have run and merged the sidecar.
	if res.PhaseB == nil {
		t.Fatal("Phase B did not run")
	}
	if res.PhaseB.TotalDirectives != 1 {
		t.Fatalf("expected 1 merged directive; got %d", res.PhaseB.TotalDirectives)
	}
	// The bootstrapped directive must be REACHABLE. Which surface carries it
	// is a tier decision: this fixture's sidecar declares no `paths:`, so the
	// topic retires to tier 3 (SPEC-011 stage 1) and its directives land in
	// the skill package rather than a tier-2 rules file.
	//
	// Re-pointed rather than deleted: the subject is "a bootstrapped sidecar
	// is not silently absent from compiled output", and that subject is
	// unchanged. Asserting the old path would fail for the right reason but
	// stop guarding anything; asserting nothing would let a bootstrap that
	// drops its directives entirely pass.
	skill := filepath.Join(root, ".claude", "skills", "edikt-testing", "SKILL.md")
	body, rerr := os.ReadFile(skill)
	if rerr != nil {
		t.Fatalf("skill package not written for the bootstrapped topic: %v", rerr)
	}
	if !strings.Contains(string(body), "Test rule MUST hold.") {
		t.Fatalf("bootstrapped directive missing from its skill package:\n%s", body)
	}

	idx := filepath.Join(root, ".claude", "rules", "governance.md")
	idxBody, ierr := os.ReadFile(idx)
	if ierr != nil {
		t.Fatalf("governance.md not written: %v", ierr)
	}
	// The bootstrapped artifact must REACH THE READER. It used to be asserted
	// via the routing table's signal row; SPEC-011 stage 1 removes the routing
	// table (629 keyword terms measured at ~3,119 tokens, loaded on every
	// edit, and measurably less precise than one-line topic descriptions —
	// BRAIN-005 E4). The artifact now reaches the reader through its TOPIC,
	// so the assertion follows the content rather than the deleted mechanism.
	//
	// Asserting a topic row keeps this test's real subject — "a bootstrapped
	// sidecar is not silently absent from compiled output" — which is why the
	// test is re-pointed rather than deleted.
	if !strings.Contains(string(idxBody), "**testing**") {
		t.Fatalf("bootstrapped artifact's topic missing from the ambient topic index:\n%s", idxBody)
	}
	// The header must count the ONE ADR and no phantom guidelines. The
	// old counter classified by ID prefix, so anything without an ADR-/INV-
	// name — READMEs, stray notes — was tallied as a guideline.
	//
	// The wording changed when the accepted/superseded split was removed:
	// it was two names for one number here, and reported "0 superseded"
	// without having counted. See TestRunTwoPhase_IndexReportsRetiredExclusions.
	// The classification property this line guards is unchanged.
	if !strings.Contains(string(idxBody), "1 ADRs, 0 invariants, 0 guidelines compiled") {
		t.Fatalf("source header miscounts compile inputs:\n%s", idxBody)
	}
}

// TestRunTwoPhase_Headless_RefusesBootstrap pins the documented opt-out:
// commands/gov/compile.md §12 specifies that a headless run disables the
// auto-chain and exits non-zero rather than dispatching. Honouring it in
// the binary is what stops CI and the golden test from spawning LLM
// subprocesses.
func TestRunTwoPhase_Headless_RefusesBootstrap(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "1")

	root := stageNeverInitialised(t)
	var errBuf, outBuf bytes.Buffer

	_, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err == nil {
		t.Fatal("headless run must exit non-zero rather than bootstrap")
	}
	// "must not dispatch" used to be read off the fake runner's tally. There
	// is no dispatch from tier-2 to tally now, so the observable is that no
	// sidecar was written for the artifact the run refused on.
	if _, serr := os.Stat(filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.edikt.yaml")); serr == nil {
		t.Fatal("headless run wrote a sidecar; it must refuse, not extract")
	}
	if !strings.Contains(errBuf.String(), "/edikt:adr:compile") {
		t.Fatalf("expected the explicit run-this list; got:\n%s", errBuf.String())
	}
}

// TestRunTwoPhase_CheckMode_RefusesBootstrap pins ADR-028: --check never
// dispatches a subagent, so a missing sidecar stays a reported error.
func TestRunTwoPhase_CheckMode_RefusesBootstrap(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageNeverInitialised(t)
	var errBuf, outBuf bytes.Buffer

	_, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		CheckOnly:   true,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
	}, model.RealClock{})
	if err == nil {
		t.Fatal("--check must exit non-zero on a missing sidecar")
	}
	// ADR-028's "--check never dispatches" is now structural rather than
	// observed: tier-2 has no dispatch path at all. What --check must still
	// not do is WRITE, so the observable is that no sidecar appeared.
	if _, serr := os.Stat(filepath.Join(root, "docs", "architecture", "decisions", "ADR-001-test.edikt.yaml")); serr == nil {
		t.Fatal("--check wrote a sidecar (ADR-028): it must report, not extract")
	}
}
