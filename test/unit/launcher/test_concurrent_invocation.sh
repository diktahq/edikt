#!/bin/bash
# Two launchers attempting to mutate state concurrently: one wins the
# lock, the other exits 4 (EX_LOCKED). We hold the lock with a short
# sleep wrapper around the launcher to guarantee overlap.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup concurrent

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"

# Pre-create EDIKT_ROOT so flock has a target.
mkdir -p "$LAUNCHER_ROOT"

# Acquire the lock manually in a background process and hold it.
LOCKFILE="$LAUNCHER_ROOT/.lock"
LOCKDIR="$LAUNCHER_ROOT/.lock.d"

if command -v flock >/dev/null 2>&1; then
    (
        # Hold the flock for 3 seconds.
        flock -n 9 || exit 1
        sleep 3
    ) 9>"$LOCKFILE" &
else
    mkdir "$LOCKDIR" 2>/dev/null
    ( sleep 3; rm -rf "$LOCKDIR" ) &
fi
sleep 1

test_start "concurrent invocation"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 4 ] && pass "second launcher exits 4 (EX_LOCKED)" || fail "EX_LOCKED" "got $rc"

wait 2>/dev/null
test_summary
