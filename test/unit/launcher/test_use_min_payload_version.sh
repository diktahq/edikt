#!/bin/bash
# use refuses to activate a payload older than MIN_PAYLOAD_VERSION (0.5.0).
# We install a 0.4.0 payload by bypassing the install command (writing
# directly into versions/) and then attempt to use it.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup use_min_payload

# Hand-craft an old payload directly in versions/.
old_dir="$LAUNCHER_ROOT/versions/0.4.0"
mkdir -p "$old_dir/templates" "$old_dir/hooks" "$old_dir/commands/edikt"
printf '0.4.0\n' >"$old_dir/VERSION"

test_start "use refuses payload older than MIN_PAYLOAD_VERSION"
run_launcher use 0.4.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 2 ] && pass "use exits 2 on old payload" || fail "use exits 2" "got $rc"

# current must NOT have flipped
if [ ! -L "$LAUNCHER_ROOT/current" ]; then
    pass "current symlink not created"
else
    fail "current symlink not created" "but it was"
fi

test_summary
