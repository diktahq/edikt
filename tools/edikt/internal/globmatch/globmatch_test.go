// Package globmatch had ZERO test reach until this file — one source file, and
// nothing anywhere in the tree exercising it — while its own doc comment calls
// it "the ONE place edikt decides whether a repo-relative path matches a
// sidecar `paths:` glob", unified precisely so its two consumers cannot
// disagree. It shipped with semantics its code did not implement, and nothing
// caught it because nothing crashed: a glob with a `*` before a `**` matched
// nothing, in every repository, forever.
//
// So the package doc comment is treated here as the specification. Each
// capability it claims is enumerated and recorded covered, and every row names
// the mutation that was measured RED against it — the enumeration is the
// deliverable, and a covered row with no kill behind it is a claim, not reach.
//
//	#  capability (as the package claims it)                        status
//	-- ------------------------------------------------------------ --------
//	1  `**` matches zero or more path segments                       COVERED
//	   (TestMatch_DoublestarZeroSegments)                            (F1)
//	2  `*` matches within a single path segment and never crosses a  COVERED
//	   separator (TestMatch_StarDoesNotCrossSeparator)                (F2)
//	3  a pattern with no `**` delegates to filepath.Match            COVERED
//	   (TestMatch_NoDoublestarDelegates)                              (F3)
//	4  `*` and `**` COMPOSE — a wildcard before a `**` is honoured   COVERED
//	   (TestMatch_WildcardBeforeDoublestar) — the defect              (F4)
//	5  both consumers' real glob shapes resolve identically here     COVERED
//	   (TestMatch_ConsumerShapes; cross-consumer agreement itself is  (F4,
//	   asserted in cmd/gov's TestConsumersAgree, which can reach      F8, F9)
//	   both, and in pathsproposal's TestMatchesAny* on the other side)
//	6  LiteralPrefix is empty exactly when a glob is anchored        COVERED
//	   nowhere (TestLiteralPrefix) — pathsproposal's catch-all rule   (F5)
//
// Two more rows, added when the phase reopened. Neither was asked for by the
// requirement — they are semantics the segment-walk rewrite changed BEYOND the
// one behaviour it was asked to fix, found by a differential over 26,376
// synthetic pairs (80 where the old matcher matched and the new one does not,
// 332 the reverse). Both are defensible and both are standard globstar
// semantics; the real corpus is unaffected (822,624 (glob, path) pairs, zero
// disagreements, measured twice by different parties). They are enumerated
// here because an unasserted semantics change is one nothing will notice being
// reverted — which is the shape this very package shipped for two years:
//
//	7  only a segment that is EXACTLY `**` is multi-segment, so a    COVERED
//	   mid-segment `**` is NARROWED: `docs/a**` no longer matches     (F6)
//	   `docs/a/b/x.md` (TestMatch_MidSegmentDoublestar)
//	8  a trailing `/**` absorbs zero segments like every other `**`, COVERED
//	   so it now matches the bare parent path: `docs/**` matches      (F7)
//	   `docs` (TestMatch_TrailingDoublestarMatchesParent)
//
// The tables are written so that reverting the fix turns them red rather than
// leaving them green: TestMatch_RevertedFixTurnsRed carries the pre-fix
// algorithm verbatim and asserts it fails the cases in capability 4, and rows
// 7 and 8 carry the pre-fix verdict alongside the current one so the CHANGE is
// the assertion rather than today's behaviour.
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Every capability above was falsified by mutation before this suite was
// trusted: the behaviour was broken in globmatch.go (or in a consumer), the
// tests were run, and the named test went RED. A test that passes against both
// the working and the broken implementation is not reach. Each mutation was
// applied to a byte-for-byte backup-verified file and reverted from that
// backup, with the sha256 checked equal before and after every one
// (globmatch.go eefb937f…, verifydiff.go dad22ffd…, pathsproposal.go
// 7412660b…). Measured 2026-09-06, `go test -count=1 -run
// 'TestMatch|TestLiteralPrefix' ./internal/globmatch/` and, for F8/F9, the
// consumer packages named:
//
//	F1  the `**` branch starts its split loop at i=1, so `**`
//	    must absorb at least one segment — bok's diagnosis,
//	    made true                                                → RED
//	    TestMatch_DoublestarZeroSegments. Also RED: capability 4,
//	    5, 8 and TestMatch_RevertedFixTurnsRed — zero-segment
//	    `**` is load-bearing for most of the surface
//	F2  Match rewrites "/*/" to "/**/" before matching: the fix
//	    for capability 4 taken the WIDENING way, which is the
//	    regression the criteria name explicitly                  → RED
//	    TestMatch_StarDoesNotCrossSeparator. Also RED:
//	    TestMatch_NoDoublestarDelegates, because `internal/*/
//	    removal.go` then matches `internal/a/b/removal.go` while
//	    filepath.Match — which that test compares against
//	    directly — says false
//	F3  the no-`**` branch delegates filepath.Match(pattern,
//	    filepath.Base(path)) instead of the whole path: still a
//	    delegation, no longer the documented one                 → RED
//	    TestMatch_NoDoublestarDelegates ALONE. Nothing else moved
//	F4  matchSegments is bypassed and Match carries the pre-fix
//	    literal-prefix algorithm verbatim — the two-year defect,
//	    reinstated                                               → RED
//	    TestMatch_WildcardBeforeDoublestar,
//	    TestMatch_RevertedFixTurnsRed, TestMatch_ConsumerShapes
//	    (capability 5), and capabilities 1, 7, 8. The revert is
//	    visible from every direction, which is the point
//	F5  LiteralPrefix's metacharacter set drops `{`, so
//	    `{a,b}/x.md` reports a literal prefix and reads as
//	    anchored — pathsproposal's catch-all rule inverted       → RED
//	    TestLiteralPrefix ALONE. Nothing else moved
//	F6  a segment is treated as multi-segment when it CONTAINS
//	    `**` rather than when it IS `**` — capability 7 undone   → RED
//	    TestMatch_MidSegmentDoublestar ALONE. Nothing else moved
//	F7  the `**` branch returns false on an exhausted path, so a
//	    trailing `**` needs a segment — capability 8 undone      → RED
//	    TestMatch_TrailingDoublestarMatchesParent ALONE. Nothing
//	    else moved
//	F8  cmd/gov/verifydiff.go's vdGlobMatch stops delegating and
//	    carries the pre-fix algorithm again — the private copy
//	    whose deletion is what made the unification real         → RED
//	    cmd/gov TestVdGlobMatch and TestConsumersAgree
//	F9  internal/pathsproposal's matchesAny answers with
//	    strings.HasPrefix over LiteralPrefix: a second opinion
//	    about what a glob means, held by the approval side       → RED
//	    pathsproposal TestMatchesAnyRoutesThroughGlobmatch and
//	    TestMatchesAnyIsNotWidenedByTheFix, AND cmd/gov
//	    TestConsumersAgree — the cross-consumer assertion catches
//	    a defect planted on either side, which is what it is for
//
// F3, F5, F6 and F7 each killed exactly one test and left the rest green,
// which is the stronger evidence: the test is sensitive to its own capability
// and not merely to any damage. The full working tree was green immediately
// before and after the sweep.
package globmatch

import (
	"path/filepath"
	"strings"
	"testing"
)

// wildcardBeforeDoublestarCases are the three shapes from the bok-services
// field report, verbatim. Every one returned false before the fix.
var wildcardBeforeDoublestarCases = []struct {
	pattern string
	path    string
}{
	{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.sql"},
	{"internal/*/x/**/*.go", "internal/a/x/y.go"},
	{"tools/*/cmd/**/*.go", "tools/edikt/cmd/gov/compile.go"},
}

// TestMatch_WildcardBeforeDoublestar is the defect, as a test.
//
// globmatch.go:46 compared the pattern text before the first `**` against the
// path with strings.HasPrefix — a LITERAL comparison no wildcard can ever
// satisfy. Consequence: any glob with a `*` before a `**` matched nothing,
// ever, in any repository. The corpus edikt dogfoods on carries four path
// globs and none of this shape, which is why it survived; a real corpus hit it
// on first contact.
func TestMatch_WildcardBeforeDoublestar(t *testing.T) {
	for _, tc := range wildcardBeforeDoublestarCases {
		if !Match(tc.pattern, tc.path) {
			t.Errorf("Match(%q, %q) = false, want true — a wildcard before a `**` must still match", tc.pattern, tc.path)
		}
	}

	// The fix must not turn these into catch-alls: a wildcard segment still
	// has to match the segment it sits at.
	for _, tc := range []struct {
		pattern string
		path    string
	}{
		{"internal/*/migrations/**/*.sql", "internal/billing/schema/001_init.sql"},    // no `migrations` segment
		{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.go"}, // wrong extension
		{"internal/*/x/**/*.go", "other/a/x/y.go"},                                    // literal head differs
		{"tools/*/cmd/**/*.go", "tools/edikt/internal/gov/compile.go"},                // no `cmd` segment
	} {
		if Match(tc.pattern, tc.path) {
			t.Errorf("Match(%q, %q) = true, want false — the fix must not widen a wildcard into a catch-all", tc.pattern, tc.path)
		}
	}
}

// TestMatch_RevertedFixTurnsRed proves the table above discriminates. A test
// that would pass against the broken implementation asserts nothing about the
// fix; this one carries the pre-fix algorithm verbatim (the literal
// strings.HasPrefix split at the first `**`) and asserts it FAILS every case
// TestMatch_WildcardBeforeDoublestar requires — so a revert of globmatch.go
// cannot leave this suite green.
func TestMatch_RevertedFixTurnsRed(t *testing.T) {
	for _, tc := range wildcardBeforeDoublestarCases {
		if preFixMatch(tc.pattern, tc.path) {
			t.Errorf("preFixMatch(%q, %q) = true — this case no longer distinguishes the fixed matcher from the broken one, so the suite would stay green through a revert",
				tc.pattern, tc.path)
		}
		if !Match(tc.pattern, tc.path) {
			t.Errorf("Match(%q, %q) = false — the fixed matcher agrees with the reverted one; the defect is back", tc.pattern, tc.path)
		}
	}

	// And the shapes the old algorithm DID get right must still be right, so
	// this is a repair rather than a swap of one broken matcher for another.
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"docs/**/*.md", "docs/c.md", true},
		{"docs/**/*.md", "docs/a/b/c.md", true},
		{"commands/**/*.md", "commands/gov/compile.md", true},
		{"docs/**/*.md", "src/c.md", false},
	} {
		if got := preFixMatch(tc.pattern, tc.path); got != tc.want {
			t.Fatalf("preFixMatch(%q, %q) = %v, want %v — the recorded pre-fix behaviour is not what this test claims it was", tc.pattern, tc.path, got, tc.want)
		}
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v — a shape the broken matcher got right was traded away", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// preFixMatch is globmatch.Match as it stood before this phase, copied
// verbatim. It exists only so TestMatch_RevertedFixTurnsRed can assert the new
// table is sensitive to the defect. It is NOT a second matcher: nothing but
// that test may call it.
func preFixMatch(pattern, path string) bool {
	if pattern == "" {
		return false
	}
	if !strings.Contains(pattern, "**") {
		ok, _ := filepath.Match(pattern, path)
		return ok
	}

	idx := strings.Index(pattern, "**")
	prefix := pattern[:idx]
	rest := strings.TrimPrefix(pattern[idx+2:], "/")

	candidate := path
	if prefix != "" {
		if !strings.HasPrefix(candidate, prefix) {
			return false
		}
		candidate = candidate[len(prefix):]
	}

	if rest == "" {
		return true
	}

	for {
		if ok, _ := filepath.Match(rest, candidate); ok {
			return true
		}
		slash := strings.Index(candidate, "/")
		if slash < 0 {
			return false
		}
		candidate = candidate[slash+1:]
	}
}

// TestMatch_DoublestarZeroSegments pins the half of the semantics bok's
// diagnosis got wrong: `**` matches ZERO segments as well as many. Their
// report blamed "the matcher requires `**` to match at least one path
// segment"; `docs/**/*.md` matching `docs/c.md` falsifies that, and this fix
// must not make it true by accident.
func TestMatch_DoublestarZeroSegments(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"docs/**/*.md", "docs/c.md", true},                                                  // zero segments
		{"docs/**/*.md", "docs/a/c.md", true},                                                // one
		{"docs/**/*.md", "docs/a/b/c.md", true},                                              // many
		{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.sql", true}, // zero, after a wildcard
		{"internal/*/migrations/**/*.sql", "internal/billing/migrations/a/b/001_init.sql", true},
		{"**/*.go", "main.go", true},              // leading `**`, zero segments
		{"**/*.go", "a/b/main.go", true},          // leading `**`, many
		{"docs/**", "docs/a/b/c.md", true},        // trailing `**`
		{"docs/**/*.md", "docs/a/b/c.txt", false}, // suffix still enforced
		{"docs/**/*.md", "other/a/c.md", false},   // head still enforced
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// TestMatch_StarDoesNotCrossSeparator guards the regression the criteria name
// explicitly: making the wildcard-before-`**` case pass by loosening `*` into
// a cross-separator wildcard would be a widening, not a fix — every `paths:`
// scope in the corpus would silently grow.
func TestMatch_StarDoesNotCrossSeparator(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		{"internal/*/x/**/*.go", "internal/a/b/x/y.go", false},  // `*` must not eat `a/b`
		{"internal/*.go", "internal/a/b.go", false},             // one segment only
		{"internal/*/*.go", "internal/a/b/c.go", false},         // two segments only
		{"docs/*/c.md", "docs/a/b/c.md", false},                 // ditto, around a `**`-free pattern
		{"tools/*/cmd/**/*.go", "tools/a/b/cmd/main.go", false}, // `*` before `**` is still one segment
		{"internal/*/x/**/*.go", "internal/a/x/y.go", true},     // ... and one segment does match
		{"docs/**/*.md", "docs/a/b/c.md", true},                 // `**` is what crosses separators
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v — `*` must match within a single path segment", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// TestMatch_NoDoublestarDelegates pins that a pattern with no `**` is still
// handed to filepath.Match unchanged — asserted by comparing against
// filepath.Match directly rather than by restating what it does, so the two
// cannot drift.
func TestMatch_NoDoublestarDelegates(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
	}{
		{"internal/pathsproposal/pathsproposal.go", "internal/pathsproposal/pathsproposal.go"},
		{"internal/*/removal.go", "internal/pathsproposal/removal.go"},
		{"internal/*/removal.go", "internal/a/b/removal.go"},
		{"*.md", "README.md"},
		{"*.md", "docs/README.md"},
		{"docs/[ab].md", "docs/a.md"},
		{"docs/[ab].md", "docs/c.md"},
		{"docs/?.md", "docs/a.md"},
	} {
		want, err := filepath.Match(tc.pattern, tc.path)
		if err != nil {
			t.Fatalf("filepath.Match(%q, %q) errored: %v — bad fixture", tc.pattern, tc.path, err)
		}
		if got := Match(tc.pattern, tc.path); got != want {
			t.Errorf("Match(%q, %q) = %v, filepath.Match = %v — a `**`-free pattern must delegate", tc.pattern, tc.path, got, want)
		}
	}

	// A malformed pattern matches nothing rather than panicking or matching
	// everything, on both branches.
	if Match("docs/[a.md", "docs/a.md") {
		t.Error(`Match("docs/[a.md", ...) = true — a malformed pattern must match nothing`)
	}
	if Match("docs/[a/**/*.md", "docs/a/b.md") {
		t.Error(`Match("docs/[a/**/*.md", ...) = true — a malformed segment must match nothing on the ** branch`)
	}
	if Match("", "docs/a.md") {
		t.Error(`Match("", ...) = true — an empty pattern must match nothing`)
	}
}

// TestMatch_MidSegmentDoublestar pins the FIRST semantics change this rewrite
// made beyond the one it was asked for, so a revert of it is visible.
//
// The segment walk treats a segment as multi-segment only when it is EXACTLY
// `**`. The pre-fix algorithm split on the first `**` wherever it sat inside a
// segment, so `docs/a**` — a literal `a` glued to a `**` — was read as "the
// prefix `docs/a`, then anything" and matched `docs/a/b/x.md`. It no longer
// does: `a**` is one segment, matched by filepath.Match against one path
// segment, and `*` does not cross a separator.
//
// This is a NARROWING, and it is the standard globstar reading (a `**` is
// multi-segment only when it is a whole segment). It is asserted here rather
// than left to be discovered, because an unasserted semantics change is one
// nothing will notice being reverted — the exact shape this package shipped for
// two years. The old verdict is carried alongside each case so the change
// itself is the assertion, not merely today's behaviour.
func TestMatch_MidSegmentDoublestar(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool // current, segment-based semantics
		wasPre  bool // what the pre-fix algorithm returned
	}{
		// Narrowed: a `**` glued to other text is no longer multi-segment.
		{"docs/a**", "docs/a/b/x.md", false, true},
		{"docs/**x/*.md", "docs/a/b/x/c.md", false, true},

		// Unchanged: within one segment, `a**` is still `a` followed by
		// filepath.Match wildcards, so the single-segment reading still hits.
		{"docs/a**", "docs/ab.md", true, true},
		{"docs/a**", "docs/a.md", true, true},
		{"docs/**x/*.md", "docs/x/c.md", true, true},

		// And a segment that IS exactly `**` is still multi-segment, which is
		// what makes this a narrowing of one shape rather than of the feature.
		{"docs/**/*.md", "docs/a/b/c.md", true, true},
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v — only a segment that is exactly `**` is multi-segment",
				tc.pattern, tc.path, got, tc.want)
		}
		if got := preFixMatch(tc.pattern, tc.path); got != tc.wasPre {
			t.Fatalf("preFixMatch(%q, %q) = %v, want %v — the recorded pre-fix behaviour is not what this test claims it was",
				tc.pattern, tc.path, got, tc.wasPre)
		}
	}
}

// TestMatch_TrailingDoublestarMatchesParent pins the SECOND semantics change
// beyond the requirement: a trailing `/**` now matches the bare parent path.
//
// `**` absorbs ZERO or more segments everywhere else in the pattern, and the
// pre-fix algorithm honoured that everywhere except at the end, where its
// literal `strings.HasPrefix(path, "docs/")` could not be satisfied by `docs`
// itself. So `docs/**` matched every file under docs/ and not the directory
// entry. The segment walk has no such asymmetry.
//
// This is a WIDENING by exactly one path per pattern, and it is what `**`
// already meant in the package doc. The corpus shapes it touches are the
// delivery-map's directory scopes (`.claude/agents/**`, `bin/**`).
func TestMatch_TrailingDoublestarMatchesParent(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
		wasPre  bool
	}{
		// Widened: the bare parent now matches.
		{"docs/**", "docs", true, false},
		{".claude/agents/**", ".claude/agents", true, false},
		{"a/**/b/**", "a/b", true, false},

		// Unchanged: everything under the parent still matches.
		{"docs/**", "docs/a.md", true, true},
		{"docs/**", "docs/a/b.md", true, true},

		// Unchanged: the parent's NAME is still a whole segment, so a sibling
		// sharing its prefix is still out of scope. The widening is one path,
		// not a prefix match.
		{"docs/**", "docsx", false, false},
		{"docs/**", "other/a.md", false, false},

		// Unchanged: a `**` followed by more pattern still needs a segment to
		// feed it, so this widening does not leak into `docs/**/*.md`.
		{"docs/**/*.md", "docs", false, false},
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v — a trailing `**` absorbs zero segments too",
				tc.pattern, tc.path, got, tc.want)
		}
		if got := preFixMatch(tc.pattern, tc.path); got != tc.wasPre {
			t.Fatalf("preFixMatch(%q, %q) = %v, want %v — the recorded pre-fix behaviour is not what this test claims it was",
				tc.pattern, tc.path, got, tc.wasPre)
		}
	}
}

// TestMatch_ConsumerShapes runs the real glob shapes each consumer feeds this
// package, so a change here is measured against what actually calls it rather
// than against invented patterns.
//
// The delivery-map globs are taken from this repo's compiled
// .claude/rules/governance/delivery-map.yaml (hookmatch's consumer); the
// proposal globs are the shapes pathsproposal validates at approval time and
// cmd/gov's verify-diff resolves at diff time.
func TestMatch_ConsumerShapes(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		path    string
		want    bool
	}{
		// hookmatch — delivery-map globs, verbatim shapes from the corpus.
		{".claude/rules/governance/**/*.md", ".claude/rules/governance/compile.md", true},
		{".claude/rules/governance/**/*", ".claude/rules/governance/delivery-map.yaml", true},
		{"commands/**/*.md", "commands/gov/compile.md", true},
		{"commands/**/*.md", "commands/config.md", true},
		{"bin/**/*", "bin/edikt", true},
		{".edikt/state/plan-eval/**/*.json", ".edikt/state/plan-eval/a/b.json", true},
		{".claude/agents/**", ".claude/agents/sidecar-extractor.md", true},
		{"commands/**/*.md", "docs/guides/compile.md", false},
		{"tools/edikt/**", "tools/edikt-other/main.go", false},

		// pathsproposal / verify-diff — proposed and approved scope globs,
		// including the shape that matched nothing before this phase.
		{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.sql", true},
		{"tools/edikt/internal/**/*.go", "tools/edikt/internal/globmatch/globmatch.go", true},
		{"tools/*/cmd/**/*.go", "tools/edikt/cmd/gov/verifydiff.go", true},
		{"internal/rag/**/*.go", "internal/rag/chunk.go", true},
		{"internal/rag/**/*.go", "internal/unrelated/whatever.go", false},
	} {
		if got := Match(tc.pattern, tc.path); got != tc.want {
			t.Errorf("Match(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// TestLiteralPrefix pins the anchor test pathsproposal's catch-all rule reads:
// a glob whose literal prefix is empty is anchored nowhere and can match the
// whole repository.
func TestLiteralPrefix(t *testing.T) {
	for _, tc := range []struct {
		pattern string
		want    string
	}{
		{"internal/*/migrations/**/*.sql", "internal/"},
		{"docs/**/*.md", "docs/"},
		{"tools/edikt/internal/globmatch/globmatch.go", "tools/edikt/internal/globmatch/globmatch.go"},
		{"**/*.go", ""},
		{"*.md", ""},
		{"*/**/*.sh", ""},
		{"[ab]/x.md", ""},
		{"{a,b}/x.md", ""},
		{"docs/?.md", "docs/"},
	} {
		if got := LiteralPrefix(tc.pattern); got != tc.want {
			t.Errorf("LiteralPrefix(%q) = %q, want %q", tc.pattern, got, tc.want)
		}
	}
}
