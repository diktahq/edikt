package cmd

import (
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
	"github.com/diktahq/edikt/tools/edikt/internal/ediktpaths"
)

// resolveEdiktRoot returns the active EDIKT_ROOT ($EDIKT_ROOT → project-mode
// ancestor walk → $EDIKT_HOME → $HOME/.edikt). It delegates to
// internal/ediktpaths, the single definition of that chain, which the
// template ladder (ADR-005 §1) resolves the installed payload through.  // edikt-guard:allow
func resolveEdiktRoot() (string, error) {
	return ediktpaths.ResolveEdiktRoot()
}

// resolveClaudeRoot returns the Claude root directory using this priority:
// 1. $CLAUDE_HOME — edikt's explicit override
// 2. $CLAUDE_CONFIG_DIR — Claude Code's own profile selector
// 3. $HOME/.claude — global default
//
// Delegates to internal/claudepaths, the single shared definition of this
// chain — internal/phasea's extractor-agent resolvers use the same one, so
// `doctor`'s agent-drift check and `gov reextract --status` can't drift
// apart on where "the active Claude profile" is.
func resolveClaudeRoot() string {
	return claudepaths.ResolveClaudeRoot()
}

// claudeConfigDirMismatch reports whether $CLAUDE_CONFIG_DIR is set but does
// not match the resolved Claude root — the case where a $CLAUDE_HOME override
// points edikt at a directory the active Claude profile never reads.
func claudeConfigDirMismatch(claudeRoot string) (string, bool) {
	ccd := os.Getenv("CLAUDE_CONFIG_DIR")
	if ccd == "" || filepath.Clean(ccd) == filepath.Clean(claudeRoot) {
		return "", false
	}
	return ccd, true
}
