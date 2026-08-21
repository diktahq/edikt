// Package claudepaths resolves the active Claude Code configuration
// directory. It exists so the resolution chain has ONE definition shared by
// every caller that needs it, instead of each package growing its own copy
// that can silently drift from the others (the GL-002 failure mode: two
// things that must agree, kept separately).
//
// Before this package existed, cmd/ediktroot.go's resolveClaudeRoot() was
// the only implementation, and internal/phasea/agentmodel.go's extractor-
// agent resolvers had no fallback at all — project-local only. That gap is
// F4/F5 (docs/internal/issues/agentmodel-resolver-no-global-fallback.md):
// `gov reextract --status` hard-failed whenever the sidecar-extractor agent
// resolved from the user's Claude profile rather than the project, even
// though it was perfectly resolvable there.
package claudepaths

import (
	"os"
	"path/filepath"
)

// ResolveClaudeRoot returns the active Claude Code configuration directory,
// following the same precedence Claude Code itself honours:
// CLAUDE_HOME (edikt's explicit override) -> CLAUDE_CONFIG_DIR -> $HOME/.claude.
func ResolveClaudeRoot() string {
	if ch := os.Getenv("CLAUDE_HOME"); ch != "" {
		return ch
	}
	if ccd := os.Getenv("CLAUDE_CONFIG_DIR"); ccd != "" {
		return ccd
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return filepath.Join("/", ".claude")
}
