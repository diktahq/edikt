package hooksoak

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook"
)

// ── test fixtures ────────────────────────────────────────────────────────

// fakeRegistry builds a synthetic hook.File with one migrated hook per
// class plus one PENDING hook per class, so GroupHooks has something to
// filter wrong if it filters on status alone (the multi-call/gate classes
// existing at all, migrated or not, would leak into group 1 if class were
// ignored) and something to filter wrong if it filters on class alone (a
// pending injector would leak into group 1 if status were ignored).
func fakeRegistry(t *testing.T) hook.File {
	t.Helper()
	ev := func(s string) *string { return &s }
	return hook.File{
		SchemaVersion: 1,
		Path:          "fixture-registry.yaml",
		Hooks: []hook.Row{
			{Name: "session-start", Event: ev("SessionStart"), Class: hook.ClassInjector, Status: hook.StatusMigrated},
			{Name: "cwd-changed", Event: ev("CwdChanged"), Class: hook.ClassInjector, Status: hook.StatusMigrated},
			{Name: "task-created", Event: ev("TaskCreated"), Class: hook.ClassInjector, Status: hook.StatusPending},
			{Name: "stop-hook", Event: ev("Stop"), Class: hook.ClassMultiCall, Status: hook.StatusMigrated},
			{Name: "subagent-stop", Event: ev("SubagentStop"), Class: hook.ClassMultiCall, Status: hook.StatusPending},
			{Name: "pre-tool-use", Event: ev("PreToolUse"), Class: hook.ClassGate, Status: hook.StatusPending},
		},
	}
}

// ── GroupHooks: derived from the registry, not hardcoded ──────────────────

func TestGroupHooks_Group1_OnlyMigratedInjectors(t *testing.T) {
	got, err := GroupHooks(fakeRegistry(t), 1)
	if err != nil {
		t.Fatalf("GroupHooks(1): %v", err)
	}
	want := []string{"cwd-changed", "session-start"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group 1 hooks = %v, want %v (pending injector and every multi-call/gate row must be excluded)", got, want)
	}
}

func TestGroupHooks_Group2_OnlyMigratedMultiCall(t *testing.T) {
	got, err := GroupHooks(fakeRegistry(t), 2)
	if err != nil {
		t.Fatalf("GroupHooks(2): %v", err)
	}
	want := []string{"stop-hook"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("group 2 hooks = %v, want %v", got, want)
	}
}

// TestGroupHooks_NewlyMigratedHook_IsPickedUpAutomatically proves the
// "derive from the registry, never hardcode" requirement actually holds: a
// hook flipped to migrated is covered with NO code change in this package,
// only a registry edit.
func TestGroupHooks_NewlyMigratedHook_IsPickedUpAutomatically(t *testing.T) {
	f := fakeRegistry(t)
	for i := range f.Hooks {
		if f.Hooks[i].Name == "task-created" {
			f.Hooks[i].Status = hook.StatusMigrated
		}
	}
	got, err := GroupHooks(f, 1)
	if err != nil {
		t.Fatalf("GroupHooks(1): %v", err)
	}
	want := []string{"cwd-changed", "session-start", "task-created"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("newly-migrated hook not picked up: got %v, want %v", got, want)
	}
}

func TestGroupHooks_ZeroMigrated_Errors(t *testing.T) {
	f := hook.File{Path: "empty.yaml", Hooks: []hook.Row{
		{Name: "stop-hook", Class: hook.ClassMultiCall, Status: hook.StatusPending},
	}}
	_, err := GroupHooks(f, 2)
	if err == nil {
		t.Fatal("group with zero migrated hooks must error, not return an empty (invalid, minItems:1) hooks list")
	}
	if !strings.Contains(err.Error(), "multi-call") {
		t.Fatalf("error does not name the class it found nothing in: %v", err)
	}
}

func TestGroupClass_Group3HasNoSoakGate(t *testing.T) {
	if _, err := GroupClass(3); err == nil {
		t.Fatal("group 3 (the gate class) must refuse — it has no soak gate of its own")
	}
	if _, err := GroupClass(0); err == nil {
		t.Fatal("group 0 must refuse")
	}
}

// ── Check: the six acceptance criteria, each with a negative control ──────

// checkFixture returns a CheckOptions wired to fakeRegistry (written to a
// real temp file, since hook.LoadRegistryFile reads from disk), a fixed
// clock, and a stubbed GitLog so no criterion below depends on git or on
// the machine's real journal.
func checkFixture(t *testing.T, group int, journalCounts map[string]JournalCounts, journalFound bool) CheckOptions {
	t.Helper()
	regPath := writeRegistryFixture(t, fakeRegistry(t))
	fixedNow := time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)
	commitDate := time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC) // 14 days before fixedNow
	return CheckOptions{
		RegistryPath: regPath,
		Group:        group,
		SinceCommit:  "abc1234",
		RepoRoot:     t.TempDir(),
		JournalPath:  "unused-because-ReadJournal-is-stubbed",
		Now:          func() time.Time { return fixedNow },
		GitLog: func(repoRoot, commit string) (time.Time, error) {
			return commitDate, nil
		},
		ReadJournal: func(path string) (map[string]JournalCounts, bool, error) {
			return journalCounts, journalFound, nil
		},
	}
}

// writeRegistryFixture marshals a hook.File to a temp YAML file so
// hook.LoadRegistryFile (which reads from disk, per its own contract) has
// something real to load — this is the one piece of Check's input surface
// that is not directly injectable, matching how internal/hook itself is
// tested.
func writeRegistryFixture(t *testing.T, f hook.File) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.yaml")
	var b strings.Builder
	b.WriteString("schema_version: 1\nhooks:\n")
	for _, r := range f.Hooks {
		event := "null"
		if r.Event != nil {
			event = *r.Event
		}
		fmt.Fprintf(&b, "  - name: %s\n    event: %s\n    file: %s.sh\n    class: %s\n    status: %s\n    python3_baseline_count: 0\n",
			r.Name, event, r.Name, r.Class, r.Status)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatalf("write registry fixture: %v", err)
	}
	return path
}

// ── Criterion: zero-invocation hook refuses and names it; full coverage passes ──

func TestCheck_ZeroInvocationForOneHook_RefusesAndNamesIt(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 3},
		// cwd-changed: absent from the map entirely == zero.
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("status = %q, want refused", rec.Status)
	}
	if !containsSubstring(rec.RefusalReasons, "cwd-changed: zero journal invocation records") {
		t.Fatalf("refusal reasons do not name cwd-changed by name: %v", rec.RefusalReasons)
	}
	for _, h := range rec.Hooks {
		if h.Name == "cwd-changed" && h.InvocationCount != 0 {
			t.Fatalf("cwd-changed invocation_count = %d, want 0", h.InvocationCount)
		}
		if h.Name == "cwd-changed" && h.JournalStatus != JournalObserved {
			t.Fatalf("cwd-changed journal_status = %q, want observed — the journal WAS read, it just found nothing for this hook", h.JournalStatus)
		}
	}
}

// Positive control for the same guard: every group-1 hook present ==> pass.
func TestCheck_AllHooksPresent_Passes(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 3},
		"cwd-changed":   {Invocation: 1},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusPass {
		t.Fatalf("status = %q, want pass; reasons: %v", rec.Status, rec.RefusalReasons)
	}
	if len(rec.RefusalReasons) != 0 {
		t.Fatalf("a passing record must carry no refusal_reasons: %v", rec.RefusalReasons)
	}
}

// ── Criterion: disqualifying outcomes refuse; their absence passes ────────

func TestCheck_PanicRecovered_Refuses(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 3, PanicRecovered: 1},
		"cwd-changed":   {Invocation: 1},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("status = %q, want refused (panic-recovered present)", rec.Status)
	}
	if !containsSubstring(rec.RefusalReasons, "session-start: 1 panic-recovered") {
		t.Fatalf("refusal reasons do not name the panic: %v", rec.RefusalReasons)
	}
}

func TestCheck_UnregisteredHookName_Refuses(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 3},
		"cwd-changed":   {Invocation: 1, Unregistered: 2},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("status = %q, want refused (unregistered-hook-name present)", rec.Status)
	}
	if !containsSubstring(rec.RefusalReasons, "cwd-changed: 2 unregistered-hook-name") {
		t.Fatalf("refusal reasons do not name it: %v", rec.RefusalReasons)
	}
}

func TestCheck_BinaryAbsent_Refuses(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 3, BinaryAbsent: 1},
		"cwd-changed":   {Invocation: 1},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("status = %q, want refused (binary-absent present)", rec.Status)
	}
	if !containsSubstring(rec.RefusalReasons, "session-start: 1 binary-absent") {
		t.Fatalf("refusal reasons do not name it: %v", rec.RefusalReasons)
	}
}

// Positive control: no disqualifying outcome anywhere ==> pass. Shared with
// TestCheck_AllHooksPresent_Passes's assertion set, restated here so each
// disqualifying-outcome test above has its own explicit "and it doesn't fire
// when absent" companion rather than relying on a single shared pass test to
// carry all three.
func TestCheck_NoDisqualifyingOutcomes_Passes(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 5},
		"cwd-changed":   {Invocation: 5},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusPass {
		t.Fatalf("status = %q, want pass; reasons: %v", rec.Status, rec.RefusalReasons)
	}
}

// ── Criterion: absent journal is UNKNOWN and refuses, never rendered as zero ──

func TestCheck_AbsentJournal_UnknownNotZero_RefusesWithDistinguishableBytes(t *testing.T) {
	o := checkFixture(t, 1, nil, false /* journal not found */)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("status = %q, want refused", rec.Status)
	}

	// Assert the STRUCTURAL field first — this is what a machine consumer
	// actually keys on.
	for _, h := range rec.Hooks {
		if h.JournalStatus != JournalUnknown {
			t.Fatalf("hook %s journal_status = %q, want %q", h.Name, h.JournalStatus, JournalUnknown)
		}
		if h.InvocationCount != 0 {
			t.Fatalf("hook %s invocation_count = %d — an unknown journal_status must carry a zero-valued PLACEHOLDER, but this test is checking it stays the documented placeholder value, not that it was computed", h.Name, h.InvocationCount)
		}
	}

	// Now assert on the ACTUAL BYTES of both the JSON encoding and the
	// refusal reason text, so this test cannot pass by only checking the
	// exit-code-shaped "refused" signal — it must be possible to tell
	// "unknown" apart from "zero, observed" by reading the output.
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	body := string(raw)
	if !strings.Contains(body, `"journal_status":"unknown"`) {
		t.Fatalf("JSON does not carry journal_status:unknown byte-for-byte:\n%s", body)
	}
	if strings.Contains(body, `"journal_status":"observed"`) {
		t.Fatalf("JSON claims an observed journal for an absent one:\n%s", body)
	}
	for _, reason := range rec.RefusalReasons {
		if strings.Contains(reason, "zero journal invocation records") {
			t.Fatalf("an absent-journal refusal reason must not use the 'zero invocation records' wording reserved for an OBSERVED, empty count — got: %q", reason)
		}
	}
	if !containsSubstring(rec.RefusalReasons, "journal_status unknown") {
		t.Fatalf("refusal reasons do not say unknown: %v", rec.RefusalReasons)
	}
}

// Negative control for the same guard, proving the byte-level assertions
// above are actually capable of telling the two cases apart (not merely
// checking a string this test also produces): a journal that IS found, with
// every hook present, must never say "unknown" anywhere.
func TestCheck_PresentJournal_NeverSaysUnknown(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{
		"session-start": {Invocation: 1},
		"cwd-changed":   {Invocation: 1},
	}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	raw, _ := json.Marshal(rec)
	if strings.Contains(string(raw), `"unknown"`) {
		t.Fatalf("a found, fully-populated journal must never render journal_status unknown:\n%s", raw)
	}
}

// ── Criterion: elapsed_days is reported as a fact alongside counts, not gated ──

func TestCheck_ElapsedDaysReported_EvenWhenRefused(t *testing.T) {
	// A group can be day-14 and still refuse — hook-soak.v1's own words. Set
	// up a refusing case and assert elapsed_days is still the real number,
	// not zeroed or omitted because the verdict failed.
	o := checkFixture(t, 1, nil, false)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.Status != StatusRefused {
		t.Fatalf("test setup: expected a refused record")
	}
	if rec.ElapsedDays != 14 {
		t.Fatalf("elapsed_days = %d, want 14 (fixture clock is 14 days after the fixture commit date) — elapsed_days must be reported regardless of the verdict", rec.ElapsedDays)
	}
}

// ── schema_version and hooks-array shape ───────────────────────────────────

func TestCheck_RecordCarriesSchemaVersionAndGroup(t *testing.T) {
	o := checkFixture(t, 2, map[string]JournalCounts{"stop-hook": {Invocation: 1}}, true)
	rec, err := Check(o)
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if rec.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", rec.SchemaVersion)
	}
	if rec.Group != 2 {
		t.Fatalf("group = %d, want 2", rec.Group)
	}
	if len(rec.Hooks) != 1 || rec.Hooks[0].Name != "stop-hook" {
		t.Fatalf("hooks = %+v, want exactly [stop-hook]", rec.Hooks)
	}
}

// ── setup errors are errors, not a fabricated Record ───────────────────────

func TestCheck_GitLogFailure_IsAnErrorNotARecord(t *testing.T) {
	o := checkFixture(t, 1, map[string]JournalCounts{}, true)
	o.GitLog = func(repoRoot, commit string) (time.Time, error) {
		return time.Time{}, fmt.Errorf("commit not found")
	}
	_, err := Check(o)
	if err == nil {
		t.Fatal("an unresolvable --since-commit must be a setup error, not a fabricated pass/refused record")
	}
}

// ── helpers ─────────────────────────────────────────────────────────────

func containsSubstring(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}
