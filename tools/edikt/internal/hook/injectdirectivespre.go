package hook

// injectdirectivespre.go — the Go replacement for
// templates/hooks/inject-directives-pre.sh (PLAN-v080-reliability-core Phase
// 22). The MUST-grade write-time injection tier, and the last deny-gate.
//
// THE E1 SPLIT. BRAIN-005 E1 measured two delivery shapes against the same
// directives: advisory text appended after the fact was read and largely
// ignored; a DENY that names the rule and forces a retry was complied with. So
// the grades get different channels and this handler carries only one —
// MUST-grade, as a bounce. Advisory goes to PostToolUse additionalContext.
// Mixing them would spend the bounce's force on advice, and an agent denied
// for a suggestion learns the gate is noise.
//
// ITS DENY CONTRACT IS ADR-060's, NOT ADR-038's. Both this hook and verify-gate
// deny, and they are deliberately NOT the same shape: ADR-060 specifies
// permissionDecision/deny for this tier so the model revises the write and
// proceeds, while ADR-038 specifies a turn-stopping shape for a completion
// claim made without evidence. The bash original regressed from its own
// prototype on exactly this point, emitting a shape that stopped the turn
// while its own message said "revise the write to comply, then proceed". The
// two must not be unified without an ADR.
//
// TWO DEGRADATION BANNERS ARE NOT PORTED, and this is the third and fourth
// application of ADR-063:d02 in this migration.
//
//	python3 absent   — true of a shim whose JSON construction WAS python;
//	                   false of a handler with no interpreter dependency.
//	binary absent    — the bash shim resolved `$EDIKT_BIN` through ADR-041's
//	                   ladder and degraded when it found nothing. Its own
//	                   comment calls that "the third instance of the same
//	                   resolution bug" already fixed twice elsewhere. This
//	                   handler IS the binary and calls injectmatch in-process,
//	                   so the branch cannot occur — the migration removes the
//	                   whole class rather than reproducing its symptom.
//
// FAIL-OPEN, NEVER SILENT (ADR-060:d07). Every early return here lets the
// write through, deliberately: blocking an editor because governance is broken
// is worse than missing an injection. What makes it non-silent is that
// injectmatch journals its outcome on every call, so a suppressed chain is
// reportable via `edikt hook report` rather than invisible.

import (
	"os"
	"strconv"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
	"github.com/diktahq/edikt/tools/edikt/internal/injectmatch"
)

func init() {
	// PAYLOAD-TOLERANT, but for the OPPOSITE reason to verify-gate's.
	//
	// verify-gate opts in so an unparseable payload still DENIES (INV-011:
	// there the payload is the claim being checked, so absence of inspectable
	// input is not a pass). This hook opts in so an unparseable payload still
	// emits bash's explicit `{"continue": true}` rather than the dispatcher's
	// bare `{}`. Both are allows and the host reads them identically, but the
	// bytes differ and the corpus compares bytes — and more importantly the
	// fail-open belongs to THIS hook's contract (ADR-060:d07), which means it
	// should be visible in this file rather than inherited from a default.
	RegisterPayloadTolerant("inject-directives-pre", injectDirectivesPre)
}

// writeTools are the only tools that carry a path worth matching.
//
// A Bash write (`>`, `tee`, `sed -i`) is NOT covered. That is a recorded gap
// (FR-009), not an oversight: matching it means parsing shell, and a
// half-working parser would give false confidence that Bash writes are
// governed.
var writeTools = map[string]bool{
	"Write":        true,
	"Edit":         true,
	"MultiEdit":    true,
	"NotebookEdit": true,
}

func injectDirectivesPre(inv *Invocation) Result {
	// The compile escape hatch, checked first. `gov compile` writes the very
	// surfaces this hook reads, and a hook that fired on its own inputs would
	// deadlock the compile.
	if os.Getenv("EDIKT_COMPILE_IN_PROGRESS") == "1" {
		return allowPlain()
	}

	// An uninspectable payload fails OPEN here — the opposite of verify-gate's
	// ruling on the same input, and deliberately so. ADR-060:d07 governs this
	// tier; INV-011 governs verify-gate, where the payload IS the claim.
	if inv.PayloadErr != nil {
		return allowPlain()
	}

	p := inv.Payload
	if !writeTools[p.ToolName] {
		return allowPlain()
	}

	filePath := p.FilePath
	if filePath == "" {
		filePath = p.Path
	}
	// "-" is bash's own no-path sentinel, carried because a payload could
	// legitimately contain it and the two implementations must agree.
	if filePath == "" || filePath == "-" {
		return allowPlain()
	}

	actor, contextID, agentType := resolveInjectionIdentity(p)

	root, err := inv.Root()
	if err != nil {
		// No root, nothing to match against. Fail open, and injectmatch is not
		// reached, so there is nothing to journal — the one class this tier
		// cannot record, same as the bash original's missing-binary case.
		return allowPlain()
	}

	out := injectmatch.Run(injectmatch.Request{
		Root:      root,
		Path:      filePath,
		Grade:     "must",
		SessionID: p.SessionID,
		Shim:      "pre",
		Dedup:     true,
		Actor:     actor,
		AgentType: agentType,
		ContextID: contextID,
	}, inv.Stderr)

	if len(out.Entries) == 0 {
		// Includes every fail-open class and the deduped repeat. injectmatch
		// has already journaled WHY, so this is not a silent pass — it is a
		// pass whose reason is recorded where a human can read it.
		return allowPlain()
	}

	lines := make([]string, 0, len(out.Entries)*2)
	for _, e := range out.Entries {
		lines = append(lines, "- "+e.Text+" ("+e.ID+")")
		if e.Intent != "" {
			lines = append(lines, "  intent: "+e.Intent)
		}
	}
	body := "edikt: this write touches " + out.NormPath + ", which is governed by " +
		strconv.Itoa(len(out.Entries)) + " MUST-grade directive(s):\n\n" + strings.Join(lines, "\n") + "\n\n"

	if out.BudgetExhausted {
		// F-052 — PAST THE BUDGET THE WRITE PROCEEDS, BUT NOT QUIETLY.
		// Denying the same directive set on every write is a loop the agent
		// cannot escape, and the rational response to an inescapable gate is
		// to route around it. Going SILENT past the budget would instead be
		// the "proceeds unheard" outcome the budget exists to rule out, so the
		// directives are delivered on the allow path's model-facing channel.
		return Result{Response: adapter.Response{
			Context: body +
				"This session bounce budget for this exact directive set " +
				"(hooks.injection.bounce_budget) is spent, so this write proceeds. " +
				"The directives still apply; revise if this write does not comply.",
		}}
	}

	return Result{Response: adapter.Response{
		Decision: adapter.DecisionDeny,
		Reason: body +
			"These are non-negotiable. Revise the write to comply, then proceed — " +
			"you will not be stopped again for these directives in this session.",
	}}
}

// resolveInjectionIdentity resolves WHO is being governed, for the dedup key.
//
// Dedup keys on the context that RECEIVES the injection, never on a session
// spanning several: a subagent that never saw the parent's bounce must not
// inherit its suppression, and measurement showed it demonstrably had not.
//
// ABSENT IS NOT EMPTY, and the three-way distinction is the whole function.
// No agent_id key at all means the main session — a RESOLVED identity that may
// legitimately share the parent dedup marker. An agent_id present but blank
// means the host told us something we could not read, which is UNKNOWN, and
// unknown must DELIVER rather than suppress. Collapsing the two let a blank id
// inherit parent suppression once already.
//
// Over-deliver rather than under-deliver: an extra bounce costs one
// regenerated write and is visible; a silent suppression costs the enforcement
// claim and is not.
func resolveInjectionIdentity(p adapter.Payload) (actor, contextID, agentType string) {
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
