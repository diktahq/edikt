package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// readTopicFile returns the compiled topic file body for the given topic.
func readTopicFile(t *testing.T, root, topic string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", topic+".md"))
	if err != nil {
		t.Fatalf("topic %s not written: %v", topic, err)
	}
	return string(b)
}

// TestSuppressedDirectives_AreSubtracted pins the three-list formula in the
// live merge path.
//
//	effective = (directives - suppressed_directives) ∪ manual_directives
//
// internal/compile implements it (EffectiveRules), but that is the legacy
// --legacy-only path. internal/phaseb — the path every compile actually
// takes since the sidecar migration — appended Sidecar.Directives wholesale
// and never read SuppressedDirectives at all.
//
// The field exists on the struct, the schema declares it, and the struct
// comment said it "are subtracted from Directives at gov:compile time".
// None of that was true of the live path, so an author who populated it got
// silent non-suppression while the documentation told them it worked. No
// sidecar in the corpus carries a non-empty list, which is the only reason
// nothing has been mis-compiled yet.
func TestSuppressedDirectives_AreSubtracted(t *testing.T) {
	root := t.TempDir()
	pair := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{
		{
			Text:          "Kept rule MUST hold. (ref: ADR-001)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "a"},
		},
		{
			Text:          "Suppressed rule MUST hold. (ref: ADR-001)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "b"},
		},
	})
	pair.Sidecar.SuppressedDirectives = []string{"Suppressed rule MUST hold. (ref: ADR-001)"}

	if _, err := Merge(root, []sidecar.Pair{pair},
		Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// The subtraction this test pins happens upstream of BOTH surfaces
	// (groupByTopic and BuildDeliveryMap read the same post-subtraction
	// set). Under ADR-079 an ADR directive is advisory, so the surviving text
	// lands in the topic file rather than the index — and the suppressed text
	// is asserted absent from BOTH, because "not in the surface I happened to
	// read" is not the property. A regression that leaked it onto the other
	// surface would otherwise pass.
	topic := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "arch.md"))
	assertNoDirectiveIndex(t, root)
	if !strings.Contains(topic, "Kept rule MUST hold.") {
		t.Errorf("unsuppressed directive missing from compiled output:\n%s", topic)
	}
	for name, body := range map[string]string{"topic file": topic} {
		if strings.Contains(body, "Suppressed rule MUST hold.") {
			t.Errorf("suppressed directive was compiled anyway, into the %s:\n%s", name, body)
		}
	}
}

// TestSuppressedDirectives_ManualWinsOverSuppression pins the formula's
// ordering, which is not incidental: the union with manual_directives
// happens AFTER the subtraction, so an author who suppresses an extracted
// directive and re-states it as a manual one gets their wording, not
// silence. Subtracting from the union instead would delete both.
func TestSuppressedDirectives_ManualWinsOverSuppression(t *testing.T) {
	root := t.TempDir()
	const text = "Contested rule MUST hold. (ref: ADR-002)"
	pair := mkPair(t, root, "ADR-002-test", "arch", []sidecar.Directive{{
		Text:          text,
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "a"},
	}})
	pair.Sidecar.SuppressedDirectives = []string{text}
	pair.Sidecar.ManualDirectives = []string{text}

	if _, err := Merge(root, []sidecar.Pair{pair},
		Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	if body := readTopicFile(t, root, "arch"); !strings.Contains(body, "Contested rule MUST hold.") {
		t.Errorf("a manual directive re-stating a suppressed one was dropped:\n%s", body)
	}
}

// TestSuppressedDirectives_EmptyIsNoOp is the control, and it is the case
// the whole corpus is in today: every sidecar carrying the field has it
// empty. If this fix changed their output at all, it would be changing
// compiled governance for no stated reason.
func TestSuppressedDirectives_EmptyIsNoOp(t *testing.T) {
	build := func(suppressed []string) string {
		root := t.TempDir()
		pair := mkPair(t, root, "ADR-003-test", "arch", []sidecar.Directive{{
			Text:          "Only rule MUST hold. (ref: ADR-003)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "a"},
		}})
		pair.Sidecar.SuppressedDirectives = suppressed
		if _, err := Merge(root, []sidecar.Pair{pair},
			Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
			t.Fatalf("merge: %v", err)
		}
		// ADR-079: an ADR directive is advisory, so its text renders into the
		// topic file. The topic file is the whole compiled directive output since
		// ADR-083; the index's absence is asserted alongside.
		assertNoDirectiveIndex(t, root)
		return readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "arch.md"))
	}

	if nilCase, emptyCase := build(nil), build([]string{}); nilCase != emptyCase {
		t.Error("nil and empty suppressed_directives produced different output")
	}
	if !strings.Contains(build(nil), "Only rule MUST hold.") {
		t.Error("an empty suppression list dropped a directive")
	}
}

// TestSuppressedDirectives_AppliedToInvariantsToo covers the second
// collection site. Invariant directives bypass groupByTopic entirely and go
// straight to governance.md's non-negotiable-constraints block, so honouring
// suppression only in the topic path would leave the field working for ADRs
// and silently not for invariants — a subtler shape of the same bug.
func TestSuppressedDirectives_AppliedToInvariantsToo(t *testing.T) {
	root := t.TempDir()
	pair := mkPair(t, root, "INV-001-test", "arch", []sidecar.Directive{
		{
			Text:          "Kept invariant MUST hold. (ref: INV-001)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "a"},
		},
		{
			Text:          "Suppressed invariant MUST hold. (ref: INV-001)",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "b"},
		},
	})
	// PATHLESS on purpose: this test's subject is the AMBIENT-CORE path, and
	// only a pathless invariant routes there. mkPair declares globs by
	// default (so ordinary topic fixtures stay at tier 2), which would make
	// this a SCOPED invariant — rendered into its topic file instead, where
	// the assertion below would fail for a reason unrelated to suppression.
	pair.Sidecar.Paths = nil
	pair.Sidecar.SuppressedDirectives = []string{"Suppressed invariant MUST hold. (ref: INV-001)"}

	if _, err := Merge(root, []sidecar.Pair{pair},
		Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	idx, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("governance.md not written: %v", err)
	}
	body := string(idx)
	if !strings.Contains(body, "Kept invariant MUST hold.") {
		t.Errorf("unsuppressed invariant missing from governance.md:\n%s", body)
	}
	if strings.Contains(body, "Suppressed invariant MUST hold.") {
		t.Errorf("suppressed invariant was compiled anyway:\n%s", body)
	}
}

// TestSuppressedDirectives_DoNotTouchProhibitions guards the boundary. The
// three-list formula names `directives` and nothing else; prohibitions are
// a separate top-level array introduced later (ADR-032). Extending
// suppression to them would be new semantics invented here rather than the
// documented contract being honoured.
func TestSuppressedDirectives_DoNotTouchProhibitions(t *testing.T) {
	root := t.TempDir()
	const text = "MUST NOT do the forbidden thing. (ref: ADR-004)"
	pair := mkPair(t, root, "ADR-004-test", "arch", nil)
	pair.Sidecar.Prohibitions = []sidecar.Prohibition{{
		Text:          text,
		SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "a"},
	}}
	pair.Sidecar.SuppressedDirectives = []string{text}

	if _, err := Merge(root, []sidecar.Pair{pair},
		Options{CompiledAt: "2026-05-22T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatalf("merge: %v", err)
	}

	// Since SR-003 an ADR prohibition grades ADVISORY, so it renders into the
	// topic file rather than the index. The subject is unchanged — suppression
	// must not reach prohibitions, which the ADR-008 formula does not cover —
	// only the surface it survives on moved. Both are checked so the assertion
	// cannot pass merely because the entry went somewhere else.
	topic := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "arch.md"))
	if !strings.Contains(topic, "MUST NOT do the forbidden thing.") {
		t.Errorf("suppression reached prohibitions, which the formula does not cover:\n%s", topic)
	}
	assertNoDirectiveIndex(t, root)
}
