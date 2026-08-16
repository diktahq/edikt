// Package surfaces resolves a rendered governance surface through the render
// manifest instead of through an assumed directory layout.
//
// # WHY THE INDIRECTION EXISTS
//
// Before it, every consumer knew where a surface lived by hardcoding the path:
// `hook match` opened .claude/rules/governance/directive-index.yaml, doctor
// walked .claude/rules/governance.md, the graders globbed .claude/skills/.
// That makes the DIRECTORY LAYOUT the contract, so moving or renaming a
// surface silently breaks every consumer at once — and breaks them by finding
// nothing, which most of them report as "no directives apply".
//
// The manifest is the contract instead: consumers ask for a KIND (and topic)
// and are told where it is. A rename becomes a manifest edit, and the
// consumers follow.
//
// A MISSING SURFACE IS AN ERROR, NEVER AN EMPTY PATH. The whole failure this
// package exists to prevent is a consumer that cannot find its input and
// carries on as though the input said nothing. Every resolution either returns
// a path that exists or an error naming what it looked for.
//
// FALLBACK IS DELIBERATE AND NARROW. When no manifest exists at all — a
// project that has not compiled since the manifest landed — resolution falls
// back to the conventional path, because refusing there would brick every
// consumer on upgrade. When a manifest DOES exist, it is authoritative: a
// manifest that lists no entry of the requested kind means the surface was not
// rendered, and inventing a conventional path there would resurrect exactly
// the layout assumption this package removes.
package surfaces

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
)

// Surface kinds, as written by the renderer into the manifest.
const (
	KindAmbientCore   = "ambient-core"
	KindDirectiveIdx  = "directive-index"
	KindTopicFile     = "topic-file"
	KindSkillPackage  = "skill-package"
	ManifestRelPath   = ".claude/rules/governance/manifest.yaml"
	conventionalIndex = ".claude/rules/governance/directive-index.yaml"
	conventionalCore  = ".claude/rules/governance.md"
)

// ErrNoSurface reports that the manifest carries no entry of the requested
// kind. It is distinct from a read error so a caller can tell "the manifest
// says this surface does not exist" from "the manifest could not be read".
type ErrNoSurface struct {
	Kind  string
	Topic string
}

func (e *ErrNoSurface) Error() string {
	if e.Topic != "" {
		return fmt.Sprintf("the render manifest lists no %q surface for topic %q", e.Kind, e.Topic)
	}
	return fmt.Sprintf("the render manifest lists no %q surface", e.Kind)
}

// Manifest is a parsed manifest plus where it was read from.
type Manifest struct {
	Path     string
	Entries  []phaseb.ManifestEntry
	Present  bool // false when no manifest file exists at all
	rootPath string
}

// Load reads the manifest under root. A missing manifest is NOT an error —
// Present is false and callers fall back to conventional paths — but an
// unreadable one is, because a manifest that exists and cannot be parsed is
// a tampered or corrupt contract, not an absent one.
func Load(root string) (*Manifest, error) {
	p := filepath.Join(root, ManifestRelPath)
	body, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return &Manifest{Path: p, Present: false, rootPath: root}, nil
		}
		return nil, fmt.Errorf("reading %s: %w", ManifestRelPath, err)
	}
	entries := phaseb.ParseManifest(string(body))
	if len(entries) == 0 && !strings.Contains(string(body), "surfaces: []") {
		return nil, fmt.Errorf("%s parsed to zero surfaces and does not declare an empty set — refusing to treat an unreadable manifest as an empty one", ManifestRelPath)
	}
	return &Manifest{Path: p, Entries: entries, Present: true, rootPath: root}, nil
}

// Resolve returns the absolute path of the surface of the given kind (and
// topic, when the kind is topic-scoped).
func (m *Manifest) Resolve(kind, topic string) (string, error) {
	if m.Present {
		for _, e := range m.Entries {
			if e.Kind != kind {
				continue
			}
			if topic != "" && e.Topic != topic {
				continue
			}
			abs := filepath.Join(m.rootPath, e.Path)
			if _, err := os.Stat(abs); err != nil {
				// The manifest names a surface that is not on disk. That is a
				// broken contract, and it must be said out loud: reporting
				// "not found" would let a consumer treat a corrupt render as
				// a corpus with nothing to say.
				return "", fmt.Errorf("manifest lists %s at %s but it is not on disk: %w", kind, e.Path, err)
			}
			return abs, nil
		}
		return "", &ErrNoSurface{Kind: kind, Topic: topic}
	}

	// No manifest at all — a project that has not compiled since the manifest
	// landed. Fall back to convention rather than bricking the consumer.
	conv := conventionalPath(kind, topic)
	if conv == "" {
		return "", &ErrNoSurface{Kind: kind, Topic: topic}
	}
	abs := filepath.Join(m.rootPath, conv)
	if _, err := os.Stat(abs); err != nil {
		return "", fmt.Errorf("no render manifest and no surface at the conventional path %s: %w", conv, err)
	}
	return abs, nil
}

func conventionalPath(kind, topic string) string {
	switch kind {
	case KindDirectiveIdx:
		return conventionalIndex
	case KindAmbientCore:
		return conventionalCore
	case KindTopicFile:
		if topic == "" {
			return ""
		}
		return filepath.Join(".claude", "rules", "governance", topic+".md")
	case KindSkillPackage:
		if topic == "" {
			return ""
		}
		return filepath.Join(".claude", "skills", "edikt-"+topic, "SKILL.md")
	}
	return ""
}

// ResolveKind is the one-shot form for callers that do not hold a Manifest.
func ResolveKind(root, kind, topic string) (string, error) {
	m, err := Load(root)
	if err != nil {
		return "", err
	}
	return m.Resolve(kind, topic)
}

// PathsOfKind returns every surface of a kind, sorted. Used by consumers that
// iterate (the graders over skill packages, doctor over topic files) rather
// than resolving one.
func (m *Manifest) PathsOfKind(kind string) []string {
	var out []string
	if m.Present {
		for _, e := range m.Entries {
			if e.Kind == kind {
				out = append(out, filepath.Join(m.rootPath, e.Path))
			}
		}
	} else {
		pattern := ""
		switch kind {
		case KindTopicFile:
			pattern = filepath.Join(m.rootPath, ".claude", "rules", "governance", "*.md")
		case KindSkillPackage:
			pattern = filepath.Join(m.rootPath, ".claude", "skills", "edikt-*", "SKILL.md")
		}
		if pattern != "" {
			matches, _ := filepath.Glob(pattern)
			out = append(out, matches...)
		}
	}
	sort.Strings(out)
	return out
}
