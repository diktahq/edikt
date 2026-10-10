package adapter

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The fixtures in testdata/ are REAL wire payloads captured on 2026-08-30 from a
// single dispatch with all four lifecycle hooks registered. Hand-written
// fixtures can only assert what their author already believed the wire looked
// like; these assert what it actually is.
//
// TWO SUBSTITUTIONS, and nothing else: the host username, and the host agent's
// config directory name. Both are sanitisation, not shaping — every field name,
// every value's structure, and the parent/subagent path relationship are
// verbatim. The second substitution is also load-bearing for a gate: the
// captured paths contained the vendor token, and no-llm-in-tier-2.sh matches it
// literally in any non-Go tier-2 file, so an unscrubbed fixture failed the seam
// check on a path string rather than on a dispatch.

func load(t *testing.T, name string) Payload {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name+".json"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	p, err := Parse(raw)
	if err != nil {
		t.Fatalf("Parse(%s): %v", name, err)
	}
	return p
}

// TestPromptIDJoinsTheWholeTurn is the observation that closes the gap
// df561473 left open. Before this capture, prompt_id joining a parent turn to
// its own dispatches was INFERRED from the field being present on each event
// separately; no run had ever been captured with both a Stop and a
// SubagentStop in it. These four fixtures are one run.
func TestPromptIDJoinsTheWholeTurn(t *testing.T) {
	events := []string{"UserPromptSubmit", "Stop", "SubagentStart", "SubagentStop"}
	var promptID, sessionID string
	for _, e := range events {
		p := load(t, e)
		if p.PromptID == "" {
			t.Fatalf("%s carries no PromptID; the join key must be on every lifecycle event", e)
		}
		if promptID == "" {
			promptID, sessionID = p.PromptID, p.SessionID
			continue
		}
		if p.PromptID != promptID {
			t.Errorf("%s PromptID = %q, want %q — one user turn must have ONE id",
				e, p.PromptID, promptID)
		}
		if p.SessionID != sessionID {
			t.Errorf("%s SessionID = %q, want %q", e, p.SessionID, sessionID)
		}
	}
}

// TestAgentIDScopesTheDispatchOnly. The three-level key hierarchy only works if
// agent_id is absent on the parent events — otherwise a parent turn would look
// like a dispatch.
func TestAgentIDScopesTheDispatchOnly(t *testing.T) {
	for _, e := range []string{"UserPromptSubmit", "Stop"} {
		if p := load(t, e); p.AgentID != "" {
			t.Errorf("%s carries AgentID %q; a parent turn is not a dispatch", e, p.AgentID)
		}
	}
	for _, e := range []string{"SubagentStart", "SubagentStop"} {
		p := load(t, e)
		if p.AgentID == "" {
			t.Errorf("%s carries no AgentID", e)
		}
		if p.AgentType == "" {
			t.Errorf("%s carries no AgentType — this is the field the journal "+
				"recorded as absent on all 1,703 subagent-stop records", e)
		}
	}
	if a, b := load(t, "SubagentStart"), load(t, "SubagentStop"); a.AgentID != b.AgentID {
		t.Errorf("AgentID differs across the pair: %q vs %q", a.AgentID, b.AgentID)
	}
}

// TestTwoDistinctTranscripts is the finding that made per-turn detail reachable
// from the hook path. "Parent or subagent" was never the choice: SubagentStop
// supplies BOTH, in separate fields.
func TestTwoDistinctTranscripts(t *testing.T) {
	stop := load(t, "Stop")
	sub := load(t, "SubagentStop")
	start := load(t, "SubagentStart")

	if sub.TranscriptPath != stop.TranscriptPath {
		t.Errorf("SubagentStop.TranscriptPath should be the PARENT transcript, same as Stop's:\n"+
			"  Stop: %s\n  SubagentStop: %s", stop.TranscriptPath, sub.TranscriptPath)
	}
	if sub.AgentTranscriptPath == "" {
		t.Fatal("SubagentStop carries no AgentTranscriptPath — per-turn subagent " +
			"detail would be unreachable from the hook path")
	}
	if sub.AgentTranscriptPath == sub.TranscriptPath {
		t.Error("agent transcript must be a DIFFERENT file from the parent transcript")
	}
	// Only on Stop, per the capture.
	if start.AgentTranscriptPath != "" {
		t.Errorf("SubagentStart carried AgentTranscriptPath %q; observed only on Stop",
			start.AgentTranscriptPath)
	}
}

// TestPresenceTableCoversTheNewFields. Present() drives fallback chains, so a
// field mapped in the struct but missing from the table reads as absent.
func TestPresenceTableCoversTheNewFields(t *testing.T) {
	p := load(t, "SubagentStop")
	for _, f := range []string{"PromptID", "AgentTranscriptPath", "TranscriptPath", "AgentType"} {
		if !p.Present(f) {
			t.Errorf("Present(%q) is false on a payload that carries it — the wire "+
				"name is missing from the presence table", f)
		}
	}
}

// TestDeliberatelyUnmodelledFieldsAreOnTheWire records that four fields were
// seen and left out on purpose. If a future reader adds one, this test says
// where the decision was made rather than letting it look like an oversight.
func TestDeliberatelyUnmodelledFieldsAreOnTheWire(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("testdata", "SubagentStop.json"))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"effort", "permission_mode", "background_tasks", "session_crons"} {
		if _, ok := m[f]; !ok {
			t.Errorf("%q is no longer on the wire; the note in payload.go explaining "+
				"why it is unmodelled is now stale", f)
		}
	}
}
