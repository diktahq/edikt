package hookcorpus

// matrix.go — test/fixtures/hook-corpus/<hook>/matrix.yaml, and the input
// digest that pins a recording to the exact bytes it was recorded from.
//
// Schema: templates/schemas/hook-corpus-matrix.v1.schema.json.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// The four axes of ADR-063 Decision §2. Coverage is measured against exactly
// these; a fifth would be a spec change, not a config option.
const (
	AxisPayload     = "payload"
	AxisConfig      = "config"
	AxisEnvironment = "environment"
	AxisOrdering    = "ordering"
)

// Axes returns the four axes in their canonical order.
func Axes() []string { return []string{AxisPayload, AxisConfig, AxisEnvironment, AxisOrdering} }

// ConfigDefault is the matrix's literal for "the sandbox's own committed config".
const ConfigDefault = "default"

// Matrix is one hook's curated scenario list.
type Matrix struct {
	SchemaVersion int        `yaml:"schema_version"`
	Hook          string     `yaml:"hook"`
	Scenarios     []Scenario `yaml:"scenarios"`

	Path string `yaml:"-"`
}

// Scenario is one recorded case.
type Scenario struct {
	ID            string             `yaml:"id"`
	Axis          []string           `yaml:"axis"`
	SourceFinding string             `yaml:"source_finding,omitempty"`
	Payload       *string            `yaml:"payload"`
	Config        string             `yaml:"config"`
	Env           map[string]*string `yaml:"env"`
	Sequence      []Step             `yaml:"sequence"`
	Notes         string             `yaml:"notes"`

	// Files seeds the sandbox PROJECT with extra fixture files before the run:
	// {sandbox-relative destination: matrix-relative source}.
	//
	// WHY IT EXISTS. Two hooks hit the same wall. Everything the write-time
	// injection tier DOES — deny on a MUST-grade match, suppress the repeat,
	// degrade past the bounce budget — needs a compiled directive index in the
	// sandbox, and payload/config/env cannot express one.
	// inject-directives-post's matrix documented the gap and worked around it
	// by choosing scenarios whose outcome converges; inject-directives-pre
	// could not, because without an index its eight scenarios collapse to ONE
	// observable outcome and check-corpus-discriminates fails them as a corpus
	// that "cannot detect a deleted guard in any branch".
	//
	// The obvious alternative — putting the index in the shared `_base` tree —
	// was tried and MEASURED to break inject-directives-post: with an index
	// present the Go run writes an index cache the bash recording predates,
	// and a migrated row can never be re-recorded against bash. Per-scenario
	// seeding keeps the blast radius at exactly the scenarios that ask for it.
	//
	// Folded into InputDigest only when non-empty, so every existing recording
	// keeps its digest.
	Files map[string]string `yaml:"files"`
}

// Step is one sub-invocation of an ordering-axis scenario. Steps share ONE
// sandbox tree, carried forward, because the point of the axis is that step 2
// observes step 1's filesystem effects.
type Step struct {
	Payload *string            `yaml:"payload"`
	Config  string             `yaml:"config"`
	Env     map[string]*string `yaml:"env"`

	// DelayMs is a SIMULATED gap, never a slept one. SPEC-014's Testing
	// Strategy is explicit: the recorder must not sleep wall-clock for a
	// dedup-TTL boundary. It is folded into the input digest and injected
	// through the clock, so a TTL scenario is reproducible and instant.
	DelayMs int `yaml:"delay_ms"`
}

// Steps returns the scenario as a step list — a single-step scenario and a
// sequence scenario are the same shape to everything downstream, which is what
// keeps the recorder from carrying two execution paths.
func (s Scenario) Steps() []Step {
	if len(s.Sequence) > 0 {
		return s.Sequence
	}
	return []Step{{Payload: s.Payload, Config: s.Config, Env: s.Env}}
}

// HasAxis reports whether this scenario is tagged for the given axis.
func (s Scenario) HasAxis(axis string) bool {
	for _, a := range s.Axis {
		if a == axis {
			return true
		}
	}
	return false
}

// LoadMatrix reads and validates one hook's matrix.
func LoadMatrix(path string) (*Matrix, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied dev tooling path
	if err != nil {
		return nil, fmt.Errorf("matrix: %w", err)
	}
	var m Matrix
	if err := yaml.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("matrix %s: %w", path, err)
	}
	m.Path = path
	if m.SchemaVersion != 1 {
		return nil, fmt.Errorf("matrix %s: schema_version %d, want 1", path, m.SchemaVersion)
	}
	if m.Hook == "" {
		return nil, fmt.Errorf("matrix %s: no hook name", path)
	}
	// A matrix with no scenarios is a hook with no corpus. It must never read
	// as "nothing failed" (INV-013) — the schema's own minItems: 1 says the
	// same thing, and this is the reader refusing to be the weaker of the two.
	if len(m.Scenarios) == 0 {
		return nil, fmt.Errorf("matrix %s declares zero scenarios — an empty matrix "+
			"measures nothing and must not read as coverage", path)
	}
	seen := map[string]bool{}
	for _, sc := range m.Scenarios {
		switch {
		case sc.ID == "":
			return nil, fmt.Errorf("matrix %s: a scenario has no id", path)
		case seen[sc.ID]:
			return nil, fmt.Errorf("matrix %s: duplicate scenario id %q — ids name recording "+
				"directories, so a duplicate silently overwrites a recording", path, sc.ID)
		case len(sc.Axis) == 0:
			return nil, fmt.Errorf("matrix %s: scenario %q tags no axis", path, sc.ID)
		}
		seen[sc.ID] = true
		for _, a := range sc.Axis {
			if !validAxis(a) {
				return nil, fmt.Errorf("matrix %s: scenario %q tags unknown axis %q (want one of %v)",
					path, sc.ID, a, Axes())
			}
		}
		if len(sc.Sequence) == 1 {
			return nil, fmt.Errorf("matrix %s: scenario %q has a one-step sequence — "+
				"an ordering scenario needs at least two steps to order", path, sc.ID)
		}
	}
	return &m, nil
}

func validAxis(a string) bool {
	for _, known := range Axes() {
		if a == known {
			return true
		}
	}
	return false
}

// Scenario resolves one scenario id.
func (m *Matrix) Scenario(id string) (Scenario, error) {
	for _, s := range m.Scenarios {
		if s.ID == id {
			return s, nil
		}
	}
	return Scenario{}, fmt.Errorf("matrix %s has no scenario %q", m.Path, id)
}

// resolvedInput is one step's inputs, already read off disk.
type resolvedInput struct {
	payload    []byte
	hasPayload bool
	configName string
	config     []byte
	env        map[string]*string
	delayMS    int
	// files are the scenario's extra seeded fixtures, destination -> bytes.
	files map[string][]byte
}

// InputDigest is the scenario_input_sha256 the recording pins.
//
// WHAT IT IS OVER, and why the exact recipe is written down here as well as in
// the schema: the canonicalized concatenation of every step's payload bytes
// (the empty string when payload is null), its resolved config bytes, and its
// env override set as sorted key=value lines with an explicit marker for a null
// (deliberately-unset) value.
//
// WHY IT EXISTS. Without it, editing a payload fixture leaves the recording's
// bytes untouched, and `dev hook-corpus diff` goes on comparing the Go
// replacement against an oracle recorded from input that is no longer on disk.
// That is a green diff that proves nothing, and it is invisible: nothing about
// the artifact looks stale. The digest turns it into a loud refusal.
//
// The env map's VALUES are hashed, not just its keys, and a null is hashed
// distinctly from an empty string — "unset" and "set to empty" are different
// environment-axis facts, and a hash that could not tell them apart would let
// one be edited into the other for free.
func (s Scenario) InputDigest(res *Resolver) (string, error) {
	inputs, err := s.resolveInputs(res)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	for i, in := range inputs {
		fmt.Fprintf(h, "step %d\n", i)
		if in.hasPayload {
			fmt.Fprintf(h, "payload %d\n", len(in.payload))
			h.Write(in.payload)
		} else {
			h.Write([]byte("payload null\n"))
		}
		fmt.Fprintf(h, "\nconfig %s %d\n", in.configName, len(in.config))
		h.Write(in.config)
		if len(in.files) > 0 {
			// Only when non-empty: a scenario that seeds nothing must hash
			// exactly as it did before this field existed, or adding the
			// field would invalidate every recording in the corpus.
			h.Write([]byte("\nfiles\n"))
			dests := make([]string, 0, len(in.files))
			for d := range in.files {
				dests = append(dests, d)
			}
			sort.Strings(dests)
			for _, d := range dests {
				fmt.Fprintf(h, "%s %d\n", d, len(in.files[d]))
				h.Write(in.files[d])
			}
		}
		h.Write([]byte("\nenv\n"))
		for _, line := range canonicalEnvLines(in.env) {
			h.Write([]byte(line))
			h.Write([]byte("\n"))
		}
		fmt.Fprintf(h, "delay_ms %d\n", in.delayMS)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// envUnsetMarker is the explicit token for a variable the scenario removes.
// It contains characters no environment value can contain, so it can never
// collide with a real value.
const envUnsetMarker = "\x00<unset>\x00"

func canonicalEnvLines(env map[string]*string) []string {
	keys := make([]string, 0, len(env))
	for k := range env {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if env[k] == nil {
			out = append(out, k+"="+envUnsetMarker)
			continue
		}
		out = append(out, k+"="+*env[k])
	}
	return out
}

func (s Scenario) resolveInputs(res *Resolver) ([]resolvedInput, error) {
	steps := s.Steps()
	out := make([]resolvedInput, 0, len(steps))
	for i, st := range steps {
		in := resolvedInput{env: st.Env, delayMS: st.DelayMs, configName: st.Config}
		if st.Payload != nil {
			p, err := res.Resolve(*st.Payload)
			if err != nil {
				return nil, fmt.Errorf("scenario %q step %d payload: %w", s.ID, i, err)
			}
			b, err := os.ReadFile(p) //nolint:gosec // path validated by Resolver
			if err != nil {
				return nil, fmt.Errorf("scenario %q step %d payload: %w", s.ID, i, err)
			}
			in.payload, in.hasPayload = b, true
		}
		cfgName := st.Config
		if cfgName == "" {
			cfgName = ConfigDefault
		}
		in.configName = cfgName
		b, err := res.ResolveConfig(cfgName)
		if err != nil {
			return nil, fmt.Errorf("scenario %q step %d config: %w", s.ID, i, err)
		}
		in.config = b
		if len(s.Files) > 0 {
			in.files = make(map[string][]byte, len(s.Files))
			for dest, src := range s.Files {
				if err := validSeedDest(dest); err != nil {
					return nil, fmt.Errorf("scenario %q files[%q]: %w", s.ID, dest, err)
				}
				fp, err := res.Resolve(src)
				if err != nil {
					return nil, fmt.Errorf("scenario %q files[%q]: %w", s.ID, dest, err)
				}
				fb, err := os.ReadFile(fp) //nolint:gosec // path validated by Resolver
				if err != nil {
					return nil, fmt.Errorf("scenario %q files[%q]: %w", s.ID, dest, err)
				}
				in.files[dest] = fb
			}
		}
		out = append(out, in)
	}
	return out, nil
}

// Resolver turns matrix-relative paths into real, in-bounds file paths.
//
// INV-006/INV-007: a matrix is a checked-in file, but it is still input to a
// program that reads whatever it names. Every resolved path must land inside
// the repo or the corpus tree; an absolute path, a `..` escape or a symlink out
// of the tree is refused by name rather than followed.
type Resolver struct {
	// RepoRoot anchors repo-relative fixture references such as
	// test/fixtures/hook-payloads/cwd-changed-basic.json.
	RepoRoot string
	// HookDir is <corpus-root>/<hook>, for corpus-local fixtures.
	HookDir string
	// DefaultConfig is the bytes of the sandbox's own .edikt/config.yaml,
	// which is what the matrix literal "default" means.
	DefaultConfig []byte
}

// Resolve maps one matrix path reference to an absolute path.
func (r *Resolver) Resolve(ref string) (string, error) {
	if ref == "" {
		return "", fmt.Errorf("empty path reference")
	}
	if filepath.IsAbs(ref) {
		return "", fmt.Errorf("%q is absolute — matrix references are relative so a corpus "+
			"is portable between checkouts", ref)
	}
	if strings.Contains(ref, "..") {
		return "", fmt.Errorf("%q contains '..' — refusing to resolve out of the corpus tree", ref)
	}
	roots := []string{r.RepoRoot, r.HookDir}
	var tried []string
	for _, root := range roots {
		if root == "" {
			continue
		}
		cand := filepath.Join(root, ref)
		tried = append(tried, cand)
		if _, err := os.Stat(cand); err != nil {
			continue
		}
		real, err := filepath.EvalSymlinks(cand)
		if err != nil {
			return "", fmt.Errorf("resolving %q: %w", ref, err)
		}
		realRoot, err := filepath.EvalSymlinks(root)
		if err != nil {
			return "", fmt.Errorf("resolving root %q: %w", root, err)
		}
		if !within(realRoot, real) {
			return "", fmt.Errorf("%q resolves to %s, outside %s — refusing", ref, real, realRoot)
		}
		return cand, nil
	}
	return "", fmt.Errorf("%q not found (tried: %s)", ref, strings.Join(tried, ", "))
}

// ResolveConfig returns the bytes of a scenario's config fixture.
func (r *Resolver) ResolveConfig(name string) ([]byte, error) {
	if name == ConfigDefault || name == "" {
		if r.DefaultConfig == nil {
			return nil, fmt.Errorf("config %q requested but the sandbox base tree carries no "+
				".edikt/config.yaml — 'default' has no meaning without one", ConfigDefault)
		}
		return r.DefaultConfig, nil
	}
	p, err := r.Resolve(name)
	if err != nil {
		return nil, err
	}
	return os.ReadFile(p) //nolint:gosec // path validated above
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// sameBytes is a small helper used by the differ's structural comparison.
func sameBytes(a, b []byte) bool { return bytes.Equal(a, b) }

// validSeedDest refuses a seed destination that could write outside the
// sandbox project. A matrix is checked in, but it is still input to a program
// that writes whatever it names (INV-007).
func validSeedDest(dest string) error {
	if dest == "" {
		return errors.New("empty destination")
	}
	if filepath.IsAbs(dest) {
		return errors.New("absolute destination")
	}
	clean := filepath.ToSlash(filepath.Clean(dest))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return errors.New("destination escapes the sandbox project")
	}
	return nil
}
