#!/bin/bash
# --ref <tag> propagates into the launcher install/use calls. Provide a
# v0.5.1-labeled payload via EDIKT_INSTALL_SOURCE and assert the installed
# tree lands at versions/0.5.1 with lock.yaml active=0.5.1.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup ref_flag
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload_051"
make_payload "$PAYLOAD" "0.5.1"

test_start "install.sh --ref v0.5.1"

EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.1 >"$TEST_HOME/out.log" 2>&1
rc=$?

if [ "$rc" -ne 0 ]; then
  echo "--- install.sh output ---"
  cat "$TEST_HOME/out.log"
  echo "-------------------------"
fi

[ "$rc" -eq 0 ] && pass "install.sh exits 0" || fail "install.sh exits 0" "got $rc"

assert_dir_exists "$TEST_EDIKT_ROOT/versions/0.5.1"
assert_file_contains "$TEST_EDIKT_ROOT/lock.yaml" 'active: "0.5.1"'

# versions/0.5.0 should NOT exist on fresh install with --ref v0.5.1.
if [ ! -d "$TEST_EDIKT_ROOT/versions/0.5.0" ]; then
  pass "only --ref tag installed (no 0.5.0 entry)"
else
  fail "only --ref tag installed" "versions/0.5.0 unexpectedly present"
fi

test_summary
