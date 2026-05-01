#!/bin/bash
# Review finding #2 — migrate_abort must NEVER rm -rf a non-empty tree that
# could contain user content. Two scenarios:
#
#   A. Populated versions/<v> exists (as it would after the staging-to-versions
#      rename at cmd_migrate step e-ii) WITH a sibling predir. Abort must
#      preserve the user-identifying file — either at its original flat path
#      OR under a quarantined .aborted-<ts>-<pid>/ sibling.
#
#   B. Same state, but the backup tarball is missing/corrupt. Abort must
#      refuse and exit non-zero with a clear message, leaving all on-disk
#      content untouched.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_abort_rm_safety

VERSION="0.4.3"
SENTINEL_CONTENT="user-content-sentinel-$(date +%s)-$$"

# Helper: fabricate a half-migrated state as if SIGKILL landed between
# step e-ii (staged → versions/<v> rename) and the symlink wiring.
build_half_migrated_state() {
    ts="$1"
    # Flat layout (original).
    mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
    printf '%s\n' "$VERSION" >"$LAUNCHER_ROOT/VERSION"
    printf '# changelog\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
    printf '#!/bin/sh\n' >"$LAUNCHER_ROOT/hooks/h.sh"
    chmod +x "$LAUNCHER_ROOT/hooks/h.sh"
    printf '# t\n' >"$LAUNCHER_ROOT/templates/t.md"
    printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"

    # Predir: the flat contents have been moved sideways here (as in
    # cmd_migrate step e-i). For this harness we leave flat in place AND
    # copy into predir — the test only needs the presence of predir to
    # trigger the touch-versions code path.
    predir="$LAUNCHER_ROOT/.pre-migration-$ts-fake"
    mkdir -p "$predir"
    cp -R "$LAUNCHER_ROOT/hooks" "$predir/hooks"
    cp -R "$LAUNCHER_ROOT/templates" "$predir/templates"
    cp -R "$LAUNCHER_ROOT/commands" "$predir/commands"
    cp "$LAUNCHER_ROOT/VERSION" "$predir/VERSION"
    cp "$LAUNCHER_ROOT/CHANGELOG.md" "$predir/CHANGELOG.md"

    # Populated versions/<v> — as step e-ii produced. Plant a sentinel
    # file the abort must not destroy.
    mkdir -p "$LAUNCHER_ROOT/versions/$VERSION/hooks"
    printf '%s\n' "$SENTINEL_CONTENT" >"$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt"

    # Backup tarball (and sha256 sidecar) — what cmd_migrate writes in
    # step a. Required for abort to proceed.
    bkdir="$LAUNCHER_ROOT/backups/migration-$ts-fake"
    mkdir -p "$bkdir"
    ( cd "$LAUNCHER_ROOT" && tar -czf "$bkdir/pre-migration.tar.gz" \
        hooks templates commands VERSION CHANGELOG.md 2>/dev/null )
    if command -v sha256sum >/dev/null 2>&1; then
        h=$(sha256sum "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
    else
        h=$(shasum -a 256 "$bkdir/pre-migration.tar.gz" | awk '{print $1}')
    fi
    printf '%s  pre-migration.tar.gz\n' "$h" >"$bkdir/pre-migration.tar.gz.sha256"
}

# ── Scenario A: abort with valid backup preserves sentinel ─────────────────
test_start "abort preserves user content in populated versions/<v>"

build_half_migrated_state "20260414T000000Z"

# Sanity: sentinel exists before abort.
if [ ! -f "$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt" ]; then
    fail "setup" "sentinel not planted"
fi

out=$(run_launcher migrate --abort 2>&1)
rc=$?
assert_rc "$rc" 0 "migrate --abort with valid backup exits 0"

# The sentinel must still exist — either at the original path (if abort
# finished restoring) or in a quarantined .aborted-<ts>-<pid> sibling.
found=0
if [ -f "$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt" ] \
    && grep -q "$SENTINEL_CONTENT" "$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt"; then
    found=1
fi
for q in "$LAUNCHER_ROOT"/versions/"$VERSION".aborted-*; do
    [ -d "$q" ] || continue
    if [ -f "$q/hooks/sentinel.txt" ] && grep -q "$SENTINEL_CONTENT" "$q/hooks/sentinel.txt"; then
        found=1
        break
    fi
done
if [ "$found" -eq 1 ]; then
    pass "sentinel file preserved (either in place or quarantined)"
else
    fail "sentinel destroyed" "rm -rf destroyed user content during abort: $out"
fi

# Quarantine log line (warn:) should be visible in output when the rm was
# refused. We assert the warn message shape only if a quarantine occurred.
if printf '%s\n' "$out" | grep -q "refusing to rm-rf"; then
    pass "abort logged loud warning when quarantining versions/<v>"
fi

# ── Scenario B: abort refuses when backup tarball is missing ───────────────
# Rebuild state and intentionally corrupt the backup.
rm -rf "$LAUNCHER_ROOT"
mkdir -p "$LAUNCHER_ROOT"
build_half_migrated_state "20260414T000001Z"
# Remove the tarball contents to simulate corruption.
rm -f "$LAUNCHER_ROOT/backups/migration-20260414T000001Z-fake/pre-migration.tar.gz"
rm -f "$LAUNCHER_ROOT/backups/migration-20260414T000001Z-fake/pre-migration.tar.gz.sha256"

test_start "abort refuses when backup tarball is missing/corrupt"
out=$(run_launcher migrate --abort 2>&1)
rc=$?
if [ "$rc" -ne 0 ]; then
    pass "migrate --abort with missing backup exits non-zero"
else
    fail "abort rc" "expected non-zero, got 0: $out"
fi

# Sentinel must be untouched — we refused to mutate.
if [ -f "$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt" ] \
    && grep -q "$SENTINEL_CONTENT" "$LAUNCHER_ROOT/versions/$VERSION/hooks/sentinel.txt"; then
    pass "sentinel untouched when abort refused"
else
    fail "sentinel drift" "abort mutated state despite refusing"
fi

if printf '%s\n' "$out" | grep -qi "no readable backup\|refusing to mutate"; then
    pass "abort surfaced clear diagnostic about missing backup"
else
    fail "abort diagnostic" "no backup-missing hint in output: $out"
fi

test_summary
