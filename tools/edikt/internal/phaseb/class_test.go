package phaseb

import (
	"testing"
)

// DESIGN-QUESTIONS-2026-08-16.md Q2 — classFor derives a structural class
// from the artifact ID prefix alone, the same signal gradeFor already
// consults for invariants. Additive: an IndexEntry with no recognizable
// prefix gets "unknown", not an error, since compile must never fail on a
// directive it can classify by grade but not by source kind.

func TestClassFor_Invariant(t *testing.T) {
	if got := classFor("INV-005"); got != "invariant" {
		t.Fatalf("INV- prefix must classify as invariant, got %q", got)
	}
}

func TestClassFor_ADR(t *testing.T) {
	if got := classFor("ADR-055"); got != "adr" {
		t.Fatalf("ADR- prefix must classify as adr, got %q", got)
	}
}

func TestClassFor_Guideline(t *testing.T) {
	if got := classFor("GL-002"); got != "guideline" {
		t.Fatalf("GL- prefix must classify as guideline, got %q", got)
	}
}

func TestClassFor_UnrecognizedPrefixIsUnknownNotError(t *testing.T) {
	// A future artifact kind, or a malformed ID, must not panic or block
	// compile — it degrades to "unknown", same fail-open posture as the
	// rest of the write-time tier (ADR-060:d07).
	got := classFor("SPEC-011")
	if got != "unknown" {
		t.Fatalf("unrecognized prefix must classify as unknown, got %q", got)
	}
}
