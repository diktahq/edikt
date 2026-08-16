package govrun

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestV1ShapedSidecarsDetection pins the atomicity pre-flight's discriminator.
//
// Both halves matter. Sensitivity: a v1 sidecar and a v2-versioned sidecar
// still carrying a singular anchor must both be caught — the second is the one
// a version-number-only check would wave through. Isolation: a genuine v2
// sidecar must NOT be flagged, or the pre-flight refuses every compile forever
// and the only way anyone would fix that is by deleting the check.
func TestV1ShapedSidecarsDetection(t *testing.T) {
	v1 := &sidecar.Sidecar{
		SchemaVersion: 1, Topic: "compile", Path: "a.md",
		Directives: []sidecar.Directive{{
			Text:          "A MUST b. (ref: ADR-001)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "A must b."},
		}},
	}
	v2mixed := &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "compile", Path: "b.md",
		Directives: []sidecar.Directive{{
			Text:          "C MUST d. (ref: ADR-002)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "C must d."},
		}},
	}
	v2 := &sidecar.Sidecar{
		SchemaVersion: 2, Topic: "compile", Path: "c.md",
		Directives: []sidecar.Directive{{
			Text:           "E MUST f. (ref: ADR-003)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 1, LineEnd: 1, Quote: "E must f."}},
		}},
	}

	got := v1ShapedSidecars([]sidecar.Pair{
		{ArtifactID: "ADR-001", Sidecar: v1},
		{ArtifactID: "ADR-002", Sidecar: v2mixed},
		{ArtifactID: "ADR-003", Sidecar: v2},
		{ArtifactID: "ADR-004", Sidecar: nil},
	})

	want := map[string]bool{"ADR-001": true, "ADR-002": true}
	if len(got) != len(want) {
		t.Fatalf("flagged %v, want exactly %v", got, []string{"ADR-001", "ADR-002"})
	}
	for _, id := range got {
		if !want[id] {
			t.Errorf("flagged %s, which is a valid v2 sidecar — the pre-flight would refuse every compile", id)
		}
	}

	// Isolation, stated as its own assertion: a fully-migrated corpus flags
	// nothing. Without this the test above would still pass if the function
	// flagged everything it was given.
	if clean := v1ShapedSidecars([]sidecar.Pair{{ArtifactID: "ADR-003", Sidecar: v2}}); len(clean) != 0 {
		t.Errorf("a fully v2 corpus was flagged: %v", clean)
	}

	if n := countLoaded([]sidecar.Pair{{Sidecar: v1}, {Sidecar: nil}, {Sidecar: v2}}); n != 2 {
		t.Errorf("countLoaded = %d, want 2 (the denominator must count loaded sidecars, not pairs)", n)
	}
}
