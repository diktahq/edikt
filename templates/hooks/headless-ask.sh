#!/usr/bin/env bash
# edikt: PreToolUse hook for headless/CI environments
# Auto-answers AskUserQuestion calls with predefined responses.
#
# When EDIKT_HEADLESS=1 is set, this hook intercepts AskUserQuestion
# and returns updatedInput with a predefined answer, enabling CI pipelines
# to run edikt commands without human interaction.
#
# Usage:
#   EDIKT_HEADLESS=1 claude --bare -p "/edikt:gov:compile --check"
#
# Configure answers in .edikt/config.yaml:
#   headless:
#     answers:
#       "proceed with compilation": "yes"
#       "which packs to update": "all"

# Only activate in headless mode
if [ "${EDIKT_HEADLESS:-0}" != "1" ]; then exit 0; fi

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# Check if this is an AskUserQuestion call
TOOL_NAME=$(echo "$INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")
if [ "$TOOL_NAME" != "AskUserQuestion" ]; then exit 0; fi

# Extract the question
QUESTION=$(echo "$INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_input',{}).get('question',''))" 2>/dev/null || echo "")

if [ -z "$QUESTION" ]; then exit 0; fi

# Look up the answer in config. YAML errors are loud per LOW-12 — a malformed
# config must not silently suppress headless policy.
ANSWER=$(python3 - <<'PY' "$QUESTION"
import sys
try:
    import yaml
except ImportError:
    print("[edikt] headless-ask: pyyaml not installed; returning default", file=sys.stderr)
    print("yes")
    sys.exit(0)
try:
    with open('.edikt/config.yaml') as f:
        config = yaml.safe_load(f) or {}
except yaml.YAMLError as e:
    print(f"[edikt] headless-ask: .edikt/config.yaml is not valid YAML: {e}", file=sys.stderr)
    sys.exit(2)
except OSError as e:
    print(f"[edikt] headless-ask: cannot read .edikt/config.yaml: {e}", file=sys.stderr)
    sys.exit(2)
answers = (config.get('headless') or {}).get('answers') or {}
question = sys.argv[1].lower()
for pattern, answer in answers.items():
    if str(pattern).lower() in question:
        print(answer)
        sys.exit(0)
# Default heuristics for unmapped questions
if any(w in question for w in ['proceed', 'continue', 'confirm', 'y/n']):
    print('yes')
elif any(w in question for w in ['which', 'select', 'choose']):
    print('skip')
else:
    print('yes')
PY
)
ANSWER_EXIT=$?
if [ "$ANSWER_EXIT" -ne 0 ]; then
  # YAML unparseable — loudly fail so the policy isn't silently suppressed.
  exit "$ANSWER_EXIT"
fi

# Return the answer via permissionDecision + updatedInput, built with
# json.dumps so quotes/newlines in $ANSWER cannot inject hook-protocol
# keys (INV-003; closes audit CRIT-4).
python3 -c 'import json,sys; print(json.dumps({"permissionDecision": "allow", "updatedInput": sys.argv[1]}))' "$ANSWER"
