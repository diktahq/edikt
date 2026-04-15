#!/usr/bin/env bash
# Phase 11b characterization: subagent-stop.sh
# subagent-stop-critical removed: blocking message embeds git user info +
# timestamps → nondeterministic, cannot be characterized (see fixtures.yaml).
# warning + ok: no gate logic fires → {"continue": true} for both.
# Opt-out: EDIKT_SKIP_HOOK_TESTS=1
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"
. "$PROJECT_ROOT/test/unit/hooks/_staged_runner.sh"

HOOK="subagent-stop.sh"
STAGED="$STAGED_PROJECTS/edikt-project"
FIXTURES=(subagent-stop-warning subagent-stop-ok)

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: $HOOK — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_staged_fixture "$HOOK" "$f" "$STAGED" || FAIL=1
done
exit "$FAIL"
