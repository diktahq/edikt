#!/usr/bin/env bash
# edikt: UserPromptSubmit hook — inject active plan phase into every prompt
# Reads the most recent plan file, extracts the current in-progress phase,
# and outputs it as additionalContext so Claude always knows what phase it's
# in — this content is for the model, not the user, so systemMessage (which
# renders only on the user's screen) is the wrong channel (F-045, ADR-061 §1).

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi
if grep -q 'plan-injection: false' .edikt/config.yaml 2>/dev/null; then exit 0; fi

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# The final emit needs python3 to wrap the dynamic MSG safely (INV-003).  edikt-guard:allow
# Absence used to crash at that last line with a raw "python3: command not
# found" and exit 127 on every prompt — opaque, and repeated on every single
# message for the rest of the session. Checked here, before the (wasted, if
# this fails) plan-file scan below.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: user-prompt-submit — python3 not found; active plan phase was not injected into this prompt" >&2
    printf '{"hookSpecificOutput":{"hookEventName":"UserPromptSubmit","additionalContext":"edikt: python3 is missing on this host — the active plan phase is not being injected into your prompts. Run /edikt:context to check phase state manually."}}\n'
    exit 0
fi

# Read base directory from config
BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
[ -z "$BASE" ] && BASE="docs"

# Find the most recent plan file
PLAN_DIR=$(grep -A1 '^plans:' .edikt/config.yaml 2>/dev/null | grep 'dir:' | awk '{print $2}' | tr -d '"')
[ -z "$PLAN_DIR" ] && PLAN_DIR="$BASE/plans"

if [ ! -d "$PLAN_DIR" ]; then exit 0; fi

# Get most recent plan file by modification time (BSD/macOS vs GNU/Linux stat).
if stat -f '%m' . >/dev/null 2>&1; then
  PLAN=$(find "$PLAN_DIR" -maxdepth 1 -name '*.md' -exec stat -f '%m %N' {} + 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)
else
  PLAN=$(find "$PLAN_DIR" -maxdepth 1 -name '*.md' -printf '%T@ %p\n' 2>/dev/null | sort -rn | head -1 | cut -d' ' -f2-)
fi
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

# Output as additionalContext (model-facing) — see header comment / F-045.
python3 -c "import json,sys; print(json.dumps({'hookSpecificOutput':{'hookEventName':'UserPromptSubmit','additionalContext':sys.argv[1]}}))" "$MSG"
