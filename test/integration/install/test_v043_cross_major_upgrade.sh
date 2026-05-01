#!/bin/bash
# TODO(Phase 7): replace synthetic fixture with test/integration/migration/fixtures/v0.4.3/ once capture.sh exists.
#
# v0.4.x → v0.5.0 cross-major upgrade. Seeds a minimal synthetic flat
# layout (the shape Phase 4 migration expects), runs install.sh, verifies
# migration fires, launcher lands, symlinks resolve, lock.yaml has active
# set to the v0.5.0 tag.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup v043_upgrade
trap install_teardown EXIT

# ─── Seed synthetic v0.4.3 layout at $TEST_EDIKT_ROOT ───────────────────────
# The minimum shape that makes needs_migration() in the launcher return
# true: a real-dir hooks/ plus a VERSION file at the root.
mkdir -p "$TEST_EDIKT_ROOT/hooks"
mkdir -p "$TEST_EDIKT_ROOT/templates/rules/base"
mkdir -p "$TEST_EDIKT_ROOT/templates/agents"
mkdir -p "$TEST_EDIKT_ROOT/templates/hooks"
mkdir -p "$TEST_CLAUDE_HOME/commands/edikt"
printf '0.4.3\n' > "$TEST_EDIKT_ROOT/VERSION"
printf '# v0.4.3 changelog\n' > "$TEST_EDIKT_ROOT/CHANGELOG.md"
printf '#!/bin/sh\necho "session-start stub"\n' > "$TEST_EDIKT_ROOT/hooks/session-start.sh"
chmod +x "$TEST_EDIKT_ROOT/hooks/session-start.sh"
printf '# dummy rule\n' > "$TEST_EDIKT_ROOT/templates/rules/base/code-quality.md"
printf '# init command\n' > "$TEST_CLAUDE_HOME/commands/edikt/init.md"

# Payload to install as v0.5.0.
PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

test_start "install.sh cross-major v0.4.3 → v0.5.0"

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

# Migration banner present
if grep -qF "Detected v0.4.x install. Migrating" "$TEST_HOME/out.log"; then
  pass "migration banner printed"
else
  fail "migration banner printed" "banner missing from output"
fi

# Launcher present
assert_file_exists "$TEST_EDIKT_ROOT/bin/edikt"

# hooks/ is now a symlink (was a real dir pre-migration)
if [ -L "$TEST_EDIKT_ROOT/hooks" ]; then
  pass "hooks/ is now a symlink"
else
  fail "hooks/ is now a symlink" "still a real dir after migrate"
fi

# lock.yaml shows v0.5.0 active
assert_file_exists "$TEST_EDIKT_ROOT/lock.yaml"
assert_file_contains "$TEST_EDIKT_ROOT/lock.yaml" 'active: "0.5.0"'

# current symlink resolves
if [ -L "$TEST_EDIKT_ROOT/current" ] && [ -d "$TEST_EDIKT_ROOT/current/" ]; then
  pass "current symlink resolves"
else
  fail "current symlink resolves" "current missing or broken"
fi

test_summary
