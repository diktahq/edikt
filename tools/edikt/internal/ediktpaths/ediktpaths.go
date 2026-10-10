// Package ediktpaths resolves the active edikt root and the templates a
// command reads from the installed payload.
//
// It exists so there is ONE definition of each. Before it, every command that
// needed a shipped template carried its own candidate list, and most of them
// joined `templates/…` onto the working directory — a path that exists only in
// a checkout of edikt itself. `gov grade-compile` therefore could not find its
// rubric on any adopter project, nor from a subdirectory of edikt's own repo
// (#11 comment item 2). The installed payload, which carries every template,
// was never a candidate, though ADR-005 §1 names it as the fallback.  // edikt-guard:allow
package ediktpaths

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveEdiktRoot returns the active EDIKT_ROOT using the same priority as
// the bash launcher:
//  1. $EDIKT_ROOT env override
//  2. Ancestor walk for .edikt/bin/edikt (project mode)
//  3. $EDIKT_HOME env override
//  4. $HOME/.edikt (global default)
func ResolveEdiktRoot() (string, error) {
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
		if IsProjectBoundary(dir) {
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

// IsProjectBoundary reports whether dir is the root of a project — it carries
// a .git or .edikt marker. The ancestor walk stops here.
//
// Both markers count. .git is the common case; .edikt matters because
// `install.sh --project` defines the project as cwd-at-install-time and
// creates .edikt/ there, which may not be a git root.
func IsProjectBoundary(dir string) bool {
	for _, marker := range []string{".git", ".edikt"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// NotFoundError is returned when no rung of the template ladder holds rel. It
// names every path tried: "not found" alone cannot tell an uninstalled
// payload from a wrong $EDIKT_HOME, and the user has to know which.
type NotFoundError struct {
	Rel        string
	Kind       string // "file" or "directory"
	Candidates []string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("template %s %q not found; looked in (in order):\n  %s",
		e.Kind, e.Rel, strings.Join(e.Candidates, "\n  "))
}

// Candidates returns the template ladder for rel, highest precedence first:
//
//  1. <projectRoot>/.edikt/templates/<rel>   project override (ADR-005 §1)  // edikt-guard:allow
//  2. <ediktRoot>/current/templates/<rel>    the installed payload
//  3. <ediktRoot>/templates/<rel>            the root's convenience link
//  4. <projectRoot>/templates/<rel>          a checkout of edikt itself (dev)
//
// Rungs 2–3 are omitted when no edikt root resolves (HOME unset), and a path
// that appears twice (project mode makes rung 3 equal rung 1) is listed once.
// Callers that must keep falling through on a parse failure iterate this list;
// everyone else calls ResolveTemplate.
func Candidates(projectRoot, rel string) []string {
	rungs := []string{filepath.Join(projectRoot, ".edikt", "templates", rel)}
	if root, err := ResolveEdiktRoot(); err == nil && root != "" {
		rungs = append(rungs,
			filepath.Join(root, "current", "templates", rel),
			filepath.Join(root, "templates", rel))
	}
	rungs = append(rungs, filepath.Join(projectRoot, "templates", rel))

	seen := make(map[string]bool, len(rungs))
	out := rungs[:0]
	for _, r := range rungs {
		c := filepath.Clean(r)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

// ResolveTemplate returns the first rung of the ladder (see Candidates) that
// holds rel as a regular file, or a *NotFoundError naming every rung tried.
func ResolveTemplate(projectRoot, rel string) (string, error) {
	return resolve(projectRoot, rel, false)
}

// ResolveTemplateDir is ResolveTemplate for a directory of templates.
func ResolveTemplateDir(projectRoot, rel string) (string, error) {
	return resolve(projectRoot, rel, true)
}

func resolve(projectRoot, rel string, wantDir bool) (string, error) {
	if err := validateRel(rel); err != nil {
		return "", err
	}
	kind := "file"
	if wantDir {
		kind = "directory"
	}
	cands := Candidates(projectRoot, rel)
	for _, c := range cands {
		info, err := os.Stat(c)
		if err != nil {
			continue
		}
		if wantDir && info.IsDir() {
			return c, nil
		}
		if !wantDir && info.Mode().IsRegular() {
			return c, nil
		}
	}
	return "", &NotFoundError{Rel: rel, Kind: kind, Candidates: cands}
}

// validateRel refuses a rel that is empty, absolute, or climbs out of the
// templates tree: it is joined onto every rung, so an escaping one would turn
// a template lookup into an arbitrary read (INV-006).  // edikt-guard:allow
func validateRel(rel string) error {
	if rel == "" {
		return fmt.Errorf("template path is empty")
	}
	if filepath.IsAbs(rel) {
		return fmt.Errorf("template path %q must be relative to a templates tree", rel)
	}
	for _, part := range strings.Split(filepath.ToSlash(rel), "/") {
		if part == ".." {
			return fmt.Errorf("template path %q escapes the templates tree", rel)
		}
	}
	return nil
}
