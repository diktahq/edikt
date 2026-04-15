#!/bin/bash
# v0.5.0 → v0.5.0 idempotent re-run. After a fresh install, running
# install.sh again with identical flags must produce no new versions/<tag>
# entry (re-install returns EX_ALREADY=3 which install.sh treats as noop)
# and must NOT duplicate the shell-rc PATH marker.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup v050_noop
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

test_start "install.sh v0.5.0 → v0.5.0 noop"

# First run → fresh install.
EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/run1.log" 2>&1
rc1=$?
[ "$rc1" -eq 0 ] && pass "first run exits 0" || { fail "first run exits 0" "got $rc1"; cat "$TEST_HOME/run1.log"; }

# Snapshot versions/ before second run.
before_versions=$(ls "$TEST_EDIKT_ROOT/versions" 2>/dev/null | sort | tr '\n' ' ')

# Snapshot rc marker count before.
rc_before=$( (grep -cF "# edikt bootstrap (do not edit)" "$TEST_HOME"/.zshrc "$TEST_HOME"/.bashrc 2>/dev/null || true) | awk -F: '{s+=$NF} END{print s+0}')

# Second run, same flags.
EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/run2.log" 2>&1
rc2=$?
[ "$rc2" -eq 0 ] && pass "second run exits 0" || { fail "second run exits 0" "got $rc2"; cat "$TEST_HOME/run2.log"; }

# versions/ unchanged.
after_versions=$(ls "$TEST_EDIKT_ROOT/versions" 2>/dev/null | sort | tr '\n' ' ')
if [ "$before_versions" = "$after_versions" ]; then
  pass "versions/ unchanged across runs"
else
  fail "versions/ unchanged across runs" "before='$before_versions' after='$after_versions'"
fi

# shell rc marker not duplicated.
rc_after=$( (grep -cF "# edikt bootstrap (do not edit)" "$TEST_HOME"/.zshrc "$TEST_HOME"/.bashrc 2>/dev/null || true) | awk -F: '{s+=$NF} END{print s+0}')
if [ "$rc_after" = "$rc_before" ]; then
  pass "rc marker not duplicated"
else
  fail "rc marker not duplicated" "before=$rc_before after=$rc_after"
fi

# Active still 0.5.0.
assert_file_contains "$TEST_EDIKT_ROOT/lock.yaml" 'active: "0.5.0"'

test_summary
