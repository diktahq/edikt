package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTopicScopeCheck_noDir(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer
	warn, ran := runTopicScopeCheck(root, &out)
	if ran {
		t.Fatalf("expected ran=false when governance dir doesn't exist, got ran=true warn=%d\n%s", warn, out.String())
	}
}

func TestTopicScopeCheck_clean(t *testing.T) {
	root := t.TempDir()
	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(govDir, "auth.md"), "<!-- scope: 1 glob(s) from all 2 source(s) -->\n# Auth\n")
	skillDir := filepath.Join(root, ".claude", "skills", "edikt-auth")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: edikt-auth\n---\n")

	var out bytes.Buffer
	warn, ran := runTopicScopeCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 0 {
		t.Fatalf("expected 0 warnings for a scoped, reachable topic, got %d\n%s", warn, out.String())
	}
}

func TestTopicScopeCheck_shadowAmbientCore(t *testing.T) {
	root := t.TempDir()
	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(govDir, "backend.md"),
		"<!-- scope: ** — UNSCOPED because 1 of 2 source(s) declare no paths: globs (INV-0008) -->\n# Backend\n")

	var out bytes.Buffer
	warn, ran := runTopicScopeCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 1 {
		t.Fatalf("expected 1 warning, got %d\n%s", warn, out.String())
	}
	s := out.String()
	if !strings.Contains(s, "backend") || !strings.Contains(s, "INV-0008") {
		t.Fatalf("must name the unscoped topic and its culprit source:\n%s", s)
	}
}

func TestTopicScopeCheck_unreachableTopic(t *testing.T) {
	root := t.TempDir()
	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// security has a skill package but no topic file — retired to tier 3.
	skillDir := filepath.Join(root, ".claude", "skills", "edikt-security")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(skillDir, "SKILL.md"), "---\nname: edikt-security\n---\n")

	var out bytes.Buffer
	warn, ran := runTopicScopeCheck(root, &out)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warn != 1 {
		t.Fatalf("expected 1 warning, got %d\n%s", warn, out.String())
	}
	if !strings.Contains(out.String(), "security") {
		t.Fatalf("must name the unreachable topic:\n%s", out.String())
	}
}
