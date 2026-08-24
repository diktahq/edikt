package hookcorpus

// diff.go — the migration gate.
//
// `bin/edikt dev hook-corpus diff --hook <name>` re-runs every scenario in a
// hook's matrix against the implementation that is registered RIGHT NOW, and
// compares the result against the recording captured from the bash oracle.
//
// WHAT IS COMPARED, AND WHAT IS NOT
//
//	exit code           exactly, never normalized      (F-036)
//	stdout / stderr     sha256 after identical masking on both sides
//	fs_diff             structurally: same paths added / removed / modified,
//	                    with added and modified content hashed under the same
//	                    masking                        (F-074)
//	wall clock          NOT compared — informational only, per SPEC-014
//	                    spec.md:265, which declines to set a runtime budget
//
// FOUR REFUSALS COME BEFORE ANY COMPARISON, and they are the reason this gate
// means anything:
//
//  1. A recording whose normalizer_version is not this build's — the two sides
//     would be masked by different rules.
//  2. A recording whose scenario_input_sha256 no longer matches the scenario's
//     current inputs — the oracle was recorded from bytes that are gone.
//  3. With --require-go, a comparison whose live side is not the Go
//     implementation — a bash-against-bash self-diff is a smoke test of this
//     tool, not evidence about a rewrite, and it is trivially green.
//  4. Zero scenarios compared, for any reason — INV-013.

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

// ScenarioResult is one scenario's verdict.
type ScenarioResult struct {
	Scenario    string `json:"scenario"`
	Passed      bool   `json:"passed"`
	Reason      string `json:"reason,omitempty"`
	Oracle      string `json:"oracle"`
	LiveOracle  string `json:"live_oracle"`
	RecordedMS  int64  `json:"recorded_wall_clock_ms"`
	LiveMS      int64  `json:"live_wall_clock_ms"`
	ExitCode    int    `json:"exit_code"`
	RecordedExe string `json:"recorded_exit_code"`
}

// DiffReport is a whole hook's verdict.
type DiffReport struct {
	Hook string `json:"hook"`

	// Compared is the denominator. A report that carries only a failure count
	// cannot distinguish "everything passed" from "nothing ran" (INV-013).
	Compared int              `json:"scenarios_compared"`
	Passed   int              `json:"scenarios_passed"`
	Failed   int              `json:"scenarios_failed"`
	Results  []ScenarioResult `json:"results"`

	// GateMeaningful is false for a bash-against-bash self-diff. Recorded so a
	// green report cannot be mistaken for migration evidence it is not.
	GateMeaningful bool  `json:"gate_meaningful"`
	TotalMS        int64 `json:"total_wall_clock_ms"`
}

// FirstFailure names the first failing scenario, or "".
func (r *DiffReport) FirstFailure() string {
	for _, res := range r.Results {
		if !res.Passed {
			return fmt.Sprintf("%s: %s", res.Scenario, res.Reason)
		}
	}
	return ""
}

// OK reports whether the gate passes.
//
// Zero comparisons is NOT ok. This is the single line that keeps an empty
// matrix, a mistyped --scenario, or a hook whose recordings were deleted from
// exiting 0 with a clean report.
func (r *DiffReport) OK() bool { return r.Compared > 0 && r.Failed == 0 }

// DiffOptions tunes one gate run.
type DiffOptions struct {
	// Scenario limits the run to one id. An id that matches nothing is an
	// error, not an empty pass.
	Scenario string

	// Live selects the implementation to compare against: "auto" follows the
	// registry, "go" forces the Go handler, "bash" forces the bash body.
	Live string

	// RequireGo refuses a self-diff. Phase 12+ migration gates set it; a
	// developer smoke-testing the tool does not.
	RequireGo bool
}

// Diff runs the gate.
func (r *Recorder) Diff(hook string, opts DiffOptions, out io.Writer) (*DiffReport, error) {
	m, err := LoadMatrix(MatrixPath(r.opts.CorpusRoot, hook))
	if err != nil {
		return nil, err
	}
	scenarios := m.Scenarios
	if opts.Scenario != "" {
		sc, serr := m.Scenario(opts.Scenario)
		if serr != nil {
			return nil, serr
		}
		scenarios = []Scenario{sc}
	}
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("hook %q selected zero scenarios — nothing would be compared, "+
			"and an empty comparison is not a pass", hook)
	}

	row, err := r.registry.Row(hook)
	if err != nil {
		return nil, err
	}
	live := opts.Live
	if live == "" {
		live = "auto"
	}
	// Resolve the implementation ONCE, before any scenario runs. An
	// unresolvable binary must fail here — loudly, with a denominator of zero —
	// rather than turning into a per-scenario error that a summary could round
	// off to "no failures".
	impl, err := r.resolveImplementation(row, live)
	if err != nil {
		return nil, err
	}
	meaningful := impl.oracle == OracleGo
	if opts.RequireGo && !meaningful {
		return nil, fmt.Errorf("--require-go: the live implementation for %q is bash, so this "+
			"would compare the oracle against itself.\n"+
			"  A self-diff is trivially green and is evidence about this tool, not about a "+
			"rewrite. %s still marks %q status: %s.",
			hook, r.registry.Path, hook, row.Status)
	}

	fmt.Fprintf(out, "comparing %d scenario(s) of %s against %s (%s, sha256 %s)\n",
		len(scenarios), hook, impl.path, impl.oracle, short(impl.digest))

	report := &DiffReport{Hook: hook, GateMeaningful: meaningful}
	for _, sc := range scenarios {
		res, rerr := r.diffScenario(hook, sc, live)
		if rerr != nil {
			return nil, rerr
		}
		report.Results = append(report.Results, res)
		report.Compared++
		report.TotalMS += res.LiveMS
		if res.Passed {
			report.Passed++
		} else {
			report.Failed++
		}
		mark := "PASS"
		if !res.Passed {
			mark = "FAIL"
		}
		fmt.Fprintf(out, "%s %s/%s (oracle=%s live=%s, %dms)\n",
			mark, hook, sc.ID, res.Oracle, res.LiveOracle, res.LiveMS)
		if !res.Passed {
			fmt.Fprintf(out, "     %s\n", res.Reason)
		}
	}
	if report.Compared == 0 {
		return nil, fmt.Errorf("hook %q: zero scenarios compared — nothing was measured", hook)
	}
	if !meaningful {
		fmt.Fprintf(out, "NOTE: live implementation is bash — this is a SELF-DIFF and proves "+
			"nothing about a Go rewrite. Pass --require-go to make that a failure.\n")
	}
	return report, nil
}

func (r *Recorder) diffScenario(hook string, sc Scenario, live string) (ScenarioResult, error) {
	res := ScenarioResult{Scenario: sc.ID}
	dir := RecordingDir(r.opts.CorpusRoot, hook, sc.ID)
	rec, err := LoadRecording(dir)
	if err != nil {
		return res, fmt.Errorf("scenario %q has no usable recording: %w", sc.ID, err)
	}
	res.Oracle = rec.Oracle
	res.RecordedMS = rec.WallClockMS

	if rec.NormalizerVersion != Version {
		return res, fmt.Errorf("scenario %q was recorded under normalizer %s but this build "+
			"normalizes at %s.\n"+
			"  The two sides would be masked by different rules, so the comparison would be "+
			"between two things neither of which is what it claims. Re-record this hook's corpus.",
			sc.ID, rec.NormalizerVersion, Version)
	}
	current, err := r.inputDigest(hook, sc)
	if err != nil {
		return res, err
	}
	if current != rec.ScenarioInputSHA256 {
		return res, fmt.Errorf("scenario %q's inputs have changed since it was recorded "+
			"(recorded %s, current %s).\n"+
			"  Its payload, config fixture or env set no longer hashes to what the recording was "+
			"captured against, so the oracle describes input that is not on disk any more. "+
			"Comparing against it would be green for a reason unrelated to the implementation. "+
			"Re-record the scenario.",
			sc.ID, short(rec.ScenarioInputSHA256), short(current))
	}

	ex, err := r.Run(hook, sc, live)
	if err != nil {
		return res, err
	}
	res.LiveOracle = ex.Impl.oracle
	res.LiveMS = ex.WallClockMS
	res.ExitCode = ex.ExitCode
	res.RecordedExe = fmt.Sprintf("%d", rec.ExitCode)

	switch {
	case ex.ExitCode != rec.ExitCode:
		// Deliberately first, and deliberately never softened: F-036 is the
		// finding that an exit code carries protocol meaning stdout cannot see.
		res.Reason = fmt.Sprintf("exit code %d, recorded %d", ex.ExitCode, rec.ExitCode)
	case ex.StdoutHash != rec.StdoutSHA256:
		res.Reason = fmt.Sprintf("normalized stdout hash %s, recorded %s (%d live bytes, %d recorded)",
			short(ex.StdoutHash), short(rec.StdoutSHA256), len(ex.Stdout), rec.StdoutBytes)
	case ex.StderrHash != rec.StderrSHA256:
		res.Reason = fmt.Sprintf("normalized stderr hash %s, recorded %s",
			short(ex.StderrHash), short(rec.StderrSHA256))
	default:
		if ok, msg := rec.FSDiff.Equal(ex.Diff); !ok {
			res.Reason = "filesystem effect differs: " + msg
		}
	}
	res.Passed = res.Reason == ""
	return res, nil
}

// ── coverage ────────────────────────────────────────────────────────────────

// AxisCoverage is one axis's verdict for one hook.
type AxisCoverage struct {
	Axis       string   `json:"axis"`
	Covered    bool     `json:"covered"`
	Applicable bool     `json:"applicable"`
	Scenarios  []string `json:"scenarios"`
}

// FindingCoverage is one finding class's verdict.
type FindingCoverage struct {
	Finding    string   `json:"finding"`
	Applicable bool     `json:"applicable"`
	Covered    bool     `json:"covered"`
	Scenarios  []string `json:"scenarios"`
	Why        string   `json:"why"`
}

// CoverageReport is the per-hook coverage-gap table AC-002-2 makes mechanical.
type CoverageReport struct {
	Hook     string            `json:"hook"`
	Axes     []AxisCoverage    `json:"axes"`
	Findings []FindingCoverage `json:"findings"`

	// Examined is the denominator: how many axes this report actually looked
	// at. A coverage check that examined zero axes has measured nothing, and
	// "no gaps found" over zero axes is INV-013's exact failure.
	Examined  int `json:"axes_examined"`
	Scenarios int `json:"scenarios_examined"`
}

// Gaps lists every uncovered applicable axis and finding class.
func (c *CoverageReport) Gaps() []string {
	var out []string
	for _, a := range c.Axes {
		if a.Applicable && !a.Covered {
			out = append(out, "axis "+a.Axis)
		}
	}
	for _, f := range c.Findings {
		if f.Applicable && !f.Covered {
			out = append(out, f.Finding+" ("+f.Why+")")
		}
	}
	return out
}

// OK reports whether this hook is eligible to migrate on coverage grounds.
func (c *CoverageReport) OK() bool {
	return c.Examined > 0 && c.Scenarios > 0 && len(c.Gaps()) == 0
}

// f074Hooks are the hooks ADR-063 Decision §2 names as having a side effect as
// their only observable on a healthy run, plus any this spec's review adds.
// Listed rather than inferred: the property is about what a hook DOES, which no
// registry field records.
func f074Hooks() map[string]string {
	return map[string]string{
		"worktree-create": "governance copy is the only observable on a healthy run",
		"task-created":    "ledger entry is the only observable on a healthy run",
		"event-log":       "JSONL append is the only observable on a healthy run",
		"session-start":   "truncates $HOME/.edikt/gate-overrides.jsonl and rotates the signals log",
		"session-end":     "appends to $HOME/.edikt/",
	}
}

// Coverage builds one hook's coverage report.
func (r *Recorder) Coverage(hook string) (*CoverageReport, error) {
	row, err := r.registry.Row(hook)
	if err != nil {
		return nil, err
	}
	if row.IsLibrary() {
		return nil, fmt.Errorf("hook %q is class %q — a sourced library has no corpus by "+
			"construction. This is not a coverage gap and reporting it as "+
			"one would be noise", hook, row.Class)
	}
	m, err := LoadMatrix(MatrixPath(r.opts.CorpusRoot, hook))
	if err != nil {
		return nil, err
	}

	rep := &CoverageReport{Hook: hook, Scenarios: len(m.Scenarios)}
	for _, axis := range Axes() {
		cov := AxisCoverage{Axis: axis, Applicable: true}
		for _, sc := range m.Scenarios {
			if sc.HasAxis(axis) {
				cov.Covered = true
				cov.Scenarios = append(cov.Scenarios, sc.ID)
			}
		}
		rep.Axes = append(rep.Axes, cov)
		rep.Examined++
	}

	byFinding := map[string][]string{}
	for _, sc := range m.Scenarios {
		if sc.SourceFinding != "" {
			byFinding[sc.SourceFinding] = append(byFinding[sc.SourceFinding], sc.ID)
		}
	}
	add := func(id string, applicable bool, why string) {
		rep.Findings = append(rep.Findings, FindingCoverage{
			Finding: id, Applicable: applicable, Covered: len(byFinding[id]) > 0,
			Scenarios: byFinding[id], Why: why,
		})
	}
	// F-041: a spaced file_path silently disabled a governance control. Every
	// hook whose payload carries a path field needs the case.
	add("F-041", payloadCarriesPath(row), "payload carries a file_path-shaped field")
	// F-036: exit codes carry protocol meaning beyond 0 for the deny classes.
	add("F-036", row.Class == ClassGate, "exit code carries protocol meaning beyond 0")
	f074 := f074Hooks()
	why, isF074 := f074[hook]
	add("F-074", isF074, why)
	return rep, nil
}

// payloadCarriesPath reports whether this hook's event delivers a path-bearing
// payload field. Derived from the registry row's event, which is the only
// machine-readable fact about a hook's payload shape available here.
func payloadCarriesPath(row RegistryRow) bool {
	ev, ok := row.EventName()
	if !ok {
		return false
	}
	switch ev {
	case "PreToolUse", "PostToolUse", "FileChanged", "WorktreeCreate", "WorktreeRemove", "CwdChanged":
		return true
	}
	return false
}

// Render writes the coverage table.
func (c *CoverageReport) Render(w io.Writer) {
	fmt.Fprintf(w, "hook: %s (%d scenarios, %d axes examined)\n", c.Hook, c.Scenarios, c.Examined)
	for _, a := range c.Axes {
		mark := "GAP "
		if a.Covered {
			mark = "ok  "
		}
		fmt.Fprintf(w, "  %s axis %-12s %s\n", mark, a.Axis, strings.Join(a.Scenarios, ", "))
	}
	findings := append([]FindingCoverage(nil), c.Findings...)
	sort.Slice(findings, func(i, j int) bool { return findings[i].Finding < findings[j].Finding })
	for _, f := range findings {
		switch {
		case !f.Applicable:
			fmt.Fprintf(w, "  n/a  %s\n", f.Finding)
		case f.Covered:
			fmt.Fprintf(w, "  ok   %s %s\n", f.Finding, strings.Join(f.Scenarios, ", "))
		default:
			fmt.Fprintf(w, "  GAP  %s — %s\n", f.Finding, f.Why)
		}
	}
}
