#!/usr/bin/env bash
# edikt: TaskCreated hook — log task creation for plan phase tracking
# Fires when a background task is created via TaskCreate.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# Extract task details
TASK_NAME=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('task_name','unknown'))" 2>/dev/null || echo "unknown")

# Log the event
source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
edikt_log_event "task_created" "{\"task\":\"${TASK_NAME}\"}" 2>/dev/null

exit 0
