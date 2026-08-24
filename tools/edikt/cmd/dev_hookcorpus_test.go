package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// corpusFixtureRepo builds a minimal fake project the binary can be run inside.
//
// Four small files plus a two-file sandbox base tree. Deliberately not the real
// repo: these tests exec the binary, and running it against the real corpus
// would make them depend on 22 hooks that change for unrelated reasons.
func corpusFixtureRepo(t *testing.T, hookStatus string) string {
	t.Helper()
	root := t.TempDir()

	write := func(rel, body string, mode os.FileMode) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), mode); err != nil {
			t.Fatal(err)
		}
	}

	write(".edikt/config.yaml", "edikt_version: 0.8.0\n", 0o644)
	write("templates/hooks/registry.yaml", `schema_version: 1
hooks:
  - name: fixture-hook
    event: FileChanged
    file: fixture-hook.sh
    class: injector
    status: `+hookStatus+`
    python3_baseline_count: 0
`, 0o644)
	write("templates/hooks/fixture-hook.sh", `#!/usr/bin/env bash
cat >/dev/null
printf '{"at":"%s"}\n' "$(date -u +%Y-%m-%dT%H:%M:%SZ)"
mkdir -p "$HOME/.edikt"
printf 'fired\n' >> "$HOME/.edikt/events.jsonl"
`, 0o755)
	write("test/fixtures/hook-corpus/_base/project/.edikt/config.yaml", "edikt_version: 0.8.0\n", 0o644)
	write("test/fixtures/hook-corpus/_base/home/.keep", "", 0o644)
	write("test/fixtures/hook-corpus/fixture-hook/matrix.yaml", `schema_version: 1
hook: fixture-hook
scenarios:
- id: baseline
  axis:
  - payload
  payload: null
  config: default
  notes: Baseline.
`, 0o644)
	return root
}

func runCorpusCmd(t *testing.T, dir string, extraEnv []string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.Command(buildBinary(t), args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "EDIKT_SKIP_VERSION_GATE=1")
	cmd.Env = append(cmd.Env, extraEnv...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// TestHookCorpusDiffFailsOnMissingBinary — AC-7.12.
//
// A gate that cannot find the implementation it is gating has measured nothing.
// The one outcome that must be impossible here is a clean exit: "no binary, so
// no failures" is INV-013's exact shape, and it would let every hook in Phases
// 12-22 migrate past a gate that never ran.
func TestHookCorpusDiffFailsOnMissingBinary(t *testing.T) {
	root := corpusFixtureRepo(t, "pending")
	missing := filepath.Join(t.TempDir(), "definitely-not-here", "edikt")

	out, err := runCorpusCmd(t, root, []string{"EDIKT_BIN=" + missing},
		"dev", "hook-corpus", "diff", "--hook", "fixture-hook", "--live", "go", "--bin", missing)
	if err == nil {
		t.Fatalf("diff exited 0 with no resolvable binary:\n%s", out)
	}
	if !strings.Contains(out, "NOTHING WAS COMPARED") {
		t.Errorf("the failure does not say that nothing was compared:\n%s", out)
	}
	if !strings.Contains(out, missing) {
		t.Errorf("the failure does not name the paths it tried:\n%s", out)
	}
	for _, forbidden := range []string{"scenarios match", "PASS "} {
		if strings.Contains(out, forbidden) {
			t.Errorf("output claims a comparison happened (%q):\n%s", forbidden, out)
		}
	}
}

// TestDiffEvidenceLogRecordsWallClock — AC-7.9.
//
// The figure is recorded; no threshold is enforced against it. SPEC-014
// spec.md:265 declines to set a runtime-path budget for the corpus recorder, so
// a pass/fail gate here would cite a number the spec refuses to set.
func TestDiffEvidenceLogRecordsWallClock(t *testing.T) {
	root := corpusFixtureRepo(t, "pending")

	if out, err := runCorpusCmd(t, root, nil,
		"dev", "hook-corpus", "record", "--hook", "fixture-hook"); err != nil {
		t.Fatalf("record: %v\n%s", err, out)
	}
	out, err := runCorpusCmd(t, root, nil, "dev", "hook-corpus", "diff", "--hook", "fixture-hook")
	if err != nil {
		t.Fatalf("diff: %v\n%s", err, out)
	}

	body, rerr := os.ReadFile(filepath.Join(root, ".edikt", "state", "hook-corpus", "evidence.jsonl"))
	if rerr != nil {
		t.Fatalf("no evidence log written: %v\n%s", rerr, out)
	}
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one evidence line, got %d", len(lines))
	}
	var ev struct {
		Hook          string           `json:"hook"`
		Compared      int              `json:"scenarios_compared"`
		TotalMS       int64            `json:"total_wall_clock_ms"`
		PerScenarioMS map[string]int64 `json:"per_scenario_wall_clock_ms"`
		Informational string           `json:"informational"`
	}
	if err := json.Unmarshal([]byte(lines[0]), &ev); err != nil {
		t.Fatalf("evidence line is not JSON: %v\n%s", err, lines[0])
	}
	if ev.Hook != "fixture-hook" {
		t.Errorf("evidence names hook %q", ev.Hook)
	}
	if ev.Compared != 1 {
		t.Errorf("evidence records %d scenarios compared, want 1 — a denominator is what "+
			"distinguishes a real run from an empty one", ev.Compared)
	}
	if _, ok := ev.PerScenarioMS["baseline"]; !ok {
		t.Errorf("evidence carries no per-scenario wall clock: %+v", ev.PerScenarioMS)
	}
	if ev.TotalMS <= 0 {
		t.Errorf("total_wall_clock_ms = %d for a run that executed a subprocess", ev.TotalMS)
	}
	if !strings.Contains(ev.Informational, "no threshold") {
		t.Errorf("the evidence line does not say the figure is not a gate: %q", ev.Informational)
	}
	if !strings.Contains(out, "wall-clock recorded to") {
		t.Errorf("the run did not say where it recorded the figure:\n%s", out)
	}
}

// TestDevVerbNotInvokedFromTier1 — AC-7.13, decided the second way.
//
// SPEC-014 spec.md:150 rules that `dev` needs no tier2-verbs.yaml permit row
// because it is never invoked from tier-1 markdown. That is a claim about the
// repo, and test/test-permits.sh cannot keep it honest: it only scans commands/
// for verbs that ARE permitted, so it passes whether or not a `bin/edikt dev`
// call appears there. This is the assertion that would notice.
//
// Adding a permit row instead would be the wrong fix. A permit row does not
// document a verb, it AUTHORIZES tier-1 to call it — so the row would license
// exactly the call path this test exists to keep out of the repo.
func TestDevVerbNotInvokedFromTier1(t *testing.T) {
	repo, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	commands := filepath.Join(repo, "commands")
	if _, err := os.Stat(commands); err != nil {
		t.Fatalf("commands/ not found at %s: %v", commands, err)
	}

	scanned := 0
	var offenders []string
	err = filepath.Walk(commands, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		scanned++
		body, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		for _, needle := range []string{"bin/edikt dev ", "edikt dev link", "edikt dev unlink",
			"edikt dev hook-corpus"} {
			if bytes.Contains(body, []byte(needle)) {
				rel, _ := filepath.Rel(repo, path)
				offenders = append(offenders, rel+" contains "+needle)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned zero command files — this assertion measured nothing (INV-013)")
	}
	t.Logf("scanned %d tier-1 command file(s)", scanned)
	if len(offenders) > 0 {
		t.Fatalf(`tier-1 markdown invokes the dev verb group:

  %s

`+"`dev`"+` carries no row in templates/permits/tier2-verbs.yaml, and SPEC-014
spec.md:150 rules that it should not: it is developer tooling, not an
orchestration verb. Either remove the call, or open the ADR that makes dev a
permitted tier-1 verb — do not add the row to make this test pass.`,
			strings.Join(offenders, "\n  "))
	}
}
