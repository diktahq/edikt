#!/usr/bin/env bash
# edikt: PostToolUse hook (Write|Edit) — auto-format files after edits
# Runs the appropriate formatter for the file's language silently.
#
# NOTE: This hook only fires for Write/Edit calls in the main Claude session.
# Subagents spawned via the Agent tool run in a subprocess and do NOT trigger
# this hook. To compensate, each code-writing agent template (backend,
# frontend, qa, mobile) includes an explicit
# "File Formatting" section instructing the agent to run the formatter itself.

# Disable flags
if [ "${EDIKT_FORMAT_SKIP:-0}" = "1" ]; then exit 0; fi
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi
if grep -q "auto-format: false" .edikt/config.yaml 2>/dev/null; then exit 0; fi

FILE="${CLAUDE_TOOL_INPUT_FILE_PATH:-${CLAUDE_TOOL_INPUT_PATH:-}}"
if [ -z "$FILE" ] || [ ! -f "$FILE" ]; then exit 0; fi

# INV-006 / audit MED-14: reject filenames containing newlines, null bytes,
# or other metacharacters before passing them to a formatter. A crafted file
# path with embedded `;` or newline could otherwise split case statements or
# smuggle arguments into the formatter invocation.
case "$FILE" in
  *[!A-Za-z0-9_./-]*)
    exit 0
    ;;
esac

EXT="${FILE##*.}"

case "$EXT" in
  go)
    command -v gofmt >/dev/null 2>&1 && gofmt -w "$FILE" 2>/dev/null || true
    ;;
  ts|tsx|js|jsx)
    if [ -f "node_modules/.bin/prettier" ]; then
      node_modules/.bin/prettier --write "$FILE" 2>/dev/null
    elif command -v prettier >/dev/null 2>&1; then
      prettier --write "$FILE" 2>/dev/null
    fi
    ;;
  py)
    if command -v black >/dev/null 2>&1; then
      black "$FILE" 2>/dev/null
    elif command -v ruff >/dev/null 2>&1; then
      ruff format "$FILE" 2>/dev/null
    fi
    ;;
  rb)
    command -v rubocop >/dev/null 2>&1 && rubocop -A "$FILE" 2>/dev/null
    ;;
  php)
    command -v php-cs-fixer >/dev/null 2>&1 && php-cs-fixer fix "$FILE" 2>/dev/null
    ;;
  rs)
    command -v rustfmt >/dev/null 2>&1 && rustfmt "$FILE" 2>/dev/null
    ;;
esac

exit 0
