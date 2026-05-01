#!/bin/bash
# use <not-installed> exits 1 with a clear message.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup use_missing

test_start "use missing version"
run_launcher use 9.9.9 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 1 ] && pass "use exits 1 on missing version" || fail "use exits 1" "got $rc"

if [ ! -L "$LAUNCHER_ROOT/current" ]; then
    pass "current symlink not created"
else
    fail "current symlink not created" "but it was"
fi

test_summary
