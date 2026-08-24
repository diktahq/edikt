package reextract

// testhelpers_sidecar_test.go — the two sidecar-fixture helpers
// reextract_test.go's pin-preservation wiring tests use. They moved out of
// this package with preserve_test.go when pin preservation became
// internal/pinpreserve (so internal/phasea could reuse it without the
// reextract → phasea import cycle); reextract's own tests still need them
// to build a fixture corpus, so they live here now. Test-only fixture
// construction, deliberately not exported from pinpreserve: a fixture
// builder is not part of that package's contract.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func anchored(text string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 7, LineEnd: 7, Quote: "A rule lives here."}},
	}
}

func writeSidecar(t *testing.T, path string, sc *sidecar.Sidecar) {
	t.Helper()
	sc.SchemaVersion = 2
	sc.Topic = "testing"
	sc.Path = "docs/architecture/decisions/ADR-900.md"
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}
