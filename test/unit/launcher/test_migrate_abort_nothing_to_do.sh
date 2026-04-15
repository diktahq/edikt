#!/bin/bash
# migrate --abort when there is nothing to abort exits 0 with a clear
# message. Fresh install AND a fully-migrated install both count.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_abort_nothing

test_start "migrate --abort on fresh install exits 0"
out=$(run_launcher migrate --abort 2>&1)
rc=$?
assert_rc "$rc" 0 "exits 0"
assert_grep "nothing to abort" "$out" "says nothing to abort"

# Now build a completed migration and run --abort again — must be a no-op.
launcher_setup migrate_abort_postmigrate
VERSION="0.4.3"
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '%s\n' "$VERSION" >"$LAUNCHER_ROOT/VERSION"
printf '# c\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\n' >"$LAUNCHER_ROOT/hooks/h.sh"
chmod +x "$LAUNCHER_ROOT/hooks/h.sh"
printf '# t\n' >"$LAUNCHER_ROOT/templates/t.md"
printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"
run_launcher migrate --yes >/dev/null 2>&1

out2=$(run_launcher migrate --abort 2>&1)
rc2=$?
assert_rc "$rc2" 0 "post-migration --abort exits 0"
assert_grep "nothing to abort" "$out2" "post-migration says nothing to abort"

# Layout must be unchanged (still versioned).
assert_test "-L '$LAUNCHER_ROOT/current'" "current still a symlink"
assert_test "-L '$LAUNCHER_ROOT/hooks'" "hooks still a symlink"

test_summary
