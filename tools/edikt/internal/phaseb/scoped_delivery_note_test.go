package phaseb

// ADR-066 routes a path-scoped directive to directive-index.yaml and out of
// the topic file's body. Three surfaces then describe a state they were
// written before: the topic file renders an empty Directives region, the skill
// stub promises that file "carries every compiled directive", and the stub's
// `sources:` header lists artifacts none of whose text is in the file it
// points at.
//
// All three were green under every existing gate, because every existing gate
// asks whether a body reached exactly one surface — and it did. None of them
// asks whether the surfaces DESCRIBE where it went. These tests ask that.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// mkScopedPair builds a sidecar that DECLARES paths — the case ADR-066 routes
// to the index — with one directive and one prohibition.
func mkScopedPair(t *testing.T, projectRoot, basename, topic string) sidecar.Pair {
	t.Helper()
	p := mkPair(t, projectRoot, basename, topic, []sidecar.Directive{
		{
			Text:          "Scoped directive body. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
		},
	})
	p.Sidecar.Prohibitions = []sidecar.Prohibition{
		{
			Text:          "MUST NOT do the scoped thing. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "y"},
		},
	}
	return p
}

func TestMerge_FullyScopedTopicExplainsItsEmptyDirectivesRegion(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{mkScopedPair(t, root, "ADR-001-test", "architecture")}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))

	// PIN THE SUBJECT FIRST. If the region were NOT empty the note would be
	// correctly absent, and this test would pass while measuring nothing —
	// the empty-result class INV-013 forbids, reached through the test rather
	// than through the renderer.
	if !strings.Contains(body, "[edikt:directives:start]: #\n[edikt:directives:end]: #") {
		t.Fatalf("fixture no longer produces an empty Directives region; this test asserts nothing:\n%s", body)
	}

	// Matched against a whitespace-flattened copy. The note is hard-wrapped
	// with a "> " continuation prefix, so a phrase can legitimately straddle
	// two lines; asserting on raw bytes would make this test fail whenever the
	// counts change width, which is a property of the fixture, not a defect.
	flat := flattenQuote(body)
	for _, want := range []string{
		"empty by design",
		"All 1 directive and 1 prohibition",
		"directive-index.yaml",
	} {
		if !strings.Contains(flat, want) {
			t.Errorf("topic file does not explain its empty region: missing %q\n%s", want, body)
		}
	}

	// The note MUST NOT cite an edikt-internal ADR. This text renders into a
	// user's own governance files, where "ADR-066" names a decision in edikt's
	// corpus that they cannot read and whose number, in their repo, belongs to
	// something else entirely. test/check-no-internal-refs.sh guards the
	// shipped strings; this guards the rendered bytes, which is the surface
	// the user actually reads. The first draft of this note cited it and the
	// ratchet caught it.
	//
	// The fixture's own `<!-- sources: ADR-001 -->` line is a REAL reference to
	// the user's artifact, so the check is scoped to the note itself.
	noteStart := strings.Index(body, "> **No unscoped directives")
	noteEnd := strings.Index(body, "[edikt:directives:start]: #")
	note := body[noteStart:noteEnd]
	if leaked := regexp.MustCompile(`\b(ADR|INV|SPEC|PRD)-\d{3}\b`).FindString(note); leaked != "" {
		t.Errorf("delivery note cites an internal artifact ID %q; it ships into user projects:\n%s", leaked, note)
	}

	// The note must stay OUT of the managed region: inside, it would be prose
	// the DirectivesSHA covers, and a generated explanation would be
	// indistinguishable from a hand-edit of a managed region.
	start := strings.Index(body, "[edikt:directives:start]: #")
	if note := strings.Index(body, "No unscoped directives"); note < 0 || note > start {
		t.Errorf("delivery note is missing or rendered inside the managed region (note at %d, region opens at %d)", note, start)
	}
}

// flattenQuote collapses a blockquote's hard wrapping into one line so a
// phrase can be asserted without depending on where it happened to wrap.
func flattenQuote(s string) string {
	return strings.Join(strings.Fields(strings.ReplaceAll(s, "\n> ", " ")), " ")
}

func TestRenderSkill_StubDoesNotClaimAnEmptyFileCarriesEverything(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{mkScopedPair(t, root, "ADR-001-test", "architecture")}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "skills", "edikt-architecture", "SKILL.md"))

	// Symptom 1: the overclaim itself.
	if strings.Contains(body, "It carries every compiled directive") {
		t.Errorf("stub still claims an empty topic file carries every directive:\n%s", body)
	}
	// Symptom 2: `sources:` implying co-location. The header must name the
	// routed artifacts as routed.
	if !strings.Contains(body, "delivered via") || !strings.Contains(body, "ADR-001") {
		t.Errorf("stub header does not disclose which sources are delivered via the index:\n%s", body)
	}
	// The stub still has to do its actual job — point at the rules file.
	if !strings.Contains(body, ".claude/rules/governance/architecture.md") {
		t.Errorf("stub no longer points at the rules file:\n%s", body)
	}
	if !strings.Contains(body, "directive-index.yaml") {
		t.Errorf("stub does not name where the directives are actually delivered from:\n%s", body)
	}
}

func TestMerge_UnroutedTopicGetsNoDeliveryNote(t *testing.T) {
	// The complement, and the reason the note is conditional: a topic whose
	// directives DO render in its own file must be byte-identical to before.
	// A note that appears unconditionally is not an honest note, it is
	// boilerplate — and boilerplate on a file that already lists its rules
	// says something false in the other direction.
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "architecture", nil),
	}
	pairs[0].Sidecar.ManualDirectives = []string{"A hand-authored rule."}

	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}

	body := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if !strings.Contains(body, "A hand-authored rule.") {
		t.Fatalf("fixture did not render its manual directive; test asserts nothing:\n%s", body)
	}
	if strings.Contains(body, "empty by design") {
		t.Errorf("delivery note rendered on a topic whose Directives region is not empty:\n%s", body)
	}
}

func TestDeliveryNoteCountsMatchTheIndexItDescribes(t *testing.T) {
	// The note states a number. If that number were computed independently of
	// BuildDirectiveIndex, the two would drift and the note would confidently
	// describe an index that holds something else — so assert the identity
	// rather than trusting the shared helper stays shared.
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkScopedPair(t, root, "ADR-001-test", "architecture"),
		mkScopedPair(t, root, "ADR-002-test", "architecture"),
	}

	groups := groupByTopic(pairs)
	g := groups["architecture"]
	claimed := g.IndexedDirectives + g.IndexedProhibitions

	// Count DISTINCT entries in the index: BuildDirectiveIndex fans each
	// sidecar's entries out across every glob it declares, so a per-glob sum
	// would double-count a two-glob artifact.
	seen := map[string]struct{}{}
	for _, entries := range BuildDirectiveIndex(pairs) {
		for _, e := range entries {
			seen[e.ID] = struct{}{}
		}
	}
	if claimed == 0 {
		t.Fatal("fixture routed nothing to the index; test asserts nothing")
	}
	if claimed != len(seen) {
		t.Errorf("note would claim %d entries, index holds %d", claimed, len(seen))
	}
}

func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}
