package hooksoak

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadJournal_AbsentFile_ReturnsNotFound(t *testing.T) {
	counts, found, err := ReadJournal(filepath.Join(t.TempDir(), "does-not-exist.jsonl"))
	if err != nil {
		t.Fatalf("absent file must not be an error: %v", err)
	}
	if found {
		t.Fatal("found = true for an absent file, want false")
	}
	if len(counts) != 0 {
		t.Fatalf("counts = %v, want empty for an absent file", counts)
	}
}

// Positive control for the same function: a present file with real records
// is found=true and tallies correctly.
func TestReadJournal_PresentFile_TalliesPerHook(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	body := `{"ts":"2026-08-21T00:00:00Z","kind":"hook_run","hook":"session-start","outcome":"hook-ran"}
{"ts":"2026-08-21T00:01:00Z","kind":"hook_run","hook":"session-start","outcome":"hook-ran"}
{"ts":"2026-08-21T00:02:00Z","kind":"hook_run","hook":"cwd-changed","outcome":"panic-recovered"}
{"ts":"2026-08-21T00:03:00Z","kind":"hook_run","hook":"cwd-changed","outcome":"unregistered-hook-name"}
{"ts":"2026-08-21T00:04:00Z","kind":"hook_run","hook":"cwd-changed","outcome":"binary-absent"}
{"ts":"2026-08-21T00:05:00Z","kind":"hook_match","shim":"pre","outcome":"matched"}
`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	counts, found, err := ReadJournal(path)
	if err != nil {
		t.Fatalf("ReadJournal: %v", err)
	}
	if !found {
		t.Fatal("found = false for a present, readable file")
	}
	ss := counts["session-start"]
	if ss.Invocation != 2 {
		t.Fatalf("session-start invocation = %d, want 2", ss.Invocation)
	}
	cc := counts["cwd-changed"]
	if cc.Invocation != 3 {
		t.Fatalf("cwd-changed invocation = %d, want 3 (one per outcome, each still an invocation)", cc.Invocation)
	}
	if cc.PanicRecovered != 1 || cc.Unregistered != 1 || cc.BinaryAbsent != 1 {
		t.Fatalf("cwd-changed outcome tallies = %+v, want one of each", cc)
	}
	// The hook_match record (no "hook" field) must not create a phantom
	// entry keyed on "" or otherwise pollute the per-hook map.
	if _, ok := counts[""]; ok {
		t.Fatal("a hook_match record with no hook field must not be tallied")
	}
	if len(counts) != 2 {
		t.Fatalf("counts has %d keys, want exactly 2 (session-start, cwd-changed): %v", len(counts), counts)
	}
}

func TestReadJournal_MalformedLine_SkippedNotFatal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "journal.jsonl")
	body := "not json at all\n" +
		`{"kind":"hook_run","hook":"session-start","outcome":"hook-ran"}` + "\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	counts, found, err := ReadJournal(path)
	if err != nil {
		t.Fatalf("a malformed line must not fail the whole read: %v", err)
	}
	if !found {
		t.Fatal("found = false, want true — the file opened fine")
	}
	if counts["session-start"].Invocation != 1 {
		t.Fatalf("valid line after a malformed one must still be tallied: %+v", counts)
	}
}

// A directory at the journal path is a real (non-ENOENT) open error, and
// must be reported as such rather than silently treated as "absent" — the
// distinction matters because Check() (hooksoak.go) folds this into
// found=false too, but this test proves ReadJournal itself still SURFACES
// the error for callers that want the detail, rather than swallowing it
// unconditionally like the not-exist case does.
func TestReadJournal_UnreadablePath_ReturnsError(t *testing.T) {
	dir := t.TempDir() // a directory, not a file — os.Open + bufio.Scanner will fail reading it
	_, found, err := ReadJournal(dir)
	if err == nil {
		t.Fatal("opening a directory as the journal must error")
	}
	if found {
		t.Fatal("found = true on a read error, want false")
	}
}
