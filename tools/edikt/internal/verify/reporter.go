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
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
	Timeout int `json:"timeout"`
	Total   int `json:"total"`
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
func (r *Report) AnyFailures() bool {
	return r.Summary.Failed > 0 || r.Summary.Timeout > 0
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
		case StatusSkippedOperational, StatusSkippedInformational:
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
	fmt.Fprintf(&b, "Summary: %d passed, %d failed, %d timeout, %d skipped (total %d)\n\n",
		r.Summary.Passed, r.Summary.Failed, r.Summary.Timeout, r.Summary.Skipped, r.Summary.Total)
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
