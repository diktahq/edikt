package hookmatch

import (
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// ResolveIdentity names who is being governed by a hook invocation, from the
// host payload alone.
//
// It lived in the write-time injection hook (inject-directives-pre) until
// ADR-083 retired that hook; the hook_run journal record still needs it, so  edikt-guard:allow
// it moved here rather than being reimplemented. The rule it encodes is that
// UNKNOWN IS NEVER GUESSED AS "parent": a payload that carries the AgentID
// key with an empty value is a subagent the host could not name, and is
// journaled as such — the parent session is only ever inferred from the key
// being ABSENT (ADR-074 / ADR-076).  edikt-guard:allow
func ResolveIdentity(p adapter.Payload) (actor, contextID, agentType string) {
	agentType = p.AgentType
	if agentType == "" {
		agentType = "-"
	}
	if !p.Present("AgentID") {
		return "parent", "parent", agentType
	}
	if aid := strings.TrimSpace(p.AgentID); aid != "" {
		return "subagent", aid, agentType
	}
	return "unknown", "", agentType
}

// governanceProducingAgents is the attested list of agents whose writes are
// governance PRODUCTION rather than governed work (INV-021): the enforcement  edikt-guard:allow
// they would otherwise be subject to is the enforcement they exist to
// produce. Membership is by the host-supplied agent_type on the payload —
// never by an env var or a field the writer sets itself, because those are
// the writer attesting to its own exemption.
var governanceProducingAgents = map[string]bool{
	"sidecar-extractor": true,
}

// IsGovernanceProducingAgent reports whether agentType is on the attested
// list. The empty and placeholder ("-") values are never exempt: an unnamed
// writer is a governed writer.
func IsGovernanceProducingAgent(agentType string) bool {
	if agentType == "" || agentType == "-" {
		return false
	}
	return governanceProducingAgents[agentType]
}
