#!/usr/bin/env bash
# Phase 11b characterization: post-compact.sh
# Hook re-injects plan phase + invariants after compaction (JSON systemMessage).
# Staged project dirs provide deterministic plan + invariant state.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="post-compact.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
run_staged_fixture "$HOOK" post-compact-with-plan           "$STAGED_PROJECTS/mid-plan"      || FAIL=1
run_staged_fixture "$HOOK" post-compact-with-failing-criteria "$STAGED_PROJECTS/post-compact" || FAIL=1
exit "$FAIL"
