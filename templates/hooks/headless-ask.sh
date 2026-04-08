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

# Check for predefined answers in config
ANSWER=$(python3 -c "
import yaml, sys
try:
    config = yaml.safe_load(open('.edikt/config.yaml'))
    answers = config.get('headless', {}).get('answers', {})
    question = sys.argv[1].lower()
    for pattern, answer in answers.items():
        if pattern.lower() in question:
            print(answer)
            sys.exit(0)
except:
    pass
# Default: answer 'yes' for yes/no questions, 'skip' for choices
if any(w in question.lower() for w in ['proceed', 'continue', 'confirm', 'y/n']):
    print('yes')
elif any(w in question.lower() for w in ['which', 'select', 'choose']):
    print('skip')
else:
    print('yes')
" "$QUESTION" 2>/dev/null || echo "yes")

# Return the answer via permissionDecision + updatedInput
echo "{\"permissionDecision\":\"allow\",\"updatedInput\":\"${ANSWER}\"}"
