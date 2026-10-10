package cmd

// spec024_fixtures_test.go — SPEC-024 phase 1, the `cmd` share.
//
// WHAT THIS FILE IS, AND WHAT IT IS NOT. It is NOT a characterisation suite in
// the usual sense. A conventional one records today's behaviour so a refactor
// can be checked against it — but most of SPEC-024's defects return a
// PLAUSIBLE WRONG VALUE rather than an error, so recording today would convert
// a defect into a target. The fixture table's intended / today / agrees_today
// triple is what stops that: a row with agrees_today: false CANNOT be
// satisfied by preserving current behaviour, and this file asserts that
// property in both directions.
//
// Three fixture groups live here because their subjects do:
//
//	version-ordering            the ordering function
//	                            (internal/versioncmp, since phase 2)
//	version-ordering-consumers  upgrade's version gate, pinRemedy and
//	                            findProjectConfig (cmd/upgrade.go's
//	                            upgradeVersionGate, cmd/pinwarn.go:65-89, :129)
//	path-identity               the ownership comparison commands/upgrade.md
//	                            § 5 Hooks Pass 2 performs
//
// EVERY MEASUREMENT BELOW CALLS THE SHIPPING CODE, or EXECUTES the shipped
// tier-1 block. Phase 1 changed no production code at all; phase 2 moved the
// ordering out of `package cmd` into internal/versioncmp and DELETED
// semverParts, semverGreater and semverMajor, so the measurements that used to
// name them now name the helper. What did not change is the direction of the
// dependency: this file reads whatever ships, and never restates it.
//
// ── HOW A VERDICT WAS PRODUCED IS PART OF THE VERDICT (AC-1.6) ───────────────
//
// Three of these rows have a tier-1 markdown subject Go cannot call, so their
// verdict has to come from somewhere other than a Go function call. There are
// two honest ways to get one and they are NOT the same evidence:
//
//	spec024ShippedArtifact  the block is extracted from commands/upgrade.md and
//	                        RUN, the way test-upgrade-cannot-fail-controls.sh
//	                        runs its own snippets. Editing the command file
//	                        changes the answer.
//	spec024Reenactment      a hand-written MODEL of the shipped rule. Editing
//	                        the command file changes NOTHING. That is INV-014:
//	                        a guard reproducing the logic it protects asserts
//	                        nothing about that logic.
//
// Every measurement therefore records the instrument that produced it, and
// TestSpec024_ExecutedSetExcludesReenactments requires the two sets to be
// disjoint: a CONTROL row — one the plan counts as executed and green — may
// never rest on a re-enactment. The kind is recorded BY the helper that
// produces the verdict rather than by a hand-maintained list beside it, so a
// row cannot be relabelled without changing how it is measured.

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	fx "github.com/diktahq/edikt/tools/edikt/internal/spec024fixtures"
	"github.com/diktahq/edikt/tools/edikt/internal/versioncmp"
)

// spec024GroupCounts is the per-group row count, WRITTEN HERE rather than
// derived from the file, because deriving it from the file is what the check
// exists to catch. A YAML indentation typo nests a row under its predecessor:
// the row disappears with no signal, the table shrinks, and every remaining
// assertion still passes.
//
// The same literal is repeated in the three other packages that hold a group
// (cmd/gov, internal/pathsproposal, internal/migrate) DELIBERATELY. Sharing one
// constant would mean one edit silently re-baselines four gates at once.
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

// spec024GoReachableControls are the rows recorded agrees_today: true that a Go
// test in cmd, cmd/gov, internal/pathsproposal or internal/migrate can actually
// execute.
//
// THE SET GROWS AS PHASES LAND, AND THAT IS THE DESIGN. A control row is one
// the table records as agreeing with its intent; a defect row becomes one the
// moment the requirement that owns it flips it. The fixture file's own field
// contract says each `false` row "must flip to true exactly once, in a named
// requirement's change", so this list is not a fixed fourteen — it is the
// running total, and every addition names the requirement that produced it.
// Leaving a flipped row out would fail TestFixtures_ControlRows' totality check
// below, which is what stops a row being flipped in the table and then quietly
// unwatched.
//
// PHASE 1 CLOSED AT FOURTEEN, with four more control rows unreachable from Go
// and asserted where they live (spec024NonGoControls), for eighteen in total.
var spec024GoReachableControls = []string{
	// Phase 1 — controls that were already green and must stay green.
	"VO-01", "VO-05", "VO-07", "VO-08", "VO-09",
	"PC-02", "PC-07",
	"CR-01", "CR-04", "CR-05", "CR-06",
	"WS-01", "WS-05",
	"PI-06",

	// Phase 2, SR-002 — flipped by internal/versioncmp. Six ordering rows and
	// the two pinRemedy consumers: a channel name no longer sorts dead last
	// (VO-03, VO-04, VO-06, VO-10), a pre-release now sorts before its release
	// (VO-02) and beside its numeric siblings (VO-11), and the remedy the pin
	// warning prints follows from the ordering rather than from a truncation
	// (PC-01, PC-03).
	"VO-02", "VO-03", "VO-04", "VO-06", "VO-10", "VO-11",
	"PC-01", "PC-03",

	// Phase 3, SR-002 / SSP-004 — flipped by the lock manifest's `source`
	// field and upgradeProvenanceGate. PC-04's intent has THREE outcomes keyed
	// on provenance, and until the field existed no behaviour could satisfy
	// them: published and absent advance, local refuses and says why.
	"PC-04",

	// Phase 7, SR-009 (AC-7.9) — flipped by removing findProjectConfig's
	// redundant double advance. PC-08 measures an odd-depth ancestor
	// (configDepthFromRoot 2), the case the doubled climb used to skip.
	"PC-08",

	// SR-011 — flipped by EnumerateFiles refusing a symlinked root by name
	// (internal/pathsproposal measures it) instead of returning the empty walk.
	"WS-03",
}

// spec024NonGoControls are the control rows no Go package can execute, with
// the phase that owns each. DR-01 joined on 2026-09-11 when SR-005 shipped
// (D411): it describes a tier-1 markdown behaviour, so nothing here could
// execute it, which is precisely why it sat recording agrees_today: false for
// a whole commit after the behaviour it describes was fixed. A row no control
// list claims is unowned, and TestFixtures_ControlRows says so by name.
var spec024NonGoControls = map[string]string{
	"MB-01": "commands/upgrade.md §2b prose — phase 7, AC-7.5",
	"MB-03": "commands/upgrade.md §2b prose — phase 7, AC-7.5",
	"DR-01": "commands/upgrade.md §2e block — phase 7, AC-7.10 (test/test-upgrade-drift-decision-is-emitted.sh)",
	"DR-02": "commands/upgrade.md §2e prose — phase 7, AC-7.6",
	"CS-02": "a verify-shape row in a plan YAML — phase 8",
}

// spec024NonBooleanRows are the ONLY rows permitted a non-boolean
// agrees_today, each with the bucket it must carry. PC-04 and CR-02 were
// `unknown` until the 2026-09-04 rulings filled them; both became plain
// `false`, and PC-04 flipped to `true` in phase 3 when the provenance field
// its intent is keyed on landed. A stale count here would be the very defect
// this table records.
var spec024NonBooleanRows = map[string]fx.Agreement{
	"PI-04": fx.AgreePartial,
	"WS-04": fx.AgreePartial,
}

// spec024InstrumentKind classifies HOW a row's verdict was produced. See the
// file header: for a tier-1 subject the difference between executing the
// shipped block and modelling it is the difference between evidence and a
// restatement, and AC-1.6 exists because that difference was invisible.
type spec024InstrumentKind string

const (
	// spec024Shipping — the verdict comes from CALLING the shipping Go code
	// under test (versioncmp.Compare, upgradeVersionGate, pinRemedy,
	// findProjectConfig).
	spec024Shipping spec024InstrumentKind = "shipping-code"
	// spec024ShippedArtifact — the verdict comes from EXECUTING the shipped
	// tier-1 artifact: a block extracted from a commands/*.md file and run.
	spec024ShippedArtifact spec024InstrumentKind = "shipped-artifact"
	// spec024Reenactment — the verdict comes from a hand-written model of the
	// shipped rule, which is evidence of a different kind and must say so.
	spec024Reenactment spec024InstrumentKind = "re-enactment"
)

// spec024Instrument records what produced one row's verdict.
type spec024Instrument struct {
	// Kind is the classification above.
	Kind spec024InstrumentKind
	// Name is the function or block that answered.
	Name string
	// Subject is what it called, executed, or modelled.
	Subject string
	// Retires is required of a re-enactment and names what replaces it. A
	// model with no stated end is a permanent one.
	Retires string
}

// spec024Run is what one pass of spec024Measure observed.
type spec024Run struct {
	// Coverage is the executed / skipped / out-of-scope accounting.
	Coverage fx.Coverage
	// Instruments maps a measured row id to the instrument that answered it.
	Instruments map[string]spec024Instrument
	// Stale lists rows whose recorded `today:` block no longer matches what
	// was just measured.
	Stale []string
}

func spec024SortedIDs[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func spec024Load(t *testing.T) *fx.Table {
	t.Helper()
	tbl, err := fx.Load()
	if err != nil {
		t.Fatalf("loading the SPEC-024 fixture table: %v", err)
	}
	return tbl
}

// ─── the constructed tree (TREE-01's path-identity subset) ───────────────────

type spec024Tree struct {
	root string
	// hookDir is what commands/upgrade.md Pass 2 computes: HOME expanded,
	// symlinks NOT resolved.
	hookDir string
	// realHookDir is where hookDir actually leads, two hops down.
	realHookDir string
	// currentHookDir is the one-hop form, <HOME>/.edikt/current/hooks.
	currentHookDir string
	otherToolHook  string
	real           string
	alias          string
	// caseVariantLink and caseVariantReal exist only on a case-insensitive
	// volume; see PI-05.
	caseVariantLink string
	caseVariantReal string
	caseInsensitive bool
}

// spec024BuildTree constructs the path-identity and walk subsets of TREE-01.
//
// NEVER the developer's real $HOME, and no absolute path outside the temp
// tree. The two-level hooks symlink is not decoration: it is the shape this
// machine actually has (hooks -> current/hooks, current -> versions/dev), and
// a one-level fixture would pass a helper that resolves only one hop.
//
// The root is EvalSymlinks-resolved once, up front, because t.TempDir()
// returns /var/folders/... on macOS and /var is itself a symlink — leaving
// that in would make "did resolution change this path?" answer differently on
// macOS and Linux for reasons having nothing to do with the fixture.
func spec024BuildTree(t *testing.T) spec024Tree {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatalf("resolving temp root: %v", err)
	}
	mkdir := func(p string) {
		t.Helper()
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", p, err)
		}
	}
	write := func(p, content string) {
		t.Helper()
		mkdir(filepath.Dir(p))
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", p, err)
		}
	}
	link := func(target, name string) {
		t.Helper()
		mkdir(filepath.Dir(name))
		if err := os.Symlink(target, name); err != nil {
			t.Fatalf("symlink %s -> %s: %v", name, target, err)
		}
	}

	ediktHome := filepath.Join(root, "home", ".edikt")
	realHooks := filepath.Join(ediktHome, "versions", "dev", "hooks")
	write(filepath.Join(realHooks, "session-start.sh"), "#!/bin/sh\n")
	write(filepath.Join(realHooks, "post-tool-use.sh"), "#!/bin/sh\n")
	link("versions/dev", filepath.Join(ediktHome, "current"))
	link("current/hooks", filepath.Join(ediktHome, "hooks"))

	otherTool := filepath.Join(root, "home", ".config", "other-tool", "hooks", "session-start.sh")
	write(otherTool, "#!/bin/sh\n")

	realDir := filepath.Join(root, "real")
	write(filepath.Join(realDir, "a.txt"), "a\n")
	write(filepath.Join(realDir, "sub", "b.txt"), "b\n")
	link("real", filepath.Join(root, "alias"))

	tree := spec024Tree{
		root:           root,
		hookDir:        filepath.Join(ediktHome, "hooks"),
		realHookDir:    realHooks,
		currentHookDir: filepath.Join(ediktHome, "current", "hooks"),
		otherToolHook:  otherTool,
		real:           realDir,
		alias:          filepath.Join(root, "alias"),
	}

	// PI-05 needs a symlink whose TARGET was written in a different case than
	// the real directory. On a case-sensitive volume that target simply does
	// not exist, the two paths are genuinely different locations, and the row
	// cannot be constructed at all — a licensed skip, not a silent pass.
	mkdir(filepath.Join(root, "CaseProbe"))
	if _, err := os.Stat(filepath.Join(root, "caseprobe")); err == nil {
		tree.caseInsensitive = true
		tree.caseVariantReal = filepath.Join(root, "MyProjects", "diktahq", "edikt-dev")
		mkdir(tree.caseVariantReal)
		tree.caseVariantLink = filepath.Join(root, "work", "supermodular", "edikt-dev")
		link(filepath.Join(root, "myprojects", "diktahq", "edikt-dev"), tree.caseVariantLink)
	}
	return tree
}

// spec024IdentityAsShipped re-enacts the path comparison that ships TODAY:
// two strings compared with `==` after no normalisation at all — no expansion,
// no symlink resolution, no trailing-separator handling. That is what
// commands/upgrade.md Pass 2 does (`upgrade.md:978` builds an expanded
// hook_dir, `:1031` compares it to the stored command's directory), and it is
// how the fixture's `today:` values were produced ("scratch Go program:
// filepath.Dir(...) == HOME+\"/.edikt/hooks\" -> false").
//
// It is a MODEL, it is labelled one (spec024Reenactment), and no CONTROL row
// may rest on it — see TestSpec024_ExecutedSetExcludesReenactments. Phase 4
// introduces tools/edikt/internal/pathid and the PI-* rows move onto the real
// helper; until then this is what the DEFECT rows are measured against.
func spec024IdentityAsShipped(left, right string) string {
	if left == right {
		return "same_location"
	}
	return "different_locations"
}

// ─── PI-06: the shipped block, executed ──────────────────────────────────────

// spec024RepoRoot walks up for commands/upgrade.md.
//
// Called BEFORE spec024FindConfigAt's t.Chdir, never after: that helper leaves
// the working directory inside a temp tree for the remainder of the test, and
// a root resolved from there is no root at all.
func spec024RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("locating the repo root: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "commands", "upgrade.md")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("no commands/upgrade.md above %q — PI-06's subject is unreadable, which is UNMEASURED, not passing (INV-013)", dir)
		}
		dir = parent
	}
}

// spec024HooksBlock extracts the python block under `#### Hooks` from
// commands/upgrade.md — the merge the command tells the model to run, and the
// same block test/test-upgrade-retired-hooks.sh extracts the same way.
func spec024HooksBlock(t *testing.T, root string) string {
	t.Helper()
	path := filepath.Join(root, "commands", "upgrade.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	i := strings.Index(string(data), "#### Hooks")
	if i < 0 {
		t.Fatalf("%s carries no `#### Hooks` section — PI-06's subject moved and this measurement would be reporting on nothing", path)
	}
	m := regexp.MustCompile("(?s)```python\n(.*?)```").FindStringSubmatch(string(data)[i:])
	if m == nil {
		t.Fatalf("%s: no fenced python block under `#### Hooks`", path)
	}
	if !strings.Contains(m[1], "Pass 2") {
		t.Fatalf("%s: the block under `#### Hooks` carries no Pass 2 — the ownership comparison PI-06 measures is not in it", path)
	}
	return m[1]
}

// spec024HooksDriver binds the free names of the command file's pseudocode to
// real ones and runs it. It supplies the I/O the block leaves abstract and
// NOTHING ELSE: no ownership rule, no comparison, no removal decision. Those
// come from the command file, which is the whole point.
const spec024HooksDriver = `
import sys, json, os
block, report_path = sys.argv[1], sys.argv[2]
HOME = os.environ["HOME"]
reports = []
def read_json(p): return json.load(open(p))
def read_file(p): return open(os.path.expandvars(p)).read()
def write_json(p, obj): json.dump(obj, open(p, "w"), indent=2)
def report(line): reports.append(line)
exec(compile(open(block).read(), block, "exec"), {**globals(), "json": json, "os": os})
open(report_path, "w").write("\n".join(reports) + "\n")
`

// spec024OwnershipAsShipped answers PI-06 by RUNNING the shipped ownership
// comparison instead of restating it.
//
// WHAT IS OBSERVED. Pass 2 decides ownership by directory — `owned_by_edikt =
// os.path.dirname(cmd) == hook_dir` — and the only place that decision becomes
// visible is the removal it licenses. So both paths are registered in one
// settings.json, the block is run once, and the answer is read off which of
// the two survived:
//
//	left removed, right kept   the shipped comparison DISTINGUISHES them
//	                           -> different_locations, PI-06's intent
//	both removed               it calls them the same place
//	                           -> same_location, and PI-06 goes red
//
// That second branch is the failure PI-06 exists to catch: Pass 2 DELETES what
// it decides edikt owns, so an ownership rule that says yes too eagerly turns
// SR-003 from a missing-removal bug into a deleting-a-user's-hook bug.
//
// WHY verify-gate.sh AND NOT session-start.sh. The fixture row writes both
// sides as `session-start.sh`. Pass 2 removes only a hook that is owned AND no
// longer shipped by the template AND absent from disk, and session-start.sh is
// still shipped — so with that basename the removal branch is unreachable, both
// entries survive for a reason having nothing to do with path identity, and the
// comparison is unobservable. verify-gate.sh is the basename retired in 0.9.0
// (upgrade.md's Pass 2 comment names it), which is also the basename PI-01
// carries in this very directory. Same basename on both sides, different
// directory: the row's structural property is preserved exactly, and the
// substitution is recorded in the detail line rather than left to be noticed.
func spec024OwnershipAsShipped(t *testing.T, root string, tree spec024Tree, leftDir, rightDir, base string) (string, string) {
	t.Helper()
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Fatalf("PI-06 is measured by EXECUTING commands/upgrade.md § 5 Hooks, which is python, and python3 is not on PATH (%v). "+
			"A control that cannot run is UNMEASURED, not passing (INV-013). test/test-upgrade-retired-hooks.sh already requires python3.", err)
	}

	work := filepath.Join(tree.root, "pass2")
	claude := filepath.Join(work, "project", ".claude")
	if err := os.MkdirAll(claude, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", claude, err)
	}
	blockPath := filepath.Join(work, "hooks-merge.py")
	if err := os.WriteFile(blockPath, []byte(spec024HooksBlock(t, root)), 0o644); err != nil {
		t.Fatalf("write %s: %v", blockPath, err)
	}

	left := filepath.Join(leftDir, base)
	right := filepath.Join(rightDir, base)
	// NEITHER file exists on disk, which is the state a retired hook is in:
	// its shim is gone and only the registration remains (PI-01's point).
	entry := func(cmd string) map[string]any {
		return map[string]any{"type": "command", "command": cmd}
	}
	doc := map[string]any{"hooks": map[string]any{"PreToolUse": []any{
		map[string]any{"matcher": "Write|Edit", "hooks": []any{entry(left), entry(right)}},
	}}}
	enc, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encoding the fixture settings.json: %v", err)
	}
	settings := filepath.Join(claude, "settings.json")
	if err := os.WriteFile(settings, enc, 0o644); err != nil {
		t.Fatalf("write %s: %v", settings, err)
	}

	reportPath := filepath.Join(work, "report.txt")
	cmd := exec.Command(py, "-", blockPath, reportPath)
	cmd.Dir = filepath.Join(work, "project")
	cmd.Stdin = strings.NewReader(spec024HooksDriver)
	env := []string{}
	for _, kv := range os.Environ() {
		switch {
		case strings.HasPrefix(kv, "HOME="), strings.HasPrefix(kv, "EDIKT_TEMPLATES="):
		default:
			env = append(env, kv)
		}
	}
	cmd.Env = append(env,
		"HOME="+filepath.Join(tree.root, "home"),
		"EDIKT_TEMPLATES="+filepath.Join(root, "templates"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the § 5 Hooks block from commands/upgrade.md did not run (%v):\n%s", err, out)
	}

	reported, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("reading the block's report: %v", err)
	}
	after, err := os.ReadFile(settings)
	if err != nil {
		t.Fatalf("reading the settings.json the block wrote: %v", err)
	}
	var parsed struct {
		Hooks map[string][]struct {
			Hooks []struct {
				Command string `json:"command"`
			} `json:"hooks"`
		} `json:"hooks"`
	}
	if err := json.Unmarshal(after, &parsed); err != nil {
		t.Fatalf("parsing the settings.json the block wrote: %v\n%s", err, after)
	}
	survives := map[string]bool{}
	for _, groups := range parsed.Hooks {
		for _, g := range groups {
			for _, h := range g.Hooks {
				survives[h.Command] = true
			}
		}
	}

	// The harness must make ownership OBSERVABLE before it may report on it.
	// If the edikt-owned side was not removed, the two paths were treated
	// alike for a reason unrelated to the comparison and there is no verdict
	// to read — that is a broken instrument, not a same_location answer.
	if survives[left] {
		t.Fatalf("PI-06: the edikt-owned registration %q survived Pass 2, so the ownership comparison never fired and nothing about path identity was observed. Report:\n%s", left, reported)
	}
	verdict := "different_locations"
	if !survives[right] {
		verdict = "same_location"
	}
	return verdict, fmt.Sprintf(
		"executed commands/upgrade.md § 5 Hooks Pass 2 with hook_dir=%s: %q removed, %q %s (basename %s — the row writes session-start.sh, which is still shipped and so never reaches the removal branch; see spec024OwnershipAsShipped)",
		filepath.Join(tree.root, "home", ".edikt", "hooks"), left, right,
		map[bool]string{true: "kept", false: "removed"}[survives[right]], base)
}

func spec024Verdict(intended, got string) fx.Agreement {
	if intended == got {
		return fx.AgreeYes
	}
	return fx.AgreeNo
}

// ─── version ordering ────────────────────────────────────────────────────────

// spec024OrderAsShipped derives an ordering verdict from the shipping
// comparison. It CALLS that comparison rather than re-deriving the rule, so a
// change to the shipping ordering changes this answer.
//
// BEFORE PHASE 2 it called semverGreater in both directions and folded the two
// bools into a verdict, and what it could not produce was the finding: a bool
// pair has no representation for `incomparable`, so every row intending it was
// forced to disagree. That was SR-002's defect stated as a type. The helper now
// returns the verdict directly and this function is a pass-through — kept as a
// named seam so the fixture's vocabulary and the helper's are joined in exactly
// one place.
//
// It still cannot produce `error`, which VO-12 intends for an empty version.
// That is deliberate and belongs to the phase implementing the empty-fetch
// guard; see the versioncmp package comment.
func spec024OrderAsShipped(left, right string) string {
	return string(versioncmp.Compare(left, right))
}

// spec024RemedyKind classifies pinRemedy's output the way the fixture records
// it, by first line rather than by branch, because the first line is what the
// user reads.
//
// `neutral_remedy` IS THE FIXTURE'S WORD, and it is used here rather than a
// synonym of this file's own choosing. PC-01 records `intended_output:
// neutral_remedy`; a classifier that answered `generic_remedy` for the same
// behaviour would leave that row red for a vocabulary mismatch while the
// behaviour it names was correct — a status read from something other than the
// thing it claims to report, inside the harness for a spec about exactly that.
// The two are kept DISTINCT because they are distinct answers: `not comparable`
// is the channel case, and the bare `upgrade-pin` line is the equal-ordering
// case that says nothing about direction because there is none to say.
func spec024RemedyKind(pinned, active string) (string, string) {
	out := pinRemedy(pinned, active)
	first := strings.SplitN(out, "\n", 2)[0]
	switch {
	case strings.Contains(first, "The pin is AHEAD"):
		return "pin_is_ahead", first
	case strings.Contains(first, "The pin is behind"):
		return "pin_is_behind", first
	case strings.Contains(first, "not comparable"):
		return "neutral_remedy", first
	case strings.Contains(first, "Run `edikt upgrade-pin`"):
		return "generic_remedy", first
	default:
		return "unclassified", first
	}
}

// spec024FindConfigAt plants a config.yaml `depth` levels above the working
// directory and reports whether findProjectConfig returns THAT file.
//
// "Returns that file" rather than "returns something" is deliberate.
// findProjectConfig only breaks at HOME, so from a temp dir it can climb past
// the fixture and find a real ancestor config on the developer's machine — a
// test that passes for a reason unrelated to its subject. Comparing against
// the planted path makes a stray ancestor a miss, which is the honest answer.
func spec024FindConfigAt(t *testing.T, configDepthFromRoot int) (bool, string) {
	t.Helper()
	root := t.TempDir()
	deep := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", deep, err)
	}
	// configDepthFromRoot 1 => <root>/a, 2 => <root>/a/b.
	holder := root
	for _, seg := range []string{"a", "b"}[:configDepthFromRoot] {
		holder = filepath.Join(holder, seg)
	}
	planted := filepath.Join(holder, ".edikt", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(planted), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(planted), err)
	}
	if err := os.WriteFile(planted, []byte("edikt_version: \"0.9.0\"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", planted, err)
	}
	t.Setenv("HOME", root)
	t.Chdir(deep)
	got := findProjectConfig()
	if got == "" {
		return false, "findProjectConfig returned nothing"
	}
	gotResolved, _ := filepath.EvalSymlinks(got)
	plantedResolved, _ := filepath.EvalSymlinks(planted)
	if gotResolved != "" && gotResolved == plantedResolved {
		return true, "found the planted config at " + planted
	}
	return false, fmt.Sprintf("returned %s, not the planted %s", got, planted)
}

// ─── the measurement ─────────────────────────────────────────────────────────

// spec024Measure runs every row this package owns against the shipping code
// and returns what it observed. The second result is the set of rows whose
// recorded `today:` block does not match what was just measured — a
// characterisation failure distinct from an expectation failure: it says the
// spec's record of current behaviour has gone stale.
func spec024Measure(t *testing.T, tbl *fx.Table) spec024Run {
	t.Helper()
	// Resolved FIRST: spec024FindConfigAt chdirs into a temp tree below and
	// does not come back for the rest of the test.
	repoRoot := spec024RepoRoot(t)
	instruments := map[string]spec024Instrument{}
	cov := fx.Coverage{
		Package:  "cmd",
		Groups:   []string{"version-ordering", "version-ordering-consumers", "path-identity"},
		Measured: map[string]fx.Agreement{},
		Detail:   map[string]string{},
		Skipped:  map[string]string{},
		Licensed: map[string]string{
			"PI-05": "the volume under t.TempDir() is case-sensitive, so a case-variant symlink target does not resolve and the row cannot be constructed",
		},
		OutOfScope: map[string]string{
			"PC-05": "commands/upgrade.md:200 channel guard, tier-1 shell — phase 7, AC-7.3",
			"PC-06": "commands/upgrade.md step 0d, tier-1 shell — phase 7, AC-7.4",
		},
	}
	var stale []string

	str := func(row fx.Row, key string) string {
		s, _ := row.Fields[key].(string)
		return s
	}
	todayField := func(row fx.Row, key string) (any, bool) {
		m, ok := row.Fields["today"].(map[string]any)
		if !ok {
			return nil, false
		}
		v, ok := m[key]
		return v, ok
	}

	// ── version-ordering ──────────────────────────────────────────────────
	for _, row := range tbl.Group("version-ordering") {
		left := str(row, "left")
		right := str(row, "right")
		got := spec024OrderAsShipped(left, right)
		cov.Measured[row.ID] = spec024Verdict(str(row, "intended"), got)
		instruments[row.ID] = spec024Instrument{
			Kind:    spec024Shipping,
			Name:    "spec024OrderAsShipped",
			Subject: "versioncmp.Compare (internal/versioncmp), the shipping ordering",
		}
		cov.Detail[row.ID] = fmt.Sprintf("versioncmp.Compare(%q,%q)=%s (reverse: %s) -> %s",
			left, right, versioncmp.Compare(left, right), versioncmp.Compare(right, left), got)
		if recorded, ok := todayField(row, "verdict"); ok && recorded != got {
			stale = append(stale, fmt.Sprintf(
				"%s records today.verdict %v but the shipping comparison answers %q — the fixture's record of CURRENT behaviour is stale",
				row.ID, recorded, got))
		}
	}

	// ── version-ordering-consumers ────────────────────────────────────────
	for _, id := range []string{"PC-01", "PC-02", "PC-03"} {
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		in, _ := row.Fields["input"].(map[string]any)
		pinned, _ := in["pinned"].(string)
		active, _ := in["active"].(string)
		got, first := spec024RemedyKind(pinned, active)
		cov.Measured[id] = spec024Verdict(str(row, "intended_output"), got)
		instruments[id] = spec024Instrument{
			Kind:    spec024Shipping,
			Name:    "spec024RemedyKind",
			Subject: "pinRemedy (cmd/pinwarn.go:129), classified by its first line",
		}
		cov.Detail[id] = fmt.Sprintf("pinRemedy(%q,%q) -> %s (%q)", pinned, active, got, first)
		if recorded, ok := todayField(row, "output"); ok && recorded != got {
			stale = append(stale, fmt.Sprintf("%s records today.output %v, measured %q", id, recorded, got))
		}
	}
	{
		// PC-04 — cmd/upgrade.go's already-up-to-date refusal, read together
		// with the lock manifest's PROVENANCE. Its `intended_output` is a
		// THREE-WAY map keyed on the recorded `source`, so the row is measured
		// THREE TIMES, once per source state, and agrees only when all three
		// answers match. A single measurement would satisfy at most one third
		// of the intent while reporting a verdict for the whole row — which is
		// what it did until phase 3 gave the field somewhere to come from.
		id := "PC-04"
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		in, _ := row.Fields["input"].(map[string]any)
		latest, _ := in["latest"].(string)
		current, _ := in["current"].(string)
		// Measured through the GATE, not through the ordering it consults, so
		// this row reports what `edikt upgrade` would do rather than what a
		// comparison returns.
		sources := []struct{ key, source string }{
			{"when_source_published", lockSourcePublished},
			{"when_source_local", lockSourceLocal},
			{"when_source_absent", lockSourceUnknown},
		}
		got := map[string]string{}
		for _, st := range sources {
			reason, _ := upgradeProvenanceGate(current, latest, st.source)
			switch reason {
			case refusalNone:
				got[st.key] = "advance"
			case refusalLocalPayload:
				got[st.key] = "refuse_with_reason"
			case refusalUpToDate:
				got[st.key] = "refuses_as_up_to_date"
			default:
				got[st.key] = string(reason)
			}
		}
		want, _ := row.Fields["intended_output"].(map[string]any)
		agree := fx.AgreeNo
		if len(want) == len(got) {
			agree = fx.AgreeYes
			for k, v := range want {
				if s, _ := v.(string); got[k] != s {
					agree = fx.AgreeNo
				}
			}
		}
		cov.Measured[id] = agree
		instruments[id] = spec024Instrument{
			Kind:    spec024Shipping,
			Name:    "spec024Measure, PC-04 branch",
			Subject: "upgradeProvenanceGate (cmd/upgrade.go — the already-up-to-date refusal read with the lock manifest's source)",
		}
		cov.Detail[id] = fmt.Sprintf(
			"upgradeProvenanceGate(current=%q, latest=%q, source) -> published:%s local:%s absent:%s (%d intended outcomes)",
			current, latest, got["when_source_published"], got["when_source_local"], got["when_source_absent"], len(want))
		if recorded, ok := todayField(row, "output"); ok {
			rm, isMap := recorded.(map[string]any)
			if !isMap {
				stale = append(stale, fmt.Sprintf(
					"%s records today.output as the single value %v; the outcome is keyed on the lock manifest source and has %d, so that record cannot say what was measured",
					id, recorded, len(got)))
			} else {
				for _, st := range sources {
					r, _ := rm[st.key].(string)
					if r != got[st.key] {
						stale = append(stale, fmt.Sprintf("%s records today.output.%s %q, measured %q", id, st.key, r, got[st.key]))
					}
				}
			}
		}
	}
	for _, tc := range []struct {
		id    string
		depth int
	}{
		{"PC-07", 1}, // config at <root>/a — an EVEN number of levels up, so
		{"PC-08", 2}, // the skipping loop lands on it. <root>/a/b is odd.
	} {
		row, ok := tbl.Lookup(tc.id)
		if !ok {
			t.Fatalf("fixture row %s is missing", tc.id)
		}
		found, detail := spec024FindConfigAt(t, tc.depth)
		want := true
		if m, ok := row.Fields["intended_output"].(map[string]any); ok {
			if b, ok := m["found"].(bool); ok {
				want = b
			}
		}
		cov.Measured[tc.id] = spec024Verdict(fmt.Sprint(want), fmt.Sprint(found))
		instruments[tc.id] = spec024Instrument{
			Kind:    spec024Shipping,
			Name:    "spec024FindConfigAt",
			Subject: "findProjectConfig (cmd/pinwarn.go:65-89)",
		}
		cov.Detail[tc.id] = detail
		if recorded, ok := todayField(row, "found"); ok {
			if b, ok := recorded.(bool); ok && b != found {
				stale = append(stale, fmt.Sprintf("%s records today.found %v, measured %v", tc.id, b, found))
			}
		}
	}

	// ── path-identity ─────────────────────────────────────────────────────
	tree := spec024BuildTree(t)
	measureIdentity := func(id, left, right, detail string) {
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		got := spec024IdentityAsShipped(left, right)
		cov.Measured[id] = spec024Verdict(str(row, "intended"), got)
		// The KIND is set here, by the helper that produced the answer, so a
		// row measured against the model cannot be labelled anything else
		// without changing how it is measured (AC-1.6).
		instruments[id] = spec024Instrument{
			Kind:    spec024Reenactment,
			Name:    "spec024IdentityAsShipped",
			Subject: "commands/upgrade.md § 5 Hooks Pass 2, `os.path.dirname(cmd) == hook_dir`",
			Retires: "phase 4 (AC-4.1) moves the PI-* rows onto tools/edikt/internal/pathid",
		}
		cov.Detail[id] = fmt.Sprintf("%s -> %s%s", id, got, detail)
	}

	// PI-01 — the stored value is the four LITERAL characters `$HOME`.
	// Never expanded by the test; that is the whole finding.
	storedCommand := "$HOME/.edikt/hooks/verify-gate.sh"
	measureIdentity("PI-01", filepath.Dir(storedCommand), tree.hookDir,
		fmt.Sprintf(" (dirname %q vs expanded hook_dir %q)", filepath.Dir(storedCommand), tree.hookDir))

	// PI-02 — trailing separator.
	measureIdentity("PI-02", tree.hookDir+string(os.PathSeparator), tree.hookDir,
		fmt.Sprintf(" (filepath.Clean makes them equal: %v; the shipping comparison never calls it)",
			filepath.Clean(tree.hookDir+string(os.PathSeparator)) == filepath.Clean(tree.hookDir)))

	// PI-03 — <HOME>/.edikt/hooks IS a symlink to current/hooks.
	{
		l, lerr := filepath.EvalSymlinks(tree.hookDir)
		r, rerr := filepath.EvalSymlinks(tree.currentHookDir)
		if lerr != nil || rerr != nil {
			t.Fatalf("PI-03: resolving the hook symlink chain: %v / %v", lerr, rerr)
		}
		measureIdentity("PI-03", tree.hookDir, tree.currentHookDir,
			fmt.Sprintf(" (resolution AGREES — both reach %q — and the shipping comparison never calls it: %v)",
				l, l == r))
	}

	// PI-04 — THE OVER-CORRECTION ROW, and the one that is `partial`.
	// Normalising ONE side inverts the defect, so the bucket is decided by
	// running both strategies and requiring them to disagree.
	{
		id := "PI-04"
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		resolvedStored, err1 := filepath.EvalSymlinks(tree.hookDir)
		resolvedReal, err2 := filepath.EvalSymlinks(tree.realHookDir)
		if err1 != nil || err2 != nil {
			t.Fatalf("PI-04: resolving: %v / %v", err1, err2)
		}
		symmetric := spec024IdentityAsShipped(resolvedStored, resolvedReal)
		asymmetric := spec024IdentityAsShipped(resolvedStored, tree.hookDir)
		intended := str(row, "intended")
		symOK := symmetric == intended
		asymOK := asymmetric == intended
		agree := fx.AgreeNo
		switch {
		case symOK && asymOK:
			agree = fx.AgreeYes
		case symOK != asymOK:
			agree = fx.AgreePartial
		}
		cov.Measured[id] = agree
		instruments[id] = spec024Instrument{
			Kind:    spec024Reenactment,
			Name:    "spec024IdentityAsShipped, run under both normalisation strategies",
			Subject: "commands/upgrade.md § 5 Hooks Pass 2, `os.path.dirname(cmd) == hook_dir`",
			Retires: "phase 4 (AC-4.1) moves the PI-* rows onto tools/edikt/internal/pathid",
		}
		cov.Detail[id] = fmt.Sprintf(
			"symmetric resolution -> %s (%v); asymmetric (stored resolved, hook_dir left raw) -> %s (%v)",
			symmetric, symOK, asymmetric, asymOK)
	}

	// PI-05 — measured on this machine and it surprised the measurement:
	// EvalSymlinks disagrees on case while os.SameFile is true.
	if !tree.caseInsensitive {
		cov.Skipped["PI-05"] = cov.Licensed["PI-05"]
	} else {
		l, lerr := filepath.EvalSymlinks(tree.caseVariantLink)
		r, rerr := filepath.EvalSymlinks(tree.caseVariantReal)
		li, lserr := os.Stat(tree.caseVariantLink)
		ri, rserr := os.Stat(tree.caseVariantReal)
		sameFile := lserr == nil && rserr == nil && os.SameFile(li, ri)
		switch {
		case lerr != nil || rerr != nil || !sameFile:
			cov.Skipped["PI-05"] = cov.Licensed["PI-05"]
		default:
			measureIdentity("PI-05", l, r,
				fmt.Sprintf(" (EvalSymlinks: %q vs %q, string-equal %v; os.SameFile %v)",
					l, r, l == r, sameFile))
		}
	}

	// PI-06 — THE NEGATIVE ROW, AND IT IS LOAD-BEARING. Pass 2 DELETES what it
	// decides edikt owns; a helper that returns true too eagerly turns a
	// missing-removal bug into a deleting-a-user's-hook bug.
	//
	// It is also a CONTROL — one of AC-1.1's fourteen — and a control may not
	// rest on a model of the thing it guards (AC-1.6, INV-014). So this row,
	// alone among the PI-*, is answered by EXECUTING commands/upgrade.md's
	// Pass 2 rather than by spec024IdentityAsShipped. Edit that command file's
	// ownership rule and this answer changes; that is what makes it evidence.
	{
		id := "PI-06"
		row, ok := tbl.Lookup(id)
		if !ok {
			t.Fatalf("fixture row %s is missing", id)
		}
		got, detail := spec024OwnershipAsShipped(t, repoRoot, tree,
			tree.hookDir, filepath.Dir(tree.otherToolHook), "verify-gate.sh")
		cov.Measured[id] = spec024Verdict(str(row, "intended"), got)
		instruments[id] = spec024Instrument{
			Kind:    spec024ShippedArtifact,
			Name:    "spec024OwnershipAsShipped",
			Subject: "commands/upgrade.md § 5 Hooks Pass 2, extracted and executed",
		}
		cov.Detail[id] = fmt.Sprintf("%s -> %s (same basename, different directory — must stay different_locations); %s",
			id, got, detail)
	}

	// PI-07 — the bridge to SR-011: a trailing separator on a SYMLINKED root
	// changes the traversal, so "strip the trailing separator" (PI-02's
	// obvious fix) silently converts a working walk into an empty one.
	{
		bare := spec024WalkCount(t, tree.alias)
		slashed := spec024WalkCount(t, tree.alias+string(os.PathSeparator))
		if bare != 1 || slashed != 4 {
			t.Errorf("PI-07: the recorded interaction did not reproduce — WalkDir(%q)=%d (recorded 1), WalkDir(%q)=%d (recorded 4)",
				tree.alias, bare, tree.alias+string(os.PathSeparator), slashed)
		}
		measureIdentity("PI-07", tree.alias, tree.alias+string(os.PathSeparator),
			fmt.Sprintf(" (WalkDir entries: bare %d, trailing-separator %d — normalisation and traversal are NOT independent)",
				bare, slashed))
	}

	return spec024Run{Coverage: cov, Instruments: instruments, Stale: stale}
}

// spec024WalkCount counts every entry filepath.WalkDir visits under root,
// the root itself included. It is the reference measurement WS-01/WS-02 name.
func spec024WalkCount(t *testing.T, root string) int {
	t.Helper()
	n := 0
	err := filepath.WalkDir(root, func(_ string, _ os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		n++
		return nil
	})
	if err != nil {
		t.Fatalf("WalkDir %s: %v", root, err)
	}
	return n
}

// ─── the four gates ──────────────────────────────────────────────────────────

// TestFixtures_RowCount — AC-1.2.
func TestFixtures_RowCount(t *testing.T) {
	tbl := spec024Load(t)
	for _, msg := range tbl.CheckCounts(spec024GroupCounts) {
		t.Error(msg)
	}
}

// TestFixtures_RowIdCoverage — AC-1.3. The EXECUTED set equals the EXPECTED
// set, licensed skips are enumerated by id with their condition, and the two
// non-boolean rows carry an explicit bucket.
func TestFixtures_RowIdCoverage(t *testing.T) {
	tbl := spec024Load(t)
	run := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckCoverage(run.Coverage) {
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
	run := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckControls(run.Coverage, spec024GoReachableControls) {
		t.Error(msg)
	}

	// Every control row is claimed by exactly one of the two lists, and the
	// non-Go ones are named where they live. No count is written here: a
	// hardcoded total goes stale the moment a row flips, and this release has
	// four recorded instances of a surface stating a number nobody re-measures.
	all := tbl.ControlRowIDs()
	if len(all) != len(spec024GoReachableControls)+len(spec024NonGoControls) {
		t.Errorf("the table records %d control rows (%v); %d Go-reachable plus %d asserted elsewhere are expected",
			len(all), all, len(spec024GoReachableControls), len(spec024NonGoControls))
	}
	named := map[string]bool{}
	for _, id := range spec024GoReachableControls {
		named[id] = true
	}
	for id := range spec024NonGoControls {
		named[id] = true
	}
	for _, id := range all {
		if !named[id] {
			t.Errorf("row %s records agrees_today: true but is claimed by no control list — a new control row is unowned until one claims it", id)
		}
	}
}

// TestFixtures_ExpectationSetTwoSided — AC-1.4.
func TestFixtures_ExpectationSetTwoSided(t *testing.T) {
	tbl := spec024Load(t)
	run := spec024Measure(t, tbl)
	for _, msg := range tbl.CheckExpectations(run.Coverage) {
		t.Error(msg)
	}
	for _, msg := range run.Stale {
		t.Error(msg)
	}
}

// TestSpec024_ExecutedSetExcludesReenactments — AC-1.6.
//
// THE DEFECT THIS CLOSES. PI-06 was counted among AC-1.1's fourteen EXECUTED
// controls while its verdict came from spec024IdentityAsShipped, a two-line
// hand-written model whose own comment says it "re-enacts the path comparison
// that ships TODAY". Fourteen executed controls of which one is a restatement
// is thirteen, and nothing in the suite said so: the model would have answered
// `different_locations` for two distinct strings no matter what commands/
// upgrade.md did, including after somebody made Pass 2's ownership rule
// eager enough to delete a user's own hook. A guard that reproduces the logic
// it protects asserts nothing about that logic (INV-014).
//
// WHAT IS ASSERTED. Every measured row names the instrument that answered it;
// no row in the executed control set was answered by a re-enactment; every
// re-enactment names the subject it models and what retires it; and PI-06
// specifically — the row the criterion is about — is answered by executing
// commands/upgrade.md.
//
// WHAT IT DOES NOT CLAIM. The kind is a label, and a label can be written
// wrongly. What keeps it honest is that it is written by the helper that
// PRODUCES the verdict, not by a list maintained beside the measurements: a
// row cannot be relabelled without being re-measured through a different
// instrument. That is weaker than a mechanical proof and stronger than a
// promise in a test name, and it is stated here rather than implied.
func TestSpec024_ExecutedSetExcludesReenactments(t *testing.T) {
	tbl := spec024Load(t)
	run := spec024Measure(t, tbl)

	for _, id := range spec024SortedIDs(run.Coverage.Measured) {
		in, ok := run.Instruments[id]
		if !ok {
			t.Errorf("row %s was measured but names no instrument — how a verdict was produced is part of the verdict, and an unattributed measurement cannot be told from a re-enactment", id)
			continue
		}
		switch in.Kind {
		case spec024Shipping, spec024ShippedArtifact, spec024Reenactment:
		default:
			t.Errorf("row %s names instrument kind %q, which is not one of %q, %q, %q",
				id, in.Kind, spec024Shipping, spec024ShippedArtifact, spec024Reenactment)
		}
		if strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.Subject) == "" {
			t.Errorf("row %s names an instrument with no %s", id, map[bool]string{true: "name", false: "subject"}[strings.TrimSpace(in.Name) == ""])
		}
		if in.Kind == spec024Reenactment && strings.TrimSpace(in.Retires) == "" {
			t.Errorf("row %s is measured by the re-enactment %s, which does not say what retires it — a model with no stated end is a permanent one", id, in.Name)
		}
	}

	// THE DISJOINTNESS. A control row is one the plan counts as executed and
	// green; it may not rest on a model of its own subject.
	controls := map[string]bool{}
	for _, id := range spec024GoReachableControls {
		controls[id] = true
	}
	checked := 0
	for _, id := range spec024SortedIDs(run.Coverage.Measured) {
		if !controls[id] {
			continue
		}
		checked++
		in := run.Instruments[id]
		if in.Kind == spec024Reenactment {
			t.Errorf("EXECUTED CONTROL MEASURED BY A RE-ENACTMENT: %s is named among the %d Go-reachable controls but its verdict comes from %s, a model of %s. "+
				"Either execute the subject, as spec024OwnershipAsShipped does for PI-06, or move %s out of the executed set and record it as evidence of a different kind — %d executed controls of which one is a restatement is %d",
				id, len(spec024GoReachableControls), in.Name, in.Subject, id,
				len(spec024GoReachableControls), len(spec024GoReachableControls)-1)
		}
	}
	if checked == 0 {
		t.Errorf("this gate examined NO control row — the cmd package's measured set (%v) intersects the named control set nowhere, so the disjointness it reports was measured against nothing",
			spec024SortedIDs(run.Coverage.Measured))
	}

	// PI-06 by name. The general rule above would also be satisfied by a
	// package that stopped measuring PI-06 altogether; the criterion is about
	// this row, so it is asserted as this row.
	pi06, ok := run.Instruments["PI-06"]
	switch {
	case !controls["PI-06"]:
		t.Error("PI-06 is no longer named among the Go-reachable controls — AC-1.1 counts it there, so removing it needs a criterion change, not a test change")
	case !ok:
		t.Error("PI-06 was not measured by the cmd package, which is the package holding the path-identity group")
	case pi06.Kind != spec024ShippedArtifact:
		t.Errorf("PI-06 is measured by %s (kind %q). Its subject is a tier-1 markdown block Go cannot call, so the only evidence that counts as EXECUTION is running that block: expected kind %q",
			pi06.Name, pi06.Kind, spec024ShippedArtifact)
	case !strings.Contains(pi06.Subject, "commands/upgrade.md"):
		t.Errorf("PI-06's instrument claims kind %q but names the subject %q, which is not a command file — an artifact that is not read cannot have been executed",
			pi06.Kind, pi06.Subject)
	}
}
