#!/bin/bash
# doctor on a freshly installed + activated tree exits 0.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup doctor_healthy

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

test_start "doctor healthy"
out=$(run_launcher doctor 2>&1)
rc=$?
[ "$rc" -eq 0 ] && pass "doctor exits 0 on healthy tree" || fail "doctor exits 0" "got $rc; out=$out"
echo "$out" | grep -q "result: healthy" && pass "doctor reports healthy" || fail "doctor reports healthy"
echo "$out" | grep -q "manifest:.*OK" && pass "manifest OK reported" || fail "manifest OK reported"

test_summary
