#!/bin/bash
# Parity test for the v0.4.x nested-layout branch of resolve_commands_target.
# Pairs with test_flat_layout_symlink.sh — without this, a future refactor
# could invert the layout-detection condition and only break v0.4.x installs
# (which the flat-layout test wouldn't catch).
#
# Strict assertion: the symlink target string MUST end in current/commands/edikt
# (NOT current/commands), AND context.md MUST be visible under the symlink
# at the nested location.
#
# (ref: PLAN-v0.5.0-stability Phase 20, finding QA-LOW-11)
#
# Entry point: invoke via test/run.sh — install_setup refuses to run
# outside the test/run.sh sandbox.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup nested_layout
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload-nested"
make_payload_nested "$PAYLOAD" "0.5.0"

test_start "nested-layout payload → claude commands symlink resolves to NESTED shape"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out.log" 2>&1
rc=$?

if [ "$rc" -ne 0 ]; then
  echo "--- install.sh output ---"
  cat "$TEST_HOME/out.log"
  echo "-------------------------"
  fail "install.sh exits 0" "got $rc"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "install.sh exits 0"

CMDS_LINK="$TEST_CLAUDE_HOME/commands/edikt"
if [ ! -L "$CMDS_LINK" ]; then
  fail "claude commands symlink exists" "missing $CMDS_LINK"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "claude commands symlink exists"

LINK_RAW=$(ls -la "$CMDS_LINK" 2>&1)
LINK_TARGET=$(readlink "$CMDS_LINK")
LINK_RESOLVED=$(readlink -f "$CMDS_LINK" 2>/dev/null || python3 -c "import os,sys;print(os.path.realpath(sys.argv[1]))" "$CMDS_LINK" 2>/dev/null || echo "(could not resolve)")
echo "  symlink (raw):       $LINK_RAW"
echo "  symlink target:      $LINK_TARGET"
echo "  symlink resolves to: $LINK_RESOLVED"

# Nested shape: context.md must be visible directly under the symlink
# (since the symlink target IS commands/edikt under nested layout, the
# payload's commands/edikt/context.md surfaces at $CMDS_LINK/context.md).
if [ ! -f "$CMDS_LINK/context.md" ]; then
  echo "--- contents at $CMDS_LINK/ ---"
  ls -la "$CMDS_LINK/" 2>&1 | head -20
  echo "-------------------------------"
  fail "nested layout exposes commands/edikt/context.md" \
       "context.md not visible at $CMDS_LINK/context.md — layout detection picked wrong target"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "nested layout exposes commands/edikt/context.md"

# Strict assertion: target string must end in current/commands/edikt.
case "$LINK_TARGET" in
  */current/commands/edikt)
    pass "symlink target uses nested layout (current/commands/edikt)"
    ;;
  */current/commands)
    fail "symlink target uses nested layout (current/commands/edikt)" \
         "target was $LINK_TARGET — resolve_commands_target picked flat branch for nested payload"
    ;;
  *)
    fail "symlink target uses nested layout (current/commands/edikt)" \
         "unexpected target: $LINK_TARGET"
    ;;
esac

test_summary
