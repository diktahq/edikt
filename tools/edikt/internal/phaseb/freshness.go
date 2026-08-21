package phaseb

// freshness.go — AC-2.9 / PC-8: is the RENDERED TREE current with respect to
// the sidecars?
//
// THE HOLE THIS CLOSES
//
// `gov compile --check` verified that every sidecar was fresh against its
// parent .md, and reported clean when they were. It never asked whether the
// RENDERED SURFACES matched those sidecars. So a tree rendered from an earlier
// sidecar state passed --check while serving stale governance (SR-020 /
// SAC-003) — the check reported clean precisely when it was most wrong.
//
// HOW IT IS ANSWERED, AND WHY NOT BY HASHING THE MANIFEST
//
// By rendering the whole tree into a scratch root and comparing bytes with the
// real one. Not by comparing the on-disk manifest's recorded hashes against
// the on-disk files: that only detects TAMPERING — files edited after the
// render — and is satisfied by a manifest and a tree that agree with each
// other while both being stale. The two failure modes are different and only
// this one answers the criterion.
//
// WHY THE DRIFT IS NAMED
//
// A bare "stale" tells the reader to re-run compile and nothing else. When the
// re-run does not fix it, they have no thread to pull. So the report names the
// surface, the direction, and the measurable deltas — directive counts and
// declared paths — because those are the two things that change when a
// sidecar's content moves under a rendered file.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// FreshnessDrift is one surface that differs between the rendered tree and a
// fresh render of the same sidecars.
type FreshnessDrift struct {
	Path    string
	Kind    string // "missing" | "unexpected" | "content"
	Details []string
}

// String renders one drift line naming what actually differs.
func (d FreshnessDrift) String() string {
	switch d.Kind {
	case "missing":
		return fmt.Sprintf("%s — MISSING from the rendered tree; a fresh render produces it", d.Path)
	case "unexpected":
		return fmt.Sprintf("%s — present in the rendered tree; a fresh render does NOT produce it "+
			"(orphan from an earlier sidecar state)", d.Path)
	default:
		if len(d.Details) == 0 {
			// Never emit a bare "content differs". If the differ cannot say
			// what moved, it says THAT — an unexplained difference is a
			// different report from a characterised one, and collapsing them
			// is how "stale" became uninformative in the first place.
			return fmt.Sprintf("%s — content differs; no characterised delta (bytes differ outside "+
				"directive counts and paths)", d.Path)
		}
		return fmt.Sprintf("%s — %s", d.Path, strings.Join(d.Details, "; "))
	}
}

// CheckRenderFreshness renders `pairs` into a scratch root and reports every
// surface whose bytes differ from the live tree.
//
// It returns an empty slice when the tree is current. An error means the check
// could not be performed — which is NOT the same as clean, and callers must
// not treat it as such (INV-011: fail closed).  edikt-guard:allow
func CheckRenderFreshness(projectRoot string, pairs []sidecar.Pair, opts Options) ([]FreshnessDrift, error) {
	// Same defaulting Merge() applies to its own local copy of opts
	// (merge.go:462-466) — duplicated here rather than assumed, because
	// Merge()'s defaulting never reaches this function's OWN opts. Below,
	// `opts` is passed to Merge() again (for the shadow render, via a
	// second copy) AND used directly by liveSurfaces() for the live-
	// directory scan. Merge() defaulting its own copy left liveSurfaces()
	// reading an empty OutDir when a caller (real production code, not
	// just tests) omitted it — os.ReadDir("") fails, the error is swallowed,
	// and the entire orphan-detection half of this check silently found
	// nothing. Reproduced directly; this is what closes it. (N1,
	// docs/internal/audits/TRIAGE-2026-08-20-bok-services-governance-projection.md)
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join(projectRoot, ".claude", "rules", "governance")
	}
	if opts.IndexPath == "" {
		opts.IndexPath = filepath.Join(projectRoot, ".claude", "rules", "governance.md")
	}

	scratch, err := os.MkdirTemp("", "edikt-freshness-")
	if err != nil {
		return nil, fmt.Errorf("freshness scratch: %w", err)
	}
	defer os.RemoveAll(scratch)

	// Same options, re-rooted. OutDir and IndexPath are absolute in the live
	// config, so they are re-derived under the scratch root — otherwise the
	// "fresh" render would write over the tree it is being compared against,
	// and the comparison would trivially pass.
	shadow := opts
	shadow.OutDir = filepath.Join(scratch, ".claude", "rules", "governance")
	shadow.IndexPath = filepath.Join(scratch, ".claude", "rules", "governance.md")

	res, err := Merge(scratch, pairs, shadow)
	if err != nil {
		return nil, fmt.Errorf("freshness shadow render: %w", err)
	}

	var drifts []FreshnessDrift
	expected := map[string]bool{}

	for _, s := range res.Surfaces {
		expected[s.Path] = true
		fresh := filepath.Join(scratch, filepath.FromSlash(s.Path))
		live := filepath.Join(projectRoot, filepath.FromSlash(s.Path))

		freshBody, ferr := os.ReadFile(fresh)
		if ferr != nil {
			return nil, fmt.Errorf("read shadow surface %s: %w", s.Path, ferr)
		}
		liveBody, lerr := os.ReadFile(live)
		if os.IsNotExist(lerr) {
			drifts = append(drifts, FreshnessDrift{Path: s.Path, Kind: "missing"})
			continue
		}
		if lerr != nil {
			return nil, fmt.Errorf("read live surface %s: %w", s.Path, lerr)
		}
		if string(freshBody) == string(liveBody) {
			continue
		}
		drifts = append(drifts, FreshnessDrift{
			Path:    s.Path,
			Kind:    "content",
			Details: characterise(string(liveBody), string(freshBody)),
		})
	}

	// Surfaces the live tree has and a fresh render does not. These are the
	// PC-8 case in its purest form: a file rendered from sidecars that no
	// longer say what it says.
	for _, s := range liveSurfaces(projectRoot, opts) {
		if !expected[s] {
			drifts = append(drifts, FreshnessDrift{Path: s, Kind: "unexpected"})
		}
	}

	sort.Slice(drifts, func(i, j int) bool { return drifts[i].Path < drifts[j].Path })
	return drifts, nil
}

// characterise names WHAT moved between two renders of the same surface.
//
// Directive count and declared paths, because those are the two deltas a
// reader can act on. It returns nil when neither moved, and the caller says so
// explicitly rather than printing a confident but empty explanation.
func characterise(live, fresh string) []string {
	var out []string

	lc, fc := countDirectiveLines(live), countDirectiveLines(fresh)
	if lc != fc {
		out = append(out, fmt.Sprintf("directive count %d -> %d", lc, fc))
	}
	lp, fp := declaredPaths(live), declaredPaths(fresh)
	if strings.Join(lp, ",") != strings.Join(fp, ",") {
		out = append(out, fmt.Sprintf("paths %v -> %v", lp, fp))
	}
	if len(out) == 0 {
		lh, fh := len(live), len(fresh)
		if lh != fh {
			out = append(out, fmt.Sprintf("size %d -> %d bytes", lh, fh))
		}
	}
	return out
}

func countDirectiveLines(body string) int {
	n := 0
	for _, line := range strings.Split(body, "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "- ") && strings.Contains(s, "(ref:") {
			n++
		}
	}
	return n
}

func declaredPaths(body string) []string {
	var out []string
	in := false
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimRight(line, " ")
		if t == "paths:" {
			in = true
			continue
		}
		if in {
			s := strings.TrimSpace(t)
			if strings.HasPrefix(s, "- ") {
				out = append(out, strings.Trim(strings.TrimPrefix(s, "- "), `"`))
				continue
			}
			break
		}
	}
	return out
}

// liveSurfaces enumerates what is actually on disk, so a surface a fresh
// render would NOT produce can be reported.
//
// Enumerated from the filesystem rather than from the live manifest: the
// manifest is itself a rendered artifact and can be as stale as anything else
// it lists. Asking it what exists would let one stale file vouch for another.
func liveSurfaces(projectRoot string, opts Options) []string {
	var out []string
	add := func(p string) {
		if rel, err := filepath.Rel(projectRoot, p); err == nil && !strings.HasPrefix(rel, "..") {
			out = append(out, filepath.ToSlash(rel))
		}
	}
	if ents, err := os.ReadDir(opts.OutDir); err == nil {
		for _, e := range ents {
			if !e.IsDir() {
				add(filepath.Join(opts.OutDir, e.Name()))
			}
		}
	}
	skillsRoot := filepath.Join(projectRoot, ".claude", "skills")
	if ents, err := os.ReadDir(skillsRoot); err == nil {
		for _, e := range ents {
			if e.IsDir() && strings.HasPrefix(e.Name(), "edikt-") {
				p := filepath.Join(skillsRoot, e.Name(), "SKILL.md")
				if _, serr := os.Stat(p); serr == nil {
					add(p)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}
