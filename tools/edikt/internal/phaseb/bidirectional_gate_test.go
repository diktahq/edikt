package phaseb

// bidirectional_gate_test.go — SPEC-009 Plan C Phase 4.
//
// Bidirectional fixture gate: every behavioral directive (and prohibition)
// MUST declare both a positive_fixture_path and a negative_fixture_path at
// compile time. Files need not exist on disk — declaration is sufficient;
// fixture-execution validation is a benchmark-time concern.
//
// AC-4.3: missing positive_fixture_path → Phase B compile MUST fail.
// AC-4.4: missing negative_fixture_path → Phase B compile MUST fail.
// AC-4.5: both fixture paths declared        → Phase B compile MUST pass.

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestBidirectionalGate_MissingPositiveFixture (AC-4.3) verifies that a
// behavioral directive with falsifying_observation set but
// positive_fixture_path missing fails Phase B with an actionable error.
func TestBidirectionalGate_MissingPositiveFixture(t *testing.T) {
	// directive arm
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:                  "Services must not share mutable state. (ref: ADR-001)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "A test that exercises cross-service state sharing passes.",
		// PositiveFixturePath intentionally absent
		NegativeFixturePath: "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
		SourceExcerpt:       sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err == nil {
		t.Fatal("expected error for behavioral directive without positive_fixture_path, got nil")
	}
	if !strings.Contains(err.Error(), "positive_fixture_path is empty") {
		t.Errorf("directive arm: error does not mention positive_fixture_path: %v", err)
	}
	if !strings.Contains(err.Error(), "Plan C") {
		t.Errorf("directive arm: error does not mention Plan C: %v", err)
	}

	// prohibition arm
	root2 := t.TempDir()
	pair2 := mkPair(t, root2, "ADR-002-test", "arch", nil)
	pair2.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:                  "Never share mutable state across services. (ref: ADR-002)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "Cross-service mutable state observed in violation test.",
		// PositiveFixturePath intentionally absent
		NegativeFixturePath: "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
		SourceExcerpt:       sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"},
	}}

	_, err2 := Merge(root2, []sidecar.Pair{pair2}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err2 == nil {
		t.Fatal("expected error for behavioral prohibition without positive_fixture_path, got nil")
	}
	if !strings.Contains(err2.Error(), "positive_fixture_path is empty") {
		t.Errorf("prohibition arm: error does not mention positive_fixture_path: %v", err2)
	}
}

// TestBidirectionalGate_MissingNegativeFixture (AC-4.4) verifies that a
// behavioral directive with positive_fixture_path declared but
// negative_fixture_path missing fails Phase B with an actionable error.
func TestBidirectionalGate_MissingNegativeFixture(t *testing.T) {
	// directive arm
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:                  "Services must not share mutable state. (ref: ADR-001)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "A test that exercises cross-service state sharing passes.",
		PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
		// NegativeFixturePath intentionally absent
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err == nil {
		t.Fatal("expected error for behavioral directive without negative_fixture_path, got nil")
	}
	if !strings.Contains(err.Error(), "negative_fixture_path is empty") {
		t.Errorf("directive arm: error does not mention negative_fixture_path: %v", err)
	}
	if !strings.Contains(err.Error(), "Plan C") {
		t.Errorf("directive arm: error does not mention Plan C: %v", err)
	}

	// prohibition arm
	root2 := t.TempDir()
	pair2 := mkPair(t, root2, "ADR-002-test", "arch", nil)
	pair2.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:                  "Never share mutable state across services. (ref: ADR-002)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "Cross-service mutable state observed in violation test.",
		PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
		// NegativeFixturePath intentionally absent
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"},
	}}

	_, err2 := Merge(root2, []sidecar.Pair{pair2}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err2 == nil {
		t.Fatal("expected error for behavioral prohibition without negative_fixture_path, got nil")
	}
	if !strings.Contains(err2.Error(), "negative_fixture_path is empty") {
		t.Errorf("prohibition arm: error does not mention negative_fixture_path: %v", err2)
	}
}

// TestBidirectionalGate_BothFixturesDeclared (AC-4.5) verifies that a
// behavioral directive with both fixture paths declared passes Phase B
// — even when the underlying files do not exist on disk. Existence is a
// benchmark-time concern, not a compile-time one.
func TestBidirectionalGate_BothFixturesDeclared(t *testing.T) {
	// directive arm
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{{
		Text:                  "Services must not share mutable state. (ref: ADR-001)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "A test that exercises cross-service state sharing passes.",
		PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
		NegativeFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
		SourceExcerpt:         sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
	}})

	_, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err != nil {
		t.Errorf("behavioral directive with both fixture paths must pass Phase B: %v", err)
	}

	// prohibition arm
	root2 := t.TempDir()
	pair2 := mkPair(t, root2, "ADR-002-test", "arch", nil)
	pair2.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:                  "Never share mutable state across services. (ref: ADR-002)",
		Verify:                "go test ./internal/...",
		VerifyKind:            "behavioral",
		FalsifyingObservation: "Cross-service mutable state observed in violation test.",
		PositiveFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.positive.sh",
		NegativeFixturePath:   "test/fixtures/bidirectional/example-behavioral-directive.negative.sh",
		SourceExcerpt:         sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"},
	}}

	_, err2 := Merge(root2, []sidecar.Pair{pair2}, Options{CompiledAt: "2026-05-23T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err2 != nil {
		t.Errorf("behavioral prohibition with both fixture paths must pass Phase B: %v", err2)
	}
}
