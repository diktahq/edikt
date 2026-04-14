#!/bin/bash
# Mutate a payload file after install; doctor must report tamper.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup doctor_tamper

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

# Tamper with a recorded payload file.
echo "tampered" >>"$LAUNCHER_ROOT/versions/0.5.0/commands/edikt/context.md"

test_start "doctor flags manifest tampering"
out=$(run_launcher doctor 2>&1)
rc=$?
[ "$rc" -eq 2 ] && pass "doctor exits 2 on tamper" || fail "doctor exits 2" "got $rc"
echo "$out" | grep -q "manifest integrity check failed" && pass "tamper error reported" || fail "tamper error reported" "out=$out"

test_summary
