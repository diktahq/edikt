#!/bin/bash
# After install, every templates/hooks/* file on disk MUST be exactly mode
# 0755. Catches a release tarball that was packed from a working tree where
# git modes were correct but on-disk modes got corrupted by a packaging step
# (umask drift, archive tool quirks).
#
# Pairs with test/test-hook-modes.sh which asserts the same invariant against
# the git index. The two layers cover both source-of-truth (git) and the
# delivered artifact (extracted payload), since they can drift independently.
#
# (ref: PLAN-v0.5.0-stability Phase 21)

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup fresh
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

test_start "templates/hooks/* are mode 0755 after install"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out.log" 2>&1
rc=$?

if [ "$rc" -ne 0 ]; then
  echo "--- install.sh output ---"
  cat "$TEST_HOME/out.log"
  echo "-------------------------"
  fail "install.sh exits 0" "got $rc — cannot check post-install hook modes"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "install.sh exits 0"

HOOKS_DIR="$TEST_EDIKT_ROOT/current/templates/hooks"
if [ ! -d "$HOOKS_DIR" ]; then
  fail "templates/hooks/ exists post-install" "missing $HOOKS_DIR"
  test_summary
  exit "$FAIL_COUNT"
fi

# Portable octal-mode read: GNU stat (-c %a) and BSD stat (-f %Lp) both
# return the permission bits as octal. Try GNU first, fall back to BSD.
get_mode() {
  if stat -c '%a' "$1" >/dev/null 2>&1; then
    stat -c '%a' "$1"
  else
    stat -f '%Lp' "$1"
  fi
}

WRONG=0
TOTAL=0
while IFS= read -r hook; do
  TOTAL=$((TOTAL + 1))
  mode=$(get_mode "$hook")
  if [ "$mode" = "755" ]; then
    pass "Mode 0755: $(basename "$hook")"
  else
    WRONG=$((WRONG + 1))
    fail "Mode WRONG: $(basename "$hook") is 0$mode (expected exactly 0755)"
  fi
done < <(find "$HOOKS_DIR" -type f \( -name '*.sh' -o -name 'pre-push' \) | sort)

if [ "$TOTAL" -eq 0 ]; then
  fail "Found at least one hook in $HOOKS_DIR" "directory is empty after install"
fi

test_summary
