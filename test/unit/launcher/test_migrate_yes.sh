#!/bin/bash
# migrate --yes happy path: flat → versioned, idempotent on re-run.
#
# Asserts:
#   - hooks/ becomes a symlink → current/hooks
#   - current → versions/<v>
#   - manifest.yaml exists inside versions/<v>/
#   - lock.yaml has active=<v>, installed_via=migration
#   - layout_migrated event emitted
#   - pre-migration tarball present under backups/migration-<ts>/
#   - second run is a no-op (exit 0, no new event)

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_yes

VERSION="0.4.3"
# Build flat layout with real files across hooks, templates, commands.
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates/rules" "$LAUNCHER_ROOT/commands/edikt"
printf '%s\n' "$VERSION" >"$LAUNCHER_ROOT/VERSION"
printf '# changelog\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\necho hi\n' >"$LAUNCHER_ROOT/hooks/session-start.sh"
chmod +x "$LAUNCHER_ROOT/hooks/session-start.sh"
printf '# rule\n' >"$LAUNCHER_ROOT/templates/rules/base.md"
printf '# context\n' >"$LAUNCHER_ROOT/commands/edikt/context.md"
# Also a preserved user file that must NOT move.
printf 'preserved: true\n' >"$LAUNCHER_ROOT/config.yaml"

test_start "migrate --yes happy path"
out=$(run_launcher migrate --yes 2>&1)
rc=$?
assert_rc "$rc" 0 "migrate --yes exits 0"

# Post-migration layout checks.
assert_test "-L '$LAUNCHER_ROOT/current'" "current is a symlink"
assert_test "-L '$LAUNCHER_ROOT/hooks'" "hooks is a symlink (no longer a real dir)"
assert_test "-L '$LAUNCHER_ROOT/templates'" "templates is a symlink"
assert_test "-d '$LAUNCHER_ROOT/versions/$VERSION'" "versions/<v> dir exists"
assert_test "-f '$LAUNCHER_ROOT/versions/$VERSION/manifest.yaml'" "manifest.yaml present"
assert_test "-f '$LAUNCHER_ROOT/versions/$VERSION/VERSION'" "VERSION file migrated"
assert_test "-x '$LAUNCHER_ROOT/hooks/session-start.sh'" "hook remains executable via symlink"
assert_test "-f '$LAUNCHER_ROOT/templates/rules/base.md'" "template file reachable via symlink"
assert_test "-L '$CLAUDE_HOME/commands/edikt'" "claude commands symlink wired"

# User data preserved at the root.
assert_test "-f '$LAUNCHER_ROOT/config.yaml'" "config.yaml preserved at root"

# lock.yaml
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'active: "'"$VERSION"'"'
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'installed_via: "migration"'

# events.jsonl
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"layout_migrated"'

# backup tarball present and readable.
backup_tar=$(ls "$LAUNCHER_ROOT"/backups/migration-*/pre-migration.tar.gz 2>/dev/null | head -1)
if [ -n "$backup_tar" ] && tar -tzf "$backup_tar" >/dev/null 2>&1; then
    pass "pre-migration.tar.gz present and readable"
else
    fail "pre-migration backup" "not found or not readable: $backup_tar"
fi

# Staging/pre-migration dirs cleaned up.
for stray in "$LAUNCHER_ROOT"/.migrate-staging-* "$LAUNCHER_ROOT"/.pre-migration-*; do
    if [ -e "$stray" ]; then
        fail "leftover $stray" "must be cleaned up"
    fi
done
pass "no staging or pre-migration leftovers"

# Idempotence: second invocation is a no-op.
events_before=$(wc -l <"$LAUNCHER_ROOT/events.jsonl" | tr -d ' ')
out2=$(run_launcher migrate --yes 2>&1)
rc2=$?
assert_rc "$rc2" 0 "second migrate --yes exits 0"
assert_grep "No migration needed" "$out2" "second run says no migration needed"
events_after=$(wc -l <"$LAUNCHER_ROOT/events.jsonl" | tr -d ' ')
if [ "$events_before" = "$events_after" ]; then
    pass "no new events on idempotent re-run"
else
    fail "event count changed" "before=$events_before after=$events_after"
fi

test_summary
