package cmd

import (
	"fmt"
	"os"
	"path/filepath"
)

// resolveEdiktRoot returns the active EDIKT_ROOT using the same priority
// as the bash launcher:
//  1. $EDIKT_ROOT env override
//  2. Ancestor walk for .edikt/bin/edikt (project-mode)
//  3. $EDIKT_HOME env override
//  4. $HOME/.edikt (global default)
func resolveEdiktRoot() (string, error) {
	if root := os.Getenv("EDIKT_ROOT"); root != "" {
		return root, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		dir = "/"
	}
	home := os.Getenv("HOME")
	for dir != "" && dir != "/" && dir != home {
		marker := filepath.Join(dir, ".edikt", "bin", "edikt")
		if info, err := os.Stat(marker); err == nil && !info.IsDir() && info.Mode()&0o111 != 0 {
			// Also check that the launcher is owned by the current user (matching
			// the bash launcher's MED-6 ownership check) — skip non-owned entries.
			return filepath.Join(dir, ".edikt"), nil
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

// resolveClaudeRoot returns the Claude root directory ($CLAUDE_HOME or $HOME/.claude).
func resolveClaudeRoot() string {
	if ch := os.Getenv("CLAUDE_HOME"); ch != "" {
		return ch
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".claude")
	}
	return filepath.Join("/", ".claude")
}
