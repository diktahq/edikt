#!/bin/bash
# upgrade-pin outside a project: no .edikt/config.yaml in ancestor walk.
# Must exit 1 with a clear error message.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup upgrade_pin_outside

# Seed an active version so upgrade-pin can at least reach the config-check.
src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

test_start "upgrade-pin outside project exits 1"

# Run from the sandbox HOME which has no .edikt/config.yaml.
# The ancestor walk is bounded by $HOME, so it will find nothing.
out=$( cd "$HOME" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher upgrade-pin 2>&1 )
rc=$?
assert_rc "$rc" "1" "exits 1 when no project config found"

if echo "$out" | grep -qi "not inside an edikt project"; then
    pass "error mentions not inside an edikt project"
else
    fail "error mentions not inside an edikt project" "output: $out"
fi

# No version_pinned event should have been emitted.
if [ -f "$LAUNCHER_ROOT/events.jsonl" ]; then
    if grep -q '"event":"version_pinned"' "$LAUNCHER_ROOT/events.jsonl"; then
        fail "no version_pinned event when outside project" \
            "event was emitted unexpectedly"
    else
        pass "no version_pinned event when outside project"
    fi
else
    pass "no version_pinned event when outside project"
fi

test_summary
