#!/usr/bin/env bash
# edikt: StopFailure hook — log API errors to events.jsonl
# Fires when a turn ends due to an API error (rate limit, auth failure, etc.)

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# StopFailure has no established stdout-JSON contract (this hook's success
# path prints nothing; its whole job is a silent events.jsonl log write), so
# stderr is the only channel available that does not invent a new protocol
# shape. Absence used to fail every extraction and the log write silently.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: stop-failure — python3 not found; this API error was not logged" >&2
    exit 0
fi

# Extract error details (already safely extracted via python3)
ERROR_TYPE=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('error',{}).get('type','unknown'))" 2>/dev/null || echo "unknown")
ERROR_MSG=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('error',{}).get('message','')[:200])" 2>/dev/null || echo "")

# Build the event JSON via json.dumps so embedded quotes/backslashes/newlines in
# the upstream error payload cannot corrupt events.jsonl. The raw
# values ride as sys.argv; they never enter a shell-concatenated string.
EVENT_JSON=$(python3 -c 'import json,sys; print(json.dumps({"error_type": sys.argv[1], "message": sys.argv[2]}))' "$ERROR_TYPE" "$ERROR_MSG")

# Log the event
source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
edikt_log_event "stop_failure" "$EVENT_JSON" 2>/dev/null

exit 0
