#!/usr/bin/env bash
# edikt: SubagentStop hook — log specialist agent activity + quality gates
# Fires after any subagent completes. Logs agent name and outcome to
# session-signals.log. If the agent is configured as a gate and returns
# a critical finding, the hook records the block in ~/.edikt/events.jsonl
# and emits a static systemMessage directing the user to the log.
#
# SECURITY (ADR-019 carve-out, INV-003, INV-004): the agent's finding text
# is attacker-influenceable (a file Claude reads can seed it). It MUST NOT
# be embedded in shell commands Claude is asked to execute, and it MUST NOT
# be concatenated into any JSON payload. The hook writes the full event
# itself using json.dumps; Claude receives only a static systemMessage.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Source event logging
if [ -f "$HOME/.edikt/hooks/event-log.sh" ]; then
  source "$HOME/.edikt/hooks/event-log.sh"
fi

# Read last assistant message from stdin
INPUT=$(cat)

# Extract agent name from the response.
# NOTE (MED-11): this detection is content-based and therefore spoofable.
# Phase 9 replaces it with structured SubagentStop payload fields. For now
# we constrain the result to a known slug and never embed it in any shell
# command — the AGENT_NAME value flows only into json.dumps-constructed JSON.
AGENT_NAME=""
INPUT_LOWER=$(echo "$INPUT" | tr '[:upper:]' '[:lower:]')

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

# Check for existing override in this session (finding prefix used as a
# coarse key; safe because override records go through json.dumps below)
if [ "$IS_GATE" = true ] && [ "$SEVERITY" = "critical" ]; then
  FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)
  if [ -f "$HOME/.edikt/gate-overrides.jsonl" ]; then
    # Match on JSON structure via python3 to avoid grep-escaping surprises.
    MATCHED=$(python3 - <<'PY' "$AGENT_NAME" "$FINDING_PREFIX" "$HOME/.edikt/gate-overrides.jsonl"
import json, sys
agent, prefix, path = sys.argv[1], sys.argv[2], sys.argv[3]
try:
    with open(path, 'r', encoding='utf-8') as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                rec = json.loads(line)
            except json.JSONDecodeError:
                continue
            if rec.get('agent') == agent and rec.get('finding_prefix') == prefix:
                print('yes')
                sys.exit(0)
except FileNotFoundError:
    pass
print('no')
PY
)
    if [ "$MATCHED" = "yes" ]; then
      # Already overridden this session — skip silently.
      printf '{"continue": true}'
      exit 0
    fi
  fi
fi

# If agent is a gate AND severity is critical, block progression.
# The hook writes the block event to events.jsonl itself (INV-004). The
# systemMessage is STATIC — no agent-derived text is ever placed into a
# shell command Claude could be coerced into running.
if [ "$IS_GATE" = true ] && [ "$SEVERITY" = "critical" ]; then
  GATE_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
  GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
  GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")

  mkdir -p "$HOME/.edikt" 2>/dev/null || true
  # Write the full gate_fired event via json.dumps. All untrusted fields
  # (finding, git identity) ride as argv; never concatenated into a string.
  python3 - "$GATE_TIMESTAMP" "$AGENT_NAME" "$SEVERITY" "$FINDING" "$FINDING_PREFIX" "$GIT_USER" "$GIT_EMAIL" "$HOME/.edikt/events.jsonl" <<'PY'
import json, sys
ts, agent, sev, finding, prefix, user, email, out = sys.argv[1:9]
rec = {
    "ts": ts,
    "event": "gate_fired",
    "agent": agent,
    "severity": sev,
    "finding": finding,
    "finding_prefix": prefix,
    "user": user,
    "email": email,
}
with open(out, 'a', encoding='utf-8') as f:
    f.write(json.dumps(rec) + "\n")
PY

  # Optional telemetry (edikt_log_event is itself json.dumps-safe now).
  if type edikt_log_event >/dev/null 2>&1; then
    GATE_TELEMETRY=$(python3 -c 'import json,sys; print(json.dumps({"agent": sys.argv[1], "severity": "critical", "finding_prefix": sys.argv[2]}))' "$AGENT_NAME" "$FINDING_PREFIX")
    edikt_log_event "gate_fired" "$GATE_TELEMETRY"
  fi

  # Static systemMessage: agent slug only (constrained to the known-slug
  # allowlist above); no finding text embedded. Users are directed to the
  # events.jsonl file for the full finding, and to a dedicated override
  # command for the override flow (Phase 2 leaves the command as a future
  # item; for now users can manually address the finding or set the
  # EDIKT_GATE_OVERRIDE env var and retry).
  SYS_MSG="⛔ GATE FIRED — ${AGENT_NAME} reported a critical finding. Full details in ~/.edikt/events.jsonl. To override this gate, address the finding or set EDIKT_GATE_OVERRIDE=1 and retry. Logging uses your git identity."
  python3 -c 'import json,sys; print(json.dumps({"decision": "block", "systemMessage": sys.argv[1]}))' "$SYS_MSG"
  exit 0
fi

# No gate or not critical — continue
printf '{"continue": true}'
