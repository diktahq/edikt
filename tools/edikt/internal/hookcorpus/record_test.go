package hookcorpus

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRecorder — AC-7.1. One scenario, one sandbox, everything captured.
func TestRecorder(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)

	res, err := rec.Record("fixture-hook", "")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(res.Scenarios) != 2 {
		t.Fatalf("recorded %d scenarios, want 2 (an empty or partial record run must not "+
			"read as success)", len(res.Scenarios))
	}
	if res.Oracle != OracleBash {
		t.Fatalf("oracle %q, want %q — the registry marks this row pending", res.Oracle, OracleBash)
	}

	dir := RecordingDir(env.corpusRoot, "fixture-hook", "baseline")
	for _, name := range []string{"recording.json", "stdout.raw", "stderr.raw", "tree-before.json", "tree-after.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("missing artifact %s: %v", name, err)
		}
	}

	r, err := LoadRecording(dir)
	if err != nil {
		t.Fatalf("LoadRecording: %v", err)
	}
	if r.ExitCode != 0 {
		t.Errorf("baseline exit code %d, want 0", r.ExitCode)
	}
	if r.StdoutSHA256 == "" || r.StderrSHA256 == "" {
		t.Error("stdout/stderr hashes are empty")
	}
	if r.StdoutBytes == 0 {
		t.Error("stdout_bytes is 0 for a hook that printed a line")
	}
	if len(r.FSDiff.Added) == 0 {
		t.Error("fs_diff records no added file, but the hook wrote two")
	}

	// The exit-code axis must actually move, or nothing downstream proves that
	// exit codes are compared rather than assumed.
	other, err := LoadRecording(RecordingDir(env.corpusRoot, "fixture-hook", "env-exit-two"))
	if err != nil {
		t.Fatalf("LoadRecording(env-exit-two): %v", err)
	}
	if other.ExitCode != 2 {
		t.Errorf("env-exit-two exit code %d, want 2", other.ExitCode)
	}

	// stdout.raw must be the pre-normalization bytes: the hash is over the
	// masked form, and a reviewer needs to see what was masked.
	raw, err := os.ReadFile(filepath.Join(dir, "stdout.raw"))
	if err != nil {
		t.Fatalf("stdout.raw: %v", err)
	}
	if !strings.Contains(string(raw), `"at":"20`) {
		t.Errorf("stdout.raw does not carry the raw timestamp: %q", raw)
	}
}

// TestRecorderHomeOverride — AC-7.2. HOME is redirected into the sandbox, and
// the fake home is part of the SAME fs_diff as the project tree.
//
// The second half is the one that matters. session-start.sh truncates
// $HOME/.edikt/gate-overrides.jsonl and six hooks append under $HOME/.edikt/; a
// diff that only watched the project tree would call every one of those "no
// observable effect" — which is F-074's entire class of finding.
func TestRecorderHomeOverride(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
		t.Fatalf("Record: %v", err)
	}
	r, err := LoadRecording(RecordingDir(env.corpusRoot, "fixture-hook", "baseline"))
	if err != nil {
		t.Fatalf("LoadRecording: %v", err)
	}

	var addedHome, addedProject bool
	for _, a := range r.FSDiff.Added {
		switch a.Path {
		case "home/.edikt/events.jsonl":
			addedHome = true
		case "project/.edikt/last-fire.txt":
			addedProject = true
		}
	}
	if !addedHome {
		t.Errorf("the hook's $HOME append is not in fs_diff.added: %+v", r.FSDiff.Added)
	}
	if !addedProject {
		t.Errorf("the hook's project write is not in fs_diff.added: %+v", r.FSDiff.Added)
	}

	var truncated bool
	for _, m := range r.FSDiff.Modified {
		if m.Path == "home/.edikt/gate-overrides.jsonl" {
			truncated = true
		}
	}
	if !truncated {
		t.Errorf("the hook's $HOME truncation (the session-start.sh:28 shape) is not in "+
			"fs_diff.modified: %+v", r.FSDiff.Modified)
	}

	// And the redirect itself: the recorded stdout carries the SANDBOX home,
	// masked to a placeholder, never a host path.
	raw, err := os.ReadFile(filepath.Join(RecordingDir(env.corpusRoot, "fixture-hook", "baseline"), "stdout.raw"))
	if err != nil {
		t.Fatal(err)
	}
	if home := os.Getenv("HOME"); home != "" && strings.Contains(string(raw), home) {
		t.Fatalf("stdout.raw carries the ambient HOME %q — the sandbox did not redirect it", home)
	}
}

// TestRecordLeavesHomeUntouched — AC-7.3.
//
// The dangerous version of this test points at the maintainer's real home and
// hopes. This one substitutes a populated stand-in for the AMBIENT home first,
// so a recorder that failed to redirect HOME would corrupt the stand-in and be
// caught, rather than truncating the developer's real gate-overrides file on
// the way to reporting a bug. The genuine ~/.edikt is additionally hashed
// read-only, before and after, because the claim is about that directory.
func TestRecordLeavesHomeUntouched(t *testing.T) {
	env := newFixtureEnv(t)

	ambient := t.TempDir()
	mustWrite(t, filepath.Join(ambient, ".edikt", "gate-overrides.jsonl"), "{\"real\":true}\n", 0o644)
	mustWrite(t, filepath.Join(ambient, ".edikt", "events.jsonl"), "pre-existing\n", 0o644)
	mustWrite(t, filepath.Join(ambient, ".claude", "projects", "p", "memory", "MEMORY.md"),
		"private auto-memory content\n", 0o644)
	t.Setenv("HOME", ambient)

	realHome, _ := os.UserHomeDir()
	realEdikt := filepath.Join(realHome, ".edikt")
	beforeReal := hashRealEdiktIfSmall(t, realEdikt)

	before := hashHomeTree(t, ambient)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-hook", ""); err != nil {
		t.Fatalf("Record: %v", err)
	}
	after := hashHomeTree(t, ambient)

	if before != after {
		t.Fatalf("the ambient $HOME was mutated by a record run.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if before == "" {
		t.Fatal("the ambient home hashed to nothing — this assertion measured no files (INV-013)")
	}

	afterReal := hashRealEdiktIfSmall(t, realEdikt)
	if beforeReal != afterReal {
		t.Fatalf("the real %s changed across a record run", realEdikt)
	}
}

// hashRealEdiktIfSmall hashes the genuine ~/.edikt when it is small enough to
// walk cheaply. Returns a sentinel — never "" — for absent or too-large, so an
// unwalkable directory cannot silently look identical to a walked one.
func hashRealEdiktIfSmall(t *testing.T, dir string) string {
	t.Helper()
	if _, err := os.Stat(dir); err != nil {
		return "absent"
	}
	count := 0
	err := filepath.Walk(dir, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info != nil && !info.IsDir() {
			count++
		}
		if count > 2000 {
			return filepath.SkipAll
		}
		return nil
	})
	if err != nil || count > 2000 {
		return "not-walked-too-large"
	}
	return hashHomeTree(t, dir)
}

// TestWallClockRecorded — AC-7.7.
func TestWallClockRecorded(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)
	res, err := rec.Record("fixture-hook", "baseline")
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if _, ok := res.WallClock["baseline"]; !ok {
		t.Fatal("the record result carries no wall-clock for the scenario it ran")
	}

	body, err := os.ReadFile(filepath.Join(RecordingDir(env.corpusRoot, "fixture-hook", "baseline"), "recording.json"))
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(body, &decoded); err != nil {
		t.Fatal(err)
	}
	v, ok := decoded["wall_clock_ms"]
	if !ok {
		t.Fatal("recording.json has no wall_clock_ms key — the figure would then exist only in " +
			"the run that produced it")
	}
	if n, isNum := v.(float64); !isNum || n < 0 {
		t.Fatalf("wall_clock_ms = %v, want a non-negative number", v)
	}

	// A hook that measurably sleeps must produce a measurably non-zero figure,
	// or the field is present but not actually measuring anything.
	mustWrite(t, filepath.Join(env.corpusRoot, "fixture-slow", "matrix.yaml"),
		"schema_version: 1\nhook: fixture-slow\nscenarios:\n- id: slow\n  axis:\n  - payload\n  payload: null\n  config: default\n", 0o644)
	slowRes, err := rec.Record("fixture-slow", "")
	if err != nil {
		t.Fatalf("Record(fixture-slow): %v", err)
	}
	if slowRes.WallClock["slow"] <= 0 {
		t.Errorf("a hook that sleeps 50ms recorded %dms — the clock is not measuring the run",
			slowRes.WallClock["slow"])
	}
}

// TestRecorderNeverWalksProjectRoot — AC-7.8.
//
// Two halves. First: a record run's filesystem operations stay inside the
// fixture tree and the sandbox, and stay bounded. Second, and stronger: a base
// tree that LOOKS like a live project is refused before it is walked at all —
// .edikt/state in this repo is 214 MB across 26,089 files, so "the recorder
// walks the project root" is not a style complaint, it is 26k stat calls per
// scenario and a recording that depends on one developer's local state.
func TestRecorderNeverWalksProjectRoot(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
		t.Fatalf("Record: %v", err)
	}

	if len(env.traced) == 0 {
		t.Fatal("the trace recorded no filesystem operations — this assertion examined nothing " +
			"(INV-013)")
	}
	realRepo, err := filepath.Abs("../../../..")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range env.traced {
		parts := strings.SplitN(op, " ", 2)
		path := parts[1]
		if strings.HasPrefix(path, filepath.Join(realRepo, ".edikt")) ||
			strings.HasPrefix(path, filepath.Join(realRepo, "docs")) ||
			strings.HasPrefix(path, filepath.Join(realRepo, ".git")) {
			t.Fatalf("the recorder touched the real project root: %s", op)
		}
	}
	// Bounded: the fixture tree is four files, so a record run that walked
	// something large would blow past this by orders of magnitude.
	if len(env.traced) > 400 {
		t.Errorf("a single scenario performed %d filesystem operations over a 4-file fixture "+
			"tree — the sandbox is not bounded", len(env.traced))
	}

	// A base tree carrying a live project's markers is refused, and refused
	// BEFORE the walk: the trace must show no copy of any file out of it.
	decoy := t.TempDir()
	mustMkdir(t, filepath.Join(decoy, ".git"))
	mustMkdir(t, filepath.Join(decoy, ".edikt", "state"))
	mustMkdir(t, filepath.Join(decoy, "project"))
	for i := 0; i < 5; i++ {
		mustWrite(t, filepath.Join(decoy, ".edikt", "state", "f"+string(rune('a'+i))), "x", 0o644)
	}
	var decoyTrace []string
	opts := env.options(t)
	opts.BaseTree = decoy
	opts.Trace = func(op, path string) { decoyTrace = append(decoyTrace, op+" "+path) }
	rec2, err := NewRecorder(opts)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rec2.Record("fixture-hook", "baseline"); err == nil {
		t.Fatal("the recorder accepted a base tree carrying .git and .edikt/state")
	} else if !strings.Contains(err.Error(), "live project") {
		t.Fatalf("refusal does not name the reason: %v", err)
	}
	for _, op := range decoyTrace {
		if strings.HasPrefix(op, "copy-file ") || strings.HasPrefix(op, "walk ") {
			t.Fatalf("the recorder walked or copied out of the refused tree: %s", op)
		}
	}

	// And there is no default that could reach the project root: an empty base
	// tree is an error, not a fallback.
	if err := ValidateBaseTree("", nil); err == nil {
		t.Fatal("an empty base tree was accepted — a fallback here is how the project root " +
			"gets walked by accident")
	}
}

// TestBaseTreeBudget — the file budget is a real refusal, not a comment.
func TestBaseTreeBudget(t *testing.T) {
	big := t.TempDir()
	mustMkdir(t, filepath.Join(big, "project"))
	for i := 0; i < MaxBaseTreeFiles+10; i++ {
		mustWrite(t, filepath.Join(big, "project", "f"+string(rune('a'+i%26))+string(rune('a'+i/26))), "x", 0o644)
	}
	err := ValidateBaseTree(big, nil)
	if err == nil {
		t.Fatal("an over-budget base tree was accepted")
	}
	if !strings.Contains(err.Error(), "budget") {
		t.Fatalf("refusal does not name the budget: %v", err)
	}
}

// TestOracleIsDerivedNotAsserted — the anti-gaming rule at the heart of the
// whole apparatus. A recording claiming `oracle: bash` must be impossible to
// produce against anything but a bash body.
func TestOracleIsDerivedNotAsserted(t *testing.T) {
	env := newFixtureEnv(t)
	rec := env.recorder(t)

	t.Run("migrated row refuses a bash recording", func(t *testing.T) {
		row, err := rec.Registry().Row("fixture-migrated")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rec.resolveImplementation(row, OracleBash); err == nil {
			t.Fatal("a migrated row was allowed to produce a bash-oracle recording — the gate " +
				"would then compare Go against Go")
		} else if !strings.Contains(err.Error(), "migrated") {
			t.Fatalf("refusal does not name the reason: %v", err)
		}
	})

	t.Run("a shim body refuses a bash recording", func(t *testing.T) {
		row, err := rec.Registry().Row("fixture-shim")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rec.resolveImplementation(row, OracleBash); err == nil {
			t.Fatal("the FR-004 exec shim was accepted as a bash oracle")
		} else if !strings.Contains(err.Error(), "shim") {
			t.Fatalf("refusal does not name the reason: %v", err)
		}
	})

	t.Run("a library row has no corpus at all", func(t *testing.T) {
		row, err := rec.Registry().Row("fixture-lib")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := rec.resolveImplementation(row, "auto"); err == nil {
			t.Fatal("a class: library row was accepted as a corpus subject")
		}
	})

	t.Run("the recording pins the executed bytes", func(t *testing.T) {
		if _, err := rec.Record("fixture-hook", "baseline"); err != nil {
			t.Fatal(err)
		}
		r, err := LoadRecording(RecordingDir(env.corpusRoot, "fixture-hook", "baseline"))
		if err != nil {
			t.Fatal(err)
		}
		want, err := fileDigest(filepath.Join(env.hooksDir, "fixture-hook.sh"))
		if err != nil {
			t.Fatal(err)
		}
		if r.OracleSHA256 != want {
			t.Fatalf("oracle_sha256 = %s, want the sha256 of the executed .sh (%s) — without "+
				"this a reviewer cannot confirm the oracle claim against git history",
				r.OracleSHA256, want)
		}
	})
}

// TestLeakGuardRefusesHostPaths — the artifact-level backstop on INV-007.
func TestLeakGuardRefusesHostPaths(t *testing.T) {
	g := NewLeakGuard("/Users/someone", "")
	if err := g.Check("stdout.raw", []byte("all clean here")); err != nil {
		t.Fatalf("clean bytes were refused: %v", err)
	}
	err := g.Check("stdout.raw", []byte(`{"memory":"/Users/someone/.claude/projects/x/memory/MEMORY.md"}`))
	if err == nil {
		t.Fatal("a host path in a to-be-committed artifact was accepted")
	}
	if !strings.Contains(err.Error(), "/Users/someone") {
		t.Fatalf("refusal does not name the leaked path: %v", err)
	}
}

// TestRegistryReaderAgreesWithFile — SSP-002 costs this package a second
// registry reader, so the two must be pinned to the same file rather than to
// each other.
func TestRegistryReaderAgreesWithFile(t *testing.T) {
	path, err := filepath.Abs("../../../../templates/hooks/registry.yaml")
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Skipf("real registry not readable: %v", err)
	}
	reg, err := LoadRegistry(path)
	if err != nil {
		t.Fatalf("LoadRegistry: %v", err)
	}
	want := strings.Count(string(body), "\n  - name:")
	if want == 0 {
		t.Fatal("counted zero rows in the registry file — this assertion measured nothing")
	}
	if len(reg.Hooks) != want {
		t.Fatalf("reader parsed %d rows; the file declares %d", len(reg.Hooks), want)
	}
	for _, r := range reg.Hooks {
		if r.Status != StatusPending && r.Status != StatusMigrated {
			t.Errorf("row %q has status %q", r.Name, r.Status)
		}
	}
}

// TestEmptyMatrixIsNotCoverage — INV-013 at the matrix reader.
func TestEmptyMatrixIsNotCoverage(t *testing.T) {
	env := newFixtureEnv(t)
	mustWrite(t, filepath.Join(env.corpusRoot, "fixture-slow", "matrix.yaml"),
		"schema_version: 1\nhook: fixture-slow\nscenarios: []\n", 0o644)
	rec := env.recorder(t)
	if _, err := rec.Record("fixture-slow", ""); err == nil {
		t.Fatal("an empty matrix recorded successfully — zero scenarios must not read as a corpus")
	}
}
