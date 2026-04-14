#!/bin/bash
# Break the templates symlink and confirm doctor exits 2 with an error.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup doctor_broken

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

# Break the chain by removing the underlying templates dir.
rm -rf "$LAUNCHER_ROOT/versions/0.5.0/templates"

test_start "doctor flags broken templates symlink"
out=$(run_launcher doctor 2>&1)
rc=$?
[ "$rc" -eq 2 ] && pass "doctor exits 2 on broken symlink" || fail "doctor exits 2" "got $rc"
echo "$out" | grep -q "ERROR" && pass "doctor prints ERROR" || fail "doctor prints ERROR"

test_summary
