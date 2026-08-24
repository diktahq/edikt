package hook

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// TestUnregisteredHookName — SP-007's "must not silently drop".
//
// Exit 0 (fail-open), a stderr diagnostic NAMING the name, and a distinctly
// typed journal record. Silence is never a valid response to this case: a shim
// pointing at an unknown name means that hook's governance is not firing, and
// it looks identical to a hook with nothing to say.
func TestUnregisteredHookName(t *testing.T) {
	var stdout, stderr bytes.Buffer
	cap := &capture{}
	code := Dispatch(Options{
		Name:     "definitely-not-a-hook",
		Stdin:    strings.NewReader("{}"),
		Stdout:   &stdout,
		Stderr:   &stderr,
		Registry: NewRegistry(),
		Journal:  cap.append,
	})

	if code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}
	if !strings.Contains(stderr.String(), "definitely-not-a-hook") {
		t.Fatalf("stderr does not name the unresolved hook: %q", stderr.String())
	}
	var reply map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(stdout.Bytes()), &reply); err != nil {
		t.Fatalf("stdout is not valid JSON: %q (%v)", stdout.String(), err)
	}
	if len(reply) != 0 {
		t.Fatalf("stdout: got %q, want an empty hook-protocol reply", stdout.String())
	}

	if len(cap.records) != 1 {
		t.Fatalf("journal records: got %d, want 1 — an unregistered name must never be a silent no-op", len(cap.records))
	}
	r := cap.records[0]
	if r.Outcome != string(hookmatch.OutcomeUnregisteredHookName) {
		t.Fatalf("outcome: got %q, want %q", r.Outcome, hookmatch.OutcomeUnregisteredHookName)
	}
	if r.Hook != "definitely-not-a-hook" {
		t.Fatalf("hook: got %q, want the requested name", r.Hook)
	}
	if !hookmatch.Outcome(r.Outcome).Suppressed() {
		t.Fatal("unregistered-hook-name is not Suppressed() — doctor would count it as a healthy invocation")
	}
	// Distinctness, the property the type exists for.
	for _, other := range []hookmatch.Outcome{
		hookmatch.OutcomeNoMatch, hookmatch.OutcomeMatched, hookmatch.OutcomeIndexMissing,
		hookmatch.OutcomeIndexCorrupt, hookmatch.OutcomeIndexEmpty, hookmatch.OutcomeBadPath,
		hookmatch.OutcomeHookRan, hookmatch.OutcomePanicRecovered, hookmatch.OutcomePayloadUnparseable,
	} {
		if hookmatch.OutcomeUnregisteredHookName == other {
			t.Fatalf("unregistered-hook-name collides with %q", other)
		}
	}
}

// TestJournalRecordFields — AC-6.7. Every registered hook journals ONE record
// per run carrying name, outcome, exit code and duration.
//
// Before this, the journal carried only pre/post/probe: three shims out of
// twenty-two hooks, so "did session-start fire?" had no answer anywhere.
// Phases 15 and 19 gate the migration on exactly this evidence.
func TestJournalRecordFields(t *testing.T) {
	names := dispatchableNames(t)
	t.Logf("%d hook name(s) under test", len(names))

	clock := &fakeClock{now: time.Unix(1700000000, 0)}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			reg := NewRegistry()
			reg.Register(name, func(inv *Invocation) Result {
				clock.advance(7 * time.Millisecond)
				return Result{Response: adapter.Response{Context: "hello"}, ExitCode: 0}
			})
			cap := &capture{}
			var stdout bytes.Buffer
			code := Dispatch(Options{
				Name: name, Stdin: strings.NewReader(`{"hook_event_name":"SessionStart","session_id":"s9"}`),
				Stdout: &stdout, Stderr: &bytes.Buffer{},
				Registry: reg, Journal: cap.append, Now: clock.Now,
			})
			if code != 0 {
				t.Fatalf("exit code: got %d, want 0", code)
			}
			if len(cap.records) != 1 {
				t.Fatalf("journal records: got %d, want exactly 1 per run", len(cap.records))
			}
			r := cap.records[0]
			if r.Kind != hookmatch.KindHookRun {
				t.Errorf("kind: got %q, want %q", r.Kind, hookmatch.KindHookRun)
			}
			if r.Hook != name {
				t.Errorf("hook: got %q, want %q", r.Hook, name)
			}
			if r.Outcome != string(hookmatch.OutcomeHookRan) {
				t.Errorf("outcome: got %q, want %q", r.Outcome, hookmatch.OutcomeHookRan)
			}
			if r.ExitCode == nil {
				t.Fatal("exit_code is nil — a hook that exited 0 must be distinguishable from a record that carried no exit code")
			}
			if *r.ExitCode != 0 {
				t.Errorf("exit_code: got %d, want 0", *r.ExitCode)
			}
			if r.DurationUS == nil {
				t.Fatal("duration_us is nil")
			}
			if *r.DurationUS != 7000 {
				t.Errorf("duration_us: got %d, want 7000", *r.DurationUS)
			}
		})
	}
}

// TestJournalCarriesNonZeroExitCode — a gate's deny must be readable in the
// journal, not flattened into the same record a clean run writes.
func TestJournalCarriesNonZeroExitCode(t *testing.T) {
	reg := NewRegistry()
	reg.Register("verify-gate", func(*Invocation) Result {
		return Result{ExitCode: 2, Response: adapter.Response{Decision: adapter.DecisionDeny, Reason: "nope"}}
	})
	cap := &capture{}
	var stdout bytes.Buffer
	code := Dispatch(Options{
		Name: "verify-gate", Stdin: strings.NewReader(`{"hook_event_name":"PreToolUse"}`),
		Stdout: &stdout, Stderr: &bytes.Buffer{}, Registry: reg, Journal: cap.append,
	})
	if code != 2 {
		t.Fatalf("exit code: got %d, want 2 — the handler's code must pass through unchanged", code)
	}
	if len(cap.records) != 1 || cap.records[0].ExitCode == nil || *cap.records[0].ExitCode != 2 {
		t.Fatalf("journal did not record exit code 2: %+v", cap.records)
	}
	if !strings.Contains(stdout.String(), "permissionDecision") {
		t.Fatalf("reply did not carry the deny: %q", stdout.String())
	}
}

// TestUnparseablePayload — the hook was found and never ran. Its own class,
// because "found and could not run" is not "never found" and not "ran and
// broke" (ADR-060:d18).
func TestUnparseablePayload(t *testing.T) {
	reg := NewRegistry()
	ran := false
	reg.Register("session-start", func(*Invocation) Result { ran = true; return Result{} })

	var stdout, stderr bytes.Buffer
	cap := &capture{}
	code := Dispatch(Options{
		Name: "session-start", Stdin: strings.NewReader(`{"tool_input": [unclosed`),
		Stdout: &stdout, Stderr: &stderr, Registry: reg, Journal: cap.append,
	})
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}
	if ran {
		t.Fatal("the handler ran on an undecodable payload")
	}
	if len(cap.records) != 1 || cap.records[0].Outcome != string(hookmatch.OutcomePayloadUnparseable) {
		t.Fatalf("journal: got %+v, want one payload-unparseable record", cap.records)
	}
	if !strings.Contains(stderr.String(), "did NOT run") {
		t.Errorf("stderr does not say the hook did not run: %q", stderr.String())
	}
}

// TestEmptyStdinIsNotAnError — several events fire with no body. Treating that
// as a parse failure would journal a suppression for a hook working perfectly.
func TestEmptyStdinIsNotAnError(t *testing.T) {
	reg := NewRegistry()
	ran := false
	reg.Register("cwd-changed", func(*Invocation) Result { ran = true; return Result{} })
	cap := &capture{}
	Dispatch(Options{
		Name: "cwd-changed", Stdin: strings.NewReader("  \n"),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Registry: reg, Journal: cap.append,
	})
	if !ran {
		t.Fatal("the handler did not run on an empty payload")
	}
	if len(cap.records) != 1 || cap.records[0].Outcome != string(hookmatch.OutcomeHookRan) {
		t.Fatalf("journal: got %+v, want one hook-ran record", cap.records)
	}
}

// TestHandlerReceivesHostNeutralPayload — the seam, from the handler's side.
func TestHandlerReceivesHostNeutralPayload(t *testing.T) {
	reg := NewRegistry()
	var got adapter.Payload
	reg.Register("pre-tool-use", func(inv *Invocation) Result { got = inv.Payload; return Result{} })

	Dispatch(Options{
		Name: "pre-tool-use",
		Stdin: strings.NewReader(`{"hook_event_name":"PreToolUse","session_id":"s1","tool_name":"Edit",
			"tool_input":{"file_path":"a/b.go","old_string":"x"},"subagent_type":"qa"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{}, Registry: reg, Journal: (&capture{}).append,
	})
	if got.Event != "PreToolUse" || got.SessionID != "s1" || got.ToolName != "Edit" {
		t.Fatalf("payload not decoded: %+v", got)
	}
	if got.FilePath != "a/b.go" || got.OldString != "x" {
		t.Fatalf("promoted tool-input fields not decoded: %+v", got)
	}
	if got.AgentType != "qa" {
		t.Fatalf("agent type: got %q, want qa", got.AgentType)
	}
	if !got.Present("FilePath") {
		t.Error("FilePath should report Present")
	}
	if got.Present("Prompt") {
		t.Error("Prompt was not sent and must not report Present")
	}
}

type fakeClock struct{ now time.Time }

func (c *fakeClock) Now() time.Time          { return c.now }
func (c *fakeClock) advance(d time.Duration) { c.now = c.now.Add(d) }
