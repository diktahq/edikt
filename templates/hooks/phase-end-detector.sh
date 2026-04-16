#!/usr/bin/env bash
# edikt: Phase-end detector — on Stop, check if Claude just completed a plan phase
# and auto-invoke the headless evaluator if so.
#
# Runs on Stop events. Reads the most recent plan file, finds the in-progress
# phase, scans the last assistant message for completion signals, and invokes
# the evaluator if a completion is detected.
#
# Output:
#   {"continue": true}              — no phase completion detected, or evaluation passed
#   {"systemMessage": "..."}        — evaluation failed, surface to user
#
# Environment:
#   EDIKT_SKIP_PHASE_EVAL=1         — skip phase-end evaluation (for testing)
#   EDIKT_EVALUATOR_DRY_RUN=1       — detect completion but don't invoke claude -p (testing)

set -uo pipefail

# Only run in edikt projects
if [ ! -f '.edikt/config.yaml' ]; then exit 0; fi

# Config: phase-end evaluation must be enabled
if ! grep -qE '^\s*phase-end:\s*true' .edikt/config.yaml 2>/dev/null; then
  # If key is absent, default is true — only skip if explicitly false
  if grep -qE '^\s*phase-end:\s*false' .edikt/config.yaml 2>/dev/null; then
    exit 0
  fi
fi

# Test/debug override
if [ "${EDIKT_SKIP_PHASE_EVAL:-0}" = "1" ]; then exit 0; fi

# Prevent infinite loops
INPUT=$(cat)
STOP_HOOK_ACTIVE=$(echo "$INPUT" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    print('true' if d.get('stop_hook_active') else 'false')
except Exception:
    print('false')
" 2>/dev/null || echo "false")

if [ "$STOP_HOOK_ACTIVE" = "true" ]; then exit 0; fi

# Extract the last assistant message
LAST_MSG=$(echo "$INPUT" | python3 -c "
import json, sys
try:
    d = json.load(sys.stdin)
    print(d.get('last_assistant_message', '').strip())
except Exception:
    print('')
" 2>/dev/null || echo "")

if [ -z "$LAST_MSG" ]; then exit 0; fi

# ─── Find the active plan ─────────────────────────────────────────────────────

BASE=$(grep '^base:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs")
[ -z "$BASE" ] && BASE="docs"

PLAN_DIR=$(grep "^  plans:" .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
[ -z "$PLAN_DIR" ] && PLAN_DIR="${BASE}/plans"

# Try both common plan locations
PLAN_FILE=""
for dir in "$PLAN_DIR" "$BASE/product/plans" "docs/plans" "docs/product/plans"; do
  [ -d "$dir" ] || continue
  LATEST=$(ls -t "$dir"/PLAN-*.md 2>/dev/null | grep -v 'criteria.yaml' | head -1)
  if [ -n "$LATEST" ]; then
    PLAN_FILE="$LATEST"
    break
  fi
done

if [ -z "$PLAN_FILE" ]; then
  echo '{"continue": true}'
  exit 0
fi

# ─── Find the in-progress phase ───────────────────────────────────────────────

PHASE_LINE=$(grep -iE '\| *(Phase )?[0-9]+ *\|.*in[_ -]progress' "$PLAN_FILE" 2>/dev/null | head -1)
if [ -z "$PHASE_LINE" ]; then
  echo '{"continue": true}'
  exit 0
fi

PHASE_NUM=$(echo "$PHASE_LINE" | sed 's/|/\n/g' | sed -n '2p' | tr -d ' ' | grep -oE '[0-9]+')
if [ -z "$PHASE_NUM" ]; then
  echo '{"continue": true}'
  exit 0
fi

# ─── Detect completion signal in last message ─────────────────────────────────

# Common patterns that indicate phase completion:
COMPLETION_DETECTED=false

# Pattern 1: Explicit "Phase N complete" / "PHASE N DONE" / "Phase N finished"
if echo "$LAST_MSG" | grep -qiE "phase[- ]?${PHASE_NUM}[^0-9].{0,40}(complete|done|finished|implemented|shipped)"; then
  COMPLETION_DETECTED=true
fi

# Pattern 2: "Completed phase N" / "Implemented phase N"
if echo "$LAST_MSG" | grep -qiE "(completed|implemented|finished|shipped) phase[- ]?${PHASE_NUM}[^0-9]"; then
  COMPLETION_DETECTED=true
fi

# Pattern 3: Explicit completion promise from plan (common format)
if echo "$LAST_MSG" | grep -qiE "PHASE[- ]?${PHASE_NUM}[- ]?[A-Z ]+DONE"; then
  COMPLETION_DETECTED=true
fi

if [ "$COMPLETION_DETECTED" = "false" ]; then
  echo '{"continue": true}'
  exit 0
fi

# ─── Find the criteria sidecar ────────────────────────────────────────────────

PLAN_STEM=$(basename "$PLAN_FILE" .md)
SIDECAR=""
for dir in "$PLAN_DIR" "$BASE/product/plans" "docs/plans" "docs/product/plans"; do
  [ -d "$dir" ] || continue
  CANDIDATE="$dir/${PLAN_STEM}-criteria.yaml"
  if [ -f "$CANDIDATE" ]; then
    SIDECAR="$CANDIDATE"
    break
  fi
done

# Log detection regardless of whether we can evaluate
TIMESTAMP=$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)
mkdir -p "$HOME/.edikt" 2>/dev/null || true
echo "{\"ts\":\"${TIMESTAMP}\",\"event\":\"phase_completion_detected\",\"plan\":\"$(basename "$PLAN_FILE")\",\"phase\":${PHASE_NUM}}" >> "$HOME/.edikt/events.jsonl" 2>/dev/null || true

# ─── Warn if criteria sidecar is missing ─────────────────────────────────────
# A missing sidecar means fail_count, last_evaluated, and block_reason cannot
# be tracked. Evaluation still runs (falling back to plan markdown) but the
# user should know they're losing history and have a path to recover it.
if [ -z "$SIDECAR" ]; then
  python3 - "$PHASE_NUM" "$(basename "$PLAN_FILE")" <<'PYEOF'
import json, sys
phase_num = sys.argv[1]
plan_name = sys.argv[2]
stem = plan_name.replace(".md", "")
msg = (
    f"⚠️  Criteria sidecar not found for {plan_name}.\n"
    f"    Expected: {stem}-criteria.yaml (sibling of the plan file)\n"
    f"\n"
    f"    Evaluation will proceed using plan markdown, but fail_count,\n"
    f"    last_evaluated, and block_reason cannot be tracked this run.\n"
    f"\n"
    f"    To restore tracking, regenerate the sidecar:\n"
    f"      /edikt:sdlc:plan --sidecar-only {stem}"
)
# Prepend to any existing system message rather than replacing it.
print(json.dumps({"systemMessage": msg}))
PYEOF
fi

# ─── Dry run mode (for testing) ───────────────────────────────────────────────

if [ "${EDIKT_EVALUATOR_DRY_RUN:-0}" = "1" ]; then
  python3 - "$PHASE_NUM" "$PLAN_FILE" "$SIDECAR" <<'PYEOF'
import json, sys
phase_num = sys.argv[1]
plan = sys.argv[2]
sidecar = sys.argv[3] if len(sys.argv) > 3 else ""
msg = f"⚙️  Phase {phase_num} completion detected (dry-run).\n    Plan: {plan}\n    Sidecar: {sidecar or '(none)'}"
print(json.dumps({"systemMessage": msg}))
PYEOF
  exit 0
fi

# ─── Invoke headless evaluator ────────────────────────────────────────────────

EVAL_TEMPLATE="$HOME/.edikt/templates/agents/evaluator-headless.md"
if [ ! -f "$EVAL_TEMPLATE" ]; then
  EVAL_TEMPLATE="templates/agents/evaluator-headless.md"
fi

if [ ! -f "$EVAL_TEMPLATE" ]; then
  python3 <<'PYEOF'
import json
msg = "⚠️  Phase completion detected but evaluator template missing.\n    Expected: ~/.edikt/templates/agents/evaluator-headless.md\n    Re-run: curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash"
print(json.dumps({"systemMessage": msg}))
PYEOF
  exit 0
fi

# Build the evaluator prompt
PROMPT=$(python3 - "$PHASE_NUM" "$PLAN_FILE" "$SIDECAR" <<'PYEOF'
import sys, os
phase_num = sys.argv[1]
plan = sys.argv[2]
sidecar = sys.argv[3] if len(sys.argv) > 3 and sys.argv[3] else ""

prompt_parts = [
    f"Evaluate Phase {phase_num} of {os.path.basename(plan)}.",
    "",
    f"Plan file: {plan}",
]
if sidecar:
    prompt_parts.append(f"Criteria sidecar: {sidecar}")
    prompt_parts.append("")
    prompt_parts.append(f"Read the criteria for phase {phase_num} from the sidecar, run each `verify` command if present, and return per-criterion PASS/FAIL verdicts.")
else:
    prompt_parts.append("")
    prompt_parts.append(f"Read the acceptance criteria for phase {phase_num} from the plan, and return per-criterion PASS/FAIL verdicts.")

prompt_parts.extend([
    "",
    "Also run `git diff --name-only HEAD~1 HEAD 2>/dev/null || git diff --name-only` to see what was changed.",
    "",
    "Return your verdict in the format specified by the evaluator system prompt.",
])
print("\n".join(prompt_parts))
PYEOF
)

# Read evaluator config values
EVAL_MODEL=$(grep -A10 '^evaluator:' .edikt/config.yaml 2>/dev/null | grep -E '^\s*model:' | awk '{print $2}' | tr -d '"' | head -1)
[ -z "$EVAL_MODEL" ] && EVAL_MODEL="sonnet"

# Invoke evaluator
EVAL_OUTPUT=$(claude -p "$PROMPT" \
  --system-prompt "$(cat "$EVAL_TEMPLATE")" \
  --allowedTools "Read,Grep,Glob,Bash" \
  --disallowedTools "Write,Edit" \
  --model "$EVAL_MODEL" \
  --output-format text \
  --bare 2>&1 | head -200)

EVAL_EXIT=$?

# Log the evaluation event
echo "{\"ts\":\"$(date -u +%Y-%m-%dT%H:%M:%SZ 2>/dev/null || date +%Y-%m-%dT%H:%M:%SZ)\",\"event\":\"phase_evaluation\",\"plan\":\"$(basename "$PLAN_FILE")\",\"phase\":${PHASE_NUM},\"exit\":${EVAL_EXIT}}" >> "$HOME/.edikt/events.jsonl" 2>/dev/null || true

# Check verdict
if echo "$EVAL_OUTPUT" | grep -qE 'VERDICT:\s*PASS|Result:\s*PASS|All criteria PASS'; then
  python3 - "$PHASE_NUM" <<'PYEOF'
import json, sys
phase = sys.argv[1]
print(json.dumps({"systemMessage": f"✓ Phase {phase} evaluation: PASS"}))
PYEOF
  exit 0
fi

# Failure — surface to user
python3 - "$PHASE_NUM" "$EVAL_OUTPUT" <<'PYEOF'
import json, sys
phase = sys.argv[1]
output = sys.argv[2][:1500]
msg = f"⚠️  Phase {phase} evaluation FAILED:\n{output}\n\nReview the findings and fix before marking the phase done."
print(json.dumps({"systemMessage": msg}))
PYEOF
