package hookmatch

// journal_reader_duration_test.go — AC-4.4: the Stop hook's own latency
// claim ("p50 under 2s, max under 10s") must be measured from the journal's
// own duration_us, never guessed. HookDurations/DurationPercentile are the
// reader this reads from.

import (
	"encoding/json"
	"os"
	"sort"
	"testing"
	"time"
)

func TestJournalReader_HookDurationsFiltersByHookAndSkipsNilDuration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	us := func(v int64) *int64 { return &v }
	for _, r := range []Record{
		{Kind: KindHookRun, Hook: "stop-hook", Outcome: string(OutcomeHookRan), DurationUS: us(1_000_000)},
		{Kind: KindHookRun, Hook: "stop-hook", Outcome: string(OutcomeHookRan), DurationUS: us(500_000)},
		{Kind: KindHookRun, Hook: "session-start", Outcome: string(OutcomeHookRan), DurationUS: us(9_999_999)},
		{Kind: KindHookRun, Hook: "stop-hook", Outcome: string(OutcomeHookRan)}, // no duration — skipped
	} {
		if err := Append(r); err != nil {
			t.Fatal(err)
		}
	}

	got, err := HookDurations("stop-hook")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("durations = %v, want exactly 2 (the session-start record and the nil-duration record must both be excluded)", got)
	}
	sort.Slice(got, func(i, j int) bool { return got[i] < got[j] })
	if got[0] != 500*time.Millisecond || got[1] != time.Second {
		t.Fatalf("durations = %v, want [500ms, 1s]", got)
	}
}

func TestJournalReader_HookDurationsAbsentJournalIsNilNotError(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	got, err := HookDurations("stop-hook")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("durations = %v, want none — no journal means nothing was measured, not zero latency", got)
	}
}

func TestJournalReader_DurationPercentile(t *testing.T) {
	sorted := []time.Duration{
		1 * time.Second, 2 * time.Second, 3 * time.Second, 4 * time.Second, 5 * time.Second,
		6 * time.Second, 7 * time.Second, 8 * time.Second, 9 * time.Second, 10 * time.Second,
	}
	if p50 := DurationPercentile(sorted, 50); p50 != 5*time.Second {
		t.Errorf("p50 = %v, want 5s", p50)
	}
	if max := DurationPercentile(sorted, 100); max != 10*time.Second {
		t.Errorf("p100 (max) = %v, want 10s", max)
	}
	if p1 := DurationPercentile(sorted, 1); p1 != 1*time.Second {
		t.Errorf("p1 = %v, want 1s", p1)
	}
	if empty := DurationPercentile(nil, 50); empty != 0 {
		t.Errorf("percentile of an empty slice = %v, want 0", empty)
	}
}

// TestJournalReader_RealWorldShapeRoundTrips proves HookDurations reads
// exactly what Append (the real write path every hook uses) writes — not
// a hand-rolled JSON shape that happens to match this test's own encoder.
func TestJournalReader_RealWorldShapeRoundTrips(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	us := int64(1_234_567)
	if err := Append(Record{Kind: KindHookRun, Hook: "stop-hook", Outcome: string(OutcomeHookRan), DurationUS: &us}); err != nil {
		t.Fatal(err)
	}

	// Confirm the on-disk bytes are ordinary JSONL (a corpus consumer's
	// assumption this reader relies on), not something HookDurations alone
	// happens to tolerate.
	p, err := JournalPath()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var decoded Record
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("journal line does not decode as a single JSON object: %v", err)
	}

	got, err := HookDurations("stop-hook")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != time.Duration(us)*time.Microsecond {
		t.Fatalf("durations = %v, want [%v]", got, time.Duration(us)*time.Microsecond)
	}
}
