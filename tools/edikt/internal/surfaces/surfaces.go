// Package surfaces resolves a rendered governance surface through the render
// manifest instead of through an assumed directory layout.
//
// # WHY THE INDIRECTION EXISTS
//
// Before it, every consumer knew where a surface lived by hardcoding the path:
// `hook match` opened .claude/rules/governance/delivery-map.yaml, doctor
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
// THAT BINDS THE ENUMERATING FORM TOO, and for a while it did not. PathsOfKind
// answers with a set rather than a single path, and it used to join the
// manifest's entries to the root and hand them back unstatted — so a manifest
// naming a surface that no compile ever wrote produced a path that was not
// there and no error, at the one caller (doctor's routed-sources check) that
// iterates instead of resolving. One exported function kept the promise and its
// neighbour did not. Both go through the same stat now, and both report a
// manifest that names an absent file as the broken contract it is.
//
// AN EMPTY SET IS STILL A LEGAL ANSWER; AN ABSENT PATH IS NOT. "No surface of
// this kind was rendered" is a fact an iterating consumer must be able to
// receive and act on — it is why PathsOfKind returns a slice at all. Resolve
// errors on the same condition because a caller that asked for ONE surface has
// nothing to carry on with. The promise both keep is narrower and stronger than
// "never empty": nothing either of them hands back is a path that is not there.
//
// FALLBACK IS DELIBERATE AND NARROW. When no manifest exists at all — a
// project that has not compiled since the manifest landed — resolution falls
// back to the conventional path, because refusing there would brick every
// consumer on upgrade. That grace covers the same four kinds in both forms:
// PathsOfKind used to fall back for the two topic-scoped kinds and return
// nothing for ambient-core and delivery-map, which is a silent empty on
// precisely the path added to avoid one. When a manifest DOES exist, it is
// authoritative: a manifest that lists no entry of the requested kind means the
// surface was not rendered, and inventing a conventional path there would
// resurrect exactly the layout assumption this package removes.
package surfaces

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
)

// Surface kinds, as written by the renderer into the manifest.
const (
	KindAmbientCore  = "ambient-core"
	KindTopicFile    = "topic-file"
	KindSkillPackage = "skill-package"
	KindDeliveryMap  = "delivery-map"
	ManifestRelPath  = ".claude/rules/governance/manifest.yaml"
	conventionalCore = ".claude/rules/governance.md"
	conventionalMap  = ".claude/rules/governance/delivery-map.yaml"
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

// listedPath joins a manifest entry to the root and refuses to hand back a path
// that is not on disk.
//
// ONE FACT, ONE PLACE. Both exported forms resolve a listed entry through this,
// so "the manifest names a surface that was never written" has a single
// behaviour and a single message. The divergence it closed was exactly a second
// copy of this decision that did not exist: Resolve statted, PathsOfKind did
// not, and the package doc spoke for both.
func (m *Manifest) listedPath(kind string, e phaseb.ManifestEntry) (string, error) {
	abs := filepath.Join(m.rootPath, e.Path)
	if _, err := os.Stat(abs); err != nil {
		// The manifest names a surface that is not on disk. That is a broken
		// contract, and it must be said out loud: reporting "not found" would
		// let a consumer treat a corrupt render as a corpus with nothing to
		// say.
		return "", fmt.Errorf("manifest lists %s at %s but it is not on disk: %w", kind, e.Path, err)
	}
	return abs, nil
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
			return m.listedPath(kind, e)
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
	case KindAmbientCore:
		return conventionalCore
	case KindDeliveryMap:
		return conventionalMap
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

// PathsOfKind returns every surface of a kind, sorted, and an error naming any
// the manifest lists that is not on disk. Used by consumers that iterate (the
// graders over skill packages, doctor over topic files) rather than resolving
// one.
//
// It returns NIL alongside an error rather than the entries it did manage to
// stat. A consumer handed a partial list reports on whatever survived as though
// that were the whole corpus, which is this package's own failure mode reached
// by a different route — and it is what doctor's routed-sources check would
// have done, since it ranges over the result.
func (m *Manifest) PathsOfKind(kind string) ([]string, error) {
	var out []string
	if m.Present {
		var problems []error
		for _, e := range m.Entries {
			if e.Kind != kind {
				continue
			}
			abs, err := m.listedPath(kind, e)
			if err != nil {
				// EVERY missing entry, not the first: an enumeration that named
				// one of three sends a reader to fix that one and re-run into
				// the next, which is how a corrupt render gets reported three
				// times as three separate incidents.
				problems = append(problems, err)
				continue
			}
			out = append(out, abs)
		}
		if len(problems) > 0 {
			return nil, errors.Join(problems...)
		}
	} else {
		// No manifest at all — the same upgrade grace the resolving form gets,
		// for the same four kinds. ambient-core and delivery-map are singletons
		// and fall back to the conventional path when it is there; the two
		// topic-scoped kinds glob, and filepath.Glob only ever returns paths
		// that exist, so neither branch can hand back an absent one.
		//
		// Nothing on disk yields an empty set rather than an error: a consumer
		// that iterates has somewhere to go with "none", which is not true of
		// one that asked for a single surface.
		switch kind {
		case KindAmbientCore, KindDeliveryMap:
			abs := filepath.Join(m.rootPath, conventionalPath(kind, ""))
			if _, err := os.Stat(abs); err == nil {
				out = append(out, abs)
			}
		case KindTopicFile, KindSkillPackage:
			pattern := filepath.Join(m.rootPath, ".claude", "rules", "governance", "*.md")
			if kind == KindSkillPackage {
				pattern = filepath.Join(m.rootPath, ".claude", "skills", "edikt-*", "SKILL.md")
			}
			// Glob's error is unreachable: it reports only a malformed pattern,
			// and both patterns are constants assembled here.
			matches, _ := filepath.Glob(pattern)
			out = append(out, matches...)
		}
	}
	sort.Strings(out)
	return out, nil
}
