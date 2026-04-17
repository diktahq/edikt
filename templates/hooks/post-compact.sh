#!/usr/bin/env bash
# edikt: PostCompact hook — re-inject plan phase and invariants after compaction
# Fires immediately after context compaction. Ensures the engineer never has to
# manually run /edikt:context to recover plan state.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

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

      # Read criteria sidecar for failing criteria
      CRITERIA_FILE="${PLAN%.md}-criteria.yaml"
      FAIL_MSG=""
      if [ -f "$CRITERIA_FILE" ]; then
        FAIL_IDS=$(grep -B2 'status: fail' "$CRITERIA_FILE" 2>/dev/null | grep 'id:' | awk '{print $2}' | tr '\n' ',' | sed 's/,$//')
        FAIL_REASON=$(grep -A1 'status: fail' "$CRITERIA_FILE" 2>/dev/null | grep 'fail_reason:' | sed 's/.*fail_reason: *//' | sed 's/"//g' | head -1)
        if [ -n "$FAIL_IDS" ]; then
          FAIL_MSG="Last failing criteria: ${FAIL_IDS}"
          [ -n "$FAIL_REASON" ] && [ "$FAIL_REASON" != "null" ] && FAIL_MSG="${FAIL_MSG} (${FAIL_REASON})"
        fi
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

python3 -c "import json,sys; print(json.dumps({'systemMessage':sys.argv[1]}))" "$MSG"
