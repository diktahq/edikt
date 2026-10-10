// This file is the Go half of a THREE-WAY agreement. `test/fixtures/
// globmatch-parity.tsv` is one table of (pattern, path, verdict) rows;
// TestMatch_PythonParityCorpus asserts this package satisfies it, and
// `test/test-globmatch-python-parity.sh` asserts every Python surface that
// reads a `paths:` glob satisfies the same rows.
//
// WHY IT EXISTS. `internal/pathsproposal/removal.go` says the Go code is
// "deliberately kept byte-for-byte equivalent to their Python counterparts so
// the two never silently drift (GL-002: one rule, one implementation, or the
// weaker one is the one that gets used)". Nothing enforced that. When this
// package's `**` semantics were corrected, both Python copies stayed where
// they were and the two languages began disagreeing on four shapes —
// `docs/a**` vs `docs/a/b/x.md`, `docs/**x/*.md` vs `docs/a/b/x/c.md`,
// `docs/**` vs `docs`, `.claude/agents/**` vs `.claude/agents`. Measured on
// the real corpus the drift moved nothing (838,796 (glob, tracked-file) pairs,
// zero disagreements; the 9 differing pairs are directory paths under a
// trailing `/**`, which no consumer feeds). That is what made it invisible:
// the gate positioned to notice — test/lib/tier1-arrival-scan.py, pinning six
// tier1 baselines — MEASURES WITH THE PYTHON COPY, so it stayed green by
// construction and would have stayed green through the first glob written in
// either shape.
//
// The fixture is therefore loaded from the repo rather than restated here. A
// table copied into each language is two tables, which is the arrangement that
// failed.
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Measured 2026-09-06. Each mutation was applied to a file backed up first and
// reverted from that backup, with the sha256 checked equal before and after:
// globmatch.go eefb937f…, test/lib/globmatch.py cb579608…,
// test/fixtures/globmatch-parity.tsv eedf06de…. F11's subject is THIS file,
// 42fe0879… when it was mutated and reverted; completing this comment block is
// the only change to it since.
//
//	F10 globmatch.go's `**` branch treats a segment as multi-segment
//	    when it CONTAINS `**` rather than when it IS `**` — the Go
//	    side put back where the Python copies still stood        → RED
//	    TestMatch_PythonParityCorpus on exactly 2 rows (`docs/a**`
//	    vs `docs/a/b/x.md`, `docs/**x/*.md` vs `docs/a/b/x/c.md`),
//	    which are 2 of the 4 shapes the live drift was measured
//	    on. test-globmatch-python-parity.sh RED with it, Go half
//	F11 the fixture path is misspelled, so the corpus cannot be
//	    read                                                     → RED
//	    TestMatch_PythonParityCorpus, by t.Fatalf rather than by
//	    passing vacuously. A parity test that cannot find its
//	    table must not report agreement (INV-013)
//	F12 test/lib/globmatch.py's trailing-`**` branch emits `/.*`
//	    instead of `(?:/.*)?`, so the Python side stops matching
//	    the bare parent — the live drift, re-created             → RED
//	    test-globmatch-python-parity.sh on its Python half, on 3
//	    rows and in ALL THREE Python surfaces at once — which is
//	    also the proof that both consumers really route through
//	    the shared module rather than merely importing it. And
//	    TestMatch_PythonParityCorpus stayed GREEN throughout. That
//	    asymmetry is the point: the Go test alone cannot see a
//	    Python regression, which is why the shell gate is the
//	    deliverable and this file is only half of it
//	F13 the two `docs/a**` rows are deleted from the shared
//	    corpus — the drift record quietly dropped               → RED
//	    test-globmatch-python-parity.sh's shape guard, naming
//	    `docs/a**`, while every implementation still agreed on
//	    the 67 rows that remained. A corpus can go green by
//	    shrinking, so the four drifted shapes are asserted by
//	    name and not only by being in the table
package globmatch

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// parityFixture resolves test/fixtures/globmatch-parity.tsv from this source
// file's own location, so the test does not depend on the working directory it
// is invoked from.
func parityFixture(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed — cannot locate the parity fixture")
	}
	root := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	p, err := filepath.Abs(filepath.Join(root, "test", "fixtures", "globmatch-parity.tsv"))
	if err != nil {
		t.Fatalf("resolving the parity fixture: %v", err)
	}
	return p
}

// TestMatch_PythonParityCorpus asserts every row of the shared table.
//
// The row count is asserted too. A table that shrinks to nothing — or a parse
// that silently drops every line — would otherwise report agreement while
// measuring nothing, which is the shape INV-013 forbids.
func TestMatch_PythonParityCorpus(t *testing.T) {
	path := parityFixture(t)
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading the shared parity corpus at %s: %v — a parity test that cannot read its table must not report agreement", path, err)
	}

	rows := 0
	for i, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("%s:%d: %d tab-separated fields, want 3 — the table is the contract and a row this reader cannot parse is a row the Python half may read differently", path, i+1, len(fields))
		}
		pattern, subject, verdict := unEmpty(fields[0]), unEmpty(fields[1]), fields[2]
		var want bool
		switch verdict {
		case "true":
			want = true
		case "false":
			want = false
		default:
			t.Fatalf("%s:%d: verdict %q is neither true nor false", path, i+1, verdict)
		}
		rows++
		if got := Match(pattern, subject); got != want {
			t.Errorf("%s:%d: Match(%q, %q) = %v, want %v — Go disagrees with the shared table, so it disagrees with every Python reader of a `paths:` glob",
				path, i+1, pattern, subject, got, want)
		}
	}

	if rows < 60 {
		t.Fatalf("%s carries %d rows — the corpus was authored with 70 covering all eight documented capabilities and both consumers' shapes; a table this small is not the one this test claims to assert", path, rows)
	}
}

// unEmpty decodes the table's `<empty>` sentinel. Neither an empty pattern nor
// an empty path can be written literally in a tab-separated row without the
// field vanishing into whitespace, and both are real cases: an empty pattern
// matches nothing, and `**` matches an empty path.
func unEmpty(field string) string {
	if field == "<empty>" {
		return ""
	}
	return field
}
