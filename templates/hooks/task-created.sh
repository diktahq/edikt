#!/usr/bin/env bash
# edikt: TaskCreated hook — log task creation with active plan phase context.
# Pairs with task-completed.sh to reconstruct plan progress.
#
# Output: {"continue": true} (JSON per Claude Code hook protocol)

set -uo pipefail

if [ ! -f '.edikt/config.yaml' ]; then
    printf '{"continue": true}\n'
    exit 0
fi

INPUT=$(cat 2>/dev/null || echo '{}')

# Extract task metadata
TASK_ID=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("task_id", "") or d.get("id", ""))
except Exception:
    pass
' 2>/dev/null)

TASK_NAME=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("task_name", "") or d.get("name", "") or d.get("subject", "unknown"))
except Exception:
    pass
' 2>/dev/null)

# Resolve the active plan file (walk up from $PWD for config, then locate plans dir)
BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
BASE="${BASE:-docs}"
PLANS_DIR=$(grep '^  plans:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
PLANS_DIR="${PLANS_DIR:-$BASE/plans}"

# Find the most recently modified plan file (primary + fallback location)
PLAN_FILE=""
for dir in "$PLANS_DIR" "$BASE/product/plans" "docs/product/plans"; do
    [ -d "$dir" ] || continue
    CAND=$(find "$dir" -maxdepth 1 -type f -name 'PLAN-*.md' 2>/dev/null | while read f; do
        printf '%s\t%s\n' "$(stat -f %m "$f" 2>/dev/null || stat -c %Y "$f" 2>/dev/null)" "$f"
    done | sort -rn | head -1 | cut -f2)
    if [ -n "$CAND" ]; then
        PLAN_FILE="$CAND"
        break
    fi
done

# Parse current in-progress phase number from the plan's progress table
PHASE=""
if [ -n "$PLAN_FILE" ] && [ -f "$PLAN_FILE" ]; then
    PHASE_LINE=$(grep -iE '^\|[[:space:]]*[0-9]+[ab]?[[:space:]]*\|.*in[_ -]?progress' "$PLAN_FILE" 2>/dev/null | head -1)
    if [ -n "$PHASE_LINE" ]; then
        PHASE=$(printf '%s' "$PHASE_LINE" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '^[0-9]+[ab]?')
    fi
fi

# Append event to the jsonl log
mkdir -p "$HOME/.edikt" 2>/dev/null || true
EVENTS="$HOME/.edikt/events.jsonl"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)

python3 -c "
import json, sys
event = {
    'ts': sys.argv[1],
    'event': 'task_created',
    'task_id': sys.argv[2],
    'task_name': sys.argv[3],
    'phase': sys.argv[4] or None,
    'plan': sys.argv[5] or None,
}
print(json.dumps(event))
" "$TS" "$TASK_ID" "$TASK_NAME" "$PHASE" "$(basename "$PLAN_FILE" 2>/dev/null)" \
    >> "$EVENTS" 2>/dev/null || true

printf '{"continue": true}\n'
