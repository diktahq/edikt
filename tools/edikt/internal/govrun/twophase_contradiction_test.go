package govrun

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// contradictionRunner writes a sidecar with one caller-chosen directive per
// artifact, all in the same topic — standing in for the extractor the same
// way scopeRunner does, but with directive text under the test's control
// instead of a fixed "Test rule MUST hold" string.
type contradictionRunner struct {
	directives map[string]string // artifact ID -> directive text
}

func (r *contradictionRunner) Preflight() error { return nil }

func (r *contradictionRunner) Resync(_ context.Context, t phasea.Task) error {
	var b strings.Builder
	fmt.Fprintf(&b, "schema_version: 1\ntopic: %q\n", "conflicttopic")
	fmt.Fprintf(&b, "path: \"docs/architecture/decisions/%s\"\n", filepath.Base(t.ParentPath))
	fmt.Fprintf(&b, "signals:\n  - %q\n", strings.ToLower(t.ArtifactID)+" signal")
	fmt.Fprintf(&b, "directives:\n  - text: %q\n", r.directives[t.ArtifactID])
	b.WriteString("    source_excerpt:\n      line_start: 9\n      line_end: 9\n      quote: \"Test rule.\"\n")
	return os.WriteFile(t.SidecarPath, []byte(b.String()), 0o644)
}

func stageContradictionCorpus(t *testing.T, ids ...string) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, id := range ids {
		body := fmt.Sprintf("---\nstatus: accepted\n---\n\n# %s — Test\n\n## Decision\n\nTest rule.\n", id)
		p := filepath.Join(adrDir, fmt.Sprintf("%s-test.md", id))
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write adr: %v", err)
		}
	}
	return root
}

func runContradictionCompile(t *testing.T, checkOnly bool, directives map[string]string) (*TwoPhaseResult, string) {
	t.Helper()
	t.Setenv("EDIKT_HEADLESS", "")

	ids := make([]string, 0, len(directives))
	for id := range directives {
		ids = append(ids, id)
	}
	root := stageContradictionCorpus(t, ids...)
	runner := &contradictionRunner{directives: directives}

	if checkOnly {
		// --check refuses on a bootstrap (missing sidecar) by design — see
		// TestRunTwoPhase_CheckMode_RefusesBootstrap. To exercise check
		// mode's conflict reporting specifically, bootstrap for real first
		// (full compile, dispatches the fake runner) so non-stale sidecars
		// already exist on disk, then run the actual check-mode call
		// against them with no dispatch needed.
		var bootErr, bootOut bytes.Buffer
		if _, err := RunTwoPhase(TwoPhaseOptions{
			ProjectRoot: root,
			Runner:      runner,
			Stderr:      &bootErr,
			Stdout:      &bootOut,
			OnLoss:      "accept",
		}, model.RealClock{}); err != nil {
			t.Fatalf("bootstrap compile failed: %v\nstderr:\n%s", err, bootErr.String())
		}
	}

	var errBuf, outBuf bytes.Buffer
	res, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
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
