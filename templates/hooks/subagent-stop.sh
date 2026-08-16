#!/usr/bin/env bash
# edikt: SubagentStop hook — log specialist agent activity + quality gates
# Fires after any subagent completes. Logs agent name and outcome to
# session-signals.log. If the agent is configured as a gate and the severity
# meets or exceeds the configured threshold, the hook records the block in
# ~/.edikt/events.jsonl and emits a static systemMessage.
#
# Security: the agent's finding text is attacker-influenceable (a file Claude
# reads can seed it). It MUST NOT be embedded in shell commands Claude is
# asked to execute, and it MUST NOT be concatenated into any JSON payload.
# The hook writes the full event itself using json.dumps; Claude receives
# only a static systemMessage.
#
# Severity MUST come from evaluator_output.severity (structured path). Keyword
# detection is the legacy fallback, deprecated in v0.6.0, removed v0.7.0.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Source event logging
if [ -f "$HOME/.edikt/hooks/event-log.sh" ]; then
  source "$HOME/.edikt/hooks/event-log.sh"
fi

# Read hook input from stdin
INPUT=$(cat)

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# Every extraction below needs python3, including the severity/threshold
# comparison that decides whether this subagent's finding BLOCKS. Absence
# used to fail every step silently, arriving at the same bare
# {"continue": true} a genuine clean pass produces — measured live with a
# critical-severity security finding (a staged hardcoded-secret example):
# WITH python3, {"decision": "block", ...}; WITHOUT, {"continue": true}.
# Not silence — the OPPOSITE verdict, in bytes indistinguishable from
# "nothing was wrong". This is the most severe row of the whole sweep.
#
# Per ADR-058:d05 this must degrade exactly as it does today (allow) rather  edikt-guard:allow
# than flip to fail-closed — that is a posture decision for its own ruling,
# not a side effect of a preflight. What changes here is visibility only:
# the allow now says explicitly that it could not evaluate anything.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: subagent-stop — python3 not found; this subagent output could NOT be evaluated against any quality gate" >&2
    printf '{"continue": true, "systemMessage": "⚠ edikt: python3 is missing — this subagent output could NOT be checked against any configured quality gate. If it contained a critical finding, it was NOT blocked."}\n'
    exit 0
fi

# ── Duplicate-invocation guard (bok-services field bug #4) ──────────────────
# The same payload reaches this hook twice when user-level AND project-level
# settings both register it — Claude Code merges scopes and fires both, so
# every emitted line printed exactly twice. First invocation records a
# payload-hash token under .edikt/state/.hook-dedup/; an identical payload
# within the TTL exits silently. EDIKT_DISABLE_HOOK_DEDUP=1 bypasses.
if [ "${EDIKT_DISABLE_HOOK_DEDUP:-0}" != "1" ] && [ -d ".edikt" ]; then
  _EDIKT_DEDUP_DIR=".edikt/state/.hook-dedup"
  mkdir -p "$_EDIKT_DEDUP_DIR" 2>/dev/null || true
  _EDIKT_DEDUP_KEY=$(printf '%s' "subagent-stop:$INPUT" | python3 -c 'import hashlib,sys; print(hashlib.sha256(sys.stdin.buffer.read()).hexdigest()[:40])' 2>/dev/null || echo "")
  if [ -n "$_EDIKT_DEDUP_KEY" ] && [ -d "$_EDIKT_DEDUP_DIR" ]; then
    _EDIKT_DEDUP_F="$_EDIKT_DEDUP_DIR/$_EDIKT_DEDUP_KEY"
    if [ -f "$_EDIKT_DEDUP_F" ]; then
      # Token age in seconds, computed in python3 rather than by branching
      # on stat(1).
      #
      # The previous form was `stat -f %m F || stat -c %Y F || echo 0`. On
      # BSD, -f is the FORMAT flag and it works. On GNU, -f means FILESYSTEM
      # status, %m is not a valid filesystem specifier, so it prints "?" and
      # EXITS 0 — which short-circuits the || chain before the GNU branch is
      # ever reached. The age became junk, the comparison never fired, and
      # THE DUPLICATE-INVOCATION GUARD HAS NEVER WORKED ON LINUX. It is the
      # fix for a field bug where every hook line printed twice, so on every
      # Linux host that bug was never actually fixed.
      #
      # A fallback whose first branch cannot fail is not a fallback. python3
      # is already required by these hooks (JSON and hashing above), so this
      # removes the platform branch instead of correcting its order.
      _EDIKT_AGE=$(python3 -c 'import os,sys,time
try:
    print(int(time.time() - os.path.getmtime(sys.argv[1])))
except Exception:
    print(-1)' "$_EDIKT_DEDUP_F" 2>/dev/null || echo -1)
      # Non-numeric means the age is UNKNOWN, never "fresh": suppressing on
      # an unreadable token would silence a real emission (INV-013).
      case "$_EDIKT_AGE" in ''|*[!0-9-]*) _EDIKT_AGE=-1 ;; esac
      if [ "$_EDIKT_AGE" -ge 0 ] && [ "$_EDIKT_AGE" -lt 15 ]; then
        exit 0
      fi
    fi
    : > "$_EDIKT_DEDUP_F" 2>/dev/null || true
    # Opportunistic prune of tokens older than 5 minutes.
    find "$_EDIKT_DEDUP_DIR" -type f -mmin +5 -delete 2>/dev/null || true
  fi
fi

# ── Structured evaluator-output path ──
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
# Priority: evaluator_output.agent > payload fields > keyword grep
AGENT_NAME=""
AGENT_IDENTITY_SOURCE="structured"

# 1. evaluator_output.agent (primary — not under attacker control via hook payload)
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

# The legacy content-keyword fallback for agent identity is removed in v0.6.0.
# Subagents that yield no identity through the structured paths above are
# non-agent subagents (e.g., forked slash commands). They exit clean — no
# severity detection, no threshold lookup, no gate firing.
if [ -z "$AGENT_NAME" ]; then
    printf '{"continue": true}'
    exit 0
fi

# ── Severity detection ──
# Structured path first; keyword fallback for legacy unstructured payloads.
SEVERITY="info"
SEVERITY_ANOMALY=""
FINDING=""
if [ -n "$EVAL_SEVERITY" ]; then
    # Structured path: evaluator_output.severity is canonical.
    case "$EVAL_SEVERITY" in
        critical|warning|info|ok) SEVERITY="$EVAL_SEVERITY" ;;
        *)
            SEVERITY="info"
            # Sanitized copy of the nonconforming value for the gate
            # message (attacker-influenceable — allowlist chars, capped).
            _SAN=$(printf '%s' "$EVAL_SEVERITY" | tr -cd 'a-z0-9_-' | cut -c1-32)
            SEVERITY_ANOMALY="nonconforming severity value: ${_SAN:-<empty>}"
            ;;
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
    # Severity MUST come from the structured field; keyword detection on free
    # text was the legacy fallback. The keyword-grep path
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
    SEVERITY_ANOMALY="missing severity (no evaluator_output.severity in the payload)"
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

# Read gate severity threshold (resolution order):
#   EDIKT_GATE_SEVERITY_THRESHOLD > config gates.<agent> > gates.default > "critical"
#
# AGENT_NAME at this point is always from a structured path
# (evaluator_output.agent or canonical payload field). Non-agent subagents
# exited above with `{"continue": true}` and never reach the threshold lookup.
THRESHOLD="${EDIKT_GATE_SEVERITY_THRESHOLD:-}"
if [ -z "$THRESHOLD" ]; then
    # Stdlib-only gates lookup (INV-001 — no PyYAML in hooks): scan the
    # flat `gates:` block for `<agent>: <threshold>` pairs. Any parse
    # trouble keeps the pre-existing fail-closed default of "critical".
    THRESHOLD=$(python3 -c '
import sys, os
config_path = os.path.join(os.environ.get("EDIKT_PROJECT_ROOT","."),".edikt","config.yaml")
agent = sys.argv[1]
try:
    gates = {}
    in_gates = False
    with open(config_path, encoding="utf-8") as f:
        for line in f:
            if not line.strip() or line.lstrip().startswith("#"):
                continue
            indent = len(line) - len(line.lstrip(" "))
            stripped = line.strip()
            if indent == 0:
                in_gates = stripped.startswith("gates:")
                continue
            if in_gates and ":" in stripped:
                k, _, v = stripped.partition(":")
                gates[k.strip().strip("\"'"'"'")] = v.strip().strip("\"'"'"'")
    print(gates.get(agent, gates.get("default", "critical")))
except Exception:
    print("critical")
' "$AGENT_NAME" 2>/dev/null)
fi
case "$THRESHOLD" in
    critical|warning|info) ;;
    *) THRESHOLD="critical" ;;
esac

# ADR-048 (amends ADR-026) / INV-011: for a GATE-LISTED agent, a missing or
# allowlist-nonconforming severity maps to the agent's configured threshold —
# an anomalous verdict field must not disarm the gate (previously it mapped
# to "info", below every threshold). Non-gate agents and identity-absent
# subagents keep their existing behavior; ADR-026's identity contract is
# unchanged.
if [ "$IS_GATE" = true ] && [ -n "$SEVERITY_ANOMALY" ]; then
    SEVERITY="$THRESHOLD"
    [ -z "$FINDING" ] && FINDING="severity fail-closed: $SEVERITY_ANOMALY"
fi

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
# The hook writes the block event to events.jsonl itself. The systemMessage
# is assembled via json.dumps with untrusted values passed as argv.
if [ "$IS_GATE" = true ] && [ "$SHOULD_BLOCK" = "yes" ]; then
    GATE_TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    GIT_USER=$(git config user.name 2>/dev/null || echo "unknown")
    GIT_EMAIL=$(git config user.email 2>/dev/null || echo "unknown")
    FINDING_PREFIX=$(echo "$FINDING" | cut -c1-80)

    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    # Write gate_fired event via json.dumps so untrusted values are data, not code.
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

    # Gate-fired systemMessage — all untrusted values passed as argv, not interpolated.
    python3 -c '
import json,sys
agent, sev, thresh, anomaly = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
msg = "🔴 BLOCKED — {} gate fired (severity: {} \u2265 threshold: {})\n   To change threshold: .edikt/config.yaml  gates.{}: {}".format(
    agent, sev, thresh, agent, thresh)
if anomaly:
    # (ref: ADR-048 / INV-011 — fail-closed to the threshold on an anomaly)
    msg += "\n   severity was fail-closed to the threshold: " + anomaly
print(json.dumps({"decision": "block", "systemMessage": msg}))
' "$AGENT_NAME" "$SEVERITY" "$THRESHOLD" "$SEVERITY_ANOMALY"
    exit 0
fi

# No gate or severity below threshold — continue
printf '{"continue": true}'
