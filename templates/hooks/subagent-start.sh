#!/usr/bin/env bash
# edikt: SubagentStart hook — inject governance context when a subagent spawns.
# Pairs with the existing subagent-stop.sh.
#
# Output: {"hookSpecificOutput": {"hookEventName": "SubagentStart",
#                                 "additionalContext": "..."}}
#         — governance heads-up for the subagent
#         OR {"continue": true} silent pass-through if no edikt project.

set -uo pipefail

# Fast opt-out
[ "${EDIKT_SUBAGENT_START_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

if [ ! -f '.edikt/config.yaml' ]; then
    printf '{"continue": true}\n'
    exit 0
fi

INPUT=$(cat 2>/dev/null || echo '{}')

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Absence used to fail silently at exit 127 with nothing on stdout — the
# governance heads-up this hook exists to deliver to every subagent (F-044)
# never arrived, and no one, not even the model, could tell.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: subagent-start — python3 not found; governance heads-up not delivered to this subagent" >&2
    # additionalContext, not systemMessage: the reader that needs to know it
    # is missing governance context is the subagent itself, matching this
    # hook's own established channel (F-044 / ADR-061 §4). A constant  edikt-guard:allow
    # literal — nothing interpolated, so json.dumps is unneeded here.
    printf '{"hookSpecificOutput":{"hookEventName":"SubagentStart","additionalContext":"⚠ edikt: python3 is missing on this host — the governance heads-up (ADR/INV count and citation reminder) could not be generated for this subagent. Proceed without it; do not assume no governance applies."}}\n'
    exit 0
fi

# Extract subagent_type to scope the context (optional)
SUBAGENT_TYPE=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("subagent_type", "") or d.get("agent", ""))
except Exception:
    pass
' 2>/dev/null)

# Count active governance
DECISIONS_DIR="docs/architecture/decisions"
INVARIANTS_DIR="docs/architecture/invariants"

ADR_COUNT=0
INV_COUNT=0

if [ -d "$DECISIONS_DIR" ]; then
    ADR_COUNT=$(grep -l -E '^status:[[:space:]]*accepted|^\*\*Status:\*\*[[:space:]]+Accepted' "$DECISIONS_DIR"/ADR-*.md 2>/dev/null | wc -l | tr -d ' ')
fi
if [ -d "$INVARIANTS_DIR" ]; then
    INV_COUNT=$(grep -l -E '^status:[[:space:]]*active|^\*\*Status:\*\*[[:space:]]+Active' "$INVARIANTS_DIR"/INV-*.md 2>/dev/null | wc -l | tr -d ' ')
fi

# Construct context message
if [ -n "$SUBAGENT_TYPE" ]; then
    MSG="Subagent ${SUBAGENT_TYPE} spawned. edikt governance active: ${ADR_COUNT} ADRs, ${INV_COUNT} invariants. Cite ADR-NNN / INV-NNN when a decision applies; defer to compiled rules in .claude/rules/ over memory."
else
    MSG="edikt governance active: ${ADR_COUNT} ADRs, ${INV_COUNT} invariants. Cite ADR-NNN / INV-NNN when a decision applies."
fi

# F-044 / ADR-061 §4: bare additionalContext is dropped by the host, so this  edikt-guard:allow
# governance heads-up was never delivered to any subagent. Wrapped now.
python3 -c 'import json,sys; print(json.dumps({"hookSpecificOutput":{"hookEventName":"SubagentStart","additionalContext":sys.argv[1]}}))' "$MSG"
