package pathsproposal

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"
)

func newReader(b []byte) io.Reader { return bytes.NewReader(b) }

// repoRoot resolves the edikt repo root from this package's directory.
//
// The fixtures live at the repo's test/fixtures/ tree rather than inside the
// Go module on purpose: they are the byte-for-byte shape of a real
// .edikt/state/pending-paths/<id>.yaml file, and a copy inside the module
// would be free to drift from the shape the binary actually reads.
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// internal/pathsproposal -> internal -> edikt -> tools -> repo root
	root := filepath.Clean(filepath.Join(wd, "..", "..", "..", ".."))
	if _, err := os.Stat(filepath.Join(root, "test", "fixtures", "paths-proposals")); err != nil {
		t.Fatalf("paths-proposal fixtures not found under %s: %v "+
			"(this test asserts against on-disk fixtures; a missing tree is UNMEASURED, not a pass)", root, err)
	}
	return root
}

type fixtureFile struct {
	ID            string     `yaml:"id"`
	SidecarPath   string     `yaml:"sidecar_path"`
	ProposedAt    string     `yaml:"proposed_at"`
	ProposedPaths []Proposal `yaml:"proposed_paths"`
}

func loadFixture(t *testing.T, rel string) fixtureFile {
	t.Helper()
	root := repoRoot(t)
	body, err := os.ReadFile(filepath.Join(root, rel))
	if err != nil {
		t.Fatalf("read fixture %s: %v", rel, err)
	}
	var f fixtureFile
	dec := yaml.NewDecoder(newReader(body))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil {
		t.Fatalf("decode fixture %s: %v", rel, err)
	}
	if len(f.ProposedPaths) == 0 {
		t.Fatalf("fixture %s carries zero proposals — it cannot test anything", rel)
	}
	return f
}

// TestProposedPathsValidFixtureAccepted is the positive half of AC-1.8: a
// well-formed proposal, validated against the REAL repository tree, is
// accepted with every entry surviving.
//
// It validates against the real tree rather than a synthetic file list
// deliberately — the globs in the fixture name packages that exist, and a
// synthetic list would let the fixture stay green after those packages were
// renamed or removed.
func TestProposedPathsValidFixtureAccepted(t *testing.T) {
	root := repoRoot(t)
	f := loadFixture(t, "test/fixtures/paths-proposals/valid/adr-028-phaseb.yaml")

	res, err := ValidateAgainstRoot(f.ProposedPaths, root)
	if err != nil {
		t.Fatalf("ValidateAgainstRoot: %v", err)
	}
	if res.Files == 0 {
		t.Fatal("enumerated 0 files under the repo root — the match rule was UNMEASURED, not satisfied")
	}
	if !res.OK() {
		for _, fi := range res.Findings {
			t.Errorf("unexpected finding: %s", fi)
		}
		t.Fatalf("valid fixture rejected: %d/%d accepted", res.Accepted, res.Checked)
	}
	if res.Accepted != len(f.ProposedPaths) {
		t.Fatalf("accepted %d of %d proposals", res.Accepted, len(f.ProposedPaths))
	}
	t.Logf("accepted %d/%d proposals against %d enumerated files", res.Accepted, res.Checked, res.Files)
}

// TestProposedPathsInvalidFixtureRejected is the negative half of AC-1.8, and
// it pins WHICH rule fired for WHICH entry rather than only that the fixture
// failed. A validator that rejected all three entries for the wrong reason
// would pass a bare "not OK" assertion while catching none of the three
// classes the criterion names.
func TestProposedPathsInvalidFixtureRejected(t *testing.T) {
	root := repoRoot(t)
	f := loadFixture(t, "test/fixtures/paths-proposals/invalid/three-failure-classes.yaml")

	res, err := ValidateAgainstRoot(f.ProposedPaths, root)
	if err != nil {
		t.Fatalf("ValidateAgainstRoot: %v", err)
	}
	if res.OK() {
		t.Fatal("invalid fixture accepted — every one of the three failure classes went undetected")
	}

	byIndex := map[int]map[string]bool{}
	for _, fi := range res.Findings {
		if byIndex[fi.Index] == nil {
			byIndex[fi.Index] = map[string]bool{}
		}
		byIndex[fi.Index][fi.Rule] = true
	}

	want := []struct {
		index int
		rule  string
		why   string
	}{
		{0, "catch-all", "**/*.go is anchored nowhere and scopes the whole repo"},
		{1, "no-match", "the glob is well-formed but names a package that does not exist"},
		{2, "evidence", `"n/a" cites nothing a reviewer could check`},
	}
	for _, w := range want {
		if !byIndex[w.index][w.rule] {
			t.Errorf("proposed_paths[%d]: expected rule %q to fire (%s); got rules %v",
				w.index, w.rule, w.why, keysOf(byIndex[w.index]))
		}
	}

	// Isolation: entry [2] is a real, anchored, matching glob. Its ONLY defect
	// is the evidence. If the match rule also fires on it, the validator is
	// rejecting things for reasons that are not there, and the three-class
	// discrimination above is an accident.
	if byIndex[2]["no-match"] {
		t.Error("proposed_paths[2]: no-match fired on a glob that does match real files — the validator is over-rejecting")
	}
	if res.Accepted != 0 {
		t.Errorf("accepted %d proposals from a fixture where every entry is defective", res.Accepted)
	}
	t.Logf("rejected %d/%d proposals with %d findings against %d files",
		res.Checked-res.Accepted, res.Checked, len(res.Findings), res.Files)
}

// TestProposedPathsEmptyTreeIsUnmeasured is the INV-013 case.
//
// With no files enumerated, every glob "matches nothing" — for a reason that
// is entirely about the file set and says nothing about the proposal. That run
// must refuse to produce a verdict rather than report a confident rejection
// that a caller could read as "the proposals were checked and found wanting".
func TestProposedPathsEmptyTreeIsUnmeasured(t *testing.T) {
	props := []Proposal{{
		Glob:     "tools/edikt/internal/phaseb/**/*.go",
		Evidence: "a perfectly good citation that names a real package",
	}}

	res, err := Validate(props, nil)
	if err == nil {
		t.Fatalf("empty file set produced a verdict (%d/%d accepted) instead of UNMEASURED",
			res.Accepted, res.Checked)
	}
	if res.Checked != 0 || res.Files != 0 {
		t.Errorf("UNMEASURED result should carry no counts, got Checked=%d Files=%d", res.Checked, res.Files)
	}
	t.Logf("refused as expected: %v", err)
}

// TestProposedPathsZeroProposalsIsMeasuredZero is the other half of the
// INV-013 boundary: a sidecar with no proposals is a measured zero, not an
// error and not a silence. The denominator must come back as 0 with the file
// count still populated, so a caller can say "0/0 checked against N files".
func TestProposedPathsZeroProposalsIsMeasuredZero(t *testing.T) {
	res, err := Validate(nil, []string{"tools/edikt/main.go"})
	if err != nil {
		t.Fatalf("zero proposals should not error: %v", err)
	}
	if !res.OK() {
		t.Fatalf("zero proposals produced findings: %v", res.Findings)
	}
	if res.Checked != 0 || res.Accepted != 0 {
		t.Errorf("expected 0/0, got %d/%d", res.Accepted, res.Checked)
	}
	if res.Files != 1 {
		t.Errorf("expected the file denominator to survive, got %d", res.Files)
	}
}

func keysOf(m map[string]bool) []string {
	out := []string{}
	for k := range m {
		out = append(out, k)
	}
	return out
}
