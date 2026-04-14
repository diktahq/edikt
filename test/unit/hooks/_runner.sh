#!/usr/bin/env bash
# Shared hook-test runner. Pipes a fixture payload to a hook and diffs
# stdout against the expected-output sibling (jq -S normalized).
#
# Currently each caller is gated behind EDIKT_ENABLE_HOOK_JSON_TESTS=1.
# Hooks today emit plaintext; Phase 2b migrates them to the Claude Code
# hook JSON protocol and flips this gate on. Until then these tests skip
# rather than fail — see docs/product/plans/PLAN-v0.5.0-stability.md
# "Phase 2b" entry for context.

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../../.." && pwd)}"
PAYLOADS="$PROJECT_ROOT/test/fixtures/hook-payloads"
EXPECTED="$PROJECT_ROOT/test/expected/hook-outputs"
HOOKS="$PROJECT_ROOT/templates/hooks"

run_hook_fixture() {
    local hook_script="$1"
    local fixture_name="$2"

    local payload="$PAYLOADS/$fixture_name.json"
    local expected="$EXPECTED/$fixture_name.expected.json"

    if [ ! -f "$payload" ]; then
        echo "  MISSING: $payload"
        return 1
    fi
    if [ ! -f "$expected" ]; then
        echo "  MISSING: $expected"
        return 1
    fi

    local actual
    actual="$(cat "$payload" | bash "$HOOKS/$hook_script" 2>/dev/null)"

    # Empty actual OR empty expected body → treat empty as "{}".
    local actual_json="${actual:-\{\}}"

    if diff <(echo "$actual_json" | jq -S .) <(jq -S . "$expected") >/dev/null 2>&1; then
        echo "  PASS: $fixture_name"
        return 0
    else
        echo "  FAIL: $fixture_name"
        diff <(echo "$actual_json" | jq -S . 2>&1) <(jq -S . "$expected" 2>&1) | sed 's/^/    /'
        return 1
    fi
}

hook_suite_skip_notice() {
    local hook_name="$1"
    echo "  SKIP: $hook_name — hook emits plaintext; awaiting Phase 2b JSON protocol migration"
    echo "        Enable with EDIKT_ENABLE_HOOK_JSON_TESTS=1 once hooks are migrated."
}
