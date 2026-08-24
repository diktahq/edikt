// Package installsurface resolves install destinations by (harness, kind)
// instead of each caller composing a path from a hardcoded ".claude"
// literal. SPEC-015 SP-005 names ~30 pre-existing call sites across
// tools/edikt/ that hardcode ".claude/..." directly — this package exists so
// FR-009's and FR-016's new Go code adds one row per new destination instead
// of one more hardcoded literal, and so a future harness (the Codex port,
// BRAIN-009) adds a row instead of touching every call site.
//
// tools/edikt/internal/surfaces is the discipline precedent, not the code to
// reuse: surfaces.go resolves governance-RENDERING surfaces from a compile
// manifest (a different subject, a different contract — entries discovered
// at compile time). This package resolves harness-CONFIG destinations from a
// small static table, keyed by (harness, kind) rather than looked up in a
// manifest. What the two packages share is the one rule that matters: a
// missing destination is an error, never a guessed or empty path.
package installsurface

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/diktahq/edikt/tools/edikt/internal/claudepaths"
)

// Harness identifies the host agent an install destination is being
// resolved for. HarnessClaudeCode is the only harness this package
// implements in v0.8.0; every other value — including the empty string —
// is unsupported and Resolve errors on it rather than guessing.
type Harness string

// HarnessClaudeCode is the only harness implemented in v0.8.0.
const HarnessClaudeCode Harness = "claude-code"

// Kind identifies which artifact's destination is being resolved.
type Kind string

const (
	// KindSettingsFile is <claude-root>/settings.json.
	KindSettingsFile Kind = "settings-file"
	// KindManagedSidecar is <edikt-home>/state/settings-managed.json — always
	// global, never project-relative, regardless of install mode.
	KindManagedSidecar Kind = "managed-sidecar"
)

// Roots carries the resolved directories Resolve needs. Callers build this
// themselves rather than Resolve reaching for environment variables directly
// — ClaudeRoot varies by install mode (project-mode installs it at
// <cwd>/.claude, global-mode installs resolve it through
// internal/claudepaths), and only the caller knows which mode is active.
// ResolveGlobalRoots is provided for the common global-mode case.
type Roots struct {
	// ClaudeRoot is the active Claude Code configuration directory —
	// project- or global-scoped depending on install mode.
	ClaudeRoot string
	// EdiktHome is $EDIKT_HOME or $HOME/.edikt — always global, never
	// project-relative, independent of install mode.
	EdiktHome string
}

// ErrUnsupportedDestination reports that Resolve was asked for a
// (harness, kind) pair it has no row for. Distinct from a generic error so a
// caller can tell "this destination is not implemented" from an
// unresolvable root — and so this package never silently substitutes a
// guessed or empty path for either.
type ErrUnsupportedDestination struct {
	Harness Harness
	Kind    Kind
}

func (e *ErrUnsupportedDestination) Error() string {
	return fmt.Sprintf("installsurface: no destination for harness %q, kind %q", e.Harness, e.Kind)
}

// Resolve returns the absolute path for kind under harness, or an error
// naming what it could not resolve. An unknown harness OR kind is always an
// error — never a guessed or empty path.
func Resolve(h Harness, k Kind, r Roots) (string, error) {
	if h != HarnessClaudeCode {
		return "", &ErrUnsupportedDestination{Harness: h, Kind: k}
	}

	switch k {
	case KindSettingsFile:
		if r.ClaudeRoot == "" {
			return "", fmt.Errorf("installsurface: ClaudeRoot is empty, cannot resolve %s for harness %q", KindSettingsFile, h)
		}
		return filepath.Join(r.ClaudeRoot, "settings.json"), nil
	case KindManagedSidecar:
		if r.EdiktHome == "" {
			return "", fmt.Errorf("installsurface: EdiktHome is empty, cannot resolve %s for harness %q", KindManagedSidecar, h)
		}
		return filepath.Join(r.EdiktHome, "state", "settings-managed.json"), nil
	default:
		return "", &ErrUnsupportedDestination{Harness: h, Kind: k}
	}
}

// ResolveGlobalRoots builds the Roots a global-mode install needs.
//
// ClaudeRoot is resolved by calling internal/claudepaths.ResolveClaudeRoot()
// — the one shared definition of the CLAUDE_HOME -> CLAUDE_CONFIG_DIR ->
// $HOME/.claude chain (commit 8b17ed7; its own doc comment names the failure
// this avoids: "two things that must agree, kept separately", GL-002).
// installsurface must never grow a second, independent copy of that chain.
//
// A project-mode install's ClaudeRoot is <cwd>/.claude, not this function's
// concern — a caller in that mode builds Roots directly instead of calling
// ResolveGlobalRoots.
func ResolveGlobalRoots() Roots {
	return Roots{
		ClaudeRoot: claudepaths.ResolveClaudeRoot(),
		EdiktHome:  resolveEdiktHome(),
	}
}

// resolveEdiktHome mirrors the global rungs of cmd/ediktroot.go's
// resolveEdiktRoot() ($EDIKT_HOME -> $HOME/.edikt) — deliberately NOT its
// project-boundary ancestor walk, because KindManagedSidecar is always
// global regardless of install mode (Roots.EdiktHome's own contract above).
func resolveEdiktHome() string {
	if h := os.Getenv("EDIKT_HOME"); h != "" {
		return h
	}
	if home := os.Getenv("HOME"); home != "" {
		return filepath.Join(home, ".edikt")
	}
	return filepath.Join(string(filepath.Separator), ".edikt")
}
