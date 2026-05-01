#!/usr/bin/env bash
# Phase 11b characterization: pre-compact.sh
# Hook always emits a plaintext warning string (not JSON).
# Cannot be characterized as JSON without the v0.6.0 JSON protocol (Phase 2b.ii).
# Fixture pair removed; suite exits 0 (honest empty test, no regressions possible).
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"

HOOK="pre-compact.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

# No fixtures: pre-compact emits plaintext, not JSON.
# Testable after Phase 2b.ii migrates it to the hook JSON protocol.
echo "  NOTE: $HOOK — plaintext output, no fixture pairs (see fixtures.yaml §9.1)"
exit 0
