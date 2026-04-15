#!/bin/bash
# migrate --dry-run on a flat layout: prints plan, mutates nothing.
#
# Asserts: exit 0, plan text present, sha256 tree-hash identical before/after.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_dry_run

# Build a flat (pre-v0.5.0) layout: hooks/ + templates/ + commands/ as real
# dirs, VERSION at the root. No current symlink, no versions/ dir.
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '0.4.3\n' >"$LAUNCHER_ROOT/VERSION"
printf '# changelog\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\necho hi\n' >"$LAUNCHER_ROOT/hooks/session-start.sh"
chmod +x "$LAUNCHER_ROOT/hooks/session-start.sh"
printf '# template\n' >"$LAUNCHER_ROOT/templates/CLAUDE.md.tmpl"
printf '# context\n' >"$LAUNCHER_ROOT/commands/edikt/context.md"

# Tree-hash helper: sha256 over sorted (relpath + sha256 + mode) lines for
# every regular file under $LAUNCHER_ROOT (excluding events.jsonl and .lock
# which legitimately change).
tree_hash() {
    root="$1"
    ( cd "$root" && find . -type f \
        ! -name events.jsonl \
        ! -name '.lock' \
        ! -path './backups/*' \
        | LC_ALL=C sort \
        | while IFS= read -r p; do
            if command -v sha256sum >/dev/null 2>&1; then
                h=$(sha256sum "$p" | awk '{print $1}')
            else
                h=$(shasum -a 256 "$p" | awk '{print $1}')
            fi
            printf '%s %s\n' "$p" "$h"
        done ) | ( if command -v sha256sum >/dev/null 2>&1; then sha256sum; else shasum -a 256; fi ) | awk '{print $1}'
}

before_hash=$(tree_hash "$LAUNCHER_ROOT")

test_start "migrate --dry-run prints plan and mutates nothing"
out=$(run_launcher migrate --dry-run 2>&1)
rc=$?
assert_rc "$rc" 0 "dry-run exits 0"

assert_grep "Migration needed" "$out" "dry-run announces migration"
assert_grep "Will move" "$out" "dry-run lists move section"
assert_grep "Will create symlinks" "$out" "dry-run lists symlink plan"
assert_grep "dry-run: no changes written" "$out" "dry-run trailer present"

after_hash=$(tree_hash "$LAUNCHER_ROOT")
if [ "$before_hash" = "$after_hash" ]; then
    pass "tree sha256 identical after dry-run"
else
    fail "tree sha256 changed" "before=$before_hash after=$after_hash"
fi

# And no staging / pre-migration leftovers.
ls -d "$LAUNCHER_ROOT"/.migrate-staging-* 2>/dev/null && fail "staging dir exists after dry-run" || pass "no staging dir after dry-run"
ls -d "$LAUNCHER_ROOT"/.pre-migration-* 2>/dev/null && fail "pre-migration dir exists after dry-run" || pass "no pre-migration dir after dry-run"

# Dry-run on a fresh install (no flat layout) prints "No migration needed"
# and exits 0.
launcher_setup migrate_dry_run_fresh
out=$(run_launcher migrate --dry-run 2>&1)
rc=$?
assert_rc "$rc" 0 "dry-run on fresh install exits 0"
assert_grep "No migration needed" "$out" "dry-run says no migration needed on fresh"

test_summary
