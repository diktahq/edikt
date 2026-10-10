package pathsproposal

// The approval-time half of the agreement asserted in cmd/gov's
// TestConsumersAgree. This validator decides whether a PROPOSED glob matches
// anything; verify-diff decides whether an APPROVED one scopes a change. Both
// must resolve a (pattern, path) pair through internal/globmatch, or a human
// approves coverage that diff time does not grant.
//
// Held here, in the validator's own package, because matchesAny is unexported:
// a test that could only reach Validate would not be able to say WHERE the
// verdict came from.
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Falsified by mutation before being trusted, on 2026-09-06. The mutation was
// applied to a pathsproposal.go verified byte-identical to a backup taken
// first (sha256 7412660b…) and reverted from it, checked equal afterwards.
// This is F9 of the record in internal/globmatch/globmatch_test.go:
//
//	F9  matchesAny answers with strings.HasPrefix over
//	    globmatch.LiteralPrefix instead of globmatch.Match — a
//	    second opinion about what a glob means, held by the side
//	    that decides what a human is allowed to approve         → RED
//	    TestMatchesAnyRoutesThroughGlobmatch,
//	    TestMatchesAnyIsNotWidenedByTheFix, and cmd/gov's
//	    TestConsumersAgree
//
// The cross-consumer test going red on a defect planted HERE is the half that
// could not be assumed: it is what distinguishes "both consumers agree" from
// "the diff-time consumer is tested twice".

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
)

// TestMatchesAnyRoutesThroughGlobmatch pins that the match rule is
// globmatch.Match over the enumerated file set and nothing else — including
// for the shape that matched nothing in every repository before this phase, a
// glob with a `*` before a `**`.
func TestMatchesAnyRoutesThroughGlobmatch(t *testing.T) {
	files := []string{
		"internal/billing/migrations/001_init.sql",
		"internal/a/x/y.go",
		"tools/edikt/cmd/gov/compile.go",
		"docs/c.md",
		"README.md",
	}

	for _, tc := range []struct {
		glob string
		want bool
	}{
		{"internal/*/migrations/**/*.sql", true},
		{"internal/*/x/**/*.go", true},
		{"tools/*/cmd/**/*.go", true},
		{"docs/**/*.md", true},
		{"internal/*/migrations/**/*.go", false},
		{"internal/rag/**/*.go", false},
	} {
		got := matchesAny(tc.glob, files)
		if got != tc.want {
			t.Errorf("matchesAny(%q, files) = %v, want %v", tc.glob, got, tc.want)
		}

		// Same verdict, computed the only other way it may legitimately be
		// computed: through the unified matcher, file by file.
		want := false
		for _, f := range files {
			if globmatch.Match(tc.glob, f) {
				want = true
			}
		}
		if got != want {
			t.Errorf("matchesAny(%q, files) = %v but globmatch.Match over the same files = %v — the validator has a second opinion about what a glob means",
				tc.glob, got, want)
		}
	}
}

// TestMatchesAnyIsNotWidenedByTheFix guards the direction of the repair: the
// wildcard-before-`**` fix must not turn `*` into a cross-separator wildcard,
// which would silently widen every approved scope in every corpus.
func TestMatchesAnyIsNotWidenedByTheFix(t *testing.T) {
	files := []string{"internal/a/b/x/y.go", "internal/a/b/c/d/e.sql"}

	if matchesAny("internal/*/x/**/*.go", files) {
		t.Error(`matchesAny("internal/*/x/**/*.go", ["internal/a/b/x/y.go"]) = true — a single * crossed a path separator`)
	}
	if matchesAny("internal/*/migrations/**/*.sql", files) {
		t.Error(`matchesAny("internal/*/migrations/**/*.sql", ...) = true — a literal segment was skipped`)
	}
}
