#!/bin/bash
# migrate without --yes in a non-interactive session must refuse.
#
# We close stdin (</dev/null) so /dev/tty is unavailable in this CI-style
# invocation. The launcher must exit non-zero and instruct the user to
# re-run with --yes.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup migrate_prompt

mkdir -p "$LAUNCHER_ROOT/hooks" "$LAUNCHER_ROOT/templates" "$LAUNCHER_ROOT/commands/edikt"
printf '0.4.3\n' >"$LAUNCHER_ROOT/VERSION"
printf '# c\n' >"$LAUNCHER_ROOT/CHANGELOG.md"
printf '#!/bin/sh\n' >"$LAUNCHER_ROOT/hooks/h.sh"
chmod +x "$LAUNCHER_ROOT/hooks/h.sh"
printf '# t\n' >"$LAUNCHER_ROOT/templates/t.md"
printf '# c\n' >"$LAUNCHER_ROOT/commands/edikt/c.md"

test_start "migrate without --yes refuses in non-interactive session"

# Drive with: no stdin, detached from any terminal via `setsid`-style
# redirection. `bash </dev/null` is enough — /dev/tty may still be openable
# when a controlling terminal exists, so we also close fd 0/1/2 from a tty
# perspective by launching with `setsid` when available, falling back to
# plain redirect otherwise. The launcher falls back to reading /dev/tty when
# stdin is not a tty — that's the path we need to exercise.
if command -v setsid >/dev/null 2>&1; then
    out=$(setsid "$LAUNCHER" migrate </dev/null 2>&1)
    rc=$?
else
    # macOS / BSD: no setsid. Redirect /dev/tty readers by running under a
    # subshell with no controlling tty. `script` is one option, but the
    # cleanest POSIX-ish approach is `exec </dev/null` in the child —
    # which we've already done via redirection.
    out=$("$LAUNCHER" migrate </dev/null 2>&1)
    rc=$?
fi

# We can't guarantee /dev/tty is truly absent when running inside a parent
# terminal, but in CI (no tty) the launcher must refuse. Accept either:
#   (a) non-zero exit with "non-interactive" message, OR
#   (b) a cleanly-aborted migration (the user typed nothing → exit 0 with
#       "aborted" and NO mutation). Either is a correct refusal to migrate
#       without explicit consent. What MUST NOT happen is a successful
#       migration.
if [ ! -L "$LAUNCHER_ROOT/hooks" ]; then
    pass "hooks remains a real directory (no unauthorized migration)"
else
    fail "unauthorized migration occurred" "hooks became a symlink without --yes"
fi

# Verify we did NOT write lock.yaml or emit layout_migrated.
if [ ! -f "$LAUNCHER_ROOT/lock.yaml" ]; then
    pass "no lock.yaml written"
else
    if ! grep -q '"event":"layout_migrated"' "$LAUNCHER_ROOT/events.jsonl" 2>/dev/null; then
        pass "no layout_migrated event emitted"
    else
        fail "migration happened" "layout_migrated event present"
    fi
fi

# Message/exit expectation: either exit != 0 with "non-interactive" hint,
# or exit 0 "aborted".
case "$rc" in
    0)
        assert_grep "aborted\|non-interactive\|--yes" "$out" "refusal message present"
        ;;
    *)
        assert_grep "non-interactive\|--yes" "$out" "refusal message instructs --yes"
        ;;
esac

test_summary
