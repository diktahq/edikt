#!/usr/bin/env bash
# edikt: SubagentStart hook — inject governance context when a subagent spawns.
# Pairs with the existing subagent-stop.sh.
#
# Output: {"additionalContext": "..."} — governance heads-up for the subagent
#         OR {"continue": true} silent pass-through if no edikt project.

set -uo pipefail

# Fast opt-out
[ "${EDIKT_SUBAGENT_START_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

if [ ! -f '.edikt/config.yaml' ]; then
    printf '{"continue": true}\n'
    exit 0
fi

INPUT=$(cat 2>/dev/null || echo '{}')

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

python3 -c 'import json,sys; print(json.dumps({"additionalContext":sys.argv[1]}))' "$MSG"
