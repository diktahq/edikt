package cmd

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeEventsJSONL(t *testing.T, path string, lines []string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := ""
	for _, l := range lines {
		body += l + "\n"
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write events.jsonl: %v", err)
	}
}

func TestDoctorUndeclaredInvoker_CountsWithinWindow(t *testing.T) {
	root := t.TempDir()
	now := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	recent := now.Add(-1 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	old := now.Add(-40 * 24 * time.Hour).UTC().Format("2006-01-02T15:04:05Z") // outside 30-day window

	writeEventsJSONL(t, filepath.Join(root, "events.jsonl"), []string{
		`{"event":"verb_invoked","timestamp":"` + recent + `","verb":"gov","invoked_by":"direct"}`,
		`{"event":"verb_invoked","timestamp":"` + recent + `","verb":"gov","invoked_by":"/edikt:gov:compile"}`,
		`{"event":"verb_invoked","timestamp":"` + old + `","verb":"gov","invoked_by":"direct"}`, // out of window
		`{"event":"something_else","timestamp":"` + recent + `"}`,                               // not a verb_invoked event
	})

	total, undeclared, err := frontDoorCounts(filepath.Join(root, "events.jsonl"), now)
	if err != nil {
		t.Fatalf("frontDoorCounts: %v", err)
	}
	if total != 2 {
		t.Errorf("expected 2 in-window verb_invoked events, got %d", total)
	}
	if undeclared != 1 {
		t.Errorf("expected 1 undeclared (direct) of 2, got %d", undeclared)
	}
}

func TestDoctorUndeclaredInvoker_MissingJournalIsNotAnError(t *testing.T) {
	root := t.TempDir()
	_, _, err := frontDoorCounts(filepath.Join(root, "events.jsonl"), time.Now())
	if err == nil || !os.IsNotExist(err) {
		t.Fatalf("expected a not-exist error for a missing journal, got %v", err)
	}
}

func TestDoctorUndeclaredInvoker_MalformedLineSkippedNotFatal(t *testing.T) {
	root := t.TempDir()
	now := time.Now()
	recent := now.Add(-1 * time.Hour).UTC().Format("2006-01-02T15:04:05Z")
	writeEventsJSONL(t, filepath.Join(root, "events.jsonl"), []string{
		`{"event":"verb_invoked","timestamp":"` + recent + `","invoked_by":"direct"}`,
		`not even json`,
		`{"event":"verb_invoked","timestamp":"` + recent + `","invoked_by":"/edikt:gov:compile"}`,
	})
	total, undeclared, err := frontDoorCounts(filepath.Join(root, "events.jsonl"), now)
	if err != nil {
		t.Fatalf("frontDoorCounts: %v", err)
	}
	// The malformed MIDDLE line must not blind the reader to the valid
	// line that follows it — a break-on-first-error reader would miss
	// this third line entirely.
	if total != 2 || undeclared != 1 {
		t.Errorf("expected both valid lines counted despite the malformed line between them, got total=%d undeclared=%d", total, undeclared)
	}
}
