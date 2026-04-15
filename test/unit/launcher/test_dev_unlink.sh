#!/bin/bash
# dev unlink: removes versions/dev/, reverts to most-recent tagged version,
# emits dev_unlinked event. Also tests: no tagged version → exit 1.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup dev_unlink

# Seed a tagged version 0.5.0.
src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1

# Build a fake devrepo and link it.
devrepo="$LAUNCHER_ROOT/_devrepo"
mkdir -p "$devrepo/templates/hooks" "$devrepo/commands/edikt"
printf '0.5.0-dev\n' >"$devrepo/VERSION"
printf '#!/bin/sh\necho hi\n' >"$devrepo/templates/hooks/session-start.sh"
chmod +x "$devrepo/templates/hooks/session-start.sh"
printf '# cmd\n' >"$devrepo/commands/edikt/context.md"

run_launcher dev link "$devrepo" >/dev/null 2>&1

# Verify dev is active.
target=$(readlink "$LAUNCHER_ROOT/current" 2>/dev/null || echo "")
if [ "$target" != "versions/dev" ]; then
    fail "precondition: current → versions/dev" "got: $target"
fi

test_start "dev unlink reverts to tagged version"

run_launcher dev unlink >/dev/null 2>&1
rc=$?
assert_rc "$rc" "0" "dev unlink exits 0"

# versions/dev gone.
if [ ! -d "$LAUNCHER_ROOT/versions/dev" ]; then
    pass "versions/dev removed"
else
    fail "versions/dev removed" "still exists"
fi

# current → versions/0.5.0
target=$(readlink "$LAUNCHER_ROOT/current" 2>/dev/null || echo "")
if [ "$target" = "versions/0.5.0" ]; then
    pass "current reverted to versions/0.5.0"
else
    fail "current reverted to versions/0.5.0" "got: $target"
fi

# lock.yaml shows 0.5.0 active.
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'active: "0.5.0"' \
    "lock.yaml active is 0.5.0"

# dev_unlinked event emitted.
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"dev_unlinked"' \
    "dev_unlinked event emitted"

test_start "dev unlink: no tagged version → exit 1"

# Set up a fresh root with dev but no tagged version.
launcher_setup dev_unlink_noversion

devrepo2="$LAUNCHER_ROOT/_devrepo"
mkdir -p "$devrepo2/templates/hooks" "$devrepo2/commands/edikt"
printf '0.5.0-dev\n' >"$devrepo2/VERSION"
printf '#!/bin/sh\n' >"$devrepo2/templates/hooks/session-start.sh"
chmod +x "$devrepo2/templates/hooks/session-start.sh"
printf '# cmd\n' >"$devrepo2/commands/edikt/context.md"

run_launcher dev link "$devrepo2" >/dev/null 2>&1

# Remove any tagged versions — only dev exists.
# (dev link just created versions/dev; no tagged versions were installed)

out=$(run_launcher dev unlink 2>&1)
rc=$?
assert_rc "$rc" "1" "dev unlink exits 1 when no tagged version"

if echo "$out" | grep -qi "no tagged version"; then
    pass "error mentions no tagged version"
else
    fail "error mentions no tagged version" "output: $out"
fi

test_start "dev unlink: no dev link → exit 0 with message"

launcher_setup dev_unlink_nodev

out=$(run_launcher dev unlink 2>&1)
rc=$?
assert_rc "$rc" "0" "dev unlink exits 0 when no dev link"

if echo "$out" | grep -qi "no dev link"; then
    pass "message says no dev link active"
else
    fail "message says no dev link active" "output: $out"
fi

test_summary
