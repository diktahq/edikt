package phasea

import (
	"os"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// The second half of #11 comment item 4: a `paths:` the extractor wrote on a
// sidecar that had none is dropped at restore. Helpers shared with the
// labelling test live in restore_paths_test.go.

// extractWithPaths stands in for an extractor dispatch whose output carries
// a `paths:` (and, if given, a receipt) of its own.
func (f *verifyFixture) extractWithPaths(t *testing.T, paths []string, approval *sidecar.PathsApproval) {
	t.Helper()
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{restorePathsDirective()},
		Paths:         paths,
		PathsApproval: approval,
	})
}

func TestRestore_DropsExtractorIntroducedPaths(t *testing.T) {
	t.Run("no prior sidecar: an introduced paths is dropped", func(t *testing.T) {
		f := newFixture(t, "")
		f.extractWithPaths(t, restorePathsGlobs, nil)

		out := f.restore(t)
		if got := liveSidecar(t, f).Paths; len(got) != 0 {
			t.Fatalf("extractor-introduced paths survived restore as enforced scope: %v\n%s", got, out.Message)
		}
		assertDropNote(t, out.Message)
		if len(out.PathsDropped) != 1 || out.PathsDropped[0] != f.itemID {
			t.Errorf("PathsDropped = %v, want [%s]", out.PathsDropped, f.itemID)
		}
		if out.PinsRestored != 0 || !strings.Contains(out.Message, "1 sidecar(s) rewritten") {
			t.Errorf("a drop is a rewrite but not a pin restore: PinsRestored %d\n%s", out.PinsRestored, out.Message)
		}
		// The stamp still landed, and the item still grades: dropping scope
		// is a correction of the output, not a failure of the extraction.
		if v := f.verify(t); onlyItem(t, v).Status != StatusOK {
			t.Fatalf("verify after the drop: %+v — %s", onlyItem(t, v), v.Message)
		}
	})

	t.Run("prior with empty paths: an introduced paths is dropped", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		f.extractWithPaths(t, restorePathsGlobs, nil)

		out := f.restore(t)
		if got := liveSidecar(t, f).Paths; len(got) != 0 {
			t.Fatalf("extractor-introduced paths survived restore as enforced scope: %v\n%s", got, out.Message)
		}
		assertDropNote(t, out.Message)
	})

	t.Run("an extractor-written receipt is not an approval", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		forged := &sidecar.PathsApproval{ApprovedAt: "2026-10-04T00:00:00Z", GlobsSHA256: sidecar.HashGlobs(restorePathsGlobs)}
		f.extractWithPaths(t, restorePathsGlobs, forged)

		out := f.restore(t)
		sc := liveSidecar(t, f)
		if len(sc.Paths) != 0 || sc.PathsApproval != nil {
			t.Fatalf("a receipt the pre-dispatch sidecar never carried made introduced paths enforced scope: "+
				"paths %v, receipt %+v\n%s", sc.Paths, sc.PathsApproval, out.Message)
		}
		assertDropNote(t, out.Message)
	})

	t.Run("kept when the snapshot carried them", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		snapshotWithPaths(t, f, restorePathsGlobs, nil)
		f.extractWithPaths(t, restorePathsGlobs, nil)

		out := f.restore(t)
		if got := liveSidecar(t, f).Paths; !sameSet(got, restorePathsGlobs) {
			t.Fatalf("paths the pre-dispatch sidecar carried were dropped: got %v\n%s", got, out.Message)
		}
		if strings.Contains(out.Message, "dropped") {
			t.Errorf("restore reported a drop of paths the snapshot carried:\n%s", out.Message)
		}
	})

	t.Run("noted, not stripped, when the snapshot is unparseable", func(t *testing.T) {
		f := newStaleFixture(t, anchoredAtStaleQuote(restorePathsDirective().Text))
		if err := os.WriteFile(PinSnapshotPath(f.root, f.planID, f.itemID), []byte("paths: [unclosed\n  : :\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		f.extractWithPaths(t, restorePathsGlobs, nil)

		out := f.restore(t)
		if got := liveSidecar(t, f).Paths; !sameSet(got, restorePathsGlobs) {
			t.Fatalf("paths were stripped on the strength of a snapshot nobody could read: got %v\n%s", got, out.Message)
		}
		if !strings.Contains(out.Message, "paths:") || !strings.Contains(out.Message, "not checked") {
			t.Errorf("restore must note that whether these paths were introduced was not checked:\n%s", out.Message)
		}
	})
}

func assertDropNote(t *testing.T, msg string) {
	t.Helper()
	for _, want := range []string{"dropped", "proposed_paths", "sidecar approve --kind paths"} {
		if !strings.Contains(msg, want) {
			t.Errorf("restore's note on the dropped paths must say %q:\n%s", want, msg)
		}
	}
}
