package cmd

// Fixtures and capture helpers for the doctor contract-line tests.
//
// This file uses the standard library and the long-standing test helpers
// (gapProject, write) only, so the same file compiles unchanged against a
// v0.9.0 checkout. That is how the golden was produced: the capture test at
// the bottom was run there, and its output is testdata/doctor-missing-sidecar-validated.golden.

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// contractEnvVars are the variables that decide which extractor and which
// profile doctor can see. Every test that reaches a path printing a compile
// remedy isolates all of them.
var contractEnvVars = []string{"HOME", "EDIKT_HOME", "EDIKT_ROOT", "CLAUDE_HOME", "CLAUDE_CONFIG_DIR"}

// isolateDoctorEnv points the five variables at empty temp dirs so nothing
// reads the developer's own profile. When claudeHomeSet is false CLAUDE_HOME
// and CLAUDE_CONFIG_DIR are unset, so the profile resolves to $HOME/.claude.
// It returns the directory the profile's agents/ lives in.
func isolateDoctorEnv(t *testing.T, claudeHomeSet bool) (profileDir string) {
	t.Helper()
	base := t.TempDir()
	for _, k := range contractEnvVars {
		t.Setenv(k, filepath.Join(base, strings.ToLower(k)))
	}
	if !claudeHomeSet {
		os.Unsetenv("CLAUDE_HOME")
		os.Unsetenv("CLAUDE_CONFIG_DIR")
		return filepath.Join(base, "home", ".claude")
	}
	return filepath.Join(base, "claude_home")
}

// installValidatedExtractor puts an extractor at a validated version (v4) in
// the project, so a test that is not about the contract resolves a fixed,
// known state instead of whatever the machine has installed.
func installValidatedExtractor(t *testing.T, root string) {
	t.Helper()
	writeExtractorAt(t, filepath.Join(root, ".claude"), 4)
}

// writeExtractorAt writes sidecar-extractor.md under dir/agents. version 0
// writes one that declares no prompt_version.
func writeExtractorAt(t *testing.T, dir string, version int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "agents"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "---\nname: sidecar-extractor\nmodel: claude-opus-5\n"
	if version > 0 {
		body += "prompt_version: " + strconv.Itoa(version) + "\n"
	}
	body += "---\n\nbody\n"
	if err := os.WriteFile(filepath.Join(dir, "agents", "sidecar-extractor.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

const (
	fxADR = "# %s\n\nHooks must emit JSON.\n\n**Status:** Accepted\n"
	fxINV = "---\nstatus: active\n---\n\n# INV-hooks\n\n## Statement\n\nSomething MUST hold.\n"
	fxGL  = "# GUIDE-style\n\nPrefer small functions.\n"
	// A proposed ADR with two considered options: the sidecar path skips it
	// and the delivery-gaps path ignores it, so the rejected-options check's
	// note is the only remedy doctor prints for it.
	fxProposed = "---\nstatus: proposed\n---\n\n# ADR-options: Test\n\n**Status:** Proposed\n\n## Considered Options\n\n### A. First\n\nx\n\n### B. Second\n\ny\n\n## Decision\n\nz\n"
)

func adrBody(id string) string { return strings.Replace(fxADR, "%s", id, 1) }

// contractFixtures are the projects every path is run over. wantRemedies is
// the number of compile remedy lines the validated output holds for the
// sidecar path, the delivery-gaps path and the rejected-options path, so a
// golden that silently lost a finding fails loudly.
type contractFixture struct {
	name         string
	files        map[string]string
	wantRemedies [3]int
}

var contractFixtures = []contractFixture{
	{"one-adr", map[string]string{
		"docs/architecture/decisions/ADR-terminology.md": adrBody("ADR-terminology"),
	}, [3]int{1, 1, 0}},
	{"three-adrs", map[string]string{
		"docs/architecture/decisions/ADR-alpha.md": adrBody("ADR-alpha"),
		"docs/architecture/decisions/ADR-beta.md":  adrBody("ADR-beta"),
		"docs/architecture/decisions/ADR-gamma.md": adrBody("ADR-gamma"),
	}, [3]int{3, 1, 0}},
	{"one-invariant", map[string]string{
		"docs/architecture/invariants/INV-hooks.md": fxINV,
	}, [3]int{1, 1, 0}},
	{"one-guideline", map[string]string{
		"docs/guidelines/GUIDE-style.md": fxGL,
	}, [3]int{1, 1, 0}},
	{"mix", map[string]string{
		"docs/architecture/decisions/ADR-alpha.md":  adrBody("ADR-alpha"),
		"docs/architecture/decisions/ADR-beta.md":   adrBody("ADR-beta"),
		"docs/architecture/invariants/INV-hooks.md": fxINV,
		"docs/guidelines/GUIDE-style.md":            fxGL,
	}, [3]int{4, 1, 0}},
	{"proposed-with-options", map[string]string{
		"docs/architecture/decisions/ADR-options.md": fxProposed,
	}, [3]int{0, 0, 1}},
}

// newGapProject is gapProject without the environment isolation: the
// baseline compiled project the fixtures are laid over.
func newGapProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{
		".edikt", "docs/architecture/decisions", "docs/architecture/invariants",
		"docs/guidelines", ".claude/rules/governance",
	} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write(t, root, ".edikt/config.yaml", "paths:\n  decisions: docs/architecture/decisions\n  invariants: docs/architecture/invariants\n  guidelines: docs/guidelines\n")
	write(t, root, ".claude/rules/governance.md", "# Governance\n\n- Something MUST hold. (ref: INV-001)\n")
	write(t, root, ".claude/rules/governance/delivery-map.yaml",
		"schema_version: 1\nglobs:\n  \"src/**/*.go\":\n    - id: \"INV-001:d01\"\n      grade: must\n")
	return root
}

// buildContractFixture writes the fixture's files into a fresh project.
func buildContractFixture(t *testing.T, f contractFixture) string {
	t.Helper()
	root := newGapProject(t)
	for p, body := range f.files {
		write(t, root, p, body)
	}
	return root
}

// capturedPaths are the three doctor paths that print a compile remedy for a
// missing sidecar, rendered in full for one project root.
type capturedPaths struct {
	sidecar, gaps, rejected string
}

func captureDoctorPaths(root string) capturedPaths {
	var a, b, c bytes.Buffer
	runSidecarChecks(root, &a)
	runDeliveryGapsCheck(root, &b)
	runRejectedOptionsCheck(root, &c)
	norm := func(s string) string { return strings.ReplaceAll(s, root, "<ROOT>") }
	return capturedPaths{norm(a.String()), norm(b.String()), norm(c.String())}
}

// The golden is a sequence of sections:
//
//	=== <fixture> | <path>
//	<the path's complete output>
const goldenPathSidecar, goldenPathGaps, goldenPathRejected = "sidecar", "delivery-gaps", "rejected-options"

func goldenSectionHeader(fixture, path string) string { return "=== " + fixture + " | " + path + "\n" }

func renderGolden(provenance string, outs map[string]capturedPaths) string {
	var sb strings.Builder
	sb.WriteString(provenance + "\n")
	for _, f := range contractFixtures {
		o := outs[f.name]
		sb.WriteString(goldenSectionHeader(f.name, goldenPathSidecar) + o.sidecar)
		sb.WriteString(goldenSectionHeader(f.name, goldenPathGaps) + o.gaps)
		sb.WriteString(goldenSectionHeader(f.name, goldenPathRejected) + o.rejected)
	}
	return sb.String()
}

// parseGolden returns the provenance line and the sections keyed by their
// header line (without the newline).
func parseGolden(raw string) (provenance string, sections map[string]string) {
	sections = map[string]string{}
	lines := strings.SplitAfter(raw, "\n")
	provenance = strings.TrimRight(lines[0], "\n")
	var key string
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "=== ") {
			key = strings.TrimRight(l, "\n")
			sections[key] = ""
			continue
		}
		sections[key] += l
	}
	return provenance, sections
}

// TestCaptureContractGolden regenerates the golden. It writes nothing unless
// EDIKT_CAPTURE_GOLDEN names the file to write, and it is meant to be run
// against a v0.9.0 checkout, never against the code under test.
func TestCaptureContractGolden(t *testing.T) {
	out := os.Getenv("EDIKT_CAPTURE_GOLDEN")
	if out == "" {
		t.Skip("set EDIKT_CAPTURE_GOLDEN=<file> (and EDIKT_GOLDEN_PROVENANCE) to capture")
	}
	profile := isolateDoctorEnv(t, true)
	writeExtractorAt(t, profile, 4)
	outs := map[string]capturedPaths{}
	for _, f := range contractFixtures {
		root := buildContractFixture(t, f)
		outs[f.name] = captureDoctorPaths(root)
	}
	if err := os.WriteFile(out, []byte(renderGolden(os.Getenv("EDIKT_GOLDEN_PROVENANCE"), outs)), 0o644); err != nil {
		t.Fatal(err)
	}
}
