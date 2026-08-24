package installsurface

import (
	"errors"
	"path/filepath"
	"testing"
)

// TestResolveUnknownHarnessErrors pins the package's one non-negotiable
// property (SPEC-015 SP-005): an unknown harness OR kind is ALWAYS an error,
// never a guessed or empty path standing in for one.
func TestResolveUnknownHarnessErrors(t *testing.T) {
	roots := Roots{ClaudeRoot: "/tmp/claude-root", EdiktHome: "/tmp/edikt-home"}

	t.Run("unknown harness errors, never a guessed path", func(t *testing.T) {
		path, err := Resolve(Harness("codex"), KindSettingsFile, roots)
		if err == nil {
			t.Fatalf("expected an error for an unknown harness, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
		var target *ErrUnsupportedDestination
		if !errors.As(err, &target) {
			t.Fatalf("expected *ErrUnsupportedDestination, got %T: %v", err, err)
		}
	})

	t.Run("empty harness errors, never a guessed path", func(t *testing.T) {
		path, err := Resolve(Harness(""), KindSettingsFile, roots)
		if err == nil {
			t.Fatalf("expected an error for an empty harness, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
	})

	t.Run("unknown kind errors, never a guessed path", func(t *testing.T) {
		path, err := Resolve(HarnessClaudeCode, Kind("unknown-kind"), roots)
		if err == nil {
			t.Fatalf("expected an error for an unknown kind, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
		var target *ErrUnsupportedDestination
		if !errors.As(err, &target) {
			t.Fatalf("expected *ErrUnsupportedDestination, got %T: %v", err, err)
		}
	})

	t.Run("unknown harness AND unknown kind still errors, not a coincidental pass", func(t *testing.T) {
		path, err := Resolve(Harness("codex"), Kind("unknown-kind"), roots)
		if err == nil {
			t.Fatalf("expected an error, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
	})
}

// TestResolveKnownDestinations is the positive complement — GL-002 rule 11:
// a check with only a negative case proves nothing about where the boundary
// actually sits.
func TestResolveKnownDestinations(t *testing.T) {
	roots := Roots{ClaudeRoot: "/tmp/claude-root", EdiktHome: "/tmp/edikt-home"}

	t.Run("settings file resolves under ClaudeRoot", func(t *testing.T) {
		path, err := Resolve(HarnessClaudeCode, KindSettingsFile, roots)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(roots.ClaudeRoot, "settings.json")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("managed sidecar resolves under EdiktHome, not ClaudeRoot", func(t *testing.T) {
		path, err := Resolve(HarnessClaudeCode, KindManagedSidecar, roots)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := filepath.Join(roots.EdiktHome, "state", "settings-managed.json")
		if path != want {
			t.Fatalf("got %q, want %q", path, want)
		}
	})

	t.Run("missing ClaudeRoot errors rather than resolving a relative path", func(t *testing.T) {
		path, err := Resolve(HarnessClaudeCode, KindSettingsFile, Roots{EdiktHome: "/tmp/edikt-home"})
		if err == nil {
			t.Fatalf("expected an error for an empty ClaudeRoot, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
	})

	t.Run("missing EdiktHome errors rather than resolving a relative path", func(t *testing.T) {
		path, err := Resolve(HarnessClaudeCode, KindManagedSidecar, Roots{ClaudeRoot: "/tmp/claude-root"})
		if err == nil {
			t.Fatalf("expected an error for an empty EdiktHome, got path %q", path)
		}
		if path != "" {
			t.Fatalf("expected an empty path alongside the error, got %q", path)
		}
	})
}

// TestResolveGlobalRootsUsesClaudepaths pins the AC that installsurface
// resolves the Claude config root by calling
// internal/claudepaths.ResolveClaudeRoot() — never by re-implementing the
// CLAUDE_HOME -> CLAUDE_CONFIG_DIR -> $HOME/.claude chain itself. Setting
// CLAUDE_HOME and asserting ResolveGlobalRoots().ClaudeRoot follows it is an
// indirect but real check: if this package ever grew its own copy of the
// chain instead of delegating, a divergent copy could still (by accident)
// honor CLAUDE_HOME too, so this is necessary but not sufficient — the
// stronger, structural half of this property (exactly one package
// implements the chain) is asserted by grep, not by a unit test.
func TestResolveGlobalRootsUsesClaudepaths(t *testing.T) {
	t.Setenv("CLAUDE_HOME", "/tmp/pinned-claude-home")
	t.Setenv("CLAUDE_CONFIG_DIR", "")

	roots := ResolveGlobalRoots()
	if roots.ClaudeRoot != "/tmp/pinned-claude-home" {
		t.Fatalf("ResolveGlobalRoots().ClaudeRoot = %q, want the CLAUDE_HOME override to win, per claudepaths' precedence", roots.ClaudeRoot)
	}
}
