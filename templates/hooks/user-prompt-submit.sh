#!/usr/bin/env bash
# edikt: UserPromptSubmit hook — inject active plan phase into every prompt
# Reads the most recent plan file, extracts the current in-progress phase,
# and outputs it as a systemMessage so Claude always knows what phase it's in.

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi
if grep -q 'plan-injection: false' .edikt/config.yaml 2>/dev/null; then exit 0; fi

# Read base directory from config
BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
[ -z "$BASE" ] && BASE="docs"

# Find the most recent plan file
PLAN_DIR=$(grep -A1 '^plans:' .edikt/config.yaml 2>/dev/null | grep 'dir:' | awk '{print $2}' | tr -d '"')
[ -z "$PLAN_DIR" ] && PLAN_DIR="$BASE/plans"

if [ ! -d "$PLAN_DIR" ]; then exit 0; fi

# Get most recent plan file by modification time
PLAN=$(find "$PLAN_DIR" -maxdepth 1 -name '*.md' -exec stat -f '%m %N' {} + 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)
if [ -z "$PLAN" ] || [ ! -f "$PLAN" ]; then exit 0; fi

# Check if plan has any in-progress phase
PHASE=$(grep -E '^\| *[0-9]+ *\|.*in.progress' "$PLAN" 2>/dev/null | head -1)
if [ -z "$PHASE" ]; then
  # Try alternate format: "| Phase N | ... | in progress |"
  PHASE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN" 2>/dev/null | head -1)
fi

if [ -z "$PHASE" ]; then exit 0; fi

# Extract phase number and theme
PHASE_NUM=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+')
PHASE_THEME=$(echo "$PHASE" | sed 's/|/\n/g' | sed -n '3p' | sed 's/^ *//;s/ *$//')

# Get plan name from first heading
PLAN_NAME=$(head -5 "$PLAN" | grep '^# ' | head -1 | sed 's/^# //')

# Build the message
MSG="Active plan: ${PLAN_NAME}. Current phase: ${PHASE_NUM}"
[ -n "$PHASE_THEME" ] && MSG="${MSG} — ${PHASE_THEME}"
MSG="${MSG}. Read ${PLAN} for full context if needed."

# Output as systemMessage
python3 -c "import json,sys; print(json.dumps({'systemMessage':sys.argv[1]}))" "$MSG"
