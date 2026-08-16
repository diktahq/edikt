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

# ── python3 preflight (F-074/F-075 loud-failure bar) ────────────────────────
# This hook exists specifically for CI/headless pipelines — the audience
# most likely to run a slim, python3-less container, and the one with no
# human present to read a systemMessage. Absence used to fail every
# extraction silently, so an AskUserQuestion call in a headless run went
# unanswered with rc=0 and zero output — indistinguishable in CI logs from
# "no question was asked". stderr is the one channel a CI operator can
# actually grep.
if ! command -v python3 >/dev/null 2>&1; then
    echo "edikt: headless-ask — python3 not found; EDIKT_HEADLESS cannot auto-answer AskUserQuestion calls on this host. Any pending question will stall waiting for interactive input that headless mode cannot provide." >&2
    exit 0
fi

# Check if this is an AskUserQuestion call
TOOL_NAME=$(echo "$INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_name',''))" 2>/dev/null || echo "")
if [ "$TOOL_NAME" != "AskUserQuestion" ]; then exit 0; fi

# Extract the question
QUESTION=$(echo "$INPUT" | python3 -c "import json,sys; print(json.load(sys.stdin).get('tool_input',{}).get('question',''))" 2>/dev/null || echo "")

if [ -z "$QUESTION" ]; then exit 0; fi

# Look up the answer in config. Unreadable config stays loud per LOW-12 —
# a suppressed headless policy must not fail silent. Stdlib-only parse
# (INV-001): the previous PyYAML ImportError path answered "yes" +
# allow to EVERY question in CI — the missing-dependency case was the
# maximally permissive answer. That path no longer exists.
ANSWER=$(python3 - <<'PY' "$QUESTION"
import sys
question = sys.argv[1].lower()
try:
    with open('.edikt/config.yaml', encoding='utf-8') as f:
        lines = f.read().split('\n')
except OSError as e:
    print(f"[edikt] headless-ask: cannot read .edikt/config.yaml: {e}", file=sys.stderr)
    sys.exit(2)

# Scan the nested `headless: / answers:` block for `pattern: answer` pairs.
answers = {}
in_headless = False
in_answers = False
answers_indent = None
for line in lines:
    if not line.strip() or line.lstrip().startswith('#'):
        continue
    indent = len(line) - len(line.lstrip(' '))
    stripped = line.strip()
    if indent == 0:
        in_headless = stripped.startswith('headless:')
        in_answers = False
        continue
    if not in_headless:
        continue
    if stripped.startswith('answers:'):
        in_answers = True
        answers_indent = indent
        continue
    if in_answers:
        if answers_indent is not None and indent <= answers_indent:
            in_answers = False
            continue
        if ':' in stripped:
            k, _, v = stripped.partition(':')
            # chr(34)+chr(39) == double+single quote; spelled via chr() to
            # keep quotes balanced for bash command-substitution parsing.
            k = k.strip().strip(chr(34) + chr(39))
            v = v.strip().strip(chr(34) + chr(39))
            if k:
                answers[k] = v

for pattern, answer in answers.items():
    if pattern.lower() in question:
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
# keys.
python3 -c 'import json,sys; print(json.dumps({"permissionDecision": "allow", "updatedInput": sys.argv[1]}))' "$ANSWER"
