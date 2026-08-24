package hookcorpus

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDiffGate — AC-7.5.
//
// The green case is the least interesting assertion here. What makes this a
// gate rather than a ritual is the four red cases: a moved exit code, a moved
// stdout byte, a moved filesystem effect, and a comparison over zero scenarios
// must each FAIL, and the failure must name the scenario.
func TestDiffGate(t *testing.T) {
	t.Run("identical implementation passes", func(t *testing.T) {
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		if _, err := rec.Record("fixture-hook", ""); err != nil {
			t.Fatalf("Record: %v", err)
		}
		var out bytes.Buffer
		rep, err := rec.Diff("fixture-hook", DiffOptions{}, &out)
		if err != nil {
			t.Fatalf("Diff: %v\n%s", err, out.String())
		}
		if !rep.OK() {
			t.Fatalf("an unchanged implementation failed its own recording: %s\n%s",
				rep.FirstFailure(), out.String())
		}
		if rep.Compared != 2 {
			t.Fatalf("compared %d scenarios, want 2", rep.Compared)
		}
		if rep.GateMeaningful {
			t.Error("a bash-against-bash run reported gate_meaningful=true — a self-diff is " +
				"evidence about this tool, not about a rewrite")
		}
	})

	t.Run("a changed exit code, stdout byte and fs effect all fail", func(t *testing.T) {
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		if _, err := rec.Record("fixture-hook", ""); err != nil {
			t.Fatalf("Record: %v", err)
		}
		env.swapHook(t, variantHookBody)

		var out bytes.Buffer
		rep, err := rec.Diff("fixture-hook", DiffOptions{}, &out)
		if err != nil {
			t.Fatalf("Diff returned an error rather than a failing report: %v", err)
		}
		if rep.OK() {
			t.Fatal("a replacement that changed exit code, stdout AND filesystem effect " +
				"passed the gate")
		}
		if rep.Failed != 2 {
			t.Errorf("%d of %d scenarios failed; both should have", rep.Failed, rep.Compared)
		}
		first := rep.FirstFailure()
		if !strings.HasPrefix(first, "baseline:") {
			t.Errorf("the first failure does not name the first failing scenario: %q", first)
		}
		if !strings.Contains(first, "exit code") {
			t.Errorf("the exit-code change is not what was reported first: %q", first)
		}
	})

	t.Run("a filesystem-only regression fails", func(t *testing.T) {
		// F-074's class: same exit code, same stdout, different side effect.
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
			t.Fatalf("Record: %v", err)
		}
		body := strings.Replace(fixtureHookBody,
			`echo "fixture-hook: ran" >&2`,
			"printf 'sneaky\\n' > .edikt/sneaky.txt\necho \"fixture-hook: ran\" >&2", 1)
		env.swapHook(t, body)

		var out bytes.Buffer
		rep, err := rec.Diff("fixture-hook", DiffOptions{Scenario: "baseline"}, &out)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if rep.OK() {
			t.Fatal("a side-effect-only regression passed — this is exactly the F-074 class " +
				"a stdout-only capture cannot see")
		}
		if !strings.Contains(rep.FirstFailure(), "filesystem effect") {
			t.Errorf("the failure does not name the filesystem difference: %q", rep.FirstFailure())
		}
	})

	t.Run("a stdout-only regression fails", func(t *testing.T) {
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
			t.Fatalf("Record: %v", err)
		}
		body := strings.Replace(fixtureHookBody, `"payload_bytes":%d}`, `"payload_bytes":%d,"x":1}`, 1)
		env.swapHook(t, body)
		var out bytes.Buffer
		rep, err := rec.Diff("fixture-hook", DiffOptions{Scenario: "baseline"}, &out)
		if err != nil {
			t.Fatalf("Diff: %v", err)
		}
		if rep.OK() {
			t.Fatal("a one-field stdout change passed the gate")
		}
		if !strings.Contains(rep.FirstFailure(), "stdout") {
			t.Errorf("the failure does not name stdout: %q", rep.FirstFailure())
		}
	})

	t.Run("zero scenarios is a failure, not a pass", func(t *testing.T) {
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		if _, err := rec.Record("fixture-hook", ""); err != nil {
			t.Fatal(err)
		}
		var out bytes.Buffer
		if _, err := rec.Diff("fixture-hook", DiffOptions{Scenario: "no-such-scenario"}, &out); err == nil {
			t.Fatal("a scenario filter matching nothing reported success — a comparison over " +
				"zero scenarios must fail (INV-013)")
		}

		empty := &DiffReport{Hook: "x", Compared: 0}
		if empty.OK() {
			t.Fatal("a report with zero comparisons and zero failures reported OK")
		}
	})

	t.Run("a missing recording is a refusal", func(t *testing.T) {
		env := newFixtureEnv(t)
		rec := env.recorder(t)
		var out bytes.Buffer
		if _, err := rec.Diff("fixture-hook", DiffOptions{}, &out); err == nil {
			t.Fatal("diffing against a corpus with no recordings reported success")
		}
	})
}

// TestDiffRefusesSelfDiffUnderRequireGo — a bash-against-bash run is trivially
// green. Phase 12+ gates pass --require-go so that greenness cannot be mistaken
// for migration evidence.
func TestDiffRefusesSelfDiffUnderRequireGo(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	_, err := rec.Diff("fixture-hook", DiffOptions{Scenario: "baseline", RequireGo: true}, &out)
	if err == nil {
		t.Fatal("--require-go accepted a bash-against-bash self-diff")
	}
	if !strings.Contains(err.Error(), "self-diff") {
		t.Fatalf("refusal does not name the reason: %v", err)
	}
}

// TestRecordingPinsInputHash — AC-7.6.
//
// Editing a payload fixture does not move a single byte of the recording. Without
// the pin, the gate would keep comparing against an oracle recorded from input
// that no longer exists — green, and meaningless, with nothing about the
// artifact looking stale.
func TestRecordingPinsInputHash(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
		t.Fatal(err)
	}
	r, err := LoadRecording(RecordingDir(env.corpusRoot, "fixture-hook", "baseline"))
	if err != nil {
		t.Fatal(err)
	}
	if len(r.ScenarioInputSHA256) != 64 {
		t.Fatalf("scenario_input_sha256 = %q, want a sha256", r.ScenarioInputSHA256)
	}
	if r.NormalizerVersion != Version {
		t.Fatalf("normalizer_version = %q, want %q", r.NormalizerVersion, Version)
	}

	t.Run("editing the payload fails the diff loudly", func(t *testing.T) {
		payload := filepath.Join(env.corpusRoot, "fixture-hook", "payloads", "basic.json")
		mustWrite(t, payload, `{"session_id":"ses_fixture_abc123","tool_input":{"file_path":"/tmp/edited.txt"}}`+"\n", 0o644)
		var out bytes.Buffer
		_, err := rec.Diff("fixture-hook", DiffOptions{Scenario: "baseline"}, &out)
		if err == nil {
			t.Fatal("the gate compared against a recording whose input no longer exists on disk")
		}
		if !strings.Contains(err.Error(), "inputs have changed") {
			t.Fatalf("refusal does not name the reason: %v", err)
		}
	})

	t.Run("editing the config fixture fails the diff too", func(t *testing.T) {
		env2 := newFixtureEnv(t)
		rec2 := env2.recorder(t)
		if _, err := rec2.Record("fixture-hook", "baseline"); err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(env2.baseTree, "project", ".edikt", "config.yaml"),
			"edikt_version: 0.8.0\nquality_gates: false\n", 0o644)
		var out bytes.Buffer
		if _, err := rec2.Diff("fixture-hook", DiffOptions{Scenario: "baseline"}, &out); err == nil {
			t.Fatal("the config fixture is part of the scenario's input and its edit went unnoticed")
		}
	})

	t.Run("a stale normalizer version is refused", func(t *testing.T) {
		env3 := newFixtureEnv(t)
		rec3 := env3.recorder(t)
		if _, err := rec3.Record("fixture-hook", "baseline"); err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(RecordingDir(env3.corpusRoot, "fixture-hook", "baseline"), "recording.json")
		body, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		edited := bytes.Replace(body, []byte(`"normalizer_version": "`+Version+`"`),
			[]byte(`"normalizer_version": "v0"`), 1)
		if bytes.Equal(edited, body) {
			t.Fatal("could not find normalizer_version in the recording to age it")
		}
		mustWrite(t, p, string(edited), 0o644)

		var out bytes.Buffer
		_, err = rec3.Diff("fixture-hook", DiffOptions{Scenario: "baseline"}, &out)
		if err == nil {
			t.Fatal("a recording normalized under a different pass list was compared anyway")
		}
		if !strings.Contains(err.Error(), "normalizer") {
			t.Fatalf("refusal does not name the reason: %v", err)
		}
	})
}

// TestCoverage — the four-axis table, and its ability to report a gap.
func TestCoverage(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)

	rep, err := rec.Coverage("fixture-hook")
	if err != nil {
		t.Fatal(err)
	}
	if rep.Examined != 4 {
		t.Fatalf("examined %d axes, want 4", rep.Examined)
	}
	if rep.OK() {
		t.Fatal("the fixture matrix covers only payload and environment, and no F-041 " +
			"scenario, but coverage reported clean")
	}
	gaps := rep.Gaps()
	joined := strings.Join(gaps, " ")
	for _, want := range []string{"axis config", "axis ordering", "F-041"} {
		if !strings.Contains(joined, want) {
			t.Errorf("gap list %v does not mention %q", gaps, want)
		}
	}

	t.Run("a library row is a category error, not a gap", func(t *testing.T) {
		if _, err := rec.Coverage("fixture-lib"); err == nil {
			t.Fatal("a class: library row was given a coverage verdict")
		}
	})

	t.Run("zero examined axes is not coverage", func(t *testing.T) {
		empty := &CoverageReport{Hook: "x", Examined: 0, Scenarios: 0}
		if empty.OK() {
			t.Fatal("a coverage report that examined zero axes reported OK (INV-013)")
		}
	})
}
