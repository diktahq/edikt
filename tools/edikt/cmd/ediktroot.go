package cmd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
)

// resolveEdiktRoot returns the active EDIKT_ROOT using the same priority
// as the bash launcher:
// 1. $EDIKT_ROOT env override
// 2. Ancestor walk for .edikt/bin/edikt (project-mode)
// 3. $EDIKT_HOME env override
// 4. $HOME/.edikt (global default)
func resolveEdiktRoot() (string, error) {
	if root := os.Getenv("EDIKT_ROOT"); root != "" {
		return root, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		dir = "/"
	}
	home := os.Getenv("HOME")
	// The walk is bounded at the INNERMOST project boundary — the first
	// directory walking up that carries a .git or .edikt marker. It stops
	// there whether or not it found a usable install.
	//
	// Before this bound the only stop condition was `dir != home`, which does
	// nothing when cwd lives outside $HOME: the walk climbed the real
	// filesystem until it found ANY .edikt/bin/edikt. Under a sandboxed HOME
	// that resolved to the developer's actual install
	// (/Users/…/.edikt) — an escape no environment pinning could close,
	// because the input is the process's working directory.
	//
	// Bounding costs nothing the walk was for. Its only unique job is finding
	// THIS project's install from a subdirectory of it; the global fallback is
	// rung 4 ($HOME/.edikt), and the loop already stopped before $HOME, so it
	// never served that case. Climbing past a project boundary only ever
	// duplicated rung 4 through an unguarded path — and that redundancy was
	// the escape.
	//
	// A workspace-level install above a child repo is no longer found
	// implicitly. That configuration is served explicitly by EDIKT_ROOT
	// (rung 1) or EDIKT_HOME (rung 3) — visible, pinnable by a sandbox guard,
	// and present in the published resolution chain, none of which is true of
	// a directory-layout accident. See docs/guides/project-mode.md.
	for dir != "" && dir != "/" && dir != home {
		marker := filepath.Join(dir, ".edikt", "bin", "edikt")
		if info, err := os.Stat(marker); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			// NOTE: the bash launcher applied an MED-6 ownership check (trust a
			// launcher only when it is owned by the current user). That check is
			// not yet ported here; resolution trusts any executable
			// .edikt/bin/edikt in an ancestor of $CWD up to $HOME (the walk is
			// bounded at $HOME below). Tracked as a v0.6.x hardening item.
			return filepath.Join(dir, ".edikt"), nil
		}
		// Innermost boundary: stop here rather than climbing out of the
		// project. Checked AFTER the marker test so a project whose own
		// .edikt/bin/edikt exists is still found at its own root.
		if isProjectBoundary(dir) {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if ediktHome := os.Getenv("EDIKT_HOME"); ediktHome != "" {
		return ediktHome, nil
	}
	if home != "" {
		return filepath.Join(home, ".edikt"), nil
	}
	return "", fmt.Errorf("cannot determine EDIKT_ROOT: HOME is not set")
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

// isProjectBoundary reports whether dir is the root of a project — it carries
// a .git or .edikt marker. The ancestor walk stops here.
//
// Both markers count. .git is the common case; .edikt matters because
// `install.sh --project` defines the project as cwd-at-install-time and
// creates .edikt/ there, which may not be a git root.
func isProjectBoundary(dir string) bool {
	for _, marker := range []string{".git", ".edikt"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}
