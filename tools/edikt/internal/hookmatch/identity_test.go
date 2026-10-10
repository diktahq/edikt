package hookmatch

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

func TestGovernanceProducingAgents(t *testing.T) {
	if !IsGovernanceProducingAgent("sidecar-extractor") {
		t.Fatal("sidecar-extractor is the attested governance producer (INV-021)")
	}
	for _, not := range []string{"", "-", "governance-verifier", "Sidecar-Extractor", "sidecar-extractor "} {
		if IsGovernanceProducingAgent(not) {
			t.Errorf("%q must not be exempt", not)
		}
	}
}

func TestResolveIdentity_UnknownIsNeverGuessedAsParent(t *testing.T) {
	actor, ctx, at := ResolveIdentity(adapter.Payload{})
	if actor != "parent" || ctx != "parent" || at != "-" {
		t.Fatalf("no AgentID key at all is the parent session: got %s/%s/%s", actor, ctx, at)
	}
	p, err := adapter.Parse([]byte(`{"hook_event_name":"PreToolUse","agent_id":"a1","agent_type":"explore"}`))
	if err != nil {
		t.Fatal(err)
	}
	actor, ctx, at = ResolveIdentity(p)
	if actor != "subagent" || ctx != "a1" || at != "explore" {
		t.Fatalf("named subagent: got %s/%s/%s", actor, ctx, at)
	}
}
