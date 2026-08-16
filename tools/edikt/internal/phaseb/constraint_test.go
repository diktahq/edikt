package phaseb

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestPhaseBConstraint_BehavioralRequiresFalsifyingObservation (AC-4.1) verifies
// that Merge returns an actionable error when verify_kind is "behavioral" but
// falsifying_observation is empty, for both directives and prohibitions.
func TestPhaseBConstraint_BehavioralRequiresFalsifyingObservation(t *testing.T) {
	// directive violation
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:       "Services must not share mutable state. (ref: ADR-001)",
		Verify:     "go test ./internal/...",
		VerifyKind: "behavioral",
		// FalsifyingObservation intentionally empty
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err == nil {
		t.Fatal("expected error for behavioral directive without falsifying_observation, got nil")
	}
	want := "ADR-001: directives[0].verify_kind is behavioral but falsifying_observation is empty"
	if err.Error() != want {
		t.Errorf("directive violation: wrong error\ngot:  %s\nwant: %s", err.Error(), want)
	}

	// prohibition violation
	root2 := t.TempDir()
	pair2 := mkPair(t, root2, "ADR-002-test", "arch", nil)
	pair2.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:       "Never share mutable state across services. (ref: ADR-002)",
		Verify:     "go test ./internal/...",
		VerifyKind: "behavioral",
		// FalsifyingObservation intentionally empty
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"},
	}}

	_, err2 := Merge(root2, []sidecar.Pair{pair2}, Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err2 == nil {
		t.Fatal("expected error for behavioral prohibition without falsifying_observation, got nil")
	}
	want2 := "ADR-002: prohibitions[0].verify_kind is behavioral but falsifying_observation is empty"
	if err2.Error() != want2 {
		t.Errorf("prohibition violation: wrong error\ngot:  %s\nwant: %s", err2.Error(), want2)
	}
}

// TestPhaseBConstraint_VerifyRequiresKind (AC-4.2) verifies that Merge returns
// an actionable error when verify is set but verify_kind is absent, for both
// directives and prohibitions.
func TestPhaseBConstraint_VerifyRequiresKind(t *testing.T) {
	// directive violation
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:   "Only use approved libraries. (ref: ADR-001)",
		Verify: "grep -r 'approved' go.mod",
		// VerifyKind intentionally empty
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err == nil {
		t.Fatal("expected error for directive with verify but no verify_kind, got nil")
	}
	want := "ADR-001: directives[0].verify set but verify_kind is empty — run `edikt migrate sidecars --apply` to default to structural"
	if err.Error() != want {
		t.Errorf("directive violation: wrong error\ngot:  %s\nwant: %s", err.Error(), want)
	}

	// prohibition violation
	root2 := t.TempDir()
	pair2 := mkPair(t, root2, "ADR-002-test", "arch", nil)
	pair2.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:   "Never use deprecated packages. (ref: ADR-002)",
		Verify: "grep -r 'deprecated' go.mod",
		// VerifyKind intentionally empty
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"},
	}}

	_, err2 := Merge(root2, []sidecar.Pair{pair2}, Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err2 == nil {
		t.Fatal("expected error for prohibition with verify but no verify_kind, got nil")
	}
	want2 := "ADR-002: prohibitions[0].verify set but verify_kind is empty — run `edikt migrate sidecars --apply` to default to structural"
	if err2.Error() != want2 {
		t.Errorf("prohibition violation: wrong error\ngot:  %s\nwant: %s", err2.Error(), want2)
	}
}

// TestPhaseBConstraint_HumanApprovedAtDeferredToPlanB (AC-4.3) asserts that a
// behavioral directive with falsifying_observation set but human_approved_at
// absent does NOT cause a Phase B compile error — that requirement is
// intentionally deferred to Plan B (AC-4.3 in SPEC-009 Plan A).
//
// SPEC-009 Plan C Phase 4 — bidirectional fixture paths are now required
// for behavioral directives; this test declares them so the Plan B
// human_approved_at deferral remains the property under test.
func TestPhaseBConstraint_HumanApprovedAtDeferredToPlanB(t *testing.T) {
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:                  "Services must not share mutable state. (ref: ADR-001)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "A test that exercises cross-service state sharing would pass, indicating a violation.",
		// HumanApprovedAt intentionally absent — deferred to Plan B
		PositiveFixturePath: "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
		NegativeFixturePath: "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
		SourceExcerpt:       sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err != nil {
		t.Errorf("behavioral directive without human_approved_at must not fail Phase B compile (deferred to Plan B): %v", err)
	}
}

// buildConstraintCorpus constructs N sidecar.Pair entries spread across 5 topics,
// mirroring the benchmark corpus shape for use in regular tests.
func buildConstraintCorpus(t *testing.T, n int) (string, []sidecar.Pair) {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	pairs := make([]sidecar.Pair, 0, n)
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("ADR-%03d", i+1)
		base := fmt.Sprintf("%s-perf", id)
		mdPath := filepath.Join(dir, base+".md")
		if err := os.WriteFile(mdPath, []byte("# "+id+" — perf\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		topic := fmt.Sprintf("perf-topic-%d", i%5)
		ycPath := filepath.Join(dir, base+".edikt.yaml")
		if err := os.WriteFile(ycPath, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		sc := &sidecar.Sidecar{
			SchemaVersion: 1,
			Topic:         topic,
			Path:          "docs/architecture/decisions/" + base + ".md",
			Signals:       []string{"perf"},
			Directives: []sidecar.Directive{{
				Text: "Perf directive for " + id + ". (ref: " + id + ")",
				SourceExcerpt: sidecar.SourceExcerpt{
					LineStart: 1, LineEnd: 1, Quote: "# " + id + " — perf",
				},
			}},
			SourcePath: ycPath,
		}
		pairs = append(pairs, sidecar.Pair{
			ParentPath:  mdPath,
			SidecarPath: ycPath,
			ArtifactID:  id,
			Sidecar:     sc,
		})
	}
	return root, pairs
}

// TestPhaseBPerf (AC-4.4) asserts Phase B wall-clock SLOs from ADR-028:
// full render of 50 sidecars < 5s, no-op recompile < 500ms.
func TestPhaseBPerf(t *testing.T) {
	root, pairs := buildConstraintCorpus(t, 50)
	opts := Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-perf"}

	// Full render: first call writes all topic files.
	start := time.Now()
	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("full merge: %v", err)
	}
	fullElapsed := time.Since(start)
	if fullElapsed > 5*time.Second {
		t.Errorf("full render of 50 sidecars took %v; ADR-028 SLO is <5s", fullElapsed)
	}

	// No-op: second call with identical corpus hits the fingerprint cache.
	start = time.Now()
	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("no-op merge: %v", err)
	}
	noopElapsed := time.Since(start)
	if noopElapsed > 500*time.Millisecond {
		t.Errorf("no-op recompile of 50 sidecars took %v; ADR-028 SLO is <500ms", noopElapsed)
	}
}
