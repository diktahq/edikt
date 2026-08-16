package phaseb

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
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

// IndexEntriesFor is the actual call site — prove Class survives the real
// construction path, not just the helper function in isolation.
func TestIndexEntriesFor_PopulatesClass(t *testing.T) {
	pair := sidecar.Pair{
		ArtifactID: "INV-901",
		Sidecar: &sidecar.Sidecar{
			Paths: []string{"tools/**/*.go"},
			Directives: []sidecar.Directive{
				{Text: "Tier-2 Go binaries MUST NOT spawn any LLM CLI. (ref: INV-901)"},
			},
		},
	}
	dirs, _ := IndexEntriesFor(pair)
	if len(dirs) != 1 {
		t.Fatalf("expected 1 directive entry, got %d", len(dirs))
	}
	if dirs[0].Class != "invariant" {
		t.Fatalf("IndexEntriesFor must populate Class from the artifact ID, got %q", dirs[0].Class)
	}
}

// The rendered YAML must carry class: so hookmatch (a separate package,
// reading the file back from disk) can see it — a Go-struct-only field with
// no serialization path would be dead weight nobody downstream ever reads.
func TestRenderDirectiveIndex_EmitsClass(t *testing.T) {
	idx := map[string][]IndexEntry{
		"tools/**/*.go": {
			{ID: "INV-901:d01", Grade: "must", Class: "invariant", Text: "x"},
		},
	}
	out := RenderDirectiveIndex(idx)
	if !strings.Contains(out, "class: invariant") {
		t.Fatalf("rendered index must contain 'class: invariant', got:\n%s", out)
	}
}
