package cmd

import (
	"path/filepath"
	"testing"
)

// INV-007: every case pins all three inputs (CLAUDE_HOME, CLAUDE_CONFIG_DIR,
// HOME) via t.Setenv so no host state leaks into the resolution under test.
// t.Setenv to "" is "unset" for the resolver, which treats empty as absent.

func TestResolveClaudeRoot_ClaudeHomeWinsOverConfigDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_HOME", "/profiles/claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/claude-config")

	if got := resolveClaudeRoot(); got != "/profiles/claude-home" {
		t.Fatalf("CLAUDE_HOME must win over CLAUDE_CONFIG_DIR; got %q", got)
	}
}

func TestResolveClaudeRoot_ConfigDirWinsOverHomeDefault(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/claude-config")

	if got := resolveClaudeRoot(); got != "/profiles/claude-config" {
		t.Fatalf("CLAUDE_CONFIG_DIR must win over $HOME/.claude; got %q", got)
	}
}

func TestResolveClaudeRoot_DefaultsToHomeDotClaude(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	want := filepath.Join(home, ".claude")
	if got := resolveClaudeRoot(); got != want {
		t.Fatalf("default must be $HOME/.claude; got %q, want %q", got, want)
	}
}

func TestResolveClaudeRoot_NoHomeFallsBackToRoot(t *testing.T) {
	t.Setenv("HOME", "")
	t.Setenv("CLAUDE_HOME", "")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	if got := resolveClaudeRoot(); got != filepath.Join("/", ".claude") {
		t.Fatalf("unset HOME must fall back to /.claude; got %q", got)
	}
}

func TestClaudeConfigDirMismatch_FiresWhenClaudeHomeOverrides(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_HOME", "/profiles/claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/claude-config")

	ccd, ok := claudeConfigDirMismatch(resolveClaudeRoot())
	if !ok || ccd != "/profiles/claude-config" {
		t.Fatalf("expected mismatch against overridden root; got ok=%v ccd=%q", ok, ccd)
	}
}

func TestClaudeConfigDirMismatch_SilentWhenAgreeing(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_HOME", "")
	// Trailing slash: agreement is judged on cleaned paths.
	t.Setenv("CLAUDE_CONFIG_DIR", "/profiles/claude-config/")

	if ccd, ok := claudeConfigDirMismatch(resolveClaudeRoot()); ok {
		t.Fatalf("no mismatch expected when CLAUDE_CONFIG_DIR is the resolved root; got ccd=%q", ccd)
	}
}

func TestClaudeConfigDirMismatch_SilentWhenUnset(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("CLAUDE_HOME", "/profiles/claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	if ccd, ok := claudeConfigDirMismatch(resolveClaudeRoot()); ok {
		t.Fatalf("no mismatch expected when CLAUDE_CONFIG_DIR is unset; got ccd=%q", ccd)
	}
}
