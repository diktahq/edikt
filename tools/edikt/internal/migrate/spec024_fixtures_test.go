package migrate_test

// spec024_fixtures_test.go — SPEC-024 phase 1, the `internal/migrate` share.
//
// ONE fixture group lives here: config-path-resolution. "Where does this
// project keep its governance artifacts" is answered by TWO live consumers —
// govrun.LoadConfig, which parses YAML, and migrate.ResolveArtifactDirs, which
// hand-scans lines — and CR-01 through CR-08 run BOTH against ONE written
// config.yaml in a temp dir, which is how the divergence was measured.
//
// WHY AN EXTERNAL TEST PACKAGE. internal/govrun imports internal/migrate, so an
// in-package test file here could not import govrun without a cycle. Both
// consumers have to run against the same file for the comparison to mean
// anything, so the test lives in migrate_test.
//
// PHASE 1 CHANGES NO PRODUCTION CODE.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
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

// spec024Project writes one config.yaml (or none, when literal is empty) into
// a temp root and returns the root.
func spec024Project(t *testing.T, literal string) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp root: %v", err)
	}
	if literal == "" {
		return root
	}
	dir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.yaml"), []byte(literal), 0o644); err != nil {
		t.Fatalf("write config.yaml: %v", err)
	}
	return root
}

// spec024Answers is what the two consumers say about one key of one project.
type spec024Answers struct {
	govrun string // repo-RELATIVE, as govrun returns it
	// migrateAbs is what migrate returns: an ABSOLUTE path (CR-08's subject).
	migrateAbs string
	// migrateRel is migrateAbs made comparable to govrun's shape, so the
	// other seven rows compare directories rather than re-discovering CR-08.
	migrateRel string
}

func spec024Resolve(t *testing.T, root, key string) spec024Answers {
	t.Helper()
	cfg, err := govrun.LoadConfig(root)
	if err != nil {
		t.Fatalf("govrun.LoadConfig(%s): %v", root, err)
	}
	dirs := migrate.ResolveArtifactDirs(root)
	var g, m string
	switch key {
	case "decisions":
		g, m = cfg.Paths.Decisions, dirs.Decisions
	case "invariants":
		g, m = cfg.Paths.Invariants, dirs.Invariants
	case "guidelines":
		g, m = cfg.Paths.Guidelines, dirs.Guidelines
	case "specs":
		g, m = cfg.Paths.Specs, dirs.Specs
	case "prds":
		g, m = cfg.Paths.Prds, dirs.Prds
	default:
		t.Fatalf("spec024Resolve: unknown key %q", key)
	}
	rel := m
	if r, err := filepath.Rel(root, m); err == nil {
		rel = filepath.ToSlash(r)
	}
	return spec024Answers{govrun: g, migrateAbs: m, migrateRel: rel}
}

const (
	spec024ConfigBaseDocs = "base: docs\npaths: {}\n"
	spec024ConfigBaseDoc  = "base: documentation\npaths: {}\n"
	spec024ConfigSpecsOvr = "base: documentation\npaths:\n  specs: docs/product/specs\n"
	spec024ConfigDecisOvr = "base: docs\npaths:\n  decisions: governance/adrs\n"
	// CR-07's config_literal, verbatim: a double-quoted YAML scalar is the
	// same value as the bare one, and both resolvers must agree.
	spec024ConfigQuoted = "base: documentation\npaths:\n  decisions: \"governance/adrs\"\n"
)

func spec024Measure(t *testing.T, tbl *fx.Table) (fx.Coverage, []string) {
	t.Helper()
	cov := fx.Coverage{
		Package:    "internal/migrate",
		Groups:     []string{"config-path-resolution"},
		Measured:   map[string]fx.Agreement{},
		Detail:     map[string]string{},
		Skipped:    map[string]string{},
		Licensed:   map[string]string{},
		OutOfScope: map[string]string{},
	}
	var stale []string

	intended := func(id string) string {
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		s, _ := row.Fields["intended"].(string)
		return s
	}
	// checkStale compares the measured answers against the row's recorded
	// `today:` block. Recorded values containing "<" are templated prose
	// rather than a literal path and are asserted by the row's own case.
	checkStale := func(id string, a spec024Answers) {
		row, ok := tbl.Lookup(id)
		if !ok {
			return
		}
		today, ok := row.Fields["today"].(map[string]any)
		if !ok {
			return
		}
		for who, got := range map[string]string{"govrun": a.govrun, "migrate": a.migrateRel} {
			want, ok := today[who].(string)
			if !ok || strings.Contains(want, "<") {
				continue
			}
			if want != got {
				stale = append(stale, fmt.Sprintf(
					"%s records today.%s = %q; measured %q — the fixture's record of CURRENT behaviour is stale",
					id, who, want, got))
			}
		}
	}

	// Seven rows resolve ONE key and agree only when BOTH consumers return
	// the intended directory. "Both agree" is not the property on its own —
	// CR-01 and CR-06 agree today and are still two resolvers — but a row
	// whose intended value only one consumer reaches is a divergence, and
	// that is what these measure.
	for _, tc := range []struct{ id, config, key string }{
		{"CR-01", spec024ConfigBaseDocs, "decisions"},
		{"CR-02", spec024ConfigBaseDoc, "decisions"},
		{"CR-03", spec024ConfigBaseDoc, "specs"},
		{"CR-04", spec024ConfigSpecsOvr, "specs"},
		{"CR-05", spec024ConfigDecisOvr, "decisions"},
		{"CR-07", spec024ConfigQuoted, "decisions"},
	} {
		root := spec024Project(t, tc.config)
		a := spec024Resolve(t, root, tc.key)
		want := intended(tc.id)
		agree := fx.AgreeNo
		if want != "" && a.govrun == want && a.migrateRel == want {
			agree = fx.AgreeYes
		}
		cov.Measured[tc.id] = agree
		cov.Detail[tc.id] = fmt.Sprintf("key %q: govrun -> %q, migrate -> %q (abs %q); intended %q",
			tc.key, a.govrun, a.migrateRel, a.migrateAbs, want)
		checkStale(tc.id, a)
	}

	// CR-07's mechanism, asserted rather than described: migrate hand-scans
	// lines and keeps the quote characters, so the resolved path contains a
	// literal `"` and the directory can never exist — every scan beneath it
	// returns zero forever.
	{
		root := spec024Project(t, spec024ConfigQuoted)
		a := spec024Resolve(t, root, "decisions")
		if !strings.Contains(a.migrateAbs, `"`) {
			stale = append(stale, fmt.Sprintf(
				"CR-07 records that migrate keeps the quote characters in the resolved path; measured %q, which has none",
				a.migrateAbs))
		}
	}

	// CR-06 — no config at all. Two independent default sets that happen to
	// match. The row is a control, so the assertion is over EVERY key, not
	// one: a coincidence that holds for `decisions` and not for `prds` is
	// still the divergence this group is about.
	{
		root := spec024Project(t, "")
		var diffs []string
		for _, key := range []string{"decisions", "invariants", "guidelines", "specs", "prds"} {
			a := spec024Resolve(t, root, key)
			if a.govrun != a.migrateRel {
				diffs = append(diffs, fmt.Sprintf("%s: govrun %q vs migrate %q", key, a.govrun, a.migrateRel))
			}
		}
		agree := fx.AgreeNo
		if len(diffs) == 0 {
			agree = fx.AgreeYes
		}
		cov.Measured["CR-06"] = agree
		cov.Detail["CR-06"] = fmt.Sprintf("with no .edikt/config.yaml, %d of 5 keys diverge%s",
			len(diffs), func() string {
				if len(diffs) == 0 {
					return ""
				}
				return ": " + strings.Join(diffs, "; ")
			}())
	}

	// CR-08 — the RETURN SHAPE, not a value. The two resolvers do not even
	// return the same KIND of path, so a call site that switches from one to
	// the other silently changes whether its result is relative to the
	// process CWD or to the project root.
	{
		root := spec024Project(t, spec024ConfigBaseDocs)
		a := spec024Resolve(t, root, "decisions")
		govAbs := filepath.IsAbs(a.govrun)
		migAbs := filepath.IsAbs(a.migrateAbs)
		agree := fx.AgreeNo
		if govAbs == migAbs {
			agree = fx.AgreeYes
		}
		cov.Measured["CR-08"] = agree
		cov.Detail["CR-08"] = fmt.Sprintf(
			"govrun returned %q (absolute=%v); migrate returned %q (absolute=%v)",
			a.govrun, govAbs, a.migrateAbs, migAbs)
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
