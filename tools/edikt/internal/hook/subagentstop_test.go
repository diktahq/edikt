package hook

// subagentstop_test.go — the branches
// test/fixtures/hook-corpus/subagent-stop/matrix.yaml states its recordings
// cannot reach, each with a POSITIVE CONTROL beside it.
//
// WHY THE CONTROLS ARE NOT OPTIONAL. A test that only asserts a refusal passes
// just as well if the code starts refusing everything: "did not block" is the
// same observation whether the guard fired or the gate logic broke outright.
// Every case below therefore comes in a pair — the branch under test, and a
// neighbouring input that MUST produce a different, named outcome.

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// newSubagentStopInvocation builds an invocation carrying a structured
// evaluator verdict, the shape the gate path actually reads.
func newSubagentStopInvocation(agent, severity, finding string) *Invocation {
	return &Invocation{
		Name:   "subagent-stop",
		Stderr: io.Discard,
		Payload: adapter.Payload{
			EvaluatorAgent:    agent,
			EvaluatorSeverity: severity,
			EvaluatorFinding:  finding,
		},
	}
}

// subagentStopSandbox chdirs into a fresh project whose config gates the
// security agent at `critical`, points HOME at a second temp dir, and puts a
// PATH carrying python3 and nothing else.
//
// The python3 stub matters: the preflight runs before every branch tested
// here, and an empty PATH would short-circuit all of them into the
// missing-interpreter banner, which is a different branch entirely.
func subagentStopSandbox(t *testing.T) (root, home string) {
	t.Helper()
	root = t.TempDir()
	t.Chdir(root)
	home = t.TempDir()
	t.Setenv("HOME", home)

	stubDir := t.TempDir()
	writeExecutable(t, filepath.Join(stubDir, "python3"), "#!/bin/sh\nexit 0\n")
	t.Setenv("PATH", stubDir)
	t.Setenv("EDIKT_DISABLE_HOOK_DEDUP", "1")
	t.Setenv("EDIKT_GATE_SEVERITY_THRESHOLD", "")
	t.Setenv("EDIKT_PROJECT_ROOT", "")

	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\ngates:\n  security: critical\n")
	return root, home
}

// subagentStopBlocked reads the one fact every case here turns on.
func subagentStopBlocked(r Result) bool { return r.Response.Block }

// subagentStopLedgerLines returns the events.jsonl lines written so far.
func subagentStopLedgerLines(t *testing.T, home string) []string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(home, ".edikt", "events.jsonl"))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(l) != "" {
			out = append(out, l)
		}
	}
	return out
}

// ── branch 1: the missing-config guard ──────────────────────────────────────

func TestSubagentStopNoConfigIsSilent(t *testing.T) {
	root, _ := subagentStopSandbox(t)

	// Positive control FIRST, so a later "silent" result cannot be explained
	// by the gate being broken in this environment.
	control := subagentStop(newSubagentStopInvocation("security", "critical", "hardcoded credential"))
	if !subagentStopBlocked(control) {
		t.Fatalf("control: with a config present a critical finding must block; got %+v", control.Response)
	}

	if err := os.Remove(filepath.Join(root, ".edikt", "config.yaml")); err != nil {
		t.Fatalf("remove config: %v", err)
	}
	got := subagentStop(newSubagentStopInvocation("security", "critical", "hardcoded credential"))
	if !got.Response.IsZero() {
		t.Fatalf("with no .edikt/config.yaml the hook must write nothing at all; got %+v", got.Response)
	}
	if got.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0 — a hook outside an edikt project is not an error", got.ExitCode)
	}
}

// ── branch 2: a gate override that MATCHES ──────────────────────────────────

func TestSubagentStopGateOverrideSuppresses(t *testing.T) {
	const finding = "hardcoded credential in config.yaml"

	cases := []struct {
		name       string
		record     string
		wantBlock  bool
		wantReason string
	}{
		{
			name:       "no override file at all",
			record:     "",
			wantBlock:  true,
			wantReason: "the control: nothing suppresses a critical finding by default",
		},
		{
			name:       "exact agent and finding_prefix",
			record:     `{"agent":"security","finding_prefix":"` + finding + `"}`,
			wantBlock:  false,
			wantReason: "an accepted finding must not be re-blocked",
		},
		{
			name:       "same agent, different finding",
			record:     `{"agent":"security","finding_prefix":"a completely different finding"}`,
			wantBlock:  true,
			wantReason: "an override is per finding, not per agent",
		},
		{
			name:       "same finding, different agent",
			record:     `{"agent":"dba","finding_prefix":"` + finding + `"}`,
			wantBlock:  true,
			wantReason: "one agent's override must not silence another's finding",
		},
		{
			name:       "both fields on SEPARATE records",
			record:     `{"agent":"security"}` + "\n" + `{"finding_prefix":"` + finding + `"}`,
			wantBlock:  true,
			wantReason: "matching is same-record, never same-file",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, home := subagentStopSandbox(t)
			if tc.record != "" {
				writeTestFile(t, filepath.Join(home, ".edikt", "gate-overrides.jsonl"), tc.record+"\n")
			}
			got := subagentStop(newSubagentStopInvocation("security", "critical", finding))
			if blocked := subagentStopBlocked(got); blocked != tc.wantBlock {
				t.Fatalf("blocked = %v, want %v — %s (response %+v)", blocked, tc.wantBlock, tc.wantReason, got.Response)
			}
			// A suppressed block must ALSO leave no ledger record: an
			// override that replied "continue" while still writing
			// gate_fired would report a block that never happened.
			lines := subagentStopLedgerLines(t, home)
			if !tc.wantBlock && len(lines) != 0 {
				t.Fatalf("suppressed by an override but wrote %d ledger line(s): %v", len(lines), lines)
			}
			if tc.wantBlock && len(lines) == 0 {
				t.Fatalf("the gate fired but wrote no ledger record — %s", tc.wantReason)
			}
		})
	}
}

// TestSubagentStopOverrideNonObjectRecordDoesNotSuppress pins the ONE
// documented divergence in this handler's override scan.
//
// The bash raises AttributeError on a line that is valid JSON but not an
// object, which escapes its loop and leaves MATCHED empty — so the gate
// fires. Go reaches the same verdict without the traceback, and, critically,
// does NOT go on to honour a later matching record: a malformed overrides
// file must never be able to suppress a block the bash would have emitted.
func TestSubagentStopOverrideNonObjectRecordDoesNotSuppress(t *testing.T) {
	const finding = "hardcoded credential in config.yaml"
	match := `{"agent":"security","finding_prefix":"` + finding + `"}`

	// Control: the matching record ALONE suppresses, so the failure below is
	// attributable to the non-object line and not to a broken matcher.
	_, home := subagentStopSandbox(t)
	writeTestFile(t, filepath.Join(home, ".edikt", "gate-overrides.jsonl"), match+"\n")
	if control := subagentStop(newSubagentStopInvocation("security", "critical", finding)); subagentStopBlocked(control) {
		t.Fatalf("control: a matching override alone must suppress the block; got %+v", control.Response)
	}

	// A bare JSON scalar ahead of it stops the scan.
	_, home = subagentStopSandbox(t)
	writeTestFile(t, filepath.Join(home, ".edikt", "gate-overrides.jsonl"), "5\n"+match+"\n")
	if got := subagentStop(newSubagentStopInvocation("security", "critical", finding)); !subagentStopBlocked(got) {
		t.Fatalf("a valid-JSON non-object record must abandon the scan and let the gate fire; got %+v", got.Response)
	}

	// A line that is not JSON at all is skipped, exactly as
	// `except json.JSONDecodeError: continue` does — so the match still wins.
	_, home = subagentStopSandbox(t)
	writeTestFile(t, filepath.Join(home, ".edikt", "gate-overrides.jsonl"), "not json at all\n"+match+"\n")
	if got := subagentStop(newSubagentStopInvocation("security", "critical", finding)); subagentStopBlocked(got) {
		t.Fatalf("an undecodable line must be skipped, not treated as a stop; got %+v", got.Response)
	}
}

// ── branch 3: the project root (AC-18.3) ───────────────────────────────────

// TestSubagentStopIgnoresHostReportedCwd pins the anchor.
//
// The corpus scenario foreign-cwd covers this end to end; this is the unit
// statement of the same fact, with the decoy config that makes the failure
// mode explicit. A handler that trusted the reported cwd would find no
// config there, exit silently, and turn a blocked critical finding into
// output byte-identical to a project with no gates at all.
func TestSubagentStopIgnoresHostReportedCwd(t *testing.T) {
	subagentStopSandbox(t)

	// Control: the same input with no reported cwd blocks.
	control := subagentStop(newSubagentStopInvocation("security", "critical", "hardcoded credential"))
	if !subagentStopBlocked(control) {
		t.Fatalf("control: a critical finding from a gate-listed agent must block; got %+v", control.Response)
	}

	for _, reported := range []string{
		"/nonexistent/edikt-project-that-is-not-here", // the corpus payload's own shape
		"/tmp/edikt-test-project",                     // subagent-stop-critical.json's
		"${HOME}/projects/greenfield",                 // unexpanded, as three fixtures carry it
		t.TempDir(),                                   // a real directory that is not a project
	} {
		inv := newSubagentStopInvocation("security", "critical", "hardcoded credential")
		inv.Payload.Cwd = reported
		got := subagentStop(inv)
		if !subagentStopBlocked(got) {
			t.Fatalf("a reported cwd of %q must not disarm the gate — the root is the process's "+
				"own working directory; got %+v", reported, got.Response)
		}
	}
}

// TestSubagentStopRootIsAbsolute is AC-18.3's own words: never an env default
// of ".". Asserted on the resolver directly, because within one process a
// relative "." and the resolved cwd name the same directory — the difference
// is only observable in what the function RETURNS.
func TestSubagentStopRootIsAbsolute(t *testing.T) {
	root, _ := subagentStopSandbox(t)

	got, err := subagentStopRoot(newSubagentStopInvocation("security", "critical", "x"))
	if err != nil {
		t.Fatalf("resolve root: %v", err)
	}
	if got == "." || !filepath.IsAbs(got) {
		t.Fatalf("resolved root = %q, which is not an absolute path", got)
	}
	if resolved, rerr := filepath.EvalSymlinks(got); rerr == nil {
		if want, werr := filepath.EvalSymlinks(root); werr == nil && resolved != want {
			t.Fatalf("resolved root = %q, want %q", resolved, want)
		}
	}

	// A relative EDIKT_PROJECT_ROOT is made absolute too — the same defect
	// would otherwise arrive through the override instead of the default.
	t.Setenv("EDIKT_PROJECT_ROOT", ".")
	got, err = subagentStopRoot(newSubagentStopInvocation("security", "critical", "x"))
	if err != nil {
		t.Fatalf("resolve root with a relative override: %v", err)
	}
	if got == "." || !filepath.IsAbs(got) {
		t.Fatalf("with EDIKT_PROJECT_ROOT=\".\" the resolved root = %q, which is not absolute", got)
	}
}

// TestSubagentStopEnvRootSelectsTheWholeProject is the OTHER half of the
// deviation, and the branch no scenario can reach: when EDIKT_PROJECT_ROOT is
// set it must select the project for EVERY read, not for the threshold alone.
//
// The split is what the bash had, and it is what makes a gate readable out of
// two trees at once. The decoy the process sits in lists NO gates, so a
// handler that kept the split asks the decoy "is security gated?", hears no,
// and never blocks — while the override's own config says it is gated at
// warning.
func TestSubagentStopEnvRootSelectsTheWholeProject(t *testing.T) {
	decoy, _ := subagentStopSandbox(t)
	writeTestFile(t, filepath.Join(decoy, ".edikt", "config.yaml"), "edikt_version: 0.8.0\n")

	// Control: with no override, the decoy governs and gates nothing.
	control := subagentStop(newSubagentStopInvocation("security", "warning", "a warning finding"))
	if subagentStopBlocked(control) {
		t.Fatalf("control: the decoy lists no gates, so nothing may block; got %+v", control.Response)
	}

	elsewhere := t.TempDir()
	writeTestFile(t, filepath.Join(elsewhere, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\ngates:\n  security: warning\n")
	t.Setenv("EDIKT_PROJECT_ROOT", elsewhere)

	got := subagentStop(newSubagentStopInvocation("security", "warning", "a warning finding"))
	if !subagentStopBlocked(got) {
		t.Fatalf("EDIKT_PROJECT_ROOT must select the project for the is-a-gate scan as well as "+
			"for the threshold; got %+v", got.Response)
	}
	if !strings.Contains(got.Response.Message, "threshold: warning") {
		t.Fatalf("the block names the wrong threshold — it did not read the override's config: %q", got.Response.Message)
	}
}

// ── branch 4: the sourced event-log helper ──────────────────────────────────

// TestSubagentStopTelemetryDoesNotDependOnTheDeletedHelper pins what replaced
// the condition the corpus cannot see.
//
// This test used to assert the OPPOSITE and was right to: the second ledger
// line was written only when $HOME/.edikt/hooks/event-log.sh existed on disk,
// which faithfully reproduced bash's `if type edikt_log_event`. Phase 23
// deletes that file, and leaving the gate would have degraded the condition to
// "does a file we no longer ship happen to still exist" — a fresh install
// writing one line where an upgraded one wrote two, forever, with the
// divergence visible nowhere.
//
// The record could not be dropped instead: nine of seventeen corpus recordings
// pin the two-line shape and a migrated row can never be re-recorded against
// bash. So the gate went and the record stayed, which is also the right way
// round on its own terms — telemetry should not be conditional on a vestige.
func TestSubagentStopTelemetryDoesNotDependOnTheDeletedHelper(t *testing.T) {
	// Helper ABSENT — the state every fresh install is now in.
	_, home := subagentStopSandbox(t)
	got := subagentStop(newSubagentStopInvocation("security", "critical", "hardcoded credential"))
	if !subagentStopBlocked(got) {
		t.Fatalf("setup: the gate must fire for this input; got %+v", got.Response)
	}
	lines := subagentStopLedgerLines(t, home)
	if len(lines) != 2 {
		t.Fatalf("with no event-log.sh on disk the gate must STILL write both records; got %d line(s): %v", len(lines), lines)
	}
	if !strings.Contains(lines[0], `"event": "gate_fired"`) {
		t.Errorf("first line is not the hook's own gate_fired record: %s", lines[0])
	}
	if !strings.Contains(lines[1], `"type": "gate_fired"`) {
		t.Errorf("second line is not the telemetry shape: %s", lines[1])
	}

	// Helper PRESENT — an upgraded install that still carries the vestige.
	// The point is that it makes NO difference; a surviving file must not
	// change what the ledger records.
	_, home2 := subagentStopSandbox(t)
	writeTestFile(t, filepath.Join(home2, ".edikt", "hooks", "event-log.sh"), "edikt_log_event() { :; }\n")
	if got := subagentStop(newSubagentStopInvocation("security", "critical", "hardcoded credential")); !subagentStopBlocked(got) {
		t.Fatalf("control: the gate must still fire with the vestige present; got %+v", got.Response)
	}
	lines2 := subagentStopLedgerLines(t, home2)
	if len(lines2) != len(lines) {
		t.Errorf("a leftover event-log.sh changed the ledger: %d line(s) with it, %d without", len(lines2), len(lines))
	}

	// MED-8: the ledger is 0600 either way. The helper used to be what
	// tightened it, so its deletion is exactly when this could regress.
	info, err := os.Stat(filepath.Join(home, ".edikt", "events.jsonl"))
	if err != nil {
		t.Fatalf("ledger not written: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("ledger mode is %o, want 600 — it carries git email, findings and session metadata", perm)
	}
}
