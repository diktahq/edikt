// Package spec024fixtures loads the comparison-and-status fixture table and
// reconciles what a package MEASURED against what the table RECORDS.
//
// TEST SUPPORT ONLY. Nothing in the shipped binary imports this package —
// verified with `go list -deps ./`, which does not name it — so it reaches no
// adopter. It exists so the four Go packages that hold a fixture group
// (cmd, cmd/gov, internal/pathsproposal, internal/migrate) share ONE loader
// rather than four copies of a YAML reader. The phase that introduced it is
// characterisation only and changes no production behaviour; this file is the
// whole of its Go non-test surface.
//
// TRACEABILITY, and why these identifiers carry the inline marker.
// check-no-internal-refs.sh scans tools/edikt/ BY PATH and cannot know a
// package is unreachable from the binary, so the three references this loader
// genuinely needs are marked rather than dropped — a loader that cannot say
// which artifact it serves is worse than one that names it.
//
//	spec: SPEC-024, comparison and status defects            // edikt-guard:allow
//	plan: PLAN-SPEC-024-comparison-and-status-defects        // edikt-guard:allow
//	rule: INV-013 — a control that observes nothing must not // edikt-guard:allow
//	      report a pass
//
// ── WHY A LOADER RATHER THAN FOUR TABLES ─────────────────────────────────────
//
// The fixture file is the spec's central deliverable, and its value comes
// entirely from carrying `intended` and `today` as SEPARATE fields with an
// `agrees_today` verdict joining them. A per-package hand-copy of the rows
// would let the copy drift from the spec silently, which is the same
// two-things-never-brought-into-one-form defect the spec is about. So the
// canonical docs/product/specs/.../fixtures.yaml is read directly, and the
// only thing a test writes down is (a) the count it expects per group and
// (b) which rows it measures.
//
// ── THE OVERRIDE, AND WHY IT EXISTS ──────────────────────────────────────────
//
// EDIKT_SPEC024_FIXTURES points the loader at an alternate fixture file.
// test/test-fixture-expectations-falsify.sh uses it to flip a defect row
// green in a scratch copy and require the gate RED — the separate invocation
// AC-1.4 asks for. Without an override the falsifier would have to mutate the
// committed spec artifact in place, which is not something a test may do.
package spec024fixtures

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// RelPath is where the canonical fixture table lives, relative to the repo root.
const RelPath = "docs/product/specs/SPEC-024-comparison-and-status-defects/fixtures.yaml" // edikt-guard:allow

// PathEnv names an alternate fixture file. See the package comment.
const PathEnv = "EDIKT_SPEC024_FIXTURES"

// Agreement is the `agrees_today` verdict: does today's shipping behaviour
// match what the row says the behaviour MUST be once the spec lands?
//
// It is deliberately not a bool. Two rows (PI-04, WS-04) record `partial` —
// today's answer is right under one reading and wrong under another — and
// collapsing those to true or false would either ratify a defect or condemn a
// correct half.
type Agreement string

const (
	// AgreeYes is `agrees_today: true` — a CONTROL row. It is green today and
	// must stay green through every phase.
	AgreeYes Agreement = "true"
	// AgreeNo is `agrees_today: false` — a DEFECT row. It must stay red until
	// the named requirement flips it, and flipping early is a failure too.
	AgreeNo Agreement = "false"
	// AgreePartial is `agrees_today: partial`.
	AgreePartial Agreement = "partial"
	// AgreeUnstated is a record with no `agrees_today` field at all — the
	// tree definition (TREE-01) and the expected-output file records.
	AgreeUnstated Agreement = ""
)

// Row is one record of one fixture group.
type Row struct {
	// ID is the record's `id`, or its `path` for the expected-output file
	// records, which are keyed by filename rather than by an identifier.
	ID string
	// Group is the scenario name the record sits under.
	Group string
	// Agreement is the parsed `agrees_today`.
	Agreement Agreement
	// Stated reports whether `agrees_today` was present at all.
	Stated bool
	// Boolean reports whether `agrees_today` was present AND a YAML bool.
	// Stated && !Boolean is the "non-boolean" bucket AC-1.3 pins to exactly
	// two rows.
	Boolean bool
	// Status is `characterized` or `aspirational`.
	Status string
	// Fields is the whole record, for tests that need `intended`, `today`,
	// `left`/`right`, `config` and so on.
	Fields map[string]any
}

// Table is the parsed fixture file.
type Table struct {
	// Path is the file the table was read from.
	Path string
	// Order lists group names in file order.
	Order []string

	rows map[string][]Row
	byID map[string]Row
}

type rawEntity struct {
	Entity  string           `yaml:"entity"`
	Records []map[string]any `yaml:"records"`
}

type rawScenario struct {
	Name     string      `yaml:"name"`
	Entities []rawEntity `yaml:"entities"`
}

type rawFile struct {
	Scenarios []rawScenario `yaml:"scenarios"`
}

// ResolvePath returns the fixture file to read: $EDIKT_SPEC024_FIXTURES when
// set, otherwise RelPath found by walking up from the working directory. Go
// tests run with the working directory set to their package directory, so the
// walk reaches the repo root from any package depth without each caller
// hand-counting "../".
func ResolvePath() (string, error) {
	if p := os.Getenv(PathEnv); p != "" {
		if _, err := os.Stat(p); err != nil {
			return "", fmt.Errorf("%s=%q: %w", PathEnv, p, err)
		}
		return p, nil
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("locating %s: %w", RelPath, err)
	}
	for {
		candidate := filepath.Join(dir, filepath.FromSlash(RelPath))
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", fmt.Errorf("no %s found above the working directory", RelPath)
}

// Load reads and parses the fixture table.
func Load() (*Table, error) {
	path, err := ResolvePath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	var raw rawFile
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	t := &Table{Path: path, rows: map[string][]Row{}, byID: map[string]Row{}}
	for _, sc := range raw.Scenarios {
		if sc.Name == "" {
			return nil, fmt.Errorf("%s: a scenario has no name", path)
		}
		if _, dup := t.rows[sc.Name]; dup {
			return nil, fmt.Errorf("%s: duplicate scenario %q", path, sc.Name)
		}
		t.Order = append(t.Order, sc.Name)
		t.rows[sc.Name] = nil
		for _, ent := range sc.Entities {
			for i, rec := range ent.Records {
				row, err := newRow(sc.Name, i, rec)
				if err != nil {
					return nil, fmt.Errorf("%s: %w", path, err)
				}
				if prev, dup := t.byID[row.ID]; dup {
					return nil, fmt.Errorf("%s: duplicate row id %q (groups %q and %q)",
						path, row.ID, prev.Group, row.Group)
				}
				t.byID[row.ID] = row
				t.rows[sc.Name] = append(t.rows[sc.Name], row)
			}
		}
	}
	return t, nil
}

func newRow(group string, idx int, rec map[string]any) (Row, error) {
	row := Row{Group: group, Fields: rec}
	switch v := rec["id"].(type) {
	case string:
		row.ID = v
	default:
		// The expected-output records are keyed by `path`, not `id`.
		if p, ok := rec["path"].(string); ok {
			row.ID = p
		}
	}
	if row.ID == "" {
		return row, fmt.Errorf("group %q record %d has neither an id nor a path", group, idx)
	}
	if s, ok := rec["status"].(string); ok {
		row.Status = s
	}
	if v, present := rec["agrees_today"]; present {
		row.Stated = true
		switch tv := v.(type) {
		case bool:
			row.Boolean = true
			if tv {
				row.Agreement = AgreeYes
			} else {
				row.Agreement = AgreeNo
			}
		case string:
			row.Agreement = Agreement(tv)
			if row.Agreement != AgreePartial {
				return row, fmt.Errorf("row %s: agrees_today %q is neither a bool nor %q",
					row.ID, tv, AgreePartial)
			}
		default:
			return row, fmt.Errorf("row %s: agrees_today has unusable type %T", row.ID, v)
		}
	}
	return row, nil
}

// Group returns the rows of one fixture group, in file order.
func (t *Table) Group(name string) []Row { return t.rows[name] }

// Lookup returns one row by id.
func (t *Table) Lookup(id string) (Row, bool) {
	r, ok := t.byID[id]
	return r, ok
}

// GroupCounts maps each group name to its record count.
func (t *Table) GroupCounts() map[string]int {
	out := make(map[string]int, len(t.rows))
	for name, rows := range t.rows {
		out[name] = len(rows)
	}
	return out
}

// CheckCounts compares the table against a per-group count written by the
// caller and returns one message per discrepancy.
//
// WHY PER GROUP AND NOT A TOTAL. A YAML indentation typo nests a row under its
// predecessor: the row vanishes, every remaining assertion still passes, and a
// total that is also wrong somewhere else can absorb it. A per-group count
// names the group that shrank.
func (t *Table) CheckCounts(want map[string]int) []string {
	var msgs []string
	got := t.GroupCounts()
	for _, name := range sortedKeys(want) {
		g, ok := got[name]
		if !ok {
			msgs = append(msgs, fmt.Sprintf(
				"fixture group %q is expected (%d rows) but is ABSENT from %s",
				name, want[name], t.Path))
			continue
		}
		if g != want[name] {
			msgs = append(msgs, fmt.Sprintf(
				"fixture group %q holds %d rows, expected %d — a row was added, removed, or swallowed by an indentation typo (%s)",
				name, g, want[name], t.Path))
		}
	}
	for _, name := range t.Order {
		if _, ok := want[name]; !ok {
			msgs = append(msgs, fmt.Sprintf(
				"fixture group %q (%d rows) is in %s but no count is written for it — a new group is unmeasured until a test claims it",
				name, got[name], t.Path))
		}
	}
	return msgs
}

// NonBooleanRows returns the rows whose agrees_today is present but not a
// bool, sorted by id.
func (t *Table) NonBooleanRows() []Row {
	var out []Row
	for _, id := range sortedKeys(t.byID) {
		r := t.byID[id]
		if r.Stated && !r.Boolean {
			out = append(out, r)
		}
	}
	return out
}

// ControlRowIDs returns every row recorded agrees_today: true, sorted.
func (t *Table) ControlRowIDs() []string {
	var out []string
	for id, r := range t.byID {
		if r.Agreement == AgreeYes {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// Coverage is what one Go package did with the fixture groups it holds.
type Coverage struct {
	// Package names the caller, for messages.
	Package string
	// Groups are the fixture groups this package is responsible for.
	Groups []string
	// Measured maps a row id to the agreement this run MEASURED against the
	// shipping code — not to what the table records.
	Measured map[string]Agreement
	// Detail carries what was observed, so a mismatch message can say it.
	Detail map[string]string
	// Skipped maps a row id skipped AT RUNTIME to the condition that caused
	// it. Every entry must also appear in Licensed.
	Skipped map[string]string
	// Licensed enumerates the rows permitted to skip at runtime, each with
	// its condition. One permitted skip must not be able to become seven
	// silent ones.
	Licensed map[string]string
	// OutOfScope maps a row id this package deliberately does not measure to
	// the phase or package that owns it.
	OutOfScope map[string]string
}

func (c Coverage) inScopeGroups() map[string]bool {
	m := make(map[string]bool, len(c.Groups))
	for _, g := range c.Groups {
		m[g] = true
	}
	return m
}

// CheckCoverage reconciles the executed set against the expected set: every
// row of every group this package holds is measured, skipped under a licensed
// condition, or declared out of Go scope — and nothing is claimed twice or
// claimed at all outside the package's groups.
func (t *Table) CheckCoverage(c Coverage) []string {
	var msgs []string
	groups := c.inScopeGroups()
	for _, g := range c.Groups {
		if _, ok := t.rows[g]; !ok {
			msgs = append(msgs, fmt.Sprintf("%s claims fixture group %q, which is not in %s",
				c.Package, g, t.Path))
		}
	}

	claimed := map[string][]string{}
	note := func(id, how string) {
		claimed[id] = append(claimed[id], how)
		row, ok := t.Lookup(id)
		if !ok {
			msgs = append(msgs, fmt.Sprintf("%s %s row %q, which is not in %s",
				c.Package, how, id, t.Path))
			return
		}
		if !groups[row.Group] {
			msgs = append(msgs, fmt.Sprintf(
				"%s %s row %s, which belongs to group %q — outside the groups this package claims (%s)",
				c.Package, how, id, row.Group, strings.Join(c.Groups, ", ")))
		}
	}
	for _, id := range sortedKeys(c.Measured) {
		note(id, "measured")
	}
	for _, id := range sortedKeys(c.Skipped) {
		note(id, "skipped")
	}
	for _, id := range sortedKeys(c.OutOfScope) {
		note(id, "declared out of Go scope for")
	}
	for _, id := range sortedKeys(claimed) {
		if len(claimed[id]) > 1 {
			msgs = append(msgs, fmt.Sprintf("%s claims row %s more than once: %s",
				c.Package, id, strings.Join(claimed[id], " and ")))
		}
	}

	for _, g := range c.Groups {
		for _, row := range t.rows[g] {
			if len(claimed[row.ID]) == 0 {
				msgs = append(msgs, fmt.Sprintf(
					"row %s (group %q) was NEITHER measured, skipped under a licensed condition, NOR declared out of Go scope by %s — an unclaimed row is an unmeasured one",
					row.ID, g, c.Package))
			}
		}
	}

	for _, id := range sortedKeys(c.Skipped) {
		cond, licensed := c.Licensed[id]
		switch {
		case !licensed:
			msgs = append(msgs, fmt.Sprintf(
				"row %s skipped at runtime (%s) but is NOT a licensed skip — one permitted skip must not become seven silent ones",
				id, c.Skipped[id]))
		case strings.TrimSpace(cond) == "":
			msgs = append(msgs, fmt.Sprintf("row %s is licensed to skip with no condition stated", id))
		}
	}
	for _, id := range sortedKeys(c.OutOfScope) {
		if strings.TrimSpace(c.OutOfScope[id]) == "" {
			msgs = append(msgs, fmt.Sprintf("row %s is declared out of Go scope with no owner named", id))
		}
	}
	return msgs
}

// CheckExpectations is the TWO-SIDED assertion.
//
// Direction one: a row recorded agrees_today: true that stops agreeing is a
// regression, named by id.
//
// Direction two: a row recorded agrees_today: false that STARTS agreeing —
// fixed as a side effect of another phase, or by an implementer who edited the
// fixture instead of the code — is equally a failure, also named by id. A
// characterisation suite that only checks direction one converts every defect
// into a target the moment somebody records today's answer as the intent.
func (t *Table) CheckExpectations(c Coverage) []string {
	var msgs []string
	for _, id := range sortedKeys(c.Measured) {
		got := c.Measured[id]
		row, ok := t.Lookup(id)
		if !ok {
			msgs = append(msgs, fmt.Sprintf("%s measured row %q, which is not in %s",
				c.Package, id, t.Path))
			continue
		}
		if !row.Stated {
			msgs = append(msgs, fmt.Sprintf(
				"row %s was measured but records no agrees_today — there is nothing to hold it to", id))
			continue
		}
		if got == row.Agreement {
			continue
		}
		detail := c.Detail[id]
		if detail != "" {
			detail = " — observed: " + detail
		}
		switch row.Agreement {
		case AgreeYes:
			msgs = append(msgs, fmt.Sprintf(
				"CONTROL ROW FLIPPED: %s (group %q) records agrees_today: true and MEASURED %q. A control that flips is a regression%s",
				id, row.Group, got, detail))
		case AgreeNo:
			msgs = append(msgs, fmt.Sprintf(
				"DEFECT ROW AGREES EARLY: %s (group %q) records agrees_today: false and MEASURED %q. Either the defect was fixed outside the requirement that owns it — record which phase did it — or the fixture was edited to match today's behaviour, which turns a defect into a target%s",
				id, row.Group, got, detail))
		default:
			msgs = append(msgs, fmt.Sprintf(
				"row %s (group %q) records agrees_today: %q and MEASURED %q%s",
				id, row.Group, row.Agreement, got, detail))
		}
	}
	return msgs
}

// CheckControls verifies the named control set against the table and against
// what this package measured.
//
// wantControls is the whole Go-reachable control list, written in the test, so
// each of the four packages independently re-checks that the fourteen rows it
// names really do carry agrees_today: true. The rows outside this package's
// groups are checked against the TABLE only; the rows inside it must also have
// been measured green.
func (t *Table) CheckControls(c Coverage, wantControls []string) []string {
	var msgs []string
	groups := c.inScopeGroups()
	inScope := 0
	for _, id := range wantControls {
		row, ok := t.Lookup(id)
		if !ok {
			msgs = append(msgs, fmt.Sprintf("control row %q is not in %s", id, t.Path))
			continue
		}
		if row.Agreement != AgreeYes {
			msgs = append(msgs, fmt.Sprintf(
				"row %s is named a control but records agrees_today: %q", id, row.Agreement))
		}
		if !groups[row.Group] {
			continue
		}
		inScope++
		got, measured := c.Measured[id]
		if !measured {
			msgs = append(msgs, fmt.Sprintf(
				"control row %s is in group %q, which %s claims, but was not measured",
				id, row.Group, c.Package))
			continue
		}
		if got != AgreeYes {
			msgs = append(msgs, fmt.Sprintf(
				"control row %s MEASURED %q, expected %q — observed: %s",
				id, got, AgreeYes, c.Detail[id]))
		}
	}
	// A control-row assertion that happened to name no row of this package's
	// groups would pass while measuring nothing, which is a control that
	// observes nothing reporting a pass.
	if inScope == 0 {
		msgs = append(msgs, fmt.Sprintf(
			"%s executed NO control row — its groups (%s) intersect the named control set nowhere, so this assertion measured nothing",
			c.Package, strings.Join(c.Groups, ", ")))
	}
	return msgs
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
