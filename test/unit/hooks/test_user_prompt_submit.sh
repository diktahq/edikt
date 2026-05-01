#!/usr/bin/env bash
# Phase 11b characterization: user-prompt-submit.sh
# no-plan: no plan files in edikt-project → exit 0 silently → {}.
# with-plan: mid-plan staged dir has Phase 2 in-progress plan → systemMessage.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="user-prompt-submit.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
run_staged_fixture "$HOOK" user-prompt-submit-no-plan   "$STAGED_PROJECTS/edikt-project" || FAIL=1
run_staged_fixture "$HOOK" user-prompt-submit-with-plan "$STAGED_PROJECTS/mid-plan"      || FAIL=1
exit "$FAIL"
