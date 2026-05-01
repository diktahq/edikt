#!/usr/bin/env bash
# Phase 11b characterization: session-start.sh
# session-start-with-edikt removed: hook emits plaintext systemMessage banner
# (not JSON) until Phase 2b.ii migrates it to the JSON protocol.
# session-start-no-edikt: no .edikt/config.yaml → hook exits 0 silently → {}.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="session-start.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
# session-start-no-edikt: run from no-edikt dir → hook exits immediately → {}
run_staged_fixture "$HOOK" session-start-no-edikt "$STAGED_PROJECTS/no-edikt" || FAIL=1
exit "$FAIL"
