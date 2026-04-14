#!/bin/bash
# rollback with no previous recorded → exit 1.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup rollback_no_prev

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

test_start "rollback with no previous"
run_launcher rollback >/dev/null 2>&1
rc=$?
[ "$rc" -eq 1 ] && pass "rollback exits 1 when no previous" || fail "rollback exits 1" "got $rc"

# Active version unchanged
v=$(run_launcher version)
[ "$v" = "0.5.0" ] && pass "active unchanged after failed rollback" || fail "active unchanged" "got $v"

test_summary
