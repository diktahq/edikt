package hook

// stophook_drift_fingerprint_test.go — AC-4.4: stopHookSidecarDriftFast
// trusts a pair that has not moved since the last compile's own
// manifest.yaml (the "compile fingerprint") WITHOUT re-running IsStale on
// it. TestStopHookStaleSidecars already proves the three OUTPUT outcomes are
// correct; this file proves the SKIP itself is real, not merely that the
// answer happens to come out right — a pair that WOULD fail IsStale if
// examined, but is older than the manifest, must still read as clean.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func TestStopHookDriftUsesFingerprint_UnchangedPairIsTrustedWithoutInspection(t *testing.T) {
	root, _ := stopHookSandbox(t)

	past := time.Now().Add(-time.Hour)
	future := time.Now()

	// A pair that IS genuinely stale (the sidecar's quote appears nowhere in
	// the prose) — if stopHookSidecarDriftFast ever called IsStale on it,
	// this test would see a finding. Both files predate the manifest.
	parentPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-002-demo.md")
	writeTestFile(t, parentPath, "# ADR-002\n\n## Decision\n\nThe original sentence.\n")
	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         "demo",
		Path:          "docs/architecture/decisions/ADR-002-demo.md",
		Directives: []sidecar.Directive{{
			Text: "The original sentence. (ref: ADR-002)",
			SourceExcerpt: sidecar.SourceExcerpt{
				LineStart: 5,
				LineEnd:   5,
				Quote:     "A sentence that does not appear in the prose at all.",
			},
		}},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	sidecarPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-002-demo.edikt.yaml")
	writeTestFile(t, sidecarPath, string(body))
	for _, p := range []string{parentPath, sidecarPath} {
		if err := os.Chtimes(p, past, past); err != nil {
			t.Fatalf("chtimes %s: %v", p, err)
		}
	}

	// Sanity check on the fixture itself: this pair really would fail
	// IsStale if it were examined, via the same reader stopHookSidecarDrift
	// calls — otherwise this test would pass for the wrong reason.
	pairSidecar, rerr := sidecar.Load(sidecarPath)
	if rerr != nil {
		t.Fatalf("load sidecar for the sanity check: %v", rerr)
	}
	if stale, _, ierr := pairSidecar.IsStale(root); ierr != nil || !stale {
		t.Fatalf("fixture sanity check failed: stale=%v err=%v, want a genuinely stale fixture", stale, ierr)
	}

	// The manifest — the compile fingerprint — postdates both files.
	manifestPath := filepath.Join(root, ".claude", "rules", "governance", "manifest.yaml")
	writeTestFile(t, manifestPath, "# manifest\n")
	if err := os.Chtimes(manifestPath, future, future); err != nil {
		t.Fatalf("chtimes manifest: %v", err)
	}

	got := stopHook(newStopHookInvocation("Updated the README wording."))
	if strings.Contains(got.Response.Message, "sidecar-stale") {
		t.Fatalf("a pair unchanged since the last compile must be trusted without re-inspection; got %q",
			got.Response.Message)
	}
	if got.Response.Text != adapter.ContinueTrueReply {
		t.Fatalf("text = %q, want %q", got.Response.Text, adapter.ContinueTrueReply)
	}
}

func TestStopHookDriftUsesFingerprint_ChangedProseIsReExamined(t *testing.T) {
	root, _ := stopHookSandbox(t)

	past := time.Now().Add(-time.Hour)
	present := time.Now()

	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         "demo",
		Path:          "docs/architecture/decisions/ADR-003-demo.md",
		Directives: []sidecar.Directive{{
			Text: "The current sentence. (ref: ADR-003)",
			SourceExcerpt: sidecar.SourceExcerpt{
				LineStart: 5,
				LineEnd:   5,
				Quote:     "The current sentence.",
			},
		}},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	sidecarPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-003-demo.edikt.yaml")
	writeTestFile(t, sidecarPath, string(body))
	if err := os.Chtimes(sidecarPath, past, past); err != nil {
		t.Fatalf("chtimes sidecar: %v", err)
	}

	manifestPath := filepath.Join(root, ".claude", "rules", "governance", "manifest.yaml")
	writeTestFile(t, manifestPath, "# manifest\n")
	if err := os.Chtimes(manifestPath, past.Add(time.Minute), past.Add(time.Minute)); err != nil {
		t.Fatalf("chtimes manifest: %v", err)
	}

	// The prose changes AFTER the manifest — a real edit the fingerprint
	// short-circuit must not skip.
	parentPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-003-demo.md")
	writeTestFile(t, parentPath, "# ADR-003\n\n## Decision\n\nA replaced sentence.\n")
	if err := os.Chtimes(parentPath, present, present); err != nil {
		t.Fatalf("chtimes parent: %v", err)
	}

	got := stopHook(newStopHookInvocation("Updated the README wording."))
	if !strings.Contains(got.Response.Message, "⚠ sidecar-stale: ADR-003") {
		t.Fatalf("a prose edit newer than the manifest must be re-examined and found stale; got %q",
			got.Response.Message)
	}
}
