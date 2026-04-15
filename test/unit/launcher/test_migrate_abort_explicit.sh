#!/bin/bash
# migrate --abort against an orphaned staging dir removes it cleanly.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_abort_explicit

# Build a flat layout.
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '0.4.3\n' >"$LAUNCHER_ROOT/VERSION"
printf '# c\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\n' >"$LAUNCHER_ROOT/hooks/h.sh"
chmod +x "$LAUNCHER_ROOT/hooks/h.sh"
printf '# t\n' >"$LAUNCHER_ROOT/templates/t.md"
printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"

# Plant orphaned staging dir (simulate a previous crashed run).
stg="$LAUNCHER_ROOT/.migrate-staging-20260101T000000Z"
mkdir -p "$stg/0.4.3/hooks"
printf 'junk\n' >"$stg/0.4.3/hooks/whatever"

test_start "migrate --abort cleans up orphaned staging"
out=$(run_launcher migrate --abort 2>&1)
rc=$?
assert_rc "$rc" 0 "migrate --abort exits 0"

assert_test "! -e '$stg'" "staging dir removed"
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"migration_aborted"'

# Plant an orphaned pre-migration dir with content to restore.
pre="$LAUNCHER_ROOT/.pre-migration-20260101T000001Z"
mkdir -p "$pre"
# Simulate a migration that moved the real hooks dir into pre.
mkdir -p "$LAUNCHER_ROOT/_hooks_saved"
cp -R "$LAUNCHER_ROOT/hooks/." "$LAUNCHER_ROOT/_hooks_saved/"
# Move hooks into pre (as migration step e(i) would) and replace with a
# symlink to simulate a partially-wired chain.
mv "$LAUNCHER_ROOT/hooks" "$pre/hooks"
ln -s current/hooks "$LAUNCHER_ROOT/hooks"

# Plant a valid backup tarball. migrate_abort (finding #2 hardening)
# refuses to mutate state without a verified backup.
bkdir="$LAUNCHER_ROOT/backups/migration-20260101T000001Z"
mkdir -p "$bkdir"
( cd "$LAUNCHER_ROOT" && tar -czf "$bkdir/pre-migration.tar.gz" \
    VERSION CHANGELOG.md templates commands 2>/dev/null )
if command -v sha256sum >/dev/null 2>&1; then
    hash=$(sha256sum "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
else
    hash=$(shasum -a 256 "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
fi
printf '%s  pre-migration.tar.gz\n' "$hash" >"$bkdir/pre-migration.tar.gz.sha256"

run_launcher migrate --abort >/dev/null 2>&1
rc=$?
assert_rc "$rc" 0 "second --abort on pre-migration exits 0"
assert_test "-d '$LAUNCHER_ROOT/hooks'" "hooks restored as real dir"
assert_test "! -L '$LAUNCHER_ROOT/hooks'" "hooks no longer a symlink"
assert_test "-f '$LAUNCHER_ROOT/hooks/h.sh'" "hook file restored"

test_summary
