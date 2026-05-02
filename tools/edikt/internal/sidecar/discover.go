package sidecar

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Pair couples a parent .md with its co-located .edikt.yaml.
// Sidecar is nil when the .yaml is missing on disk; LoadErr captures any
// validation/parse error so the caller can surface it without aborting the
// whole walk.
type Pair struct {
	ParentPath  string
	SidecarPath string
	ArtifactID  string
	Sidecar     *Sidecar
	LoadErr     error
}

// Discover walks the artifact dirs (decisions, invariants, guidelines) under
// projectRoot and returns one Pair per parent .md. Sidecars without a
// matching parent are reported separately by the doctor (out of scope here).
func Discover(projectRoot string, dirs []string) ([]Pair, error) {
	var pairs []Pair
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			parentPath := filepath.Join(absDir, name)
			sidecarPath := strings.TrimSuffix(parentPath, ".md") + ".edikt.yaml"
			p := Pair{
				ParentPath:  parentPath,
				SidecarPath: sidecarPath,
				ArtifactID:  artifactIDFromName(name),
			}
			if _, err := os.Stat(sidecarPath); err == nil {
				sc, lerr := Load(sidecarPath)
				if lerr != nil {
					p.LoadErr = lerr
				} else {
					p.Sidecar = sc
				}
			}
			pairs = append(pairs, p)
		}
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].ParentPath < pairs[j].ParentPath })
	return pairs, nil
}

// HasAnySidecar returns true iff at least one .edikt.yaml exists under any
// of the artifact dirs. Used by the cobra entry point to dispatch
// two-phase compile vs. surfacing the pre-migration error per ADR-027 §5.
func HasAnySidecar(projectRoot string, dirs []string) bool {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.HasSuffix(e.Name(), ".edikt.yaml") {
				return true
			}
		}
	}
	return false
}

// HasAnyGovernanceMarkdown returns true iff at least one governance .md
// (a non-sidecar .md whose basename does not start with "_") exists under
// any of the artifact dirs. Distinguishes a pre-migration project (has
// .md but no .edikt.yaml — must hard-fail per ADR-027 §5) from an empty
// project (no .md at all — compile is a no-op).
func HasAnyGovernanceMarkdown(projectRoot string, dirs []string) bool {
	for _, dir := range dirs {
		if dir == "" {
			continue
		}
		absDir := filepath.Join(projectRoot, dir)
		entries, err := os.ReadDir(absDir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			if !strings.HasSuffix(name, ".md") {
				continue
			}
			if strings.HasPrefix(name, "_") {
				continue
			}
			return true
		}
	}
	return false
}

func artifactIDFromName(name string) string {
	base := strings.TrimSuffix(name, ".md")
	if strings.HasPrefix(base, "ADR-") || strings.HasPrefix(base, "INV-") {
		end := 4
		for end < len(base) && base[end] >= '0' && base[end] <= '9' {
			end++
		}
		return base[:end]
	}
	return base
}
