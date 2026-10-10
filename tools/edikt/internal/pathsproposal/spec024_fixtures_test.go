package pathsproposal

// spec024_fixtures_test.go — SPEC-024 phase 1, the `internal/pathsproposal`
// share.
//
// ONE fixture group lives here: walk-root-symlink. WS-03's call site is
// EnumerateFiles, whose own comment says an unreadable subtree is "a hole in
// the instrument, not an absence of files" and returns the error. It is
// exactly right about a SUBTREE and blind about the ROOT, where WalkDir hands
// it no error at all — so the approval ceremony validates a human-proposed
// glob against an empty universe and reports "matches none of the 0 files".
//
// WS-01, WS-02 and WS-05 are reference measurements over raw filepath.WalkDir,
// measured here and in cmd/gov both. WS-04 belongs to cmd/gov's schema-check
// and is declared out of scope here rather than measured twice.
//
// PHASE 1 CHANGES NO PRODUCTION CODE.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	fx "github.com/diktahq/edikt/tools/edikt/internal/spec024fixtures"
)

// spec024GroupCounts — see the note in cmd/spec024_fixtures_test.go on why the
// literal is repeated per package rather than shared.
var spec024GroupCounts = map[string]int{
	"version-ordering":               12,
	"version-ordering-consumers":     8,
	"path-identity":                  7,
	"walk-root-symlink":              5,
	"empty-subject-expected-outputs": 3,
	"path-fixture-tree":              1,
	"config-path-resolution":         8,
	"managed-block-and-drift":        5,
	"criterion-status-shape":         3,
}

// spec024GoReachableControls — the whole Go-reachable control list, re-written
// here so each package independently re-checks that every row it names really
// does carry agrees_today: true. It GROWS as phases land: a defect row becomes
// a control the moment the requirement that owns it flips it, and the eight
// added below were flipped by SR-002 (plan phase 2, internal/versioncmp). See
// the note in cmd/spec024_fixtures_test.go.
var spec024GoReachableControls = []string{
	"VO-01", "VO-05", "VO-07", "VO-08", "VO-09",
	"PC-02", "PC-07",
	"CR-01", "CR-04", "CR-05", "CR-06",
	"WS-01", "WS-05",
	"PI-06",

	// Flipped by SR-002, plan phase 2.
	"VO-02", "VO-03", "VO-04", "VO-06", "VO-10", "VO-11",
	"PC-01", "PC-03",

	// Flipped by SR-011: EnumerateFiles refuses a symlinked root by name.
	"WS-03",
}

var spec024NonBooleanRows = map[string]fx.Agreement{
	"PI-04": fx.AgreePartial,
	"WS-04": fx.AgreePartial,
}

func spec024Load(t *testing.T) *fx.Table {
	t.Helper()
	tbl, err := fx.Load()
	if err != nil {
		t.Fatalf("loading the SPEC-024 fixture table: %v", err)
	}
	return tbl
}

func spec024WalkTree(t *testing.T) (real, alias string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp root: %v", err)
	}
	real = filepath.Join(root, "real")
	if err := os.MkdirAll(filepath.Join(real, "sub"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for _, f := range []string{"a.txt", "sub/b.txt"} {
		if err := os.WriteFile(filepath.Join(real, filepath.FromSlash(f)), []byte("x\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", f, err)
		}
	}
	alias = filepath.Join(root, "alias")
	if err := os.Symlink("real", alias); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	return real, alias
}

func spec024WalkEntries(root string) ([]string, error) {
	var seen []string
	err := filepath.WalkDir(root, func(p string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		seen = append(seen, p)
		return nil
	})
	return seen, err
}

func spec024Measure(t *testing.T, tbl *fx.Table) (fx.Coverage, []string) {
	t.Helper()
	cov := fx.Coverage{
		Package:  "internal/pathsproposal",
		Groups:   []string{"walk-root-symlink"},
		Measured: map[string]fx.Agreement{},
		Detail:   map[string]string{},
		Skipped:  map[string]string{},
		Licensed: map[string]string{},
		OutOfScope: map[string]string{
			"WS-04": "cmd/gov schema-check owns this call site and measures it there",
		},
	}
	var stale []string

	real, alias := spec024WalkTree(t)

	// WS-01 — the control the symlink count is wrong against.
	{
		seen, err := spec024WalkEntries(real)
		agree := fx.AgreeNo
		if err == nil && len(seen) == 4 {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-01"] = agree
		cov.Detail["WS-01"] = fmt.Sprintf("WalkDir(real) visited %d entries, err=%v", len(seen), err)
	}

	// WS-02 — the mechanism, measured against the real path IN THE SAME TEST
	// so the expected number is a comparison rather than a constant that rots
	// on the next file added.
	{
		seen, err := spec024WalkEntries(alias)
		control, cerr := spec024WalkEntries(real)
		agree := fx.AgreeNo
		if err == nil && cerr == nil && len(seen) == len(control) {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-02"] = agree
		cov.Detail["WS-02"] = fmt.Sprintf(
			"WalkDir(alias) visited %d entries against %d on the real path, err=%v", len(seen), len(control), err)
		if len(seen) != 1 || err != nil {
			stale = append(stale, fmt.Sprintf(
				"WS-02 records today {files_found: 1, error: null}; the walk visited %d entries with err=%v — the fixture's record of CURRENT behaviour is stale",
				len(seen), err))
		}
	}

	// WS-03 — EnumerateFiles on a symlinked root refuses it by name (SR-011),
	// rather than returning the empty walk WalkDir produces for a link.
	{
		files, err := EnumerateFiles(alias)
		control, cerr := EnumerateFiles(real)
		agree := fx.AgreeNo
		if err != nil && strings.Contains(err.Error(), alias) {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-03"] = agree
		cov.Detail["WS-03"] = fmt.Sprintf(
			"EnumerateFiles(alias) = %v, err=%v; EnumerateFiles(real) = %d file(s), err=%v",
			files, err, len(control), cerr)
		if len(files) != 0 || err == nil || !strings.Contains(err.Error(), alias) {
			stale = append(stale, fmt.Sprintf(
				"WS-03 records today {files: null, error naming the symlinked root}; EnumerateFiles returned %d file(s) with err=%v",
				len(files), err))
		}
		if len(control) == 0 || cerr != nil {
			t.Fatalf("WS-03 fixture is not set up: EnumerateFiles on the real root returned %d file(s), err=%v",
				len(control), cerr)
		}
	}

	// WS-05 — a missing root and a symlinked root must stay distinguishable.
	{
		missing := filepath.Join(real, "definitely-not-here")
		seen, err := spec024WalkEntries(missing)
		agree := fx.AgreeNo
		if err != nil && strings.Contains(err.Error(), missing) {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-05"] = agree
		cov.Detail["WS-05"] = fmt.Sprintf("WalkDir(missing) visited %d entries, err=%v", len(seen), err)
	}

	return cov, stale
}

// ─── the four gates ──────────────────────────────────────────────────────────

// TestFixtures_RowCount — AC-1.2.
func TestFixtures_RowCount(t *testing.T) {
	tbl := spec024Load(t)
	for _, msg := range tbl.CheckCounts(spec024GroupCounts) {
		t.Error(msg)
	}
}

// TestFixtures_RowIdCoverage — AC-1.3.
func TestFixtures_RowIdCoverage(t *testing.T) {
	tbl := spec024Load(t)
	cov, _ := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckCoverage(cov) {
		t.Error(msg)
	}
	got := map[string]fx.Agreement{}
	for _, row := range tbl.NonBooleanRows() {
		got[row.ID] = row.Agreement
	}
	if len(got) != len(spec024NonBooleanRows) {
		t.Errorf("the table carries %d non-boolean agrees_today row(s) %v; exactly %d are expected: %v",
			len(got), got, len(spec024NonBooleanRows), spec024NonBooleanRows)
	}
	for id, want := range spec024NonBooleanRows {
		if got[id] != want {
			t.Errorf("row %s must carry agrees_today: %q, table has %q", id, want, got[id])
		}
	}
}

// TestFixtures_ControlRows — AC-1.1.
func TestFixtures_ControlRows(t *testing.T) {
	tbl := spec024Load(t)
	cov, _ := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckControls(cov, spec024GoReachableControls) {
		t.Error(msg)
	}
}

// TestFixtures_ExpectationSetTwoSided — AC-1.4.
func TestFixtures_ExpectationSetTwoSided(t *testing.T) {
	tbl := spec024Load(t)
	cov, stale := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckExpectations(cov) {
		t.Error(msg)
	}
	for _, msg := range stale {
		t.Error(msg)
	}
}
