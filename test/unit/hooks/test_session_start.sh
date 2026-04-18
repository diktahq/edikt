#!/usr/bin/env bash
# Phase 11b characterization: session-start.sh
# session-start-with-edikt re-added (SPEC-006): hook now emits JSON additionalContext
# (ADR-014 migration). _staged_runner.sh provisions stable env via stub_git_identity
# + stub_clock so output is deterministic.
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
# session-start-with-edikt: .edikt/config.yaml present → hook emits JSON additionalContext
run_staged_fixture "$HOOK" session-start-with-edikt "$STAGED_PROJECTS/edikt-project" || FAIL=1
# session-start-no-edikt: run from no-edikt dir → hook exits immediately → {}
run_staged_fixture "$HOOK" session-start-no-edikt "$STAGED_PROJECTS/no-edikt" || FAIL=1
exit "$FAIL"
