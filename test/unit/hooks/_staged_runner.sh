#!/usr/bin/env bash
# Staged fixture runner — extends _runner.sh with CWD-aware hook execution.
# Source _runner.sh first, then source this file.
#
# run_staged_fixture <hook_script> <fixture_name> <staged_dir>
#   Runs the hook from <staged_dir> so filesystem-dependent hooks
#   (those that check for .edikt/config.yaml, plan files, etc.) see
#   controlled, deterministic state rather than the developer's live repo.
#
# _runner.sh is NOT modified — this file is an extension layer only.

STAGED_PROJECTS="$PROJECT_ROOT/test/fixtures/projects"

run_staged_fixture() {
    local hook_script="$1"
    local fixture_name="$2"
    local staged_dir="$3"

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

    # Run hook from staged_dir; PAYLOADS/EXPECTED/HOOKS are absolute from PROJECT_ROOT.
    local actual actual_json
    actual="$(cd "$staged_dir" && cat "$payload" | bash "$HOOKS/$hook_script" 2>/dev/null)"

    # Use explicit branch instead of ${:-\{\}} — the latter produces literal \{\}
    # in bash rather than the intended {} when the variable is empty.
    if [ -z "$actual" ]; then actual_json="{}"; else actual_json="$actual"; fi

    if diff <(echo "$actual_json" | jq -S .) <(jq -S . "$expected") >/dev/null 2>&1; then
        echo "  PASS: $fixture_name"
        return 0
    else
        echo "  FAIL: $fixture_name"
        diff <(echo "$actual_json" | jq -S . 2>&1) <(jq -S . "$expected" 2>&1) | sed 's/^/    /'
        return 1
    fi
}
