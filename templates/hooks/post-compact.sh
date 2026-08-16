#!/usr/bin/env bash
# edikt: PostCompact hook — re-inject plan phase and invariants after compaction
# Fires immediately after context compaction. Ensures the engineer never has to
# manually run /edikt:context to recover plan state.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# The final emit below needs python3 to wrap the dynamic PLAN_MSG/INV_MSG
# safely (INV-003: dynamic content must go through json.dumps, never  edikt-guard:allow
# hand-built). Absence used to crash at that last line with a raw
# "python3: command not found" and exit 127 — a real signal, but an opaque
# one that names no cause and offers no remedy. Checked here, before any of
# the (wasted, if this fails) plan/invariant scanning below, with a
# constant-literal message the crash could never produce.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: post-compact — python3 not found; plan phase and invariants were not re-injected after compaction" >&2
    # F-045 fix: additionalContext (wrapped in hookSpecificOutput, per ADR-061/
    # F-044) is the model-facing channel — this content re-informs the model  edikt-guard:allow
    # of its own active plan phase and invariants, so the model is the reader.
    printf '{"hookSpecificOutput":{"hookEventName":"PostCompact","additionalContext":"edikt: python3 is missing on this host — the active plan phase and invariants could not be re-injected after context compaction. Run /edikt:context to recover manually."}}\n'
    exit 0
fi

# Read base directory from config
BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
[ -z "$BASE" ] && BASE="docs"

# --- Find active plan phase ---
PLAN_DIR=$(grep -A1 '^plans:' .edikt/config.yaml 2>/dev/null | grep 'dir:' | awk '{print $2}' | tr -d '"')
[ -z "$PLAN_DIR" ] && PLAN_DIR="$BASE/plans"

PLAN_MSG=""
if [ -d "$PLAN_DIR" ]; then
  PLAN=$(ls -t "$PLAN_DIR"/*.md 2>/dev/null | head -1)
  if [ -n "$PLAN" ] && [ -f "$PLAN" ]; then
    PHASE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN" 2>/dev/null | head -1)
    if [ -n "$PHASE" ]; then
      PHASE_NUM=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+')
      PHASE_THEME=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '3p' | sed 's/^ *//;s/ *$//')
      PLAN_NAME=$(head -5 "$PLAN" | grep '^# ' | head -1 | sed 's/^# //')

      # Extract attempt count from column 4 (4-column table: Phase|Status|Attempt|Updated)
      ATTEMPT=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '4p' | sed 's/^ *//;s/ *$//')
      # Backward compat: only use ATTEMPT if it matches N/N pattern
      if ! echo "$ATTEMPT" | grep -qE '^[0-9]+/[0-9]+$'; then
        ATTEMPT=""
      fi

      # Read evaluation state for failing criteria. The criteria sidecar is
      # spec-only (strict-parsed by bin/edikt verify); status/fail_reason live
      # in .edikt/state/plan-eval/<stem>-eval.json.
      EVAL_STATE=".edikt/state/plan-eval/$(basename "${PLAN%.md}")-eval.json"
      FAIL_MSG=""
      if [ -f "$EVAL_STATE" ]; then
        FAIL_MSG=$(python3 - "$EVAL_STATE" <<'PYEOF'
import json, sys
try:
    with open(sys.argv[1], encoding="utf-8") as f:
        state = json.load(f)
except (OSError, json.JSONDecodeError):
    sys.exit(0)
ids = []
reason = None
for p in state.get("phases") or []:
    for c in p.get("criteria") or []:
        if isinstance(c, dict) and c.get("status") == "fail":
            ids.append(str(c.get("id", "?")))
            if reason is None and c.get("fail_reason"):
                reason = str(c["fail_reason"])
if ids:
    msg = "Last failing criteria: " + ",".join(ids)
    if reason:
        msg += f" ({reason})"
    print(msg)
PYEOF
)
      fi

      # Read Context Needed from active phase in plan
      CONTEXT_MSG=""
      if [ -n "$PHASE_NUM" ] && [ -f "$PLAN" ]; then
        CONTEXT_LINES=$(sed -n "/## Phase ${PHASE_NUM}[^0-9]/,/## Phase [0-9]/p" "$PLAN" 2>/dev/null | grep -A20 'Context Needed:' | grep '^ *-' | head -5 | sed 's/^ *//')
        if [ -n "$CONTEXT_LINES" ]; then
          CONTEXT_MSG="Before continuing, read:
  ${CONTEXT_LINES}"
        fi
      fi

      PLAN_MSG="Active plan: ${PLAN_NAME}. Phase ${PHASE_NUM}"
      [ -n "$PHASE_THEME" ] && PLAN_MSG="${PLAN_MSG} — ${PHASE_THEME}"
      [ -n "$ATTEMPT" ] && PLAN_MSG="${PLAN_MSG} (attempt ${ATTEMPT})"
      PLAN_MSG="${PLAN_MSG}."
      [ -n "$FAIL_MSG" ] && PLAN_MSG="${PLAN_MSG} ${FAIL_MSG}."
      [ -n "$CONTEXT_MSG" ] && PLAN_MSG="${PLAN_MSG} ${CONTEXT_MSG}"
      PLAN_MSG="${PLAN_MSG} Re-read ${PLAN} for full phase details."
    fi
  fi
fi

# --- Collect invariants ---
INV_DIR=""
for dir in "$BASE/invariants" "$BASE/architecture/invariants"; do
  if [ -d "$dir" ]; then
    INV_DIR="$dir"
    break
  fi
done

INV_MSG=""
if [ -n "$INV_DIR" ]; then
  INV_COUNT=$(ls "$INV_DIR"/*.md 2>/dev/null | wc -l | tr -d ' ')
  if [ "$INV_COUNT" -gt 0 ]; then
    INV_NAMES=$(ls "$INV_DIR"/*.md 2>/dev/null | xargs -I{} head -1 {} | sed 's/^# //' | paste -sd', ' -)
    INV_MSG="Invariants (${INV_COUNT}): ${INV_NAMES}. These are hard constraints — never violate them."
  fi
fi

# --- Build output ---
if [ -z "$PLAN_MSG" ] && [ -z "$INV_MSG" ]; then
  exit 0
fi

MSG="Context recovered after compaction."
[ -n "$PLAN_MSG" ] && MSG="${MSG} ${PLAN_MSG}"
[ -n "$INV_MSG" ] && MSG="${MSG} ${INV_MSG}"

python3 -c "import json,sys; print(json.dumps({'hookSpecificOutput':{'hookEventName':'PostCompact','additionalContext':sys.argv[1]}}))" "$MSG"
