#!/bin/bash
# Regression test: under v0.5.x flat-layout payload (commands/*.md, no
# commands/edikt/ subdirectory), CLAUDE_ROOT/commands/edikt MUST resolve
# to the FLAT shape — i.e. resolve_commands_target picked
# current/commands, NOT current/commands/edikt.
#
# Strict assertion: the symlink must expose the payload's flat *.md files
# directly under CLAUDE_ROOT/commands/edikt/, AND the legacy nested shape
# (CLAUDE_ROOT/commands/edikt/edikt/*.md) MUST NOT exist. If only the
# nested shape resolved, the layout-detection helper picked the wrong
# branch and the bug is back.
#
# Pair test: test_nested_layout_symlink.sh covers the v0.4.x branch.
#
# (ref: PLAN-v0.5.0-stability Phase 20)
#
# Entry point: invoke via test/run.sh — install_setup refuses to run
# outside the test/run.sh sandbox.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup flat_layout
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload-flat"
make_payload_flat "$PAYLOAD" "0.5.0"

test_start "flat-layout payload → claude commands symlink resolves to FLAT shape"

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

# Diagnostics for any failure below — show the symlink itself (no slash,
# so we don't follow it) and its fully-resolved target.
LINK_RAW=$(ls -la "$CMDS_LINK" 2>&1)
LINK_TARGET=$(readlink "$CMDS_LINK")
LINK_RESOLVED=$(readlink -f "$CMDS_LINK" 2>/dev/null || python3 -c "import os,sys;print(os.path.realpath(sys.argv[1]))" "$CMDS_LINK" 2>/dev/null || echo "(could not resolve)")
echo "  symlink (raw):       $LINK_RAW"
echo "  symlink target:      $LINK_TARGET"
echo "  symlink resolves to: $LINK_RESOLVED"

# Strict regression assertion #1: flat shape MUST exist (context.md visible
# directly under the symlink without an /edikt/ prefix).
if [ ! -f "$CMDS_LINK/context.md" ]; then
  echo "--- contents at $CMDS_LINK/ (follows symlink) ---"
  ls -la "$CMDS_LINK/" 2>&1 | head -20
  echo "------------------------------------------------"
  fail "flat layout exposes commands/context.md" \
       "context.md not visible at $CMDS_LINK/context.md — layout detection picked wrong target"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "flat layout exposes commands/context.md"

# Strict regression assertion #2: nested shape MUST NOT exist. If both
# resolve, the symlink target ends in current/commands and the payload
# happens to ship a stray edikt/ dir — that's the legacy branch fixture
# leaking into a flat-layout test. The whole point of the bug fix is that
# the symlink target itself is current/commands (NOT current/commands/edikt).
if [ -f "$CMDS_LINK/edikt/context.md" ]; then
  fail "nested shape MUST NOT resolve under flat payload" \
       "found $CMDS_LINK/edikt/context.md — payload polluted with edikt/ subdir, or symlink picked nested branch"
fi

# Strict regression assertion #3: the symlink TARGET STRING must end in
# current/commands (no trailing /edikt). This proves resolve_commands_target
# selected the flat branch — even if the file-content check above passed by
# coincidence, the wrong target would fail this.
case "$LINK_TARGET" in
  */current/commands)
    pass "symlink target uses flat layout (current/commands)"
    ;;
  */current/commands/edikt)
    fail "symlink target uses flat layout (current/commands)" \
         "target was $LINK_TARGET — resolve_commands_target picked nested branch for flat payload"
    ;;
  *)
    fail "symlink target uses flat layout (current/commands)" \
         "unexpected target: $LINK_TARGET"
    ;;
esac

test_summary
