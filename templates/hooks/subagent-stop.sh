#!/usr/bin/env bash
# edikt: SubagentStop hook — log specialist agent activity + quality gates
# Fires after any subagent completes. Logs agent name and outcome to
# session-signals.log. If the agent is configured as a gate and returns
# a critical finding, blocks progression with an acknowledged override flow.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Source event logging
if [ -f "$HOME/.edikt/hooks/event-log.sh" ]; then
  source "$HOME/.edikt/hooks/event-log.sh"
fi

# Read last assistant message from stdin
INPUT=$(cat)

# Extract agent name from the response
# Agents declare themselves by domain (e.g., "architecture specialist", "database specialist")
AGENT_NAME=""
INPUT_LOWER=$(echo "$INPUT" | tr '[:upper:]' '[:lower:]')

# Check domain keywords (how agents declare themselves in v0.1.0+)
if echo "$INPUT_LOWER" | grep -qE "architect|architecture specialist"; then AGENT_NAME="architect"
elif echo "$INPUT_LOWER" | grep -qE "database|dba|schema|migration specialist"; then AGENT_NAME="dba"
elif echo "$INPUT_LOWER" | grep -qE "security specialist|security engineer|appsec"; then AGENT_NAME="security"
elif echo "$INPUT_LOWER" | grep -qE "api specialist|api engineer|api design"; then AGENT_NAME="api"
elif echo "$INPUT_LOWER" | grep -qE "backend specialist|backend engineer"; then AGENT_NAME="backend"
elif echo "$INPUT_LOWER" | grep -qE "frontend specialist|frontend engineer"; then AGENT_NAME="frontend"
elif echo "$INPUT_LOWER" | grep -qE "qa specialist|testing specialist|quality"; then AGENT_NAME="qa"
elif echo "$INPUT_LOWER" | grep -qE "sre specialist|reliability|observability"; then AGENT_NAME="sre"
elif echo "$INPUT_LOWER" | grep -qE "platform specialist|ci/cd|infrastructure"; then AGENT_NAME="platform"
elif echo "$INPUT_LOWER" | grep -qE "documentation specialist|docs specialist"; then AGENT_NAME="docs"
elif echo "$INPUT_LOWER" | grep -qE "product manager|product specialist|pm specialist"; then AGENT_NAME="pm"
elif echo "$INPUT_LOWER" | grep -qE "ux specialist|accessibility specialist"; then AGENT_NAME="ux"
elif echo "$INPUT_LOWER" | grep -qE "data specialist|data engineer|pipeline"; then AGENT_NAME="data"
elif echo "$INPUT_LOWER" | grep -qE "performance specialist|optimization"; then AGENT_NAME="performance"
elif echo "$INPUT_LOWER" | grep -qE "compliance specialist|regulatory"; then AGENT_NAME="compliance"
elif echo "$INPUT_LOWER" | grep -qE "mobile specialist|ios|android|flutter"; then AGENT_NAME="mobile"
elif echo "$INPUT_LOWER" | grep -qE "seo specialist|search engine"; then AGENT_NAME="seo"
elif echo "$INPUT_LOWER" | grep -qE "gtm specialist|analytics|tracking"; then AGENT_NAME="gtm"
fi

# Fallback: check for agent slug directly in the text
if [ -z "$AGENT_NAME" ]; then
  for agent in architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm; do
    if echo "$INPUT_LOWER" | grep -qF "$agent"; then
      AGENT_NAME="$agent"
      break
    fi
  done
fi

# If no known agent detected, try to extract from "As <Role>" pattern
if [ -z "$AGENT_NAME" ]; then
  AGENT_NAME=$(echo "$INPUT" | grep -oiE 'As (Staff |Senior |Principal )?[A-Za-z]+' | head -1 | awk '{print $NF}' | tr '[:upper:]' '[:lower:]')
fi

# If still no agent name, exit silently
if [ -z "$AGENT_NAME" ]; then
  printf '{"continue": true}'
  exit 0
fi

# Detect severity from output
SEVERITY="info"
FINDING=""
if echo "$INPUT" | grep -qiE '🔴|critical|CRITICAL|must be addressed|security vulnerability|data loss'; then
  SEVERITY="critical"
  FINDING=$(echo "$INPUT" | grep -iE '🔴|critical|CRITICAL|must be addressed|security vulnerability|data loss' | head -1 | sed 's/^[[:space:]]*//' | cut -c1-120)
elif echo "$INPUT" | grep -qiE '🟡|warning|WARNING|should be addressed|missing index|no rollback'; then
  SEVERITY="warning"
  FINDING=$(echo "$INPUT" | grep -iE '🟡|warning|WARNING|should be addressed' | head -1 | sed 's/^[[:space:]]*//' | cut -c1-120)
elif echo "$INPUT" | grep -qiE '🟢|OK|looks (good|stable|healthy)'; then
  SEVERITY="ok"
fi

# Log to session signals
mkdir -p "$HOME/.edikt" 2>/dev/null || true
LOG_FILE="$HOME/.edikt/session-signals.log"
TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

echo "${TIMESTAMP} AGENT ${AGENT_NAME} severity=${SEVERITY}" >> "$LOG_FILE"

# ============================================================
# Quality gate logic
# ============================================================

# Check if quality gates are disabled
if grep -q 'quality-gates: false' .edikt/config.yaml 2>/dev/null; then
  exit 0
fi

# Check if this agent is configured as a gate
IS_GATE=false
GATE_CHECK=$(awk '/^gates:/{found=1} found && /'"${AGENT_NAME}"'/{print "yes"; exit}' .edikt/config.yaml 2>/dev/null)
if [ "$GATE_CHECK" = "yes" ]; then
  IS_GATE=true
fi

# Check for existing override in this session
if [ "$IS_GATE" = true ] && [ "$SEVERITY" = "critical" ]; then
  FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)
  if [ -f "$HOME/.edikt/gate-overrides.jsonl" ]; then
    if grep -qF "\"agent\":\"${AGENT_NAME}\"" "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null &&
       grep -qF "\"finding_prefix\":\"${FINDING_PREFIX}\"" "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null; then
      # Already overridden this session — skip silently
      printf '{"continue": true}'
      exit 0
    fi
  fi
fi

# If agent is a gate AND severity is critical, block progression
if [ "$IS_GATE" = true ] && [ "$SEVERITY" = "critical" ]; then
  ESCAPED_FINDING=$(python3 -c "import json,sys; print(json.dumps(sys.argv[1])[1:-1])" "$FINDING")

  # Log gate event
  if type edikt_log_event >/dev/null 2>&1; then
    edikt_log_event "gate_fired" "{\"agent\":\"${AGENT_NAME}\",\"severity\":\"critical\",\"finding\":\"${ESCAPED_FINDING}\"}"
  fi

  # Write to events.jsonl (structured audit log)
  GATE_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  mkdir -p "$HOME/.edikt" 2>/dev/null || true
  echo "{\"ts\":\"${GATE_TIMESTAMP}\",\"event\":\"gate_fired\",\"agent\":\"${AGENT_NAME}\",\"severity\":\"critical\",\"finding\":\"${ESCAPED_FINDING}\"}" >> "$HOME/.edikt/events.jsonl"

  GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
  GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")

  GATE_MSG="GATE BLOCKED: ${AGENT_NAME} found a critical issue: ${FINDING}.

Present this to the user:

⛔ GATE: ${AGENT_NAME} — critical finding
   ${FINDING}

   This gate must be resolved before proceeding.
   Override this gate? (y/n)
   Note: override will be logged with your git identity.

If the user says YES:
1. Run this command to log the override:
   echo '{\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"event\":\"gate_override\",\"agent\":\"${AGENT_NAME}\",\"finding\":\"${ESCAPED_FINDING}\",\"user\":\"${GIT_USER}\",\"email\":\"${GIT_EMAIL}\"}' >> ~/.edikt/events.jsonl
2. Run this command to prevent re-firing:
   echo '{\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"agent\":\"${AGENT_NAME}\",\"finding_prefix\":\"${FINDING_PREFIX}\"}' >> ~/.edikt/gate-overrides.jsonl
3. Confirm: Gate overridden. Logged to events.jsonl. Proceeding.

If the user says NO:
1. Run this command to log the block:
   echo '{\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ)\",\"event\":\"gate_blocked\",\"agent\":\"${AGENT_NAME}\",\"finding\":\"${ESCAPED_FINDING}\",\"user\":\"${GIT_USER}\",\"email\":\"${GIT_EMAIL}\"}' >> ~/.edikt/events.jsonl
2. Stop and let the user fix the issue."
  JSON=$(python3 -c "import json,sys; print(json.dumps({'decision':'block','systemMessage':sys.argv[1]}))" "$GATE_MSG")
  echo "$JSON"
  exit 0
fi

# No gate or not critical — continue
printf '{"continue": true}'
