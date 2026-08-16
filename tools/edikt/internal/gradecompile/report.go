// report.go — the compile-quality grader's output type and atomic writer.
//
// SPEC-009 Plan H (SR-016). Report mirrors the JSON shape declared in  // edikt-guard:allow
// templates/schemas/compile-quality-report.v1.schema.json; field
// ordering matches the schema document so json.Marshal output stays
// diff-stable against the committed example/stub fixtures. WriteReport
// mirrors cheatrate.WriteReport — temp-file-plus-rename for atomicity.
package gradecompile

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Report is the compile-quality grader output, validated against
// templates/schemas/compile-quality-report.v1.schema.json.
type Report struct {
	SchemaVersion int       `json:"schema_version"`
	GradedAt      string    `json:"graded_at"`
	GraderModel   string    `json:"grader_model"`
	TargetDir     string    `json:"target_dir"`
	Scores        Scores    `json:"scores"`
	Overall       int       `json:"overall"`
	Findings      []Finding `json:"findings"`
	Summary       string    `json:"summary"`
}

// Scores holds the four per-dimension integer scores (each 0–10).
type Scores struct {
	Coherence            int `json:"coherence"`
	Conciseness          int `json:"conciseness"`
	SignalToNoise        int `json:"signal_to_noise"`
	// These three replaced the retired signal→file dimension. Delivery is now
	// decided by the registry description a reader routes on, by which tier a
	// topic lands in, and by nothing appearing on two ambient surfaces at
	// once. Scoring whether the old table pointed at the right file would
	// grade a surface this release deleted.
	DescriptionQuality int `json:"description_quality"`
	TierAssignment     int `json:"tier_assignment"`
	NoDoubleLoading    int `json:"no_double_loading"`
}

// Finding is one editorial observation. SuggestedFix is optional and is
// omitted from JSON when empty (the schema forbids a null value).
type Finding struct {
	Dimension    string `json:"dimension"`
	Severity     string `json:"severity"`
	Message      string `json:"message"`
	SuggestedFix string `json:"suggested_fix,omitempty"`
}

// ReportPath returns the canonical destination for a grade-compile
// report: <stateDir>/compile-quality/<timestamp>.json. The caller is
// responsible for resolving stateDir (typically <project>/.edikt/state)
// and for normalizing the timestamp to a filesystem-safe form.
func ReportPath(stateDir, timestamp string) string {
	return filepath.Join(stateDir, "compile-quality", fmt.Sprintf("%s.json", timestamp))
}

// WriteReport marshals report to indented JSON and writes it atomically
// (temp file + rename) to the canonical compile-quality path under
// stateDir. Returns the written path.
func WriteReport(stateDir string, report *Report) (string, error) {
	if report == nil {
		return "", fmt.Errorf("gradecompile: report must not be nil")
	}
	if report.GradedAt == "" {
		return "", fmt.Errorf("gradecompile: report.GradedAt must not be empty")
	}
	out := ReportPath(stateDir, fsSafeTimestamp(report.GradedAt))
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", fmt.Errorf("gradecompile: create report dir: %w", err)
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", fmt.Errorf("gradecompile: marshal report: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(out), ".report-*.json.tmp")
	if err != nil {
		return "", fmt.Errorf("gradecompile: create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("gradecompile: write report: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("gradecompile: close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, out); err != nil {
		_ = os.Remove(tmpPath)
		return "", fmt.Errorf("gradecompile: rename report into place: %w", err)
	}
	return out, nil
}

// ParseReport unmarshals a grader's JSON stdout into a Report.
//
// The grader is dispatched via `claude -p … --output-format json`, which
// wraps the model's text output in a result envelope:
//
//	{"type":"result","subtype":"success","result":"<assistant text>", …}
//
// The grader's JSON report is that .result string. ParseReport unwraps the
// envelope when present, then parses the inner payload. If raw is already a
// bare report object (no envelope — e.g. a direct caller or test), it is
// parsed directly. A markdown code fence around the report is tolerated
// defensively even though the grader prompt forbids one.
func ParseReport(raw []byte) (*Report, error) {
	payload := bytes.TrimSpace(raw)
	if inner, ok := unwrapClaudeEnvelope(payload); ok {
		payload = inner
	}
	payload = stripCodeFence(payload)

	var r Report
	if err := json.Unmarshal(payload, &r); err != nil {
		return nil, fmt.Errorf("gradecompile: parse report: %w", err)
	}
	return &r, nil
}

// unwrapClaudeEnvelope detects a `claude --output-format json` result
// envelope and returns the inner assistant text (the .result string).
// Returns (nil, false) when raw is not such an envelope — notably when it
// is already the bare report object: a Report has no top-level "type" or
// "result" field, so the discriminator cannot misfire on it.
func unwrapClaudeEnvelope(raw []byte) ([]byte, bool) {
	var env struct {
		Type   string  `json:"type"`
		Result *string `json:"result"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, false
	}
	if env.Type == "result" && env.Result != nil {
		return []byte(strings.TrimSpace(*env.Result)), true
	}
	return nil, false
}

// stripCodeFence removes a single leading ```… fence line and a trailing
// ``` fence when both are present, tolerating a model that wraps its JSON
// in a markdown block despite the prompt forbidding it. No-op otherwise.
func stripCodeFence(b []byte) []byte {
	s := strings.TrimSpace(string(b))
	if !strings.HasPrefix(s, "```") {
		return b
	}
	if nl := strings.IndexByte(s, '\n'); nl != -1 {
		s = s[nl+1:]
	}
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "```")
	return []byte(strings.TrimSpace(s))
}

// fsSafeTimestamp turns an RFC3339 timestamp (which may contain colons)
// into a path-safe form by replacing ':' with '-'.
func fsSafeTimestamp(ts string) string {
	return strings.ReplaceAll(ts, ":", "-")
}

// ValidateRaw checks the grader's JSON for the fields the report schema
// declares required, operating on the RAW document rather than the decoded
// struct.
//
// That distinction is the whole point. Report's fields are plain ints and
// strings, so json.Unmarshal cannot tell "the grader omitted scores" from
// "the grader scored everything 0" — both decode to the zero value. The
// pre-ADR-044 dispatcher hit exactly that: it fed an unwrapped
// `claude --output-format json` envelope to the decoder, every field missed,
// and a report of all zeros was persisted and displayed as a real 0/10 grade
// with sub-threshold warnings. A control reporting results it never
// computed.
//
// So absence is detected by key presence, and a missing score is an error —
// never a zero.
func ValidateRaw(raw []byte) error {
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("grader report is not a JSON object: %w", err)
	}
	for _, k := range []string{"schema_version", "graded_at", "scores", "overall"} {
		if _, ok := doc[k]; !ok {
			return fmt.Errorf("grader report is missing required field %q — "+
				"an absent field is not a zero score", k)
		}
	}

	var schemaVersion int
	if err := json.Unmarshal(doc["schema_version"], &schemaVersion); err != nil || schemaVersion < 1 {
		return fmt.Errorf("grader report has schema_version %s — expected >= 1; "+
			"a 0 here usually means the result envelope was never unwrapped",
			strings.TrimSpace(string(doc["schema_version"])))
	}

	var scores map[string]json.RawMessage
	if err := json.Unmarshal(doc["scores"], &scores); err != nil {
		return fmt.Errorf("grader report scores is not an object: %w", err)
	}
	for _, dim := range []string{"coherence", "conciseness", "signal_to_noise", "description_quality", "tier_assignment", "no_double_loading"} {
		v, ok := scores[dim]
		if !ok {
			return fmt.Errorf("grader report is missing score %q — "+
				"an ungraded dimension is not a 0", dim)
		}
		var n int
		if err := json.Unmarshal(v, &n); err != nil {
			return fmt.Errorf("grader report score %q is not a number: %s", dim, string(v))
		}
		if n < 0 || n > 10 {
			return fmt.Errorf("grader report score %q = %d is outside the 0-10 rubric range", dim, n)
		}
	}

	var overall int
	if err := json.Unmarshal(doc["overall"], &overall); err != nil {
		return fmt.Errorf("grader report overall is not a number: %s", string(doc["overall"]))
	}
	if overall < 0 || overall > 10 {
		return fmt.Errorf("grader report overall = %d is outside the 0-10 rubric range", overall)
	}
	return nil
}

// UnwrapForValidation returns the innermost JSON document — the same one
// ParseReport decodes — so ValidateRaw inspects the grader's actual report
// rather than a transport envelope wrapped around it. Agents may return a
// bare object or a `claude --output-format json` result envelope, and
// validating the envelope would pass on a report that has no scores at all.
func UnwrapForValidation(raw []byte) []byte {
	if inner, ok := unwrapClaudeEnvelope(raw); ok {
		return inner
	}
	return raw
}
