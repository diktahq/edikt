#!/usr/bin/env bash
# Phase 11b characterization: stop-hook.sh
# Reads last_assistant_message from stdin JSON and emits signal systemMessages.
# Requires .edikt/config.yaml in CWD (edikt-project staged dir).
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="stop-hook.sh"
STAGED="$STAGED_PROJECTS/edikt-project"
FIXTURES=(
    stop-adr-candidate
    stop-new-route
    stop-new-env-var
    stop-security-change
    stop-clean
    stop-loop-guard
)

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_staged_fixture "$HOOK" "$f" "$STAGED" || FAIL=1
done
exit "$FAIL"
