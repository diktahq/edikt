#!/bin/bash
# rollback: install + use two versions, then rollback flips current back
# to the previous tag and emits rollback_performed.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup rollback_happy

s1="$LAUNCHER_ROOT/_src1"
s2="$LAUNCHER_ROOT/_src2"
make_payload "$s1" "0.5.0"
make_payload "$s2" "0.5.1"

EDIKT_INSTALL_SOURCE="$s1" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1
EDIKT_INSTALL_SOURCE="$s2" run_launcher install 0.5.1 >/dev/null 2>&1
run_launcher use 0.5.1 >/dev/null 2>&1

test_start "rollback happy"
v=$(run_launcher version)
[ "$v" = "0.5.1" ] && pass "active is 0.5.1 before rollback" || fail "pre-rollback active" "got $v"

run_launcher rollback >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "rollback exits 0" || fail "rollback exits 0" "got $rc"

v=$(run_launcher version)
[ "$v" = "0.5.0" ] && pass "active is 0.5.0 after rollback" || fail "post-rollback active" "got $v"

assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"rollback_performed"'

test_summary
