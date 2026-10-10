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

// TestJournalCarriesSubagentIdentity is the Stage 0 control.
//
// BEFORE this change, every subagent-stop record was kind=hook_run carrying
// only hook/exit_code/duration_us — 1,703 of them in this repo's journal, none
// able to say WHICH skill produced it, while the host was sending agent_type on
// the very payload the hook received. A trace that cannot name its skill is the
// same defect one layer up, so this asserts the identity reaches the record.
func TestJournalCarriesSubagentIdentity(t *testing.T) {
	// Real wire shape, matching the 2026-08-30 capture.
	const payload = `{"hook_event_name":"SubagentStop","session_id":"sess-1",` +
		`"prompt_id":"prompt-1","agent_id":"agent-1","agent_type":"sidecar-extractor",` +
		`"transcript_path":"/p/sess-1.jsonl",` +
		`"agent_transcript_path":"/p/sess-1/subagents/agent-agent-1.jsonl"}`

	reg := NewRegistry()
	reg.Register("subagent-stop", func(inv *Invocation) Result { return Result{ExitCode: 0} })
	cap := &capture{}
	if code := Dispatch(Options{
		Name: "subagent-stop", Stdin: strings.NewReader(payload),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Registry: reg, Journal: cap.append,
	}); code != 0 {
		t.Fatalf("exit code: got %d, want 0", code)
	}
	if len(cap.records) != 1 {
		t.Fatalf("journal records: got %d, want 1", len(cap.records))
	}
	r := cap.records[0]
	for _, c := range []struct{ field, got, want string }{
		{"AgentType", r.AgentType, "sidecar-extractor"},
		{"ContextID", r.ContextID, "agent-1"},
		{"Actor", r.Actor, "subagent"},
		{"PromptID", r.PromptID, "prompt-1"},
		{"AgentTranscriptPath", r.AgentTranscriptPath, "/p/sess-1/subagents/agent-agent-1.jsonl"},
	} {
		if c.got != c.want {
			t.Errorf("%s: got %q, want %q", c.field, c.got, c.want)
		}
	}
}

// TestJournalIdentityAbsentWhenHostSendsNone is the NEGATIVE control, and it
// guards the distinction the scope doc got wrong once already: the constant "-"
// on 491 injection-path records is NOT edikt dropping the identity, it is the
// honest fallback for an event that carries none. This must keep reading as
// "the host sent nothing", never as a subagent.
func TestJournalIdentityAbsentWhenHostSendsNone(t *testing.T) {
	reg := NewRegistry()
	reg.Register("session-start", func(inv *Invocation) Result { return Result{ExitCode: 0} })
	cap := &capture{}
	Dispatch(Options{
		Name:   "session-start",
		Stdin:  strings.NewReader(`{"hook_event_name":"SessionStart","session_id":"s1"}`),
		Stdout: &bytes.Buffer{}, Stderr: &bytes.Buffer{},
		Registry: reg, Journal: cap.append,
	})
	r := cap.records[0]
	if r.Actor != "parent" {
		t.Errorf("Actor: got %q, want %q for an event with no agent identity", r.Actor, "parent")
	}
	if r.AgentType != "-" {
		t.Errorf("AgentType: got %q, want %q — the honest unknown, not a guess", r.AgentType, "-")
	}
	if r.PromptID != "" || r.AgentTranscriptPath != "" {
		t.Errorf("a payload with no prompt_id/agent_transcript_path must leave both "+
			"empty (omitempty), got PromptID=%q AgentTranscriptPath=%q",
			r.PromptID, r.AgentTranscriptPath)
	}
}
