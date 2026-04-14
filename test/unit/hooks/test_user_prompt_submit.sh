#!/usr/bin/env bash
set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
export PROJECT_ROOT
# shellcheck source=./_runner.sh
. "$PROJECT_ROOT/test/unit/hooks/_runner.sh"

HOOK="user-prompt-submit.sh"
FIXTURES=(user-prompt-submit-no-plan user-prompt-submit-with-plan)

if [ "${EDIKT_ENABLE_HOOK_JSON_TESTS:-0}" != "1" ]; then
    hook_suite_skip_notice "$HOOK"
    exit 0
fi

FAIL=0
for f in "${FIXTURES[@]}"; do
    run_hook_fixture "$HOOK" "$f" || FAIL=1
done
exit "$FAIL"
