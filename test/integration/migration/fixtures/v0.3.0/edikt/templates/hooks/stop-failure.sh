#!/usr/bin/env bash
# edikt: StopFailure hook — log API errors to events.jsonl
# Fires when a turn ends due to an API error (rate limit, auth failure, etc.)

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# Extract error details
ERROR_TYPE=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('error',{}).get('type','unknown'))" 2>/dev/null || echo "unknown")
ERROR_MSG=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('error',{}).get('message','')[:200])" 2>/dev/null || echo "")

# Log the event
source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
edikt_log_event "stop_failure" "{\"error_type\":\"${ERROR_TYPE}\",\"message\":\"${ERROR_MSG}\"}" 2>/dev/null

exit 0
