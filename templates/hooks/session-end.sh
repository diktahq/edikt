#!/usr/bin/env bash
# edikt: SessionEnd hook — flush event log, write session summary
# Fires when a Claude Code session ends. Symmetric counterpart to session-start.sh.
#
# Output: {"continue": true} (JSON per Claude Code hook protocol)

set -uo pipefail

# Fast opt-out
[ "${EDIKT_SESSION_END_SKIP:-0}" = "1" ] && { printf '{"continue": true}\n'; exit 0; }

# Only run in edikt projects
if [ ! -f '.edikt/config.yaml' ]; then
    printf '{"continue": true}\n'
    exit 0
fi

INPUT=$(cat 2>/dev/null || echo '{}')

# Extract session_id from payload (optional)
SESSION_ID=$(printf '%s' "$INPUT" | python3 -c '
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get("session_id", ""))
except Exception:
    pass
' 2>/dev/null)

mkdir -p "$HOME/.edikt/state" 2>/dev/null || true
SUMMARY_FILE="$HOME/.edikt/state/last-session.json"
EVENTS_FILE="$HOME/.edikt/events.jsonl"

# Rotate events.jsonl if over 10MB
if [ -f "$EVENTS_FILE" ]; then
    SIZE=$(wc -c < "$EVENTS_FILE" 2>/dev/null | tr -d ' ' || echo 0)
    if [ "${SIZE:-0}" -gt 10485760 ]; then
        DATE=$(date +%Y%m%d)
        mv "$EVENTS_FILE" "${EVENTS_FILE}.${DATE}" 2>/dev/null || true
    fi
fi

# Write session summary (best-effort; failures don't block).
# Use ensure_ascii=False + errors="replace" to tolerate exotic paths with
# non-UTF8 bytes — falls back to placeholder chars rather than dropping
# the summary entirely.
TS=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
python3 -c "
import json, sys
def safe(s):
    try:
        return s.encode('utf-8', errors='replace').decode('utf-8')
    except Exception:
        return repr(s)
summary = {
    'ts': safe(sys.argv[1]),
    'session_id': safe(sys.argv[2]),
    'cwd': safe(sys.argv[3]),
}
try:
    with open(sys.argv[4], 'w', encoding='utf-8') as f:
        json.dump(summary, f, indent=2, ensure_ascii=False)
except Exception as e:
    # Last-ditch: write an error marker so the user knows the summary existed
    try:
        with open(sys.argv[4], 'w') as f:
            json.dump({'ts': safe(sys.argv[1]), 'error': str(e)}, f)
    except Exception:
        pass
" "$TS" "$SESSION_ID" "$PWD" "$SUMMARY_FILE" 2>/dev/null || true

printf '{"continue": true}\n'
