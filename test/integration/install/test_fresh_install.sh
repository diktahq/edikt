#!/bin/bash
# Fresh install: empty sandbox → install.sh --global --ref v0.5.0 → verify
# launcher, symlinks, lock.yaml, then run `edikt doctor` and assert 0.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup fresh
trap install_teardown EXIT

# Build a local payload that looks like the current repo so the launcher
# can install it as v0.5.0.
PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

test_start "install.sh fresh install --global --ref v0.5.0"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out.log" 2>&1
rc=$?

if [ "$rc" -ne 0 ]; then
  echo "--- install.sh output ---"
  cat "$TEST_HOME/out.log"
  echo "-------------------------"
fi

[ "$rc" -eq 0 ] && pass "install.sh exits 0" || fail "install.sh exits 0" "got $rc"

assert_file_exists "$TEST_EDIKT_ROOT/bin/edikt"
assert_dir_exists "$TEST_EDIKT_ROOT/versions/0.5.0"
assert_file_exists "$TEST_EDIKT_ROOT/lock.yaml"
assert_file_contains "$TEST_EDIKT_ROOT/lock.yaml" 'active: "0.5.0"'

# `current` symlink
if [ -L "$TEST_EDIKT_ROOT/current" ]; then
  pass "current symlink exists"
else
  fail "current symlink exists" "missing $TEST_EDIKT_ROOT/current"
fi

# Claude commands symlink should point into the versioned layout after `use`.
if [ -L "$TEST_CLAUDE_HOME/commands/edikt" ]; then
  pass "claude commands symlink exists"
else
  fail "claude commands symlink exists" "missing $TEST_CLAUDE_HOME/commands/edikt"
fi

# shell-rc PATH append with idempotent marker
RC="$TEST_HOME/.zshrc"
[ -f "$TEST_HOME/.bashrc" ] && RC="$TEST_HOME/.bashrc"
# either file acceptable — just check marker presence somewhere
if grep -qF "# edikt bootstrap (do not edit)" "$TEST_HOME"/.zshrc "$TEST_HOME"/.bashrc 2>/dev/null; then
  pass "PATH marker appended to shell rc"
else
  fail "PATH marker appended to shell rc" "no marker in .zshrc or .bashrc"
fi

# Doctor should exit 0 on a clean v0.5.0 layout.
"$TEST_EDIKT_ROOT/bin/edikt" doctor >"$TEST_HOME/doctor.log" 2>&1
drc=$?
if [ "$drc" -eq 0 ]; then
  pass "edikt doctor exits 0"
else
  echo "--- doctor output ---"
  cat "$TEST_HOME/doctor.log"
  echo "---------------------"
  fail "edikt doctor exits 0" "got $drc"
fi

test_summary
