#!/bin/bash
# dev link safety: if versions/dev/ contains regular files (user content),
# it must be quarantined rather than silently destroyed.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup dev_link_clobber

# Build a fake source tree for the dev link target.
devrepo="$LAUNCHER_ROOT/_devrepo"
mkdir -p "$devrepo/templates/hooks" "$devrepo/commands/edikt"
printf '0.5.0-dev\n' >"$devrepo/VERSION"
printf '#!/bin/sh\necho hi\n' >"$devrepo/templates/hooks/session-start.sh"
chmod +x "$devrepo/templates/hooks/session-start.sh"
printf '# context cmd\n' >"$devrepo/commands/edikt/context.md"

# Pre-populate versions/dev/ with a regular file that should NOT be destroyed.
dev_dir="$LAUNCHER_ROOT/versions/dev"
mkdir -p "$dev_dir"
printf 'important content\n' >"$dev_dir/important.txt"

test_start "dev link: quarantines versions/dev/ with regular files instead of destroying"

run_launcher dev link "$devrepo" >/dev/null 2>&1
rc=$?

# The command should succeed (it either quarantined and re-created, or errored).
# We accept rc=0 (quarantine path) or rc=1 (refused path) — both are safe.
# What is NOT acceptable: important.txt silently gone with no trace.

# Check that important.txt was not silently destroyed. It should either:
#   (a) still exist at the original path (refused/re-used), or
#   (b) exist inside a quarantine dir (dev.aborted-<ts>-<pid>/important.txt).
survived=0

# Case (a): still at original path.
if [ -f "$dev_dir/important.txt" ]; then
    survived=1
fi

# Case (b): quarantined — look for any .aborted-* sibling dir containing it.
for qdir in "$LAUNCHER_ROOT/versions"/dev.aborted-*; do
    [ -d "$qdir" ] || continue
    if [ -f "$qdir/important.txt" ]; then
        survived=1
        break
    fi
done

if [ "$survived" -eq 1 ]; then
    pass "important.txt survived (not silently destroyed)"
else
    fail "important.txt survived (not silently destroyed)" \
        "file was destroyed without quarantine"
fi

# If the command succeeded (rc=0), verify the new dev link was created properly.
if [ "$rc" -eq 0 ]; then
    assert_dir_exists "$dev_dir" "versions/dev/ recreated after quarantine"
    if [ -L "$dev_dir/VERSION" ]; then
        pass "new dev link is properly wired after quarantine"
    else
        fail "new dev link is properly wired after quarantine" \
            "dev/VERSION is not a symlink"
    fi
fi

test_summary
