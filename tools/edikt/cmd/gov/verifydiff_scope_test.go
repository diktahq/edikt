package gov

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestVdSidecarInScope_UndeclaredPathsMeansEverywhere pins B2b (SPEC-010
// phase 8, AC-8.3): a sidecar with no declared Paths globs must be treated
// as in scope for any changed file, matching phaseb/merge.go's scopeFor
// ("an undeclared sidecar contributes everywhere"). Before the fix, an
// undeclared sidecar was in scope only when the changed file matched its own
// governance doc Path — the opposite reading from the compiled topic files.
func TestVdSidecarInScope_UndeclaredPathsMeansEverywhere(t *testing.T) {
	sc := &sidecar.Sidecar{
		Path:  "docs/architecture/decisions/ADR-999-example.md",
		Paths: nil,
	}

	// A changed file with no relation at all to the sidecar's own doc path
	// or topic must still be in scope, because Paths is undeclared.
	if !vdSidecarInScope(sc, []string{"internal/unrelated/whatever.go"}) {
		t.Fatal("undeclared Paths must mean everywhere, not \"matches nothing\" — a completely unrelated changed file was excluded")
	}
}

// TestVdSidecarInScope_DeclaredPathsAreStillRestrictive pins the unchanged
// half of the contract: a sidecar that DOES declare Paths globs is scoped
// normally — declaring globs must still narrow, not just widen.
func TestVdSidecarInScope_DeclaredPathsAreStillRestrictive(t *testing.T) {
	sc := &sidecar.Sidecar{
		Path:  "docs/architecture/decisions/ADR-999-example.md",
		Paths: []string{"internal/rag/**/*.go"},
	}

	if vdSidecarInScope(sc, []string{"internal/unrelated/whatever.go"}) {
		t.Fatal("a sidecar with declared Paths must not match a file outside every declared glob and outside its own doc path")
	}
	if !vdSidecarInScope(sc, []string{"internal/rag/chunk.go"}) {
		t.Fatal("a sidecar with declared Paths must match a file inside a declared glob")
	}
	if !vdSidecarInScope(sc, []string{sc.Path}) {
		t.Fatal("a sidecar must always be in scope for an edit to its own governance doc")
	}
}

// TestVdSidecarInScope_NoChangedFilesIsNeverInScope guards the edge case:
// an undeclared sidecar with zero changed files (e.g. an empty diff) must
// not be reported in scope — "everywhere" only applies across a non-empty
// set of changed files.
func TestVdSidecarInScope_NoChangedFilesIsNeverInScope(t *testing.T) {
	sc := &sidecar.Sidecar{Paths: nil}
	if vdSidecarInScope(sc, nil) {
		t.Fatal("an undeclared sidecar with zero changed files must not be reported in scope")
	}
}
