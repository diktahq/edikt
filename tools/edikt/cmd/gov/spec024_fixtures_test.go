package gov

// spec024_fixtures_test.go — SPEC-024 phase 1, the `cmd/gov` share.
//
// ONE fixture group lives here: walk-root-symlink. WS-04's call site is
// cmd/gov's schema-check (schema_corpus.go), and it is THE MOST INSTRUCTIVE
// ROW IN THE FILE — schema-check already fails closed and says UNMEASURED,
// which is correct, while the accompanying text says "no sidecars found",
// which is false. A correct verdict wrapped around a false statement of fact,
// and any test asserting only the exit status passes it today.
//
// WS-01, WS-02 and WS-05 are reference measurements over raw filepath.WalkDir
// ("reference measurement" and "any WalkDir root" in the fixture), so they are
// measured identically here and in internal/pathsproposal. That is deliberate:
// they are the denominator both call sites are wrong against, and a reference
// that only exists next to one of them is worth less.
//
// WS-03 belongs to internal/pathsproposal.EnumerateFiles and is declared out of
// scope here rather than measured twice.
//
// PHASE 1 CHANGES NO PRODUCTION CODE.

import (
	"fmt"
	"io"
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

// spec024WalkTree builds TREE-01's walk subset: real/a.txt, real/sub/b.txt and
// alias -> real. Four entries under the real root, one under the symlink.
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

// spec024WalkEntries returns every path filepath.WalkDir visits, and the error
// the walk ended with. THE ASSERTION MUST BE A COUNT: a test that only checks
// "walk returned no error" passes on the real root and on the symlink alike.
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

// spec024RunSchemaCheck invokes schema-check's own RunE in-process against
// root and returns (stderr, error).
//
// In-process rather than through the built binary on purpose: ADR-084's front
// door refuses `gov` when EDIKT_INVOKER is undeclared and stdin is not a
// terminal, so a subprocess would measure the wall rather than the walk. The
// subject here is what schema-check SAYS about a root it could not traverse.
func spec024RunSchemaCheck(t *testing.T, root string) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	saved := os.Stderr
	os.Stderr = w
	runErr := schemaCheckCmd.RunE(schemaCheckCmd, []string{root})
	os.Stderr = saved
	if err := w.Close(); err != nil {
		t.Fatalf("closing capture pipe: %v", err)
	}
	out, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("reading capture pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("closing capture pipe: %v", err)
	}
	return string(out), runErr
}

func spec024Measure(t *testing.T, tbl *fx.Table) (fx.Coverage, []string) {
	t.Helper()
	cov := fx.Coverage{
		Package:  "cmd/gov",
		Groups:   []string{"walk-root-symlink"},
		Measured: map[string]fx.Agreement{},
		Detail:   map[string]string{},
		Skipped:  map[string]string{},
		Licensed: map[string]string{},
		OutOfScope: map[string]string{
			"WS-03": "internal/pathsproposal.EnumerateFiles owns this call site and measures it there",
		},
	}
	var stale []string

	real, alias := spec024WalkTree(t)

	// WS-01 — the control. Without it, WS-02's `1` is a number with nothing
	// to be wrong against.
	{
		seen, err := spec024WalkEntries(real)
		agree := fx.AgreeNo
		if err == nil && len(seen) == 4 {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-01"] = agree
		cov.Detail["WS-01"] = fmt.Sprintf("WalkDir(real) visited %d entries, err=%v", len(seen), err)
	}

	// WS-02 — THE MECHANISM: filepath.WalkDir lstats its root. A symlink is
	// not a directory, so the walk visits the link itself as a single
	// non-directory entry and stops. NO ERROR. Exit 0.
	{
		seen, err := spec024WalkEntries(alias)
		agree := fx.AgreeNo
		if err == nil && len(seen) == 4 {
			agree = fx.AgreeYes
		}
		cov.Measured["WS-02"] = agree
		cov.Detail["WS-02"] = fmt.Sprintf("WalkDir(alias) visited %d entries (%v), err=%v", len(seen), seen, err)
		if len(seen) != 1 || err != nil {
			stale = append(stale, fmt.Sprintf(
				"WS-02 records today {files_found: 1, error: null}; the walk visited %d entries with err=%v — the fixture's record of CURRENT behaviour is stale",
				len(seen), err))
		}
	}

	// WS-04 — schema-check over a symlinked root that DOES hold sidecars.
	{
		corpusRoot, err := filepath.EvalSymlinks(t.TempDir())
		if err != nil {
			t.Fatalf("resolving temp root: %v", err)
		}
		corpusReal := filepath.Join(corpusRoot, "repo", "docs", "architecture", "decisions")
		if err := os.MkdirAll(corpusReal, 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(filepath.Join(corpusReal, "ADR-100-test.edikt.yaml"),
			[]byte(validCorpusSidecar), 0o644); err != nil {
			t.Fatalf("write sidecar: %v", err)
		}
		corpusAlias := filepath.Join(corpusRoot, "alias")
		if err := os.Symlink("repo", corpusAlias); err != nil {
			t.Fatalf("symlink: %v", err)
		}
		// GROUND TRUTH. "There are sidecars, the walk could not reach them"
		// is the whole claim; assert the first half rather than assume it.
		ground, _ := spec024WalkEntries(filepath.Join(corpusRoot, "repo"))
		sidecars := 0
		for _, p := range ground {
			if strings.HasSuffix(p, ".edikt.yaml") {
				sidecars++
			}
		}
		if sidecars == 0 {
			t.Fatalf("WS-04 fixture is not set up: no *.edikt.yaml under the real corpus root")
		}

		stderr, runErr := spec024RunSchemaCheck(t, corpusAlias)
		text := stderr
		if runErr != nil {
			text += "\n" + runErr.Error()
		}
		verdictOK := runErr != nil && strings.Contains(stderr, "UNMEASURED")
		// The reason must name the unfollowed root AND must not assert an
		// absence of sidecars. Fail-closed is necessary and not sufficient.
		textOK := !strings.Contains(text, "no sidecars found")
		agree := fx.AgreeNo
		switch {
		case verdictOK && textOK:
			agree = fx.AgreeYes
		case verdictOK != textOK:
			agree = fx.AgreePartial
		}
		cov.Measured["WS-04"] = agree
		cov.Detail["WS-04"] = fmt.Sprintf(
			"%d sidecar(s) exist under the real root; schema-check on the alias: verdict UNMEASURED=%v, text free of \"no sidecars found\"=%v; said: %q / err %v",
			sidecars, verdictOK, textOK, strings.TrimSpace(stderr), runErr)
	}

	// WS-05 — pinned so the SR-011 fix does not collapse two different
	// failures into one message. "I could not follow this symlink" and "this
	// path is not there" are different operator actions.
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

// TestFixtures_ExpectationSetTwoSided — AC-1.4's assertion, applied to this
// package's group as well as to the two the criterion names, because a group
// whose expectations are never checked in both directions is a group where a
// defect can be fixed early and silently.
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
