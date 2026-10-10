package hook

// stophook_tally_test.go — SR-005 (AC-4.3): the end-of-session governance
// tally line, appended alongside an already-nonempty Stop-hook fire. Seeds
// the hookmatch journal and a compiled delivery-map.yaml directly (this
// package and internal/hookmatch deliberately do not import each other's
// internals beyond the public API — SSP-002), then drives the real stopHook
// entry point so the rendered line is exactly what a session would see.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

const deliveryMapFixture = `schema_version: 1
globs:
  "commands/adr/new.md":
    - id: ADR-900:d01
      grade: must
      surface: topic-file
    - id: ADR-900:d02
      grade: advisory
      surface: topic-file
  "commands/gov/audit.md":
    - id: ADR-900:d01
      grade: must
      surface: topic-file
    - id: SPEC-900:d01
      grade: advisory
      surface: skill
`

func writeDeliveryMap(t *testing.T, root string) {
	t.Helper()
	writeTestFile(t, filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml"), deliveryMapFixture)
}

func newStopHookInvocationWithSession(msg, sessionID string) *Invocation {
	inv := newStopHookInvocation(msg)
	inv.Payload.SessionID = sessionID
	return inv
}

func TestStopHookTally_AppendsWhenAlreadyNonEmpty(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)

	// Two distinct touched files, deduping one repeated directive id across
	// them (ADR-900:d01 appears on both globs above) — N must be 3
	// (ADR-900:d01, ADR-900:d02, SPEC-900:d01), not 4.
	t.Setenv("HOME", home)
	for _, r := range []hookmatch.Record{
		{Outcome: "matched", SessionID: "sess-123", Path: "commands/adr/new.md"},
		{Outcome: "matched", SessionID: "sess-123", Path: "commands/gov/audit.md"},
	} {
		if err := hookmatch.Append(r); err != nil {
			t.Fatal(err)
		}
	}

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-123"))
	if !strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("control: the decision signal must still fire; got %q", got.Response.Message)
	}
	want := "governance this session: 3 rules in scope for 2 files touched " +
		"(ambient + topic + skill loads, hook-journal.jsonl session sess-123); " +
		// This fire's own adr-candidate signal is itself freshly appended to
		// the ledger before the tally is computed, so held is 1, not 0 — a
		// candidate surfaced this very turn is still an outstanding one.
		"1 candidate artifact(s) held (stop-hook)"
	if !strings.Contains(got.Response.Message, want) {
		t.Fatalf("message = %q, want it to contain %q", got.Response.Message, want)
	}
}

func TestStopHookTally_NoCitedClauseYet(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if strings.Contains(got.Response.Message, "cited") {
		t.Fatalf("SR-006's counter does not exist yet — the line must carry no cited clause at all; got %q",
			got.Response.Message)
	}
}

// TestStopHookTally_Cited covers SR-006/AC-5.3: once a post-flight
// report carries cited_directives, the tally line appends "; K cited
// by phase-end review (post-flight reports)" with K the count of
// distinct directive ids cited across all reports on disk.
func TestStopHookTally_Cited(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}
	// Two reports; ADR-900.directive[0] cited by both (deduped), plus
	// one distinct id in the second report — K must be 2, not 3.
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "post-flight", "x-1-20260101T000000Z.json"),
		`{"cited_directives":["ADR-900.directive[0]"]}`)
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "post-flight", "x-2-20260102T000000Z.json"),
		`{"cited_directives":["ADR-900.directive[0]","SPEC-900.directive[3]"]}`)

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if !strings.Contains(got.Response.Message, "2 cited by phase-end review (post-flight reports)") {
		t.Fatalf("message = %q, want it to contain the cited clause with count 2", got.Response.Message)
	}
}

// TestStopHookTally_CitedIgnoresReportsWithoutTheField confirms a
// report predating cited_directives contributes nothing — neither a
// citation nor a false zero — matching TestStopHookTally_NoCitedClauseYet
// but with a mix of old and new reports present.
func TestStopHookTally_CitedIgnoresReportsWithoutTheField(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}
	// A pre-field report (no cited_directives key at all) alongside a
	// real one — the old report must not suppress or zero out the new
	// report's real citation.
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "post-flight", "x-1-20260101T000000Z.json"),
		`{"meta":{"plan":"x","phase":1}}`)
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "post-flight", "x-2-20260102T000000Z.json"),
		`{"cited_directives":["ADR-900.directive[0]"]}`)

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if !strings.Contains(got.Response.Message, "1 cited by phase-end review (post-flight reports)") {
		t.Fatalf("message = %q, want it to contain the cited clause with count 1", got.Response.Message)
	}
}

func TestStopHookTally_NoForbiddenWords(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	for _, forbidden := range []string{"saved", "prevented", "enforced"} {
		if strings.Contains(strings.ToLower(got.Response.Message), forbidden) {
			t.Errorf("message contains forbidden word %q: %q", forbidden, got.Response.Message)
		}
	}
}

func TestStopHookTally_HeldCandidatesCounted(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}
	// Pre-seed the ledger with one already-held candidate (a different
	// subject than this fire's own, so it does not collapse this fire's
	// own signal — it is there purely to be counted).
	writeTestFile(t, filepath.Join(root, ".edikt", "state", "hook-ledger.jsonl"),
		`{"ts":"2026-01-01T00:00:00Z","event":"surfaced","kind":"adr-candidate","subject":"prior decision","fingerprint":"aaaaaaaaaaaaaaaa"}`+"\n")

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if !strings.Contains(got.Response.Message, "2 candidate artifact(s) held") {
		// The pre-seeded fingerprint plus this fire's own freshly-surfaced
		// signal (a different fingerprint) — both count.
		t.Fatalf("message = %q, want it to report 2 held candidates", got.Response.Message)
	}
}

func TestStopHookTally_OmittedWhenNoJournal(t *testing.T) {
	root, _ := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	// No hookmatch journal ever written in this HOME.

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if !strings.Contains(got.Response.Message, "adr-candidate") {
		t.Fatalf("control: the decision signal must still fire; got %q", got.Response.Message)
	}
	if strings.Contains(got.Response.Message, "governance this session") {
		t.Fatalf("no journal exists — the tally must be omitted rather than report a false zero; got %q",
			got.Response.Message)
	}
}

func TestStopHookTally_OmittedWhenNoDeliveryMap(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	// No delivery-map.yaml written.
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}

	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "sess-1"))
	if strings.Contains(got.Response.Message, "governance this session") {
		t.Fatalf("no compiled delivery map exists — the tally must be omitted; got %q", got.Response.Message)
	}
}

func TestStopHookTally_OmittedWhenSessionIDMissing(t *testing.T) {
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "sess-1", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}

	// No session id on the payload at all.
	got := stopHook(newStopHookInvocation(stopHookDecisionMessage))
	if strings.Contains(got.Response.Message, "governance this session") {
		t.Fatalf("an empty session id must never be rendered; got %q", got.Response.Message)
	}
}

func TestSessionIDValidated(t *testing.T) {
	valid := []string{"a", "sess-123", "SESSION_ABC", strings.Repeat("a", 64)}
	for _, s := range valid {
		if !adapter.ValidSessionID(s) {
			t.Errorf("ValidSessionID(%q) = false, want true", s)
		}
	}
	invalid := []string{
		"",
		strings.Repeat("a", 65),
		"has spaces",
		"semi;colon",
		"path/traversal",
		"$(injection)",
		"quote\"here",
		"new\nline",
	}
	for _, s := range invalid {
		if adapter.ValidSessionID(s) {
			t.Errorf("ValidSessionID(%q) = true, want false", s)
		}
	}

	// End-to-end: a malformed session id must not reach the rendered line
	// even when everything else needed to compute the tally is present.
	root, home := stopHookSandbox(t)
	writeTestFile(t, filepath.Join(root, ".edikt", "config.yaml"),
		"edikt_version: 0.8.0\nsignals:\n  adr_candidate: eager\n")
	writeDeliveryMap(t, root)
	t.Setenv("HOME", home)
	if err := hookmatch.Append(hookmatch.Record{Outcome: "matched", SessionID: "has spaces", Path: "commands/adr/new.md"}); err != nil {
		t.Fatal(err)
	}
	got := stopHook(newStopHookInvocationWithSession(stopHookDecisionMessage, "has spaces"))
	if strings.Contains(got.Response.Message, "has spaces") || strings.Contains(got.Response.Message, "governance this session") {
		t.Fatalf("a malformed session id reached the rendered line: %q", got.Response.Message)
	}
}
