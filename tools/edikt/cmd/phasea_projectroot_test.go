package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// D40 — `phasea plan` resolves the PROJECT root (the directory holding
// .edikt/config.yaml, walking up from cwd), never the state root. Before
// the fix cmd/phasea.go passed resolveEdiktRoot() — EDIKT_ROOT > EDIKT_HOME
// > ~/.edikt — as ProjectRoot, so a consumer project with the extractor
// pinned at .claude/agents/ was refused with "reading ~/.edikt/.claude/
// agents/sidecar-extractor.md: no such file" (bok, 2026-09-02).
func TestPhaseAPlan_ProjectRootIsTheProjectNotTheStateRoot(t *testing.T) {
	bin := buildBinary(t)
	project := t.TempDir()
	for _, d := range []string{".edikt", ".claude/agents", "docs/architecture/decisions", "sub/dir"} {
		if err := os.MkdirAll(filepath.Join(project, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(project, ".edikt", "config.yaml"), []byte("version: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	agent := "---\nname: sidecar-extractor\nmodel: claude-opus-5\nprompt_version: 4\n---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(project, ".claude", "agents", "sidecar-extractor.md"), []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}

	// One accepted ADR with no sidecar, so the plan has an item to carry.
	adr := "---\nstatus: accepted\n---\n# ADR-001: fixture\n\n**Status:** Accepted\n\nThe system MUST do the thing.\n"
	if err := os.WriteFile(filepath.Join(project, "docs", "architecture", "decisions", "ADR-001-fixture.md"), []byte(adr), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(bin, "phasea", "plan")
	cmd.Dir = filepath.Join(project, "sub", "dir")
	cmd.Env = append(os.Environ(),
		"EDIKT_ROOT=", "EDIKT_HOME="+t.TempDir(), "CLAUDE_HOME="+t.TempDir(), "HOME="+t.TempDir())
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	combined := stdout.String() + stderr.String()
	if strings.Contains(combined, "prompt_version resolved to UNKNOWN") {
		t.Fatalf("plan did not read the project's own extractor pin:\n%s", combined)
	}
	if err != nil {
		t.Fatalf("phasea plan from a subdirectory of a pinned project failed: %v\n%s", err, combined)
	}
	if _, statErr := os.Stat(filepath.Join(project, ".edikt", "state", "phasea")); statErr != nil {
		t.Fatalf("plan state must land under the project, not the state root: %v", statErr)
	}
}

// A cwd with no .edikt/config.yaml above it is a usage error, not a plan
// against the state root.
func TestPhaseAPlan_NoProjectAboveCwdRefuses(t *testing.T) {
	bin := buildBinary(t)
	cmd := exec.Command(bin, "phasea", "plan")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "EDIKT_ROOT=", "EDIKT_HOME="+t.TempDir(), "HOME="+t.TempDir())
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("expected a refusal outside any project, got success:\n%s", out)
	}
	// A refusal is exit 4 in the documented table (commands/gov/compile.md);
	// 2 is "plan rejected" on verify and must not be reused here.
	if ee, ok := err.(*exec.ExitError); !ok || ee.ExitCode() != 4 {
		t.Fatalf("expected exit 4 (refusal), got %v:\n%s", err, out)
	}
	if !strings.Contains(string(out), ".edikt/config.yaml") {
		t.Fatalf("refusal must name what was looked for:\n%s", out)
	}
}
