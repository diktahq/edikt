#!/usr/bin/env bash
# Phase 11b characterization: pre-tool-use.sh
# Hook warns if docs/project-context.md is missing. Staged dir has it → silent.
# Characterization: exit 0 silently → {}.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="pre-tool-use.sh"
STAGED="$STAGED_PROJECTS/edikt-project"
FIXTURES=(pre-tool-use-write)

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_staged_fixture "$HOOK" "$f" "$STAGED" || FAIL=1
done
exit "$FAIL"
