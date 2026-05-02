#!/usr/bin/env bash
# edikt: SubagentStop hook — log specialist agent activity + quality gates
# Fires after any subagent completes. Logs agent name and outcome to
# session-signals.log. If the agent is configured as a gate and the severity
# meets or exceeds the configured threshold, the hook records the block in
# ~/.edikt/events.jsonl and emits a static systemMessage.
#
# SECURITY (ADR-019 carve-out, INV-003, INV-004): the agent's finding text
# is attacker-influenceable (a file Claude reads can seed it). It MUST NOT
# be embedded in shell commands Claude is asked to execute, and it MUST NOT
# be concatenated into any JSON payload. The hook writes the full event
# itself using json.dumps; Claude receives only a static systemMessage.
#
# ADR-023: Severity MUST come from evaluator_output.severity (structured path).
# Keyword detection is the legacy fallback, deprecated in v0.6.0, removed v0.7.0.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Source event logging
if [ -f "$HOME/.edikt/hooks/event-log.sh" ]; then
  source "$HOME/.edikt/hooks/event-log.sh"
fi

# Read hook input from stdin
INPUT=$(cat)

# ── Structured evaluator-output path (ADR-023) ──
# Read agent domain + severity from evaluator_output before any keyword detection.
EVAL_OUT=$(printf '%s' "$INPUT" | python3 -c '
import json,sys
try:
    d = json.load(sys.stdin)
    eo = d.get("evaluator_output", {})
    print(eo.get("agent","").strip().lower()+"|"+eo.get("severity","").strip().lower())
except Exception:
    print("|")
' 2>/dev/null)
EVAL_AGENT="${EVAL_OUT%%|*}"
EVAL_SEVERITY="${EVAL_OUT##*|}"

# ── Agent identity resolution ──
# Priority: evaluator_output.agent (ADR-023) > payload fields > keyword grep
AGENT_NAME=""
AGENT_IDENTITY_SOURCE="structured"

# 1. evaluator_output.agent (ADR-023 primary — not under attacker control via hook payload)
if [ -n "$EVAL_AGENT" ]; then
    for _allowed in architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm; do
        if [ "$EVAL_AGENT" = "$_allowed" ]; then
            AGENT_NAME="$_allowed"
            AGENT_IDENTITY_SOURCE="evaluator_output"
            break
        fi
    done
fi

# 2. Canonical Claude Code payload fields (Agent tool sets these — NOT
#    attacker-controlled by subagent content)
if [ -z "$AGENT_NAME" ]; then
    PAYLOAD_AGENT=$(printf '%s' "$INPUT" | python3 -c 'import json,sys
try:
    d = json.load(sys.stdin)
    for k in ("agent_name", "subagent_type", "tool_name", "agent"):
        v = d.get(k)
        if isinstance(v, str) and v:
            print(v.strip().lower())
            break
except Exception:
    pass' 2>/dev/null)
    if [ -n "$PAYLOAD_AGENT" ]; then
        for _allowed in architect dba security api backend frontend qa sre platform docs pm ux data performance compliance mobile seo gtm; do
            if [ "$PAYLOAD_AGENT" = "$_allowed" ]; then
                AGENT_NAME="$_allowed"
                AGENT_IDENTITY_SOURCE="payload"
                break
            fi
        done
    fi
fi

# Per ADR-026 (v0.6.0): the legacy content-keyword fallback for agent identity
# is removed entirely (was scheduled for v0.7.0 in ADR-023). Subagents that
# yield no identity through the structured paths above are non-agent subagents
# (e.g., forked slash commands). They exit clean — no severity detection,
# no threshold lookup, no gate firing.
if [ -z "$AGENT_NAME" ]; then
    printf '{"continue": true}'
    exit 0
fi

# ── Severity detection (ADR-023) ──
# Structured path first; keyword fallback for legacy unstructured payloads.
SEVERITY="info"
FINDING=""
if [ -n "$EVAL_SEVERITY" ]; then
    # Structured path: evaluator_output.severity is canonical (ADR-023).
    case "$EVAL_SEVERITY" in
        critical|warning|info|ok) SEVERITY="$EVAL_SEVERITY" ;;
        *) SEVERITY="info" ;;
    esac
    # Extract first finding description from evaluator_output.findings[].
    FINDING=$(printf '%s' "$INPUT" | python3 -c '
import json,sys
try:
    d = json.load(sys.stdin)
    findings = d.get("evaluator_output", {}).get("findings", [])
    if findings:
        item = findings[0]
        desc = item.get("description","") if isinstance(item, dict) else str(item)
        print(str(desc)[:120])
except Exception:
    pass
' 2>/dev/null)
else
    # Legacy unstructured payload — no evaluator_output.severity available.
    # Per ADR-023, severity MUST come from the structured field; keyword
    # detection on free text was the legacy fallback. The keyword-grep path
    # caused false-positive gate fires on subagents whose content happened
    # to mention severity terms (e.g., a status dashboard reporting prior
    # gate activity). Severity is left at the default "info"; threshold
    # resolution below uses gates.default so unstructured payloads get a
    # conservative non-blocking treatment unless explicitly configured.
    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    python3 -c '
import json,sys,os,datetime
path=os.path.join(os.environ.get("HOME",""),".edikt","events.jsonl")
try:
    with open(path,"a",encoding="utf-8") as f:
        f.write(json.dumps({"ts":datetime.datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ"),"event":"legacy_payload","hook":"subagent-stop"})+"\n")
except Exception:
    pass
' 2>/dev/null
    echo "warn: legacy evaluator payload (no evaluator_output.severity); severity defaulted to info" >&2
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

# Read gate severity threshold (ADR-023 §4 resolution order):
#   EDIKT_GATE_SEVERITY_THRESHOLD > config gates.<agent> > gates.default > "critical"
#
# Per ADR-026, AGENT_NAME at this point is always from a structured path
# (evaluator_output.agent or canonical payload field). Non-agent subagents
# exited above with `{"continue": true}` and never reach the threshold lookup.
THRESHOLD="${EDIKT_GATE_SEVERITY_THRESHOLD:-}"
if [ -z "$THRESHOLD" ]; then
    THRESHOLD=$(python3 -c '
import yaml,sys,os
config_path = os.path.join(os.environ.get("EDIKT_PROJECT_ROOT","."),".edikt","config.yaml")
try:
    with open(config_path) as f:
        cfg = yaml.safe_load(f)
    agent = sys.argv[1]
    gates = (cfg or {}).get("gates",{})
    print(gates.get(agent, gates.get("default","critical")))
except Exception:
    print("critical")
' "$AGENT_NAME" 2>/dev/null)
fi
case "$THRESHOLD" in
    critical|warning|info) ;;
    *) THRESHOLD="critical" ;;
esac

# Determine whether severity meets or exceeds threshold (critical=3, warning=2, info=1)
SHOULD_BLOCK=$(python3 -c '
import sys
levels={"critical":3,"warning":2,"info":1}
sev=sys.argv[1].lower(); thresh=sys.argv[2].lower()
print("yes" if levels.get(sev,0)>=levels.get(thresh,3) and levels.get(sev,0)>0 else "no")
' "$SEVERITY" "$THRESHOLD" 2>/dev/null)

# Check for existing override in this session
if [ "$IS_GATE" = true ] && [ "$SHOULD_BLOCK" = "yes" ]; then
    FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)
    if [ -f "$HOME/.edikt/gate-overrides.jsonl" ]; then
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
            printf '{"continue": true}'
            exit 0
        fi
    fi
fi

# If agent is a gate AND severity meets threshold, block progression.
# The hook writes the block event to events.jsonl itself (INV-004). The
# systemMessage is assembled via json.dumps with untrusted values as argv (INV-003, ADR-023 §5).
if [ "$IS_GATE" = true ] && [ "$SHOULD_BLOCK" = "yes" ]; then
    GATE_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
    GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")
    FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)

    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    # Write gate_fired event via json.dumps (INV-003, INV-004).
    python3 - "$GATE_TIMESTAMP" "$AGENT_NAME" "$SEVERITY" "$THRESHOLD" "$FINDING" "$FINDING_PREFIX" "$GIT_USER" "$GIT_EMAIL" "$AGENT_IDENTITY_SOURCE" "$HOME/.edikt/events.jsonl" <<'PY'
import json, sys
ts, agent, sev, thresh, finding, prefix, user, email, identity_source, out = sys.argv[1:11]
rec = {
    "ts": ts,
    "event": "gate_fired",
    "agent": agent,
    "severity": sev,
    "threshold": thresh,
    "finding": finding,
    "finding_prefix": prefix,
    "user": user,
    "email": email,
    "identity_source": identity_source,
}
with open(out, 'a', encoding='utf-8') as f:
    f.write(json.dumps(rec) + "\n")
PY

    # Optional telemetry.
    if type edikt_log_event >/dev/null 2>&1; then
        GATE_TELEMETRY=$(python3 -c 'import json,sys; print(json.dumps({"agent":sys.argv[1],"severity":sys.argv[2],"threshold":sys.argv[3],"finding_prefix":sys.argv[4]}))' "$AGENT_NAME" "$SEVERITY" "$THRESHOLD" "$FINDING_PREFIX")
        edikt_log_event "gate_fired" "$GATE_TELEMETRY"
    fi

    # Gate-fired systemMessage per ADR-023 §5 (INV-003: all values passed as argv).
    python3 -c '
import json,sys
agent, sev, thresh = sys.argv[1], sys.argv[2], sys.argv[3]
msg = "🔴 BLOCKED — {} gate fired (severity: {} \u2265 threshold: {})\n   To change threshold: .edikt/config.yaml  gates.{}: {}".format(
    agent, sev, thresh, agent, thresh)
print(json.dumps({"decision": "block", "systemMessage": msg}))
' "$AGENT_NAME" "$SEVERITY" "$THRESHOLD"
    exit 0
fi

# No gate or severity below threshold — continue
printf '{"continue": true}'
