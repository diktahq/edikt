package cmd

// hook_soak_test.go — CLI-level wiring for `bin/edikt hook soak`
// (PLAN-v080-reliability-core Phase 15/19). The gate LOGIC's six criteria
// are proven exhaustively in tools/edikt/internal/hooksoak; this file proves
// the command actually wires flags to that package correctly, that its
// --json output has the field names hook-soak.v1 expects, and that its exit
// codes are distinguishable (0 pass / 1 refused / 3 usage error) end to end
// through the real built binary — including one real git repository, so
// "elapsed days come from git, not a state file" is proven at the command
// layer too, not only inside hooksoak's own unit tests.

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// soakProject creates a throwaway git repo with a minimal, valid
// hook-registry.v1 registry (two migrated injectors) committed to it, and
// chdirs the test into it. Returns the repo dir and the migration commit's
// hash.
func soakProject(t *testing.T) (dir, commit string) {
	t.Helper()
	// Isolate HOME so this test's output is never contaminated by a
	// version-pin warning sourced from whatever edikt install happens to be
	// active on the machine actually running the test (INV-007: no test
	// depends on ambient, machine-local state). Without this, `hook soak
	// --json`'s stdout is exactly what a real developer machine with a
	// mismatched pin would see — a stderr banner is fine there, but
	// runCmd() below captures stdout+stderr combined, so the assertions in
	// this file need a clean run to parse.
	t.Setenv("HOME", t.TempDir())
	dir = t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q")
	run("config", "user.email", "hook-soak-cmd-test@example.invalid")
	run("config", "user.name", "hook soak cmd test")

	regDir := filepath.Join(dir, ".edikt", "hooks")
	if err := os.MkdirAll(regDir, 0o755); err != nil {
		t.Fatal(err)
	}
	registry := `schema_version: 1
hooks:
  - name: session-start
    event: SessionStart
    file: session-start.sh
    class: injector
    status: migrated
    python3_baseline_count: 0
  - name: cwd-changed
    event: CwdChanged
    file: cwd-changed.sh
    class: injector
    status: migrated
    python3_baseline_count: 0
  - name: stop-hook
    event: Stop
    file: stop-hook.sh
    class: multi-call
    status: pending
    python3_baseline_count: 14
`
	if err := os.WriteFile(filepath.Join(regDir, "registry.yaml"), []byte(registry), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".edikt", "config.yaml"), []byte("edikt_version: \"0.8.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-q", "-m", "migrate group 1")

	hashCmd := exec.Command("git", "rev-parse", "HEAD")
	hashCmd.Dir = dir
	out, err := hashCmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse: %v", err)
	}
	commit = strings.TrimSpace(string(out))

	t.Chdir(dir)
	return dir, commit
}

func writeJournal(t *testing.T, path string, lines ...string) {
	t.Helper()
	body := strings.Join(lines, "\n")
	if len(lines) > 0 {
		body += "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// ── usage errors (exit 3), distinguishable from a refusal (exit 1) ────────

func TestHookSoak_BadGroup_ExitsUsageError(t *testing.T) {
	soakProject(t)
	_, err := runCmd(t, "hook", "soak", "--group", "3", "--since-commit", "abc1234")
	if !isExitCode(err, 3) {
		t.Fatalf("group 3: err = %v, want exit 3", err)
	}
}

func TestHookSoak_MissingSinceCommit_ExitsUsageError(t *testing.T) {
	soakProject(t)
	_, err := runCmd(t, "hook", "soak", "--group", "1")
	if !isExitCode(err, 3) {
		t.Fatalf("missing --since-commit: err = %v, want exit 3", err)
	}
}

func TestHookSoak_MalformedSinceCommit_ExitsUsageError(t *testing.T) {
	soakProject(t)
	_, err := runCmd(t, "hook", "soak", "--group", "1", "--since-commit", "not-a-hash!!")
	if !isExitCode(err, 3) {
		t.Fatalf("malformed --since-commit: err = %v, want exit 3", err)
	}
}

func TestHookSoak_UnresolvableSinceCommit_ExitsUsageError(t *testing.T) {
	soakProject(t)
	_, err := runCmd(t, "hook", "soak", "--group", "1", "--since-commit", "deadbeef0000")
	if !isExitCode(err, 3) {
		t.Fatalf("unresolvable --since-commit: err = %v, want exit 3", err)
	}
}

// ── refusal (exit 1), distinguishable from a usage error (exit 3) ─────────

func TestHookSoak_AbsentJournal_ExitsRefused_JSONShapeMatchesHookSoakV1(t *testing.T) {
	dir, commit := soakProject(t)
	journalPath := filepath.Join(dir, "does-not-exist.jsonl")

	out, _, err := runCmdSplit(t, "hook", "soak", "--group", "1", "--since-commit", commit,
		"--journal", journalPath, "--json")
	if !isExitCode(err, 1) {
		t.Fatalf("absent journal: err = %v, want exit 1 (refused, not a usage error)\noutput:\n%s", err, out)
	}

	var rec map[string]any
	if jerr := json.Unmarshal([]byte(out), &rec); jerr != nil {
		t.Fatalf("--json output did not decode as JSON: %v\noutput:\n%s", jerr, out)
	}
	// hook-soak.v1's own required top-level fields.
	for _, field := range []string{"schema_version", "group", "since_commit", "elapsed_days", "hooks", "status"} {
		if _, ok := rec[field]; !ok {
			t.Fatalf("--json output missing required field %q:\n%s", field, out)
		}
	}
	if rec["status"] != "refused" {
		t.Fatalf("status = %v, want refused", rec["status"])
	}
	if rec["schema_version"] != float64(1) {
		t.Fatalf("schema_version = %v, want 1", rec["schema_version"])
	}
	reasons, _ := rec["refusal_reasons"].([]any)
	if len(reasons) == 0 {
		t.Fatalf("status refused but refusal_reasons is empty (hook-soak.v1 requires minItems:1 when refused):\n%s", out)
	}
	hooks, _ := rec["hooks"].([]any)
	if len(hooks) != 2 {
		t.Fatalf("hooks array has %d entries, want 2 (session-start, cwd-changed from the fixture registry)", len(hooks))
	}
	for _, h := range hooks {
		entry := h.(map[string]any)
		if entry["journal_status"] != "unknown" {
			t.Fatalf("hook %v journal_status = %v, want unknown for an absent journal file", entry["name"], entry["journal_status"])
		}
		if entry["invocation_count"] != float64(0) {
			t.Fatalf("hook %v invocation_count = %v, want the documented 0 placeholder", entry["name"], entry["invocation_count"])
		}
	}
}

func TestHookSoak_JournalMissingOneHook_ExitsRefused_NamesHook(t *testing.T) {
	dir, commit := soakProject(t)
	journalPath := filepath.Join(dir, "journal.jsonl")
	writeJournal(t, journalPath,
		`{"kind":"hook_run","hook":"session-start","outcome":"hook-ran"}`,
		// cwd-changed never appears.
	)

	out, _, err := runCmdSplit(t, "hook", "soak", "--group", "1", "--since-commit", commit, "--journal", journalPath)
	if !isExitCode(err, 1) {
		t.Fatalf("err = %v, want exit 1 (refused)\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "cwd-changed") {
		t.Fatalf("refusal output does not name cwd-changed:\n%s", out)
	}
}

// ── pass (exit 0) — the positive control every refusal test above needs ───

func TestHookSoak_AllHooksClean_ExitsPass(t *testing.T) {
	dir, commit := soakProject(t)
	journalPath := filepath.Join(dir, "journal.jsonl")
	writeJournal(t, journalPath,
		`{"kind":"hook_run","hook":"session-start","outcome":"hook-ran"}`,
		`{"kind":"hook_run","hook":"cwd-changed","outcome":"hook-ran"}`,
	)

	out, _, err := runCmdSplit(t, "hook", "soak", "--group", "1", "--since-commit", commit,
		"--journal", journalPath, "--json")
	if err != nil {
		t.Fatalf("expected exit 0, got err = %v\noutput:\n%s", err, out)
	}
	var rec map[string]any
	if jerr := json.Unmarshal([]byte(out), &rec); jerr != nil {
		t.Fatalf("--json output did not decode: %v\n%s", jerr, out)
	}
	if rec["status"] != "pass" {
		t.Fatalf("status = %v, want pass", rec["status"])
	}
	if _, ok := rec["refusal_reasons"]; ok {
		t.Fatalf("a passing record must not carry refusal_reasons: %v", rec["refusal_reasons"])
	}
}

func TestHookSoak_PanicRecovered_ExitsRefused(t *testing.T) {
	dir, commit := soakProject(t)
	journalPath := filepath.Join(dir, "journal.jsonl")
	writeJournal(t, journalPath,
		`{"kind":"hook_run","hook":"session-start","outcome":"panic-recovered"}`,
		`{"kind":"hook_run","hook":"cwd-changed","outcome":"hook-ran"}`,
	)

	out, _, err := runCmdSplit(t, "hook", "soak", "--group", "1", "--since-commit", commit, "--journal", journalPath)
	if !isExitCode(err, 1) {
		t.Fatalf("err = %v, want exit 1 (refused)\noutput:\n%s", err, out)
	}
	if !strings.Contains(out, "panic-recovered") {
		t.Fatalf("refusal output does not name the panic outcome:\n%s", out)
	}
}

// ── elapsed_days comes from the real git commit, not a state file ─────────

func TestHookSoak_ElapsedDays_FromRealGitCommit(t *testing.T) {
	dir, commit := soakProject(t)
	journalPath := filepath.Join(dir, "journal.jsonl")
	writeJournal(t, journalPath,
		`{"kind":"hook_run","hook":"session-start","outcome":"hook-ran"}`,
		`{"kind":"hook_run","hook":"cwd-changed","outcome":"hook-ran"}`,
	)

	out, _, err := runCmdSplit(t, "hook", "soak", "--group", "1", "--since-commit", commit,
		"--journal", journalPath, "--json")
	if err != nil {
		t.Fatalf("err = %v\noutput:\n%s", err, out)
	}
	var rec map[string]any
	if jerr := json.Unmarshal([]byte(out), &rec); jerr != nil {
		t.Fatalf("decode: %v", jerr)
	}
	// The fixture commit was just made, so elapsed_days must be 0 — proving
	// the number reflects the commit this run actually resolved, not a
	// hardcoded or cached figure.
	if rec["elapsed_days"] != float64(0) {
		t.Fatalf("elapsed_days = %v, want 0 for a commit made moments ago", rec["elapsed_days"])
	}
	if rec["since_commit"] != commit {
		t.Fatalf("since_commit = %v, want %v", rec["since_commit"], commit)
	}
}
