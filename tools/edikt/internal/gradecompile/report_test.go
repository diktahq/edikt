package gradecompile

import (
	"encoding/json"
	"testing"
)

const bareReport = `{"schema_version":1,"graded_at":"2026-05-29T00:00:00Z","grader_model":"claude-opus-4-7","target_dir":"x","scores":{"coherence":7,"conciseness":4,"signal_to_noise":5,"description_quality": 5, "tier_assignment": 5, "no_double_loading": 5},"overall":5,"findings":[],"summary":"ok"}`

func TestParseReport_BareObject(t *testing.T) {
	r, err := ParseReport([]byte(bareReport))
	if err != nil {
		t.Fatalf("ParseReport(bare): %v", err)
	}
	if r.Overall != 5 || r.Scores.Coherence != 7 {
		t.Fatalf("bare report parsed wrong: overall=%d coherence=%d", r.Overall, r.Scores.Coherence)
	}
}

func TestParseReport_ClaudeEnvelope(t *testing.T) {
	// claude -p --output-format json wraps the assistant text in .result.
	env, err := mustEnvelope(bareReport)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	r, err := ParseReport([]byte(env))
	if err != nil {
		t.Fatalf("ParseReport(envelope): %v", err)
	}
	if r.Overall != 5 || r.Scores.SignalToNoise != 5 {
		t.Fatalf("envelope unwrap parsed wrong: overall=%d s2n=%d", r.Overall, r.Scores.SignalToNoise)
	}
}

func TestParseReport_EnvelopeWithFencedResult(t *testing.T) {
	fenced := "```json\n" + bareReport + "\n```"
	env, err := mustEnvelope(fenced)
	if err != nil {
		t.Fatalf("build envelope: %v", err)
	}
	r, err := ParseReport([]byte(env))
	if err != nil {
		t.Fatalf("ParseReport(fenced envelope): %v", err)
	}
	if r.Overall != 5 {
		t.Fatalf("fenced envelope parsed wrong: overall=%d", r.Overall)
	}
}

// mustEnvelope wraps an assistant text payload in the claude
// --output-format json result envelope, JSON-escaping the payload string.
func mustEnvelope(result string) (string, error) {
	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	return `{"type":"result","subtype":"success","is_error":false,"result":` + string(b) + `}`, nil
}
