package claudepaths

import (
	"path/filepath"
	"testing"
)

// F5 — docs/internal/issues/agentmodel-resolver-no-global-fallback.md.
// doctor's agent-drift check (cmd/doctor_agents.go) and agentmodel.go's
// extractor resolvers both depend on this precedence chain resolving
// correctly; a regression here would silently affect both. Pins the exact
// order Claude Code itself honours.
func TestResolveClaudeRoot_Precedence(t *testing.T) {
	t.Run("CLAUDE_HOME wins over everything", func(t *testing.T) {
		t.Setenv("CLAUDE_HOME", "/claude-home")
		t.Setenv("CLAUDE_CONFIG_DIR", "/claude-config-dir")
		t.Setenv("HOME", "/home-dir")
		if got := ResolveClaudeRoot(); got != "/claude-home" {
			t.Fatalf("got %q, want %q", got, "/claude-home")
		}
	})

	t.Run("CLAUDE_CONFIG_DIR wins when CLAUDE_HOME is unset", func(t *testing.T) {
		t.Setenv("CLAUDE_HOME", "")
		t.Setenv("CLAUDE_CONFIG_DIR", "/claude-config-dir")
		t.Setenv("HOME", "/home-dir")
		if got := ResolveClaudeRoot(); got != "/claude-config-dir" {
			t.Fatalf("got %q, want %q", got, "/claude-config-dir")
		}
	})

	t.Run("falls back to $HOME/.claude when neither override is set", func(t *testing.T) {
		t.Setenv("CLAUDE_HOME", "")
		t.Setenv("CLAUDE_CONFIG_DIR", "")
		t.Setenv("HOME", "/home-dir")
		want := filepath.Join("/home-dir", ".claude")
		if got := ResolveClaudeRoot(); got != want {
			t.Fatalf("got %q, want %q", got, want)
		}
	})
}
