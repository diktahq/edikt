package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Install-boundary check for the two symlinks doctor already reads —
// $EDIKT_ROOT/current and $CLAUDE_ROOT/commands/edikt.
//
// WHY THIS IS A TRIPWIRE AND NOT A FAILURE. Claude Code 2.1.2xx refuses a
// plugin whose declared files resolve, through a symlink, to a location
// outside the plugin. The 2.1.257 binary's own wording:
//
//	"Files a plugin declares must resolve to a location inside the plugin
//	 (or, for a local marketplace, inside the marketplace). If a symlink
//	 points elsewhere, replace it with a copy of the file or point Claude
//	 Code at the directory ..."
//
// Today that rule is applied to PLUGINS ONLY. edikt's install is not a
// plugin: `commands/edikt` and `current` are ordinary symlinks the host
// follows without comment, and a dev link (`edikt dev link`, marked by
// DEV_SOURCE) deliberately resolves into a checkout outside ~/.edikt. So a
// link resolving outside the install boundary loads fine today — this check
// reports that shape rather than failing on it, because the day the host
// extends the refusal past plugins, every dev-linked install stops loading
// with no edikt-side signal. The tripwire exists so that day is visible
// here first.
//
// WARN, never ERROR. Advisory by design, for the same reason the
// surface-budget check is: the condition is real and standing, and it is
// not a defect in this install.

// installBoundaryFinding is one link's resolution against a boundary.
type installBoundaryFinding struct {
	Link     string // the symlink path as checked
	Resolved string // fully resolved target
	Boundary string // the directory the target is expected to live under
	Inside   bool
	DevLink  bool // DEV_SOURCE marker present at the resolved `current`
	Err      error
}

// resolvesInside reports whether link's fully-resolved target lies under
// boundary. Both sides are resolved through EvalSymlinks so a boundary that
// is itself a symlink (a $HOME on a linked volume, say) compares on real
// paths rather than on spelling.
func resolvesInside(link, boundary string) installBoundaryFinding {
	f := installBoundaryFinding{Link: link, Boundary: boundary}
	resolved, err := filepath.EvalSymlinks(link)
	if err != nil {
		f.Err = err
		return f
	}
	f.Resolved = resolved
	b, err := filepath.EvalSymlinks(boundary)
	if err != nil {
		b = filepath.Clean(boundary)
	}
	rel, err := filepath.Rel(b, resolved)
	if err != nil {
		return f
	}
	f.Inside = rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel))
	return f
}

// runInstallBoundaryCheck checks `current` and `commands/edikt` against
// ediktRoot (an installed commands/edikt points at $EDIKT_ROOT/current/
// commands, so the boundary is the same for both). Returns the number of
// WARN lines emitted. A missing or non-symlink entry is not this check's
// business — the symlink checks above it already report those.
func runInstallBoundaryCheck(ediktRoot, claudeRoot string, out io.Writer) int {
	warns := 0
	links := []string{
		filepath.Join(ediktRoot, "current"),
		filepath.Join(claudeRoot, "commands", "edikt"),
	}
	devLink := false
	if _, err := os.Stat(filepath.Join(ediktRoot, "current", "DEV_SOURCE")); err == nil {
		devLink = true
	}
	for _, link := range links {
		if info, err := os.Lstat(link); err != nil || info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		f := resolvesInside(link, ediktRoot)
		f.DevLink = devLink
		if f.Err != nil || f.Inside {
			continue
		}
		warns++
		fmt.Fprintf(out, "  WARN: %s resolves outside the install boundary\n", link)
		fmt.Fprintf(out, "        -> %s\n", f.Resolved)
		fmt.Fprintf(out, "        boundary: %s\n", ediktRoot)
		if f.DevLink {
			fmt.Fprintf(out, "        dev link (DEV_SOURCE present) — intended for a checkout, and it loads today.\n")
		}
		fmt.Fprintf(out, "        Claude Code 2.1.2xx refuses this shape for PLUGINS: \"Files a plugin declares\n")
		fmt.Fprintf(out, "        must resolve to a location inside the plugin ... If a symlink points\n")
		fmt.Fprintf(out, "        elsewhere, replace it with a copy of the file.\" Plugins only, so far.\n")
		fmt.Fprintf(out, "        This line is the tripwire for when it is not.\n")
	}
	return warns
}
