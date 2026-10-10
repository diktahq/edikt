package phasea

import (
	"os"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// #11 comment item 4: restore's account of artifact-level `paths:`.
//
// Two defects, one per test. Restore reported every carried glob set as
// "approved paths", receipt or not; and a `paths:` the extractor wrote on a
// sidecar that had none became enforced scope with no approval at all. Scope
// enters through `proposed_paths` and `edikt sidecar approve --kind paths`,
// and through nothing else.

var restorePathsGlobs = []string{"tools/edikt/**/*.go"}

func restorePathsDirective() sidecar.Directive {
	return anchoredAtGateQuote("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)")
}

// snapshotWithPaths overwrites the plan's pin snapshot with a prior sidecar
// carrying paths (and optionally their receipt).
func snapshotWithPaths(t *testing.T, f *verifyFixture, paths []string, approval *sidecar.PathsApproval) {
	t.Helper()
	writeMarshalledSidecar(t, PinSnapshotPath(f.root, f.planID, f.itemID), &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtStaleQuote(restorePathsDirective().Text)},
		Paths:         paths,
		PathsApproval: approval,
	})
}

func liveSidecar(t *testing.T, f *verifyFixture) *sidecar.Sidecar {
	t.Helper()
	sc, err := sidecar.Load(f.scPath)
	if err != nil {
		t.Fatalf("load restored sidecar: %v", err)
	}
	return sc
}

func TestRestore_UnreceiptedPathsNotLabelledApproved(t *testing.T) {
	approval := &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs(restorePathsGlobs)}

	t.Run("unreceipted paths are carried forward and labelled unreceipted", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		snapshotWithPaths(t, f, restorePathsGlobs, nil)
		f.extractGood(t, restorePathsDirective())

		out := f.restore(t)
		if got := liveSidecar(t, f).Paths; !sameSet(got, restorePathsGlobs) {
			t.Fatalf("restore must still carry unreceipted paths forward (grandfathered): got %v", got)
		}
		if strings.Contains(out.Message, "approved paths") {
			t.Errorf("restore called paths no receipt covers \"approved paths\":\n%s", out.Message)
		}
		if !strings.Contains(out.Message, "unreceipted") {
			t.Errorf("restore must say the carried paths are unreceipted:\n%s", out.Message)
		}
	})

	t.Run("receipted paths are labelled approved", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		snapshotWithPaths(t, f, restorePathsGlobs, approval)
		f.extractGood(t, restorePathsDirective())

		out := f.restore(t)
		if !strings.Contains(out.Message, "approved paths") {
			t.Errorf("restore must call receipted paths approved:\n%s", out.Message)
		}
		if strings.Contains(out.Message, "unreceipted") {
			t.Errorf("receipted paths labelled unreceipted:\n%s", out.Message)
		}
	})

	t.Run("an unparseable snapshot makes no claim and does not panic", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		if err := os.WriteFile(PinSnapshotPath(f.root, f.planID, f.itemID), []byte("paths: [unclosed\n  : :\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.extractGood(t, restorePathsDirective())

		out := f.restore(t)
		if strings.Contains(out.Message, "approved paths") || strings.Contains(out.Message, "unreceipted paths") {
			t.Errorf("a snapshot nobody could read produced a paths claim:\n%s", out.Message)
		}
	})
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	seen := map[string]int{}
	for _, s := range a {
		seen[s]++
	}
	for _, s := range b {
		seen[s]--
	}
	for _, n := range seen {
		if n != 0 {
			return false
		}
	}
	return true
}
