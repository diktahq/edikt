#!/bin/bash
# Critical test: SIGKILL mid-migration must leave $EDIKT_ROOT recoverable.
#
# Flow:
#   1. Build a flat layout with known files. Record a sha256 manifest.
#   2. Inject a slow-step into the migration by making the source large
#      enough that the tar-pipe copy takes measurable time. We then
#      kill -9 the launcher while it's mid-flight.
#   3. Run `edikt doctor` — expect exit != 0 and an "interrupted migration"
#      recommendation.
#   4. Run `edikt migrate --abort`. Assert exit 0.
#   5. Re-compute sha256 manifest over user-owned files; must match the
#      pre-migration manifest byte-for-byte.
#
# The test accepts either the pre-copy crash (nothing moved; abort is a
# no-op) or the post-sideways-move crash (files in .pre-migration-<ts>/;
# abort restores them). Both paths must yield byte-for-byte identical
# restoration.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_crash

VERSION="0.4.3"

# Build a flat layout. Make hooks/ and templates/ large-ish so the tar-pipe
# copy isn't instant (improves our chances of catching it mid-flight).
mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '%s\n' "$VERSION" >"$LAUNCHER_ROOT/VERSION"
printf '# changelog\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
# 50 hook files, 200 template files. Each ~2KB of junk. Total ~500KB.
# Enough to make the staging copy take a few ms on fast disks.
i=0; while [ "$i" -lt 50 ]; do
    printf '#!/bin/sh\n# hook %s\n%s\n' "$i" "$(head -c 2048 </dev/urandom | base64 | head -c 2000)" >"$LAUNCHER_ROOT/hooks/h-$i.sh"
    chmod +x "$LAUNCHER_ROOT/hooks/h-$i.sh"
    i=$((i+1))
done
i=0; while [ "$i" -lt 200 ]; do
    printf '# template %s\n%s\n' "$i" "$(head -c 2048 </dev/urandom | base64 | head -c 2000)" >"$LAUNCHER_ROOT/templates/t-$i.md"
    i=$((i+1))
done
printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"

# User data that must not be touched.
printf 'user: true\n' >"$LAUNCHER_ROOT/config.yaml"
mkdir -p "$LAUNCHER_ROOT/custom"
printf 'custom\n' >"$LAUNCHER_ROOT/custom/mine.md"

# Record pre-migration manifest over user-owned paths.
pre_manifest() {
    ( cd "$LAUNCHER_ROOT" && find hooks templates commands custom VERSION CHANGELOG.md config.yaml -type f 2>/dev/null \
        | LC_ALL=C sort \
        | while IFS= read -r p; do
            if command -v sha256sum >/dev/null 2>&1; then
                sha256sum "$p"
            else
                shasum -a 256 "$p"
            fi
        done )
}

before=$(pre_manifest)

test_start "crash recovery: kill -9 mid-migration, then --abort restores"

# Launch migration in background. Poll for the staging dir to appear —
# that's the evidence we've crossed into the critical window. Once visible,
# SIGKILL. If the migration completes before staging is observable, we
# accept it as a valid race (see below).
"$LAUNCHER" migrate --yes >/dev/null 2>&1 &
bg=$!

# Poll up to ~2s in 10ms increments waiting for the staging dir.
killed=0
tries=0
while [ "$tries" -lt 200 ]; do
    for stg in "$LAUNCHER_ROOT"/.migrate-staging-* "$LAUNCHER_ROOT"/.pre-migration-*; do
        if [ -e "$stg" ]; then
            kill -9 "$bg" 2>/dev/null || true
            killed=1
            break 2
        fi
    done
    # Migration may have finished already.
    if ! kill -0 "$bg" 2>/dev/null; then
        break
    fi
    sleep 0.01
    tries=$((tries + 1))
done
wait "$bg" 2>/dev/null || true

# Doctor must detect something wrong (either interrupted migration left
# leftovers, or migration completed before kill — in which case test the
# happy-path was clean). Differentiate:
if [ -L "$LAUNCHER_ROOT/hooks" ] && [ -d "$LAUNCHER_ROOT/versions/$VERSION" ]; then
    # Migration actually completed before the kill landed. Still valid —
    # assert the output is valid.
    pass "migration raced ahead of kill (fully-complete state — valid)"
    # Still test abort idempotency.
    out=$(run_launcher migrate --abort 2>&1)
    rc=$?
    assert_rc "$rc" 0 "migrate --abort after completion exits 0"
    test_summary
    exit 0
fi

# Interrupted. Assert doctor catches it.
doctor_out=$(run_launcher doctor 2>&1); doctor_rc=$?
if [ "$doctor_rc" -ne 0 ]; then
    pass "doctor returns non-zero on interrupted migration"
else
    fail "doctor rc" "expected non-zero, got 0: $doctor_out"
fi

if printf '%s\n' "$doctor_out" | grep -qi "interrupted migration\|migrate --abort"; then
    pass "doctor suggests migrate --abort"
else
    fail "doctor output" "no --abort hint: $doctor_out"
fi

# Now run abort.
abort_out=$(run_launcher migrate --abort 2>&1)
abort_rc=$?
assert_rc "$abort_rc" 0 "migrate --abort exits 0"

# Post-abort: flat layout back in place, no leftovers.
for stray in "$LAUNCHER_ROOT"/.migrate-staging-* "$LAUNCHER_ROOT"/.pre-migration-*; do
    if [ -e "$stray" ]; then
        fail "leftover after abort" "$stray"
    fi
done
pass "no staging or pre-migration leftovers after abort"

# hooks must be a real directory again (not a symlink).
if [ -d "$LAUNCHER_ROOT/hooks" ] && [ ! -L "$LAUNCHER_ROOT/hooks" ]; then
    pass "hooks restored as real directory"
else
    fail "hooks layout" "not a real dir after abort"
fi

# Byte-for-byte comparison.
after=$(pre_manifest)
if [ "$before" = "$after" ]; then
    pass "user-owned files restored byte-for-byte"
else
    fail "content drift after abort" \
        "$(diff <(printf '%s\n' "$before") <(printf '%s\n' "$after") | head -20)"
fi

test_summary
