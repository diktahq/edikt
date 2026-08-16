package gradecompile

import "testing"

// The defect these lock down: Report's fields are plain ints, so
// json.Unmarshal cannot distinguish "the grader omitted scores" from "the
// grader scored 0". The pre-ADR-044 dispatcher fed an unwrapped
// `claude --output-format json` envelope to the decoder, every field missed,
// and an all-zero report was persisted AND displayed by doctor as a real
// 0/10 grade with sub-threshold warnings — a control reporting results it
// never computed.
//
// The first case below is byte-for-byte the shape of the void report found
// in .edikt/state/compile-quality/ after that dispatcher ran.
func TestValidateRaw_RejectsSilentZeroShapes(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{
			name: "the recovered void report (schema_version 0, all dimensions 0)",
			raw: `{"schema_version":0,"graded_at":"2026-05-29T01:51:44Z","grader_model":"claude-opus-4-7",
			       "target_dir":".claude/rules/governance","scores":{"coherence":0,"conciseness":0,
			       "signal_to_noise":0,"description_quality": 0, "tier_assignment": 0, "no_double_loading": 0},"overall":0,"findings":null,"summary":""}`,
		},
		{name: "unrelated object", raw: `{"not":"a report"}`},
		{name: "scores object present but empty", raw: `{"schema_version":1,"graded_at":"x","scores":{},"overall":5}`},
		{name: "one dimension missing", raw: `{"schema_version":1,"graded_at":"x","scores":{"coherence":5,"conciseness":5,"signal_to_noise":5},"overall":5}`},
		{name: "score above rubric range", raw: `{"schema_version":1,"graded_at":"x","scores":{"coherence":99,"conciseness":5,"signal_to_noise":5,"description_quality": 5, "tier_assignment": 5, "no_double_loading": 5},"overall":5}`},
		{name: "score below rubric range", raw: `{"schema_version":1,"graded_at":"x","scores":{"coherence":-1,"conciseness":5,"signal_to_noise":5,"description_quality": 5, "tier_assignment": 5, "no_double_loading": 5},"overall":5}`},
		{name: "overall missing", raw: `{"schema_version":1,"graded_at":"x","scores":{"coherence":5,"conciseness":5,"signal_to_noise":5,"description_quality": 5, "tier_assignment": 5, "no_double_loading": 5}}`},
		{name: "score is a string", raw: `{"schema_version":1,"graded_at":"x","scores":{"coherence":"ok","conciseness":5,"signal_to_noise":5,"description_quality": 5, "tier_assignment": 5, "no_double_loading": 5},"overall":5}`},
		{name: "not JSON at all", raw: `Credit balance is too low`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := ValidateRaw(UnwrapForValidation([]byte(c.raw))); err == nil {
				t.Fatal("accepted a report that carries no real grade")
			}
		})
	}
}

func TestValidateRaw_AcceptsGenuineReport(t *testing.T) {
	raw := `{"schema_version":1,"graded_at":"2026-08-07T00:00:00Z","grader_model":"claude-opus-4-7",
	         "target_dir":".claude/rules/governance","scores":{"coherence":7,"conciseness":6,
	         "signal_to_noise":6,"description_quality": 7, "tier_assignment": 7, "no_double_loading": 7},"overall":6,"findings":[],"summary":"ok"}`
	if err := ValidateRaw(UnwrapForValidation([]byte(raw))); err != nil {
		t.Fatalf("rejected a valid report: %v", err)
	}
}

// A genuine report wrapped in a claude result envelope must validate on the
// INNER document. Validating the envelope would pass on a payload with no
// scores at all — the same blindness in a new place.
func TestValidateRaw_UnwrapsEnvelopeBeforeChecking(t *testing.T) {
	inner := `{\"schema_version\":1,\"graded_at\":\"x\",\"grader_model\":\"m\",\"target_dir\":\"d\",\"scores\":{\"coherence\":7,\"conciseness\":6,\"signal_to_noise\":6,\"description_quality\":7,\"tier_assignment\":7,\"no_double_loading\":7},\"overall\":6,\"findings\":[],\"summary\":\"ok\"}`
	env := `{"type":"result","subtype":"success","is_error":false,"result":"` + inner + `"}`
	if err := ValidateRaw(UnwrapForValidation([]byte(env))); err != nil {
		t.Fatalf("rejected a valid enveloped report: %v", err)
	}

	emptyEnv := `{"type":"result","subtype":"success","is_error":false,"result":"{}"}`
	if err := ValidateRaw(UnwrapForValidation([]byte(emptyEnv))); err == nil {
		t.Fatal("accepted an envelope whose inner report has no scores")
	}
}
