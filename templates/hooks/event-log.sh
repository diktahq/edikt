#!/usr/bin/env bash
# edikt: Event logging utility — append structured events to ~/.edikt/events.jsonl
# Sourced by other hook scripts, not executed directly.
#
# Usage:
#   source "$HOME/.edikt/hooks/event-log.sh"
#   edikt_log_event "gate_fired" '{"agent":"security","severity":"critical","finding":"Hardcoded JWT secret"}'
#   edikt_log_event "status_change" '{"artifact":"SPEC-005","from":"draft","to":"accepted"}'
#   edikt_log_event "gate_override" '{"agent":"security","finding":"Hardcoded JWT secret"}'

edikt_log_event() {
  local event_type="$1"
  local data="${2:-{\}}"
  local log_file="$HOME/.edikt/events.jsonl"
  local timestamp user event

  mkdir -p "$(dirname "$log_file")" 2>/dev/null || true
  # MED-8: tighten permissions on the log. events.jsonl can carry git email,
  # gate findings, and session metadata; on multi-user systems the default
  # umask (0022) leaves it world-readable.
  if [ ! -e "$log_file" ]; then
    touch "$log_file" 2>/dev/null && chmod 0600 "$log_file" 2>/dev/null || true
  else
    # Ensure existing logs have the right perms (idempotent).
    chmod 0600 "$log_file" 2>/dev/null || true
  fi
  timestamp=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  user=$(git config user.email 2>/dev/null || echo "unknown")

  event=$(python3 -c "
import json,sys
base = json.loads(sys.argv[3])
base['type'] = sys.argv[1]
base['at'] = sys.argv[2]
base['by'] = sys.argv[4] if len(sys.argv) > 4 else 'unknown'
print(json.dumps(base))
" "$event_type" "$timestamp" "$data" "$user" 2>/dev/null) || return 0

  echo "$event" >> "$log_file"
}

# Rotate events monthly (call from session-start if needed)
edikt_rotate_events() {
  local events_file="$HOME/.edikt/events.jsonl"
  if [ ! -f "$events_file" ]; then return; fi

  local file_month
  file_month=$(date -r "$events_file" +"%Y-%m" 2>/dev/null || stat -f '%Sm' -t '%Y-%m' "$events_file" 2>/dev/null)
  local current_month
  current_month=$(date +"%Y-%m")

  if [ "$file_month" != "$current_month" ] && [ -n "$file_month" ]; then
    mv "$events_file" "$HOME/.edikt/events-${file_month}.jsonl" 2>/dev/null || true
  fi
}
