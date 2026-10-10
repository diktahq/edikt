package hookmatch

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeJournal plants a journal with n records under a scratch HOME and
// returns its path. Each record is tagged so a reader can tell head from tail.
func writeJournal(t *testing.T, n int) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := JournalPath()
	if err != nil {
		t.Fatalf("journal path: %v", err)
	}
	var b strings.Builder
	for i := 0; i < n; i++ {
		line, err := json.Marshal(Record{
			TS:      fmt.Sprintf("2026-08-20T00:00:%02dZ", i%60),
			Kind:    KindHookRun,
			Hook:    fmt.Sprintf("hook-%d", i),
			Outcome: string(OutcomeHookRan),
			// Pad so the file crosses the window with a manageable record
			// count, without needing thousands of lines.
			Detail: strings.Repeat("x", 512),
		})
		if err != nil {
			t.Fatal(err)
		}
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatalf("write journal: %v", err)
	}
	return p
}

// TestJournalWindowedRead — AC-6.9. A windowed read must not parse all
// history, and must SAY that it did not.
//
// ReadSummary used to os.ReadFile the whole journal. With two call sites that
// was fine; with twenty-two hooks journaling, every `doctor` run would parse
// the machine's entire hook history to answer a question about its recent
// state.
func TestJournalWindowedRead(t *testing.T) {
	const records = 400
	p := writeJournal(t, records)
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("planted %d record(s) in %d byte(s)", records, st.Size())

	full, err := ReadSummaryWindow(0)
	if err != nil {
		t.Fatalf("full read: %v", err)
	}
	if full.Total != records {
		t.Fatalf("full read total: got %d, want %d", full.Total, records)
	}
	if full.Windowed {
		t.Fatal("a full read must not report itself as windowed")
	}

	window := st.Size() / 4
	win, err := ReadSummaryWindow(window)
	if err != nil {
		t.Fatalf("windowed read: %v", err)
	}
	if !win.Windowed {
		t.Fatal("a windowed read must report Windowed — a tail count that reads as a total is a lie about the denominator")
	}
	if win.Total >= full.Total {
		t.Fatalf("windowed total %d did not undercount the full total %d — the window parsed everything", win.Total, full.Total)
	}
	if win.Total == 0 {
		t.Fatal("windowed read counted nothing — the window is too small to be a measurement")
	}
	if win.BytesScanned > window+1024 {
		t.Fatalf("scanned %d bytes for a %d-byte window", win.BytesScanned, window)
	}
	if win.BytesTotal != st.Size() {
		t.Fatalf("BytesTotal: got %d, want %d — the denominator must be the real file size", win.BytesTotal, st.Size())
	}
	if win.Malformed != 0 {
		t.Fatalf("malformed: got %d, want 0 — the partial first line is a windowing artifact, not corruption", win.Malformed)
	}

	// The report must not present a tail count as a total.
	rep := win.Report()
	if !strings.Contains(rep, "WINDOWED") {
		t.Fatalf("report does not disclose the window:\n%s", rep)
	}

	// The head records must be genuinely absent from the windowed counts —
	// otherwise "windowed" is a label on a full read.
	if _, seen := win.ByHook["hook-0"]; seen {
		t.Fatal("the oldest record was counted; the window did not skip history")
	}
	if _, seen := win.ByHook[fmt.Sprintf("hook-%d", records-1)]; !seen {
		t.Fatal("the newest record was not counted; the window read the wrong end")
	}
}

// TestJournalMalformedLinesAreCounted — a corrupt journal must not read as a
// quiet one. Before this, undecodable lines were skipped in silence.
func TestJournalMalformedLinesAreCounted(t *testing.T) {
	p := writeJournal(t, 3)
	f, err := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("{not json at all\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	s, err := ReadSummaryWindow(0)
	if err != nil {
		t.Fatal(err)
	}
	if s.Malformed != 1 {
		t.Fatalf("malformed: got %d, want 1", s.Malformed)
	}
	if !strings.Contains(s.Report(), "MALFORMED") {
		t.Fatalf("report does not disclose malformed lines:\n%s", s.Report())
	}
}

// TestJournalRotates — bounded LIVE file, and the generation is preserved.
//
// Updated 2026-08-29 with the contract it pins. It previously asserted a
// rotated file at <name>.1, i.e. exactly one retained generation. That fixed
// suffix meant every rotation overwrote the previous one, and the assertion
// was pinning the data loss. Rotation now moves the generation into
// journal-archive/ under a unique timestamped name; the live-file half of the
// contract, asserted below, is unchanged.
func TestJournalRotates(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := JournalPath()
	if err != nil {
		t.Fatal(err)
	}
	// Seed just under the threshold, then append one record to cross it.
	filler := make([]byte, MaxJournalBytes)
	for i := range filler {
		filler[i] = '\n'
	}
	if err := os.WriteFile(p, filler, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Append(Record{Kind: KindHookRun, Hook: "session-start", Outcome: string(OutcomeHookRan)}); err != nil {
		t.Fatalf("append: %v", err)
	}
	arc := filepath.Join(filepath.Dir(p), ArchiveDirName)
	ents, err := os.ReadDir(arc)
	if err != nil {
		t.Fatalf("no archive directory at %s: %v", arc, err)
	}
	var gens []string
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), "hook-journal-") {
			gens = append(gens, e.Name())
		}
	}
	if len(gens) != 1 {
		t.Fatalf("want exactly one preserved generation in %s, got %v", arc, gens)
	}
	if _, err := os.Stat(p + ".1"); err == nil {
		t.Fatalf("the fixed-suffix .1 name reappeared — that shape overwrites history")
	}
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("the live journal should have been rotated away, stat gave: %v", err)
	}
	// The next append starts a fresh file rather than failing.
	if err := Append(Record{Kind: KindHookRun, Hook: "session-end", Outcome: string(OutcomeHookRan)}); err != nil {
		t.Fatalf("append after rotation: %v", err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("read post-rotation journal: %v", err)
	}
	if !strings.Contains(string(raw), "session-end") {
		t.Fatalf("post-rotation journal does not carry the new record: %s", raw)
	}
}

// TestAbsentJournalIsNotClean — ADR-060:d10. Absence is UNKNOWN, never zero.
func TestAbsentJournalIsNotClean(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s, err := ReadSummary()
	if err != nil {
		t.Fatal(err)
	}
	if s.JournalFound {
		t.Fatal("no journal exists but JournalFound is true")
	}
	rep := s.Report()
	if !strings.Contains(rep, "NO JOURNAL") {
		t.Fatalf("an absent journal did not render as absent:\n%s", rep)
	}
	if strings.Contains(rep, "no suppressed invocations recorded") {
		t.Fatalf("an absent journal rendered as clean:\n%s", rep)
	}
}

// TestHookRunOutcomesAreSuppressed — AC-6.6, at the source.
//
// If panic-recovered were not Suppressed(), ReadSummary would count a hook
// that crashed as a healthy invocation and `hook probe` would exit 0 on a
// chain that crashed.
func TestHookRunOutcomesAreSuppressed(t *testing.T) {
	for _, o := range []Outcome{OutcomePanicRecovered, OutcomeUnregisteredHookName, OutcomePayloadUnparseable} {
		if !o.Suppressed() {
			t.Errorf("%q must be Suppressed() — a hook that did not run must never count as healthy", o)
		}
	}
	if OutcomeHookRan.Suppressed() {
		t.Error("hook-ran must NOT be Suppressed() — it is the healthy class")
	}
	// Distinctness across the whole vocabulary.
	seen := map[Outcome]bool{}
	all := []Outcome{
		OutcomeMatched, OutcomeNoMatch, OutcomeIndexMissing, OutcomeIndexCorrupt,
		OutcomeBadPath, OutcomeIndexEmpty, OutcomeHookRan, OutcomePanicRecovered,
		OutcomeUnregisteredHookName, OutcomePayloadUnparseable,
	}
	for _, o := range all {
		if seen[o] {
			t.Errorf("outcome %q is declared twice — two fail-open classes that collapse to one are indistinguishable", o)
		}
		seen[o] = true
	}
	if len(seen) != len(all) {
		t.Fatalf("%d distinct outcomes for %d declared values", len(seen), len(all))
	}
}

// TestSuppressedCountsInSummary wires the two together: a journal carrying the
// new outcomes must report them as suppressed, not merely store them.
func TestSuppressedCountsInSummary(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, o := range []Outcome{OutcomePanicRecovered, OutcomeUnregisteredHookName, OutcomeHookRan} {
		if err := Append(Record{Kind: KindHookRun, Hook: "stop-hook", Outcome: string(o)}); err != nil {
			t.Fatal(err)
		}
	}
	s, err := ReadSummary()
	if err != nil {
		t.Fatal(err)
	}
	if s.Total != 3 {
		t.Fatalf("total: got %d, want 3", s.Total)
	}
	if s.Suppressed != 2 {
		t.Fatalf("suppressed: got %d, want 2", s.Suppressed)
	}
	if s.ByHook["stop-hook"] != 3 {
		t.Fatalf("per-hook count: got %d, want 3", s.ByHook["stop-hook"])
	}
	rep := s.Report()
	for _, want := range []string{string(OutcomePanicRecovered), string(OutcomeUnregisteredHookName), "SUPPRESSED", "per-hook"} {
		if !strings.Contains(rep, want) {
			t.Errorf("report omits %q:\n%s", want, rep)
		}
	}
}

// TestJournalRecordPointerFieldsRoundTrip — the INV-013 decoder property: an
// absent exit code and a recorded 0 must not decode to the same thing.
func TestJournalRecordPointerFieldsRoundTrip(t *testing.T) {
	zero := 0
	us := int64(0)
	withCode, _ := json.Marshal(Record{Hook: "h", ExitCode: &zero, DurationUS: &us})
	withoutCode, _ := json.Marshal(Record{Hook: "h"})
	if string(withCode) == string(withoutCode) {
		t.Fatal("a record carrying exit code 0 serializes identically to one carrying none")
	}
	var back Record
	if err := json.Unmarshal(withoutCode, &back); err != nil {
		t.Fatal(err)
	}
	if back.ExitCode != nil {
		t.Fatal("an absent exit code decoded to a non-nil value")
	}
}
