package phaseb

import (
	"strings"
	"testing"
)

// Reported from the field 2026-09-18. A consumer project carried one orphaned
// skill package. `gov compile --check` named it correctly and then printed
// "Run `edikt gov compile` to re-render." Compile re-renders; it does not
// remove. Running it twice left the surface stale both times, so a user
// following the advice loops forever on a check that was right all along.
//
// The diagnosis was never the problem. These pin the remedy to the kind.

func TestRemedyFor_OrphanIsNotResolvedByRecompiling(t *testing.T) {
	out := strings.Join(RemedyFor([]FreshnessDrift{
		{Path: ".claude/skills/edikt-governance-authoring/SKILL.md", Kind: "unexpected"},
	}), "\n")

	if !strings.Contains(out, "will NOT clear") {
		t.Errorf("an orphan-only drift must say compile does not clear it.\ngot:\n%s", out)
	}
	if !strings.Contains(out, "rm -rf .claude/skills/edikt-governance-authoring/SKILL.md") {
		t.Errorf("the remedy must name the path to remove, not describe removal in the abstract.\ngot:\n%s", out)
	}
	if strings.Contains(out, "Run `edikt gov compile` to re-render the missing") {
		t.Errorf("an orphan-only drift must NOT offer the re-render remedy — it is the advice that loops.\ngot:\n%s", out)
	}
}

func TestRemedyFor_MissingAndContentStillSayRecompile(t *testing.T) {
	for _, kind := range []string{"missing", "content"} {
		out := strings.Join(RemedyFor([]FreshnessDrift{{Path: "a/b.md", Kind: kind}}), "\n")
		if !strings.Contains(out, "Run `edikt gov compile`") {
			t.Errorf("kind %q IS resolved by recompiling and must still say so.\ngot:\n%s", kind, out)
		}
		if strings.Contains(out, "rm -rf") {
			t.Errorf("kind %q must not be offered a deletion remedy.\ngot:\n%s", kind, out)
		}
	}
}

func TestRemedyFor_MixedSetGivesBothAndKeepsThemApart(t *testing.T) {
	out := strings.Join(RemedyFor([]FreshnessDrift{
		{Path: "a/gone.md", Kind: "missing"},
		{Path: "b/orphan.md", Kind: "unexpected"},
	}), "\n")
	if !strings.Contains(out, "Run `edikt gov compile`") {
		t.Error("the recompilable half must still be offered its remedy")
	}
	if !strings.Contains(out, "rm -rf b/orphan.md") {
		t.Error("the orphan half must still name its path")
	}
	// The orphan must not be swept into the recompile advice; that is the bug.
	if strings.Contains(out, "rm -rf a/gone.md") {
		t.Error("a MISSING surface must never be offered for deletion — recompiling creates it")
	}
}

func TestRemedyFor_WarnsBeforeDeleting(t *testing.T) {
	out := strings.Join(RemedyFor([]FreshnessDrift{{Path: "x.md", Kind: "unexpected"}}), "\n")
	if !strings.Contains(out, "symptom") {
		t.Error("deleting an orphan whose topic is still claimed hides the cause; the remedy must say so")
	}
}
