package phaseb

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestCountSources_NoPhantomGuidelines reproduces the reported miscount:
// governance.md's source header claimed "3 guidelines" for a project whose
// guidelines directory held a single README, and still claimed "2
// guidelines" after that directory was deleted outright. No directives had
// originated from a guideline in either case.
//
// The cause was classification by filename: "no ADR-/INV- prefix ⇒
// guideline" tallied every README and stray note living in the decisions
// and invariants directories.
func TestCountSources_NoPhantomGuidelines(t *testing.T) {
	sc := &sidecar.Sidecar{}

	pairs := []sidecar.Pair{
		// Real compile inputs.
		{ArtifactID: "ADR-001", Kind: sidecar.KindADR, Sidecar: sc},
		{ArtifactID: "ADR-002", Kind: sidecar.KindADR, Sidecar: sc},
		{ArtifactID: "INV-001", Kind: sidecar.KindInvariant, Sidecar: sc},

		// READMEs are skip-listed, so they carry no sidecar and contribute
		// nothing. Under the old prefix rule each of these counted as a
		// guideline — including the two that are not in the guidelines
		// directory at all.
		{ArtifactID: "README", Kind: sidecar.KindADR},
		{ArtifactID: "README", Kind: sidecar.KindInvariant},
		{ArtifactID: "README", Kind: sidecar.KindGuideline},

		// A stray note in the decisions directory: no ADR- prefix, but
		// not a guideline either.
		{ArtifactID: "NOTES", Kind: sidecar.KindADR},
	}

	if got := countSources(pairs, sidecar.KindGuideline); got != 0 {
		t.Errorf("guidelines = %d, want 0 — no guideline contributed a sidecar", got)
	}
	if got := countSources(pairs, sidecar.KindADR); got != 2 {
		t.Errorf("ADRs = %d, want 2", got)
	}
	if got := countSources(pairs, sidecar.KindInvariant); got != 1 {
		t.Errorf("invariants = %d, want 1", got)
	}
}

// A guideline that actually contributes a sidecar must be counted.
func TestCountSources_RealGuidelineCounts(t *testing.T) {
	pairs := []sidecar.Pair{
		{ArtifactID: "error-handling", Kind: sidecar.KindGuideline, Sidecar: &sidecar.Sidecar{}},
		{ArtifactID: "naming", Kind: sidecar.KindGuideline, Sidecar: &sidecar.Sidecar{}},
		// Present but never compiled — not an input to this compile.
		{ArtifactID: "draft-guideline", Kind: sidecar.KindGuideline},
	}
	if got := countSources(pairs, sidecar.KindGuideline); got != 2 {
		t.Errorf("guidelines = %d, want 2", got)
	}
}
