#!/usr/bin/env bash
# edikt: TaskCompleted hook — close the loop on TaskCreated.
# Pairs with task-created.sh. Emits a structured event to events.jsonl.
#
# Output: {"continue": true}

set -uo pipefail

# Fast opt-out
[ "${EDIKT_TASK_COMPLETED_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

if [ ! -f '.edikt/config.yaml' ]; then
    printf '{"continue": true}\n'
    exit 0
fi

INPUT=$(cat 2>/dev/null || echo '{}')

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
    print(d.get("task_name", "") or d.get("name", "") or d.get("subject", ""))
except Exception:
    pass
' 2>/dev/null)

STATUS=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("status", "") or d.get("result", "completed"))
except Exception:
    pass
' 2>/dev/null)

mkdir -p "$HOME/.edikt" 2>/dev/null || true
EVENTS="$HOME/.edikt/events.jsonl"
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)

python3 -c "
import json, sys
event = {
    'ts': sys.argv[1],
    'event': 'task_completed',
    'task_id': sys.argv[2],
    'task_name': sys.argv[3],
    'status': sys.argv[4],
}
print(json.dumps(event))
" "$TS" "$TASK_ID" "$TASK_NAME" "$STATUS" >> "$EVENTS" 2>/dev/null || true

printf '{"continue": true}\n'
