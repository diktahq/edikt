#!/usr/bin/env bash
# edikt: FileChanged hook — detect external governance file modifications
# Fires when files are modified outside of Claude (e.g., by another editor or git).

# Only run in edikt projects
if [ ! -f ".edikt/config.yaml" ]; then exit 0; fi

# Read hook input from stdin
INPUT=$(cat)

# Extract changed file path
CHANGED_FILE=$(echo "$INPUT" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('file_path',''))" 2>/dev/null || echo "")

if [ -z "$CHANGED_FILE" ]; then exit 0; fi

# Only warn about governance-related files
case "$CHANGED_FILE" in
  *.claude/rules/*|*.edikt/*|*docs/architecture/decisions/*|*docs/architecture/invariants/*)
    TIMESTAMP=$(date -u +"%Y-%m-%dT%H:%M:%SZ")
    mkdir -p "$HOME/.edikt" 2>/dev/null || true
    echo "${TIMESTAMP} FILE_CHANGED_EXTERNAL ${CHANGED_FILE}" >> "$HOME/.edikt/session-signals.log"

    # Surface warning via systemMessage
    echo '{"systemMessage":"⚠ Governance file modified externally: '"${CHANGED_FILE}"'. Run /edikt:gov:compile if this affects ADRs or invariants."}'
    ;;
esac

exit 0
