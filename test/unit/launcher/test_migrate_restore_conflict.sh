#!/bin/bash
# Review finding #13 — migrate_restore_from_predir must never rm -rf a
# non-empty directory at the destination. If content is present (because
# a previous recovery attempt, or a user intervention, put something
# there), the function must quarantine under ".conflict-<ts>-<pid>/"
# instead and restore the predir contents over a cleared path.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_restore_conflict

VERSION="0.4.3"
SENTINEL="conflict-sentinel-$(date +%s)-$$"

# Build flat layout (required to pass the dispatch gate? No — the
# dispatch gate sees the predir as migration_in_progress and admits
# migrate/doctor. Not relevant here: we invoke migrate --abort directly.).
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '%s\n' "$VERSION" >"$LAUNCHER_ROOT/VERSION"
printf '# c\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\n' >"$LAUNCHER_ROOT/hooks/h.sh"
printf '# t\n' >"$LAUNCHER_ROOT/templates/t.md"
printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"

# Plant predir with content to restore.
pre="$LAUNCHER_ROOT/.pre-migration-20260414T000000Z-fake"
mkdir -p "$pre/hooks"
printf 'from-predir\n' >"$pre/hooks/h.sh"

# Now the conflict: the destination $LAUNCHER_ROOT/hooks already holds a
# non-empty directory with user content. For abort to run we first move
# the flat hooks out of the way (as step e-i did) — but simulate that
# some OTHER content appeared at the destination after.
mv "$LAUNCHER_ROOT/hooks" "$pre/hooks-original"
mkdir -p "$LAUNCHER_ROOT/hooks"
printf '%s\n' "$SENTINEL" >"$LAUNCHER_ROOT/hooks/unexpected.txt"

# Write a minimal backup so abort doesn't refuse.
bkdir="$LAUNCHER_ROOT/backups/migration-20260414T000000Z-fake"
mkdir -p "$bkdir"
( cd "$LAUNCHER_ROOT" && tar -czf "$bkdir/pre-migration.tar.gz" \
    VERSION CHANGELOG.md templates commands 2>/dev/null )
if command -v sha256sum >/dev/null 2>&1; then
    h=$(sha256sum "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
else
    h=$(shasum -a 256 "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
fi
printf '%s  pre-migration.tar.gz\n' "$h" >"$bkdir/pre-migration.tar.gz.sha256"

test_start "restore_from_predir quarantines non-empty destination"
out=$(run_launcher migrate --abort 2>&1)
rc=$?
assert_rc "$rc" 0 "migrate --abort exits 0 despite conflict"

# The unexpected content must survive — at a .conflict-<ts>-<pid>/ sibling.
found=0
for q in "$LAUNCHER_ROOT"/hooks.conflict-*; do
    [ -d "$q" ] || continue
    if [ -f "$q/unexpected.txt" ] && grep -q "$SENTINEL" "$q/unexpected.txt"; then
        found=1
        break
    fi
done
if [ "$found" -eq 1 ]; then
    pass "conflicting destination content preserved under .conflict-<ts>-<pid>/"
else
    fail "content destroyed" "sentinel not found in quarantine; output: $out"
fi

# The predir content should have been restored at the destination.
if [ -f "$LAUNCHER_ROOT/hooks/h.sh" ] && grep -q "from-predir" "$LAUNCHER_ROOT/hooks/h.sh"; then
    pass "predir content restored at destination"
else
    fail "restore" "predir content not at destination"
fi

if printf '%s\n' "$out" | grep -q "refusing to rm-rf"; then
    pass "restore logged loud warning on quarantine"
else
    fail "log" "no 'refusing to rm-rf' warning in output: $out"
fi

test_summary
