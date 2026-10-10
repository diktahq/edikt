package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The compiled skill is the one surface nobody can hand-edit into agreement,
// so it reads the contract from the guideline's amendment at render time.
func TestRenderSkill_ContractBlockFromAmendmentHeading(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "guidelines")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	block := contractBlockStart + " -->\n**The subject axis, before the questions.** A rule binds two things.\n\n### Q1 — a property\n" + contractBlockEnd
	body := "# GL-001\n\n## Rules\n\n- something\n\n## Amendment — 2026-09-03\n\n" + block + "\n"
	if err := os.WriteFile(filepath.Join(dir, "GL-001-capture-gates.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	got := ContractBlockFor(root, "governance-authoring")
	if got != block {
		t.Fatalf("the block must be carried VERBATIM.\n got: %q\nwant: %q", got, block)
	}

	// It renders into the skill, whole.
	out := RenderSkill(SkillView{
		Topic:         "governance-authoring",
		Description:   "authoring",
		ContractBlock: got,
	})
	if !strings.Contains(out, "### Q1 — a property") {
		t.Fatalf("the rendered skill does not carry the block:\n%s", out)
	}
	if strings.Count(out, contractBlockStart) != 1 {
		t.Errorf("the block must appear exactly once in the rendered skill")
	}
}

// Every other topic legitimately has none, and a missing source is not a
// finding here — the five-surface gate is what catches a block that failed
// to render.
func TestRenderSkill_ContractBlockOnlyForItsOwnTopic(t *testing.T) {
	root := t.TempDir()
	if got := ContractBlockFor(root, "hooks"); got != "" {
		t.Fatalf("only the authoring topic carries the contract, got %q", got)
	}
	if got := ContractBlockFor(root, "governance-authoring"); got != "" {
		t.Fatalf("no guideline on disk means no block, got %q", got)
	}
}
