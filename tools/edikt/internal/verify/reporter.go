package verify

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"
)

// Summary aggregates per-criterion outcomes.
type Summary struct {
	Passed int `json:"passed"`
	Failed int `json:"failed"`
	// Unmeasured: a verify existed and its subject could not be read, so its
	// exit code says nothing about the rule (SPEC-018 SR-004). Distinct from  edikt-guard:allow
	// Skipped, which means no verify was declared at all.
	Unmeasured int `json:"unmeasured"`
	Skipped    int `json:"skipped"`
	Timeout    int `json:"timeout"`
	Total      int `json:"total"`
}

// Report is the JSON document written under .edikt/state/verify/.
type Report struct {
	PlanID   string   `json:"plan_id"`
	Phase    string   `json:"phase"`
	RanAt    string   `json:"ran_at"`
	RanBy    string   `json:"ran_by"`
	GitSHA   string   `json:"git_sha"`
	Summary  Summary  `json:"summary"`
	Criteria []Result `json:"criteria"`
}

// AnyFailures returns true if at least one criterion failed or timed out.
//
// Unmeasured is deliberately NOT counted here — see AnyUnmeasured for the
// scope decision and why the two questions are kept apart.
func (r *Report) AnyFailures() bool {
	return r.Summary.Failed > 0 || r.Summary.Timeout > 0
}

// AnyUnmeasured returns true if at least one criterion's subject could not be
// read, so its verify never established anything about the rule it names.
//
// SEPARATE FROM AnyFailures ON PURPOSE, and the separation is the decision.
// Both are non-zero-worthy in ONE caller and only one is in the others:
//
//   - `edikt verify <plan-id> [--phase N]` — the exit code here is a
//     COMPLETION CLAIM. The plan command gates the phase row-flip from
//     in-progress to `done` on exit 0, so an all-unmeasured phase exiting 0
//     marks work done having measured nothing. That is the absence-is-never-a-
//     pass rule broken by the runner that enforces it everywhere else, and a
//     completion claim asserted with no evidence. A criterion there names an
//     artifact the phase PROMISED — an absent subject means the phase is not
//     done, not that its rule is intact. That caller gates on
//     AnyFailures() || AnyUnmeasured().
//
//   - the governance / PRD / spec sweeps — the exit code there is a VIOLATION
//     REPORT over a corpus, not a completion claim, and a subject may be
//     legitimately absent because the work is deliberately unbuilt. Measured on
//     the dogfood corpus when this landed: 48 unmeasured items across 17
//     sidecars, every one naming forward work — a test file not yet written, a
//     hook not yet added. Gating on unmeasured there would flip EIGHT sidecars
//     carrying ZERO failures from green to red, none of them describing a
//     violated rule, and the artifact-authoring and compile commands consume
//     those exit codes. That is noise, not enforcement: it would make the sweep
//     useless rather than stricter.
//
// The absence rule requires that an unmeasured control REPORT that nothing was
// measured — never that its subject passed. Every caller already satisfies
// that: Summary.Line() names the unmeasured count on every run. What only the
// plan path must additionally do is refuse to convert that absence into a done
// row, which is what this method exists for.
//
// (ref: INV-013, INV-009)  edikt-guard:allow
func (r *Report) AnyUnmeasured() bool {
	return r.Summary.Unmeasured > 0
}

// UnmeasuredCriteria returns the criteria whose subject could not be read, in
// report order. Callers use it to name WHAT was not measured rather than
// reporting a generic failure — a reader told only "exit 1" would look for a
// broken assertion that does not exist.
func (r *Report) UnmeasuredCriteria() []Result {
	var out []Result
	for _, c := range r.Criteria {
		if c.Status == StatusUnmeasured {
			out = append(out, c)
		}
	}
	return out
}

// BuildSummary fills the report's Summary from its Criteria slice.
func (r *Report) BuildSummary() {
	r.Summary = Summary{Total: len(r.Criteria)}
	for _, c := range r.Criteria {
		switch c.Status {
		case StatusPassed:
			r.Summary.Passed++
		case StatusFailed:
			r.Summary.Failed++
		case StatusTimeout:
			r.Summary.Timeout++
		case StatusUnmeasured:
			// NOT folded into Skipped, and the distinction is the point of
			// SR-004: skipped means no verify was declared, unmeasured means one
			// exists and its subject could not be read. Collapsing them would
			// hide the second inside a number nobody reads as a problem.
			r.Summary.Unmeasured++
		case StatusSkippedOperational, StatusSkippedInformational, StatusSkippedSuppressed, StatusSkippedOwner:
			// Every status must land in a bucket. A new one missing from this
			// list would count in Total and in nothing else, so Total would
			// stop equalling the sum and the report would under-state silently.
			r.Summary.Skipped++
		}
	}
}

// NewReport stamps a fresh report with metadata. phase is "all" for full
// runs or the phase id for --phase N runs.
func NewReport(planID, phase, gitSHA string, criteria []Result) *Report {
	r := &Report{
		PlanID:   planID,
		Phase:    phase,
		RanAt:    time.Now().UTC().Format(time.RFC3339),
		RanBy:    runnerIdentity(),
		GitSHA:   gitSHA,
		Criteria: criteria,
	}
	r.BuildSummary()
	return r
}

// runnerIdentity returns "<user>@<host>" or a "?" sentinel when either
// component cannot be resolved.
func runnerIdentity() string {
	u := "?"
	if cur, err := user.Current(); err == nil && cur.Username != "" {
		u = cur.Username
	} else if env := os.Getenv("USER"); env != "" {
		u = env
	}
	h := "?"
	if hn, err := os.Hostname(); err == nil && hn != "" {
		h = hn
	}
	return u + "@" + h
}

// WriteReports writes both the JSON and text report files under
// dir/<plan>-phase-<phase>-<timestamp>.{json,txt}. Phase "all" produces
// dir/<plan>-all-<timestamp>.{json,txt}. Returns the JSON path.
func WriteReports(dir string, r *Report) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", dir, err)
	}
	stamp := time.Now().UTC().Format("20060102T150405Z")
	stem := r.PlanID
	if r.Phase == "all" {
		stem += "-all-" + stamp
	} else {
		stem += "-phase-" + r.Phase + "-" + stamp
	}
	jsonPath := filepath.Join(dir, stem+".json")
	txtPath := filepath.Join(dir, stem+".txt")

	body, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal report: %w", err)
	}
	body = append(body, '\n')
	if err := os.WriteFile(jsonPath, body, 0o644); err != nil {
		return "", fmt.Errorf("write %s: %w", jsonPath, err)
	}

	tf, err := os.Create(txtPath)
	if err != nil {
		return "", fmt.Errorf("create %s: %w", txtPath, err)
	}
	if err := WriteText(tf, r); err != nil {
		tf.Close()
		return "", err
	}
	if err := tf.Close(); err != nil {
		return "", err
	}
	return jsonPath, nil
}

// WriteText renders the human-readable report to w.
func WriteText(w io.Writer, r *Report) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan:    %s\n", r.PlanID)
	fmt.Fprintf(&b, "Phase:   %s\n", r.Phase)
	fmt.Fprintf(&b, "Ran at:  %s\n", r.RanAt)
	fmt.Fprintf(&b, "Ran by:  %s\n", r.RanBy)
	fmt.Fprintf(&b, "Git:     %s\n", r.GitSHA)
	fmt.Fprintf(&b, "Summary: %d passed, %d failed, %d unmeasured, %d timeout, %d skipped (total %d)\n\n",
		r.Summary.Passed, r.Summary.Failed, r.Summary.Unmeasured, r.Summary.Timeout, r.Summary.Skipped, r.Summary.Total)
	for _, c := range r.Criteria {
		fmt.Fprintf(&b, "[%s] %s — %s (%dms)\n", c.Status, c.ID, c.Statement, c.DurationMS)
		if c.Status == StatusFailed || c.Status == StatusTimeout {
			if c.StdoutExcerpt != "" {
				fmt.Fprintf(&b, "  stdout: %s\n", indent(c.StdoutExcerpt))
			}
			if c.StderrExcerpt != "" {
				fmt.Fprintf(&b, "  stderr: %s\n", indent(c.StderrExcerpt))
			}
		}
	}
	_, err := io.WriteString(w, b.String())
	return err
}

func indent(s string) string {
	return strings.ReplaceAll(strings.TrimRight(s, "\n"), "\n", "\n          ")
}

// Line is the one console summary every verify verb prints. It names EVERY
// class the report can hold, unmeasured included, with its count — a summary
// that listed passed/failed/timeout/skipped and omitted unmeasured let a run
// with seven unmeasured criteria read "4 passed, 0 failed" (D74 a), which is
// INV-013's mistake inside the runner itself: a class that was not counted  edikt-guard:allow
// looked like a class with nothing in it.
func (s Summary) Line() string {
	return fmt.Sprintf("summary: %d passed, %d failed, %d unmeasured, %d timeout, %d skipped (total %d)",
		s.Passed, s.Failed, s.Unmeasured, s.Timeout, s.Skipped, s.Total)
}
