#!/usr/bin/env bash
# edikt: SessionStart hook — git-aware session summary
# Surfaces what changed since last session and which specialist agents are relevant.
#
# Output format: Claude Code hook protocol JSON
#   {"additionalContext": "..."}
# Migration: ADR-014 Phase 15 — plaintext echoes wrapped in JSON, content
# preserved byte-for-byte.

set -uo pipefail

# Only run in edikt projects
if [ ! -f '.edikt/config.yaml' ]; then exit 0; fi

# Clear gate overrides from previous session
> "$HOME/.edikt/gate-overrides.jsonl" 2>/dev/null || true

# Rotate session signals log — archive previous session, start fresh
mkdir -p "$HOME/.edikt" 2>/dev/null || true
LOG_FILE="$HOME/.edikt/session-signals.log"
if [ -f "$LOG_FILE" ]; then
    mv "$LOG_FILE" "${LOG_FILE}.prev" 2>/dev/null || true
fi

# Emit a message as JSON additionalContext and exit 0
emit() {
    python3 -c 'import json,sys; print(json.dumps({"additionalContext":sys.argv[1]}))' "$1"
    exit 0
}

ENCODED=$(echo "$PWD" | sed 's|/|-|g')
MEMORY="$HOME/.claude/projects/${ENCODED}/memory/MEMORY.md"

# Compute memory age
if [ -f "$MEMORY" ]; then
  AGE=$(( ($(date +%s) - $(date -r "$MEMORY" +%s 2>/dev/null || stat -f %m "$MEMORY" 2>/dev/null || echo 0)) / 86400 ))
else
  AGE=0
fi

# If git analysis is disabled, fall back to simple age check
if grep -q 'session-summary: false' .edikt/config.yaml 2>/dev/null; then
  if [ ! -f "$MEMORY" ]; then
    emit "📋 edikt project detected. Run /edikt:context to load project context before writing code."
  elif [ "$AGE" -gt 7 ]; then
    emit "⚠️  edikt memory is ${AGE}d old. Run /edikt:context to refresh."
  else
    emit "📋 edikt project — memory ${AGE}d old. Run /edikt:context to load context."
  fi
fi

# No memory file yet
if [ ! -f "$MEMORY" ]; then
  emit "📋 edikt project detected. Run /edikt:context to load project context before writing code."
fi

# Stale memory — skip git analysis, just warn
if [ "$AGE" -gt 7 ]; then
  emit "⚠️  edikt memory is ${AGE}d old. Run /edikt:context to refresh."
fi

# Get changed files since last session
MTIME=$(date -r "$MEMORY" '+%Y-%m-%dT%H:%M:%S' 2>/dev/null || stat -f '%Sm' -t '%Y-%m-%dT%H:%M:%S' "$MEMORY" 2>/dev/null)
CHANGED=$(git log --since="$MTIME" --name-only --pretty=format: 2>/dev/null | grep -v '^$' | sort -u)

if [ -z "$CHANGED" ]; then
  emit "📋 edikt — ${AGE}d since last session. Run /edikt:context to load context."
fi

# Classify by domain
AGENTS=''
SUMMARY=''

N_MIGRATION=$(echo "$CHANGED" | grep -ciE 'migration|schema|\.sql' || true)
N_INFRA=$(echo "$CHANGED"     | grep -ciE 'docker|compose|\.tf|helm|k8s|Dockerfile' || true)
N_SECURITY=$(echo "$CHANGED"  | grep -ciE 'auth|jwt|oauth|payment|token|secret' || true)
N_API=$(echo "$CHANGED"       | grep -ciE 'route|handler|controller|api|endpoint' || true)

[ "$N_MIGRATION" -gt 0 ] && SUMMARY="${SUMMARY}${N_MIGRATION} migration/schema file(s), " && AGENTS="${AGENTS}dba, "
[ "$N_INFRA" -gt 0 ]     && SUMMARY="${SUMMARY}${N_INFRA} infra file(s), "              && AGENTS="${AGENTS}sre, "
[ "$N_SECURITY" -gt 0 ]  && SUMMARY="${SUMMARY}${N_SECURITY} security file(s), "        && AGENTS="${AGENTS}security, "
[ "$N_API" -gt 0 ]       && SUMMARY="${SUMMARY}${N_API} API file(s), "                  && AGENTS="${AGENTS}api, "

SUMMARY=$(echo "$SUMMARY" | sed 's/, $//')
AGENTS=$(echo "$AGENTS"   | sed 's/, $//')

if [ -n "$AGENTS" ]; then
  MSG=$(printf '📋 edikt — since your last session (%sd ago):\n   %s changed\n   Relevant agents: %s\n   Run /edikt:context to load full project context.' \
    "$AGE" "$SUMMARY" "$AGENTS")
  emit "$MSG"
else
  emit "📋 edikt — ${AGE}d since last session. Run /edikt:context to load context."
fi

# Surface most recent unresolved gate finding (Phase 8 / FR-008).
# Only the single most recent — do not list all. Only if events.jsonl exists and
# contains an unresolved gate_fired. Exits 0 silently on any error (INV-003).
EVENTS_FILE="$HOME/.edikt/events.jsonl"
if [ -f "$EVENTS_FILE" ]; then
  GATE_MSG=$(python3 - "$EVENTS_FILE" <<'PY'
import json, sys, datetime, os
path = sys.argv[1]
try:
    events = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                try:
                    events.append(json.loads(line))
                except Exception:
                    pass
    resolved_keys = set()
    for e in events:
        if e.get("event") == "gate_resolved":
            resolved_keys.add((e.get("agent"), e.get("finding_prefix")))
    cutoff = datetime.datetime.utcnow() - datetime.timedelta(days=7)
    unresolved = []
    for e in events:
        if e.get("event") != "gate_fired":
            continue
        try:
            ts = datetime.datetime.strptime(e["ts"], "%Y-%m-%dT%H:%M:%SZ")
        except Exception:
            continue
        if ts < cutoff:
            continue
        key = (e.get("agent"), e.get("finding_prefix"))
        if key not in resolved_keys:
            unresolved.append((ts, e))
    if not unresolved:
        sys.exit(0)
    unresolved.sort(key=lambda x: x[0], reverse=True)
    _, latest = unresolved[0]
    agent = latest.get("agent", "unknown")
    finding = latest.get("finding_prefix", latest.get("finding", "no description"))
    print(f"\u26a0 Last session: {agent} gate fired on {finding!r} — was it resolved?\n  To dismiss: run /edikt:session (end-of-session sweep)")
except Exception:
    pass
PY
  2>/dev/null)
  if [ -n "$GATE_MSG" ]; then
    python3 -c 'import json,sys; print(json.dumps({"additionalContext":sys.argv[1]}))' "$GATE_MSG"
    exit 0
  fi
fi
