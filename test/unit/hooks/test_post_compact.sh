#!/usr/bin/env bash
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
# shellcheck source=./_runner.sh
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"

HOOK="post-compact.sh"
FIXTURES=(post-compact-with-plan post-compact-with-failing-criteria)

if [ "${EDIKT_ENABLE_HOOK_JSON_TESTS:-0}" != "1" ]; then
    hook_suite_skip_notice "$HOOK"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_hook_fixture "$HOOK" "$f" || FAIL=1
done
exit "$FAIL"
