#!/bin/bash
# After installing a v0.5.x flat-layout payload (commands/*.md, no
# commands/edikt/ subdirectory), CLAUDE_ROOT/commands/edikt MUST resolve to
# a directory containing *.md files — i.e. resolve_commands_target picked the
# flat target path, not the legacy nested one.
#
# Regression test for the bug where ensure_external_symlinks hardcoded
# `current/commands/edikt`, producing a dangling symlink under the v0.5.x
# flat layout that hid every /edikt:* slash command from Claude Code.
#
# (ref: PLAN-v0.5.0-stability Phase 20)

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup flat-layout
trap install_teardown EXIT

# Build a flat-layout payload ourselves (make_payload from _lib.sh creates
# the nested layout). Mirrors the v0.5.x source-tree shape.
PAYLOAD="$TEST_HOME/_payload-flat"
rm -rf "$PAYLOAD"
mkdir -p "$PAYLOAD/templates/hooks" "$PAYLOAD/commands"
printf '0.5.0\n' > "$PAYLOAD/VERSION"
printf '# changelog 0.5.0\n' > "$PAYLOAD/CHANGELOG.md"
# Flat layout: commands/*.md directly, no commands/edikt/ subdir.
printf '# context\n' > "$PAYLOAD/commands/context.md"
printf '# adr/new\n' > "$PAYLOAD/commands/adr-new.md"
# Required hook scaffolding for install to validate.
printf '#!/bin/sh\necho hi\n' > "$PAYLOAD/templates/hooks/session-start.sh"
chmod +x "$PAYLOAD/templates/hooks/session-start.sh"

test_start "flat-layout payload → claude commands symlink resolves"

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

# Resolve and verify it points at a directory that actually exists. This is
# the exact thing the bug broke: the symlink existed but pointed at a
# nested path that didn't exist under v0.5.x.
LINK_TARGET=$(readlink "$CMDS_LINK")
echo "  symlink target: $LINK_TARGET"

if [ ! -d "$CMDS_LINK/" ]; then
  fail "symlink resolves to a directory" "target $LINK_TARGET does not resolve"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "symlink resolves to a directory"

# Under flat layout, the symlink should point at current/commands (no
# trailing /edikt). Permit either shape since resolve_commands_target
# auto-selects, but the resolved path MUST contain the *.md files we put
# in the payload.
if [ ! -f "$CMDS_LINK/context.md" ] && [ ! -f "$CMDS_LINK/edikt/context.md" ]; then
  echo "--- contents at $CMDS_LINK ---"
  ls -la "$CMDS_LINK/" 2>&1 | head -20
  echo "------------------------------"
  fail "context.md visible through symlink" "neither flat nor nested layout shape found"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "context.md visible through symlink"

# For a flat-layout payload the target path SHOULD be current/commands
# (proves the layout detection picked the right branch).
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
