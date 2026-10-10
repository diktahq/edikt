package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

func plantJournal(t *testing.T, lines ...string) {
	t.Helper()
	home := t.TempDir()
	// The journal resolves $EDIKT_ROOT, then $EDIKT_HOME, then $HOME/.edikt
	// (hookmatch/journal.go). test/run.sh exports EDIKT_HOME=$HOME/.edikt, so
	// redirecting HOME alone left this test reading the LIVE journal under the
	// suite — found 2026-09-02 when that journal carried suppressed entries
	// and "absent journal" rendered four real invocations.
	t.Setenv("EDIKT_ROOT", "")
	t.Setenv("EDIKT_HOME", "")
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".edikt", "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join(lines, "\n")
	if len(lines) > 0 {
		body += "\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "hook-journal.jsonl"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestDoctorSurfacesHookJournal — AC-6.8.
//
// hookmatch.Summary.Report()'s doc comment claimed doctor read it and doctor
// never did, so 20 recorded suppressions on this machine were surfaced to
// nobody. This asserts the wiring, and asserts the two cases doctor must never
// conflate: an ABSENT journal is UNKNOWN, not clean.
func TestDoctorSurfacesHookJournal(t *testing.T) {
	t.Run("absent journal renders as unknown and warns", func(t *testing.T) {
		t.Setenv("EDIKT_ROOT", "")
		t.Setenv("EDIKT_HOME", "")
		t.Setenv("HOME", t.TempDir())
		var out bytes.Buffer
		warn := runHookJournalCheck(&out)
		if warn != 1 {
			t.Fatalf("warnings: got %d, want 1 — an absent journal must not let doctor report healthy", warn)
		}
		if !strings.Contains(out.String(), "NO JOURNAL") {
			t.Fatalf("absent journal did not render as absent:\n%s", out.String())
		}
		if strings.Contains(out.String(), "no suppressed invocations recorded") {
			t.Fatalf("absent journal rendered as clean:\n%s", out.String())
		}
	})

	t.Run("empty journal is unknown, not clean", func(t *testing.T) {
		plantJournal(t)
		var out bytes.Buffer
		if warn := runHookJournalCheck(&out); warn != 1 {
			t.Fatalf("warnings: got %d, want 1", warn)
		}
		if !strings.Contains(out.String(), "EMPTY") {
			t.Fatalf("empty journal did not say so:\n%s", out.String())
		}
	})

	t.Run("doctor counts the new hook-run suppression classes", func(t *testing.T) {
		plantJournal(t,
			`{"ts":"2026-08-20T10:00:00Z","kind":"hook_run","hook":"session-start","outcome":"hook-ran","exit_code":0,"duration_us":2100}`,
			`{"ts":"2026-08-20T10:00:01Z","kind":"hook_run","hook":"stop-hook","outcome":"panic-recovered","exit_code":0,"duration_us":900}`,
			`{"ts":"2026-08-20T10:00:02Z","kind":"hook_run","hook":"typo-hook","outcome":"unregistered-hook-name","exit_code":0,"duration_us":100}`,
		)
		var out bytes.Buffer
		warn := runHookJournalCheck(&out)
		if warn != 1 {
			t.Fatalf("warnings: got %d, want 1 for a journal carrying suppressions", warn)
		}
		body := out.String()
		for _, want := range []string{
			string(hookmatch.OutcomePanicRecovered),
			string(hookmatch.OutcomeUnregisteredHookName),
			"SUPPRESSED",
			"per-hook",
			"session-start",
		} {
			if !strings.Contains(body, want) {
				t.Errorf("doctor output omits %q:\n%s", want, body)
			}
		}
		// The count itself: two of the three records are suppressions.
		if !strings.Contains(body, "SUPPRESSED: 2") {
			t.Errorf("doctor did not count both suppressed classes:\n%s", body)
		}
	})

	t.Run("a clean journal contributes no warning", func(t *testing.T) {
		plantJournal(t,
			`{"ts":"2026-08-20T10:00:00Z","kind":"hook_run","hook":"session-start","outcome":"hook-ran","exit_code":0,"duration_us":2100}`,
		)
		var out bytes.Buffer
		if warn := runHookJournalCheck(&out); warn != 0 {
			t.Fatalf("warnings: got %d, want 0 for a clean journal — otherwise the check is a constant\n%s", warn, out.String())
		}
	})
}

// TestDoctorCallsTheHookJournalCheck guards the wiring itself.
//
// The unit tests above would all pass if runHookJournalCheck were never called
// from doctor's RunE — which is exactly the state this phase found the code in:
// a report function with no caller.
func TestDoctorCallsTheHookJournalCheck(t *testing.T) {
	raw, err := os.ReadFile("doctor.go")
	if err != nil {
		t.Fatalf("read doctor.go: %v", err)
	}
	found := false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") {
			continue
		}
		// Either entry point counts: the check proper takes the journal
		// Summary as an argument now, so doctor reads the append-only
		// journal once and shares it. What this guard is about is that the
		// REPORT HAS A CALLER — it once had none — not which overload.
		if strings.Contains(trimmed, "runHookJournalCheck(") ||
			strings.Contains(trimmed, "runHookJournalCheckWith(") {
			found = true
		}
	}
	if !found {
		t.Fatal("doctor.go does not call runHookJournalCheck — the report has no caller, " +
			"which is the exact defect this check exists to prevent recurring")
	}
}
