package hookmatch

// journal_sessionscope_test.go — SR-005 (AC-4.3): ReadSessionScope reads the
// WHOLE live journal, never a tail window, and reports when the file's own
// history may already be missing part of the session it is being asked about.

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestReadSessionScope_FiltersByPathAndSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	for _, r := range []Record{
		{Outcome: string(OutcomeMatched), SessionID: "session-a", Path: "commands/adr/new.md"},
		{Outcome: string(OutcomeMatched), SessionID: "session-a", Path: "commands/adr/new.md"}, // dup path
		{Outcome: string(OutcomeMatched), SessionID: "session-a", Path: "commands/gov/audit.md"},
		{Outcome: string(OutcomeMatched), SessionID: "session-b", Path: "README.md"},
	} {
		if err := Append(r); err != nil {
			t.Fatal(err)
		}
	}

	scope, err := ReadSessionScope("session-a")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.JournalFound {
		t.Fatal("journal was written but JournalFound is false")
	}
	want := []string{"commands/adr/new.md", "commands/gov/audit.md"}
	if len(scope.Paths) != len(want) {
		t.Fatalf("paths = %v, want %v", scope.Paths, want)
	}
	for i, p := range want {
		if scope.Paths[i] != p {
			t.Errorf("paths[%d] = %q, want %q (got %v)", i, scope.Paths[i], p, scope.Paths)
		}
	}

	other, err := ReadSessionScope("session-b")
	if err != nil {
		t.Fatal(err)
	}
	if len(other.Paths) != 1 || other.Paths[0] != "README.md" {
		t.Fatalf("session-b paths = %v, want [README.md]", other.Paths)
	}

	none, err := ReadSessionScope("session-c-never-fired")
	if err != nil {
		t.Fatal(err)
	}
	if len(none.Paths) != 0 {
		t.Fatalf("an unseen session id must yield zero paths, got %v", none.Paths)
	}
}

func TestReadSessionScope_NoWindowing(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := JournalPath()
	if err != nil {
		t.Fatal(err)
	}

	// The target session's one record comes FIRST, then enough padding to
	// push it well outside DefaultWindowBytes (512 KiB) if this reader ever
	// windowed the way ReadSummaryWindow does.
	var b strings.Builder
	head, _ := json.Marshal(Record{Outcome: string(OutcomeMatched),
		SessionID: "early-session", Path: "docs/architecture/decisions/ADR-001-demo.md"})
	b.Write(head)
	b.WriteByte('\n')
	filler, _ := json.Marshal(Record{Kind: KindHookRun, Hook: "noise", Outcome: string(OutcomeHookRan),
		Detail: strings.Repeat("x", 900)})
	for i := 0; i < 1000; i++ { // ~900KB of filler after the target record
		b.Write(filler)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Size() <= DefaultWindowBytes {
		t.Fatalf("test setup did not exceed the window: %d bytes", st.Size())
	}

	scope, err := ReadSessionScope("early-session")
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Paths) != 1 || scope.Paths[0] != "docs/architecture/decisions/ADR-001-demo.md" {
		t.Fatalf("a tail-windowed read would have missed this record; got paths=%v — "+
			"ReadSessionScope must read the whole file, not a window", scope.Paths)
	}
}

func TestReadSessionScope_MayUndercountWhenFirstRecordIsTheSession(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Append(Record{Outcome: string(OutcomeMatched),
		SessionID: "the-session", Path: "a.md"}); err != nil {
		t.Fatal(err)
	}

	scope, err := ReadSessionScope("the-session")
	if err != nil {
		t.Fatal(err)
	}
	if !scope.MayUndercount {
		t.Fatal("the session's own record is the journal's first surviving record — " +
			"MayUndercount must be true, since earlier history (if any) cannot be ruled out")
	}
}

func TestReadSessionScope_NoUndercountWhenAnEarlierRecordExists(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	if err := Append(Record{Outcome: string(OutcomeMatched),
		SessionID: "earlier-session", Path: "z.md"}); err != nil {
		t.Fatal(err)
	}
	if err := Append(Record{Outcome: string(OutcomeMatched),
		SessionID: "the-session", Path: "a.md"}); err != nil {
		t.Fatal(err)
	}

	scope, err := ReadSessionScope("the-session")
	if err != nil {
		t.Fatal(err)
	}
	if scope.MayUndercount {
		t.Fatal("the journal's first record belongs to a DIFFERENT, earlier session — " +
			"this session's own history is fully visible, MayUndercount must be false")
	}
}

func TestReadSessionScope_AbsentJournalIsNotFound(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	scope, err := ReadSessionScope("whatever")
	if err != nil {
		t.Fatal(err)
	}
	if scope.JournalFound {
		t.Fatal("no journal exists but JournalFound is true")
	}
	if len(scope.Paths) != 0 {
		t.Fatalf("an absent journal must yield zero paths, got %v", scope.Paths)
	}
}

// TestReadSessionScope_LargeJournalIsFast pins the doc comment's own claim:
// a 4 MiB journal parses in single-digit milliseconds, so reading the WHOLE
// file (rather than windowing) costs the Stop hook nothing perceptible.
func TestReadSessionScope_LargeJournalIsFast(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	p, err := JournalPath()
	if err != nil {
		t.Fatal(err)
	}

	var b strings.Builder
	line, _ := json.Marshal(Record{Outcome: string(OutcomeMatched),
		SessionID: "target", Path: "a.md", Detail: strings.Repeat("x", 400)})
	lineLen := len(line) + 1
	target := int64(4 << 20)            // 4 MiB
	n := int(target/int64(lineLen)) + 2 // round up past the target, not just to it
	for i := 0; i < n; i++ {
		b.Write(line)
		b.WriteByte('\n')
	}
	if err := os.WriteFile(p, []byte(b.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if st, _ := os.Stat(p); st.Size() < target {
		t.Fatalf("test setup only wrote %d bytes, want >= %d", st.Size(), target)
	}

	start := time.Now()
	scope, err := ReadSessionScope("target")
	elapsed := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	if len(scope.Paths) != 1 {
		t.Fatalf("paths = %v, want exactly [a.md]", scope.Paths)
	}
	if elapsed > 200*time.Millisecond {
		t.Fatalf("ReadSessionScope over a 4 MiB journal took %s — single-digit ms was the claim", elapsed)
	}
}
