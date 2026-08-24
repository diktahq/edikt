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

// writeContradictionSidecar writes the sidecar the extractor would have
// produced for one artifact: one caller-chosen directive, all artifacts in
// the same topic.
//
// This used to be a contradictionRunner injected as TwoPhaseOptions.Runner,
// which let Phase A "dispatch" it and bootstrap the sidecar mid-compile.
// Phase A no longer dispatches from tier-2 (SPEC-013), so there is no
// dispatch to fake. Writing the sidecar at staging time is closer to what
// this test was always about: it exercises PHASE B's contradiction
// detection, and the fake dispatch was only ever the cheapest way to get a
// corpus with sidecars in it.
//
// The quote must appear verbatim in the parent body staged below, or the
// sidecar reads as anchor-drifted, Phase A finds work, and the compile
// refuses before Phase B runs.
func writeContradictionSidecar(t *testing.T, adrDir, id, directive string) {
	t.Helper()
	var b strings.Builder
	fmt.Fprintf(&b, "schema_version: 1\ntopic: %q\n", "conflicttopic")
	fmt.Fprintf(&b, "path: \"docs/architecture/decisions/%s-test.md\"\n", id)
	fmt.Fprintf(&b, "signals:\n  - %q\n", strings.ToLower(id)+" signal")
	fmt.Fprintf(&b, "directives:\n  - text: %q\n", directive)
	b.WriteString("    source_excerpt:\n      line_start: 9\n      line_end: 9\n      quote: \"Test rule.\"\n")
	p := filepath.Join(adrDir, fmt.Sprintf("%s-test.edikt.yaml", id))
	if err := os.WriteFile(p, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

func stageContradictionCorpus(t *testing.T, directives map[string]string) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for id, directive := range directives {
		body := fmt.Sprintf("---\nstatus: accepted\n---\n\n# %s — Test\n\n## Decision\n\nTest rule.\n", id)
		p := filepath.Join(adrDir, fmt.Sprintf("%s-test.md", id))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write adr: %v", err)
		}
		writeContradictionSidecar(t, adrDir, id, directive)
	}
	return root
}

func runContradictionCompile(t *testing.T, checkOnly bool, directives map[string]string) (*TwoPhaseResult, string) {
	t.Helper()
	t.Setenv("EDIKT_HEADLESS", "")

	// Sidecars are staged on disk, so Phase A finds nothing stale and Phase B
	// runs directly. The old bootstrap pre-pass existed only to make the fake
	// dispatch produce those sidecars; with them staged there is nothing to
	// bootstrap, and check mode no longer needs a warm-up compile.
	root := stageContradictionCorpus(t, directives)

	if checkOnly {
		// --check compares the rendered tree against a fresh render, so it
		// needs a real compile to have rendered it. That used to be a
		// "bootstrap" pass whose job was to make the fake dispatch write the
		// sidecars; with sidecars staged, this is now what it always should
		// have been — an ordinary full compile, no dispatch involved.
		var warmErr, warmOut bytes.Buffer
		if _, err := RunTwoPhase(TwoPhaseOptions{
			ProjectRoot: root,
			Stderr:      &warmErr,
			Stdout:      &warmOut,
			OnLoss:      "accept",
		}, model.RealClock{}); err != nil {
			t.Fatalf("warm-up compile failed: %v\nstderr:\n%s", err, warmErr.String())
		}
	}

	var errBuf, outBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		CheckOnly:   checkOnly,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	if err != nil {
		t.Fatalf("compile failed: %v\nstderr:\n%s", err, errBuf.String())
	}
	return res, outBuf.String() + errBuf.String()
}

// TestRunTwoPhase_DetectsPlantedContradiction pins AC-9.1 end to end
// (not just internal/contradiction's unit tests): a real RunTwoPhase call,
// full compile mode, surfaces a planted same-topic opposing-modality pair
// on TwoPhaseResult.Conflicts and in the printed report.
func TestRunTwoPhase_DetectsPlantedContradiction(t *testing.T) {
	res, out := runContradictionCompile(t, false, map[string]string{
		"ADR-001": "Diagram images MUST be stored in MinIO. (ref: ADR-001)",
		"ADR-002": "Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)",
	})
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on TwoPhaseResult, got %d: %+v", len(res.Conflicts), res.Conflicts)
	}
	if !strings.Contains(out, "Conflicts — 1 detected") {
		t.Errorf("expected report to name 1 detected conflict, got:\n%s", out)
	}
	if !strings.Contains(out, "NOT auto-resolved") {
		t.Errorf("expected report to state conflicts are not auto-resolved (AC-9.2), got:\n%s", out)
	}
}

// TestRunTwoPhase_DetectsPlantedContradiction_CheckMode pins the same
// behavior in --check mode specifically, since --check takes an early
// return path that skips Phase B entirely — the detector must be wired
// into BOTH exit points, not just the full-compile one.
func TestRunTwoPhase_DetectsPlantedContradiction_CheckMode(t *testing.T) {
	res, out := runContradictionCompile(t, true, map[string]string{
		"ADR-001": "Diagram images MUST be stored in MinIO. (ref: ADR-001)",
		"ADR-002": "Diagram images MUST NOT be stored in MinIO. (ref: ADR-002)",
	})
	if len(res.Conflicts) != 1 {
		t.Fatalf("expected 1 conflict on TwoPhaseResult in check mode, got %d: %+v", len(res.Conflicts), res.Conflicts)
	}
	if !strings.Contains(out, "conflicts: 1 detected") {
		t.Errorf("expected check-mode report to name 1 detected conflict, got:\n%s", out)
	}
}

// TestRunTwoPhase_NoConflictsReportsNone is the control: a corpus with no
// contradiction reports zero, explicitly (not silently) — matching
// INV-013's "a control that observes nothing must say so" posture.
func TestRunTwoPhase_NoConflictsReportsNone(t *testing.T) {
	res, out := runContradictionCompile(t, false, map[string]string{
		"ADR-001": "Diagram images MUST be stored in MinIO. (ref: ADR-001)",
		"ADR-002": "Backup archives MUST NOT exceed 7 days retention. (ref: ADR-002)",
	})
	if len(res.Conflicts) != 0 {
		t.Fatalf("expected 0 conflicts, got %d: %+v", len(res.Conflicts), res.Conflicts)
	}
	if !strings.Contains(out, "Conflicts — none detected") {
		t.Errorf("expected explicit 'none detected' line, got:\n%s", out)
	}
}
