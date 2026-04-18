#!/usr/bin/env bash
# Phase 11b characterization: subagent-stop.sh
# subagent-stop-critical re-added (SPEC-006): ADR-023 structured payload rewrite
# eliminates git-identity nondeterminism — blocking message now derives from
# structured payload + config only, not git env vars.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="subagent-stop.sh"
STAGED="$STAGED_PROJECTS/edikt-project"
FIXTURES=(subagent-stop-critical subagent-stop-warning subagent-stop-ok)

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_staged_fixture "$HOOK" "$f" "$STAGED" || FAIL=1
done
exit "$FAIL"
