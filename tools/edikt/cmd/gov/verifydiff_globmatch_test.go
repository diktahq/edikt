package gov

// The glob a human approves and the glob that scopes a diff must mean the same
// thing. internal/globmatch exists to make that structural — "the ONE place
// edikt decides whether a repo-relative path matches a sidecar `paths:` glob",
// unified so its consumers cannot disagree — but this file kept its own copy
// of the algorithm after that package landed, so the unification was nominal.
// Both copies carried the same defect (a literal strings.HasPrefix on the
// pattern text before the first `**`), which is why nothing surfaced: they
// agreed on being wrong.
//
// The tests here assert the two properties separately:
//
//	TestVdGlobMatch      — the diff-time resolver routes through globmatch and
//	                       therefore honours a wildcard before a `**`
//	TestConsumersAgree   — approval time (internal/pathsproposal) and diff time
//	                       (vdSidecarInScope) return the SAME verdict for the
//	                       same (pattern, path) pair
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Both tests were falsified by mutation before they were trusted, on
// 2026-09-06. Each mutation was applied to a file verified byte-identical to a
// backup taken first and reverted from that backup, sha256 checked equal
// before and after. This is F8 and F9 of the record in
// internal/globmatch/globmatch_test.go, recorded here too because a
// cross-consumer assertion is only worth its name if a defect planted on
// EITHER side turns it red:
//
//	F8  vdGlobMatch (verifydiff.go) stops delegating to globmatch
//	    and carries the pre-fix literal-prefix algorithm again —
//	    the private copy this file's deletion removed, which is
//	    what made globmatch's "ONE place" claim true             → RED
//	    TestVdGlobMatch and TestConsumersAgree
//	F9  the defect is planted on the OTHER side instead:
//	    pathsproposal's matchesAny answers with strings.HasPrefix
//	    over globmatch.LiteralPrefix                             → RED
//	    TestConsumersAgree (and both pathsproposal tests)
//
// So the agreement assertion is sensitive in both directions, not only to the
// consumer whose file it lives beside.

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/globmatch"
	"github.com/diktahq/edikt/tools/edikt/internal/pathsproposal"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// consumerCases are the (pattern, path) pairs both consumers are held to.
// The first three are the shapes that matched nothing before this phase in
// EVERY repository — a glob with a `*` before a `**`.
var consumerCases = []struct {
	pattern string
	path    string
	want    bool
}{
	{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.sql", true},
	{"internal/*/x/**/*.go", "internal/a/x/y.go", true},
	{"tools/*/cmd/**/*.go", "tools/edikt/cmd/gov/compile.go", true},

	{"docs/**/*.md", "docs/c.md", true},
	{"docs/**/*.md", "docs/a/b/c.md", true},
	{"internal/rag/**/*.go", "internal/rag/chunk.go", true},
	{"tools/edikt/internal/**/*.go", "tools/edikt/internal/globmatch/globmatch.go", true},

	{"internal/*/migrations/**/*.sql", "internal/billing/migrations/001_init.go", false},
	{"internal/*/x/**/*.go", "internal/a/b/x/y.go", false}, // `*` must not cross a separator
	{"internal/rag/**/*.go", "internal/unrelated/whatever.go", false},
	{"docs/**/*.md", "src/c.md", false},
}

// TestVdGlobMatch pins that verify-diff's scope resolver is globmatch and not
// a private copy of it: every case resolves identically to globmatch.Match,
// including the wildcard-before-`**` shapes the private copy got wrong.
func TestVdGlobMatch(t *testing.T) {
	for _, tc := range consumerCases {
		got := vdGlobMatch(tc.pattern, tc.path)
		if got != tc.want {
			t.Errorf("vdGlobMatch(%q, %q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
		if want := globmatch.Match(tc.pattern, tc.path); got != want {
			t.Errorf("vdGlobMatch(%q, %q) = %v but globmatch.Match = %v — the diff-time resolver has drifted from the unified matcher",
				tc.pattern, tc.path, got, want)
		}
	}

	// And through the resolver the command actually calls, so the delegation
	// is asserted at the call site rather than only at the helper.
	for _, tc := range consumerCases {
		sc := &sidecar.Sidecar{Paths: []string{tc.pattern}}
		if got := vdSidecarInScope(sc, []string{tc.path}); got != tc.want {
			t.Errorf("vdSidecarInScope(paths=%q, changed=%q) = %v, want %v", tc.pattern, tc.path, got, tc.want)
		}
	}
}

// TestConsumersAgree asserts the disagreement globmatch was created to prevent,
// rather than assuming it away: for each pair, the APPROVAL-time validator
// (internal/pathsproposal — does this proposed glob match a real file?) and the
// DIFF-time scope resolver (vdSidecarInScope) must return the same verdict.
//
// A disagreement here is the failure the package doc names: "a human approves a
// glob that matched a file during the ceremony and silently scopes nothing at
// diff time — an approval that grants no coverage while reading as coverage."
func TestConsumersAgree(t *testing.T) {
	for _, tc := range consumerCases {
		// Approval time. The single-file "tree" is the path under test, so a
		// "no-match" finding is exactly the matcher's verdict — and Validate
		// refuses outright on an empty file set (INV-013), so a vacuous run
		// cannot masquerade as agreement.
		res, err := pathsproposal.Validate(
			[]pathsproposal.Proposal{{
				Glob:     tc.pattern,
				Evidence: "cross-consumer agreement fixture: " + tc.pattern,
			}},
			[]string{tc.path},
		)
		if err != nil {
			t.Fatalf("pathsproposal.Validate(%q, [%q]) errored: %v — UNMEASURED, not agreement", tc.pattern, tc.path, err)
		}
		if res.Checked != 1 || res.Files != 1 {
			t.Fatalf("pathsproposal.Validate reported Checked=%d Files=%d, want 1/1 — the run did not observe what this test claims",
				res.Checked, res.Files)
		}
		approvalMatched := true
		for _, f := range res.Findings {
			if f.Rule == "no-match" {
				approvalMatched = false
			} else {
				t.Fatalf("unexpected %q finding for %q: %s — this fixture is meant to isolate the match rule", f.Rule, tc.pattern, f.Reason)
			}
		}

		// Diff time.
		diffMatched := vdSidecarInScope(&sidecar.Sidecar{Paths: []string{tc.pattern}}, []string{tc.path})

		if approvalMatched != diffMatched {
			t.Errorf("consumers disagree on (%q, %q): approval-time=%v diff-time=%v — an approval that reads as coverage and grants none",
				tc.pattern, tc.path, approvalMatched, diffMatched)
		}
		if approvalMatched != tc.want {
			t.Errorf("both consumers agree on (%q, %q) = %v, but the correct answer is %v — agreeing on the wrong verdict is not agreement",
				tc.pattern, tc.path, approvalMatched, tc.want)
		}
	}
}
