#!/bin/bash
# dev link: creates versions/dev/ with correct symlinks, activates dev,
# emits dev_linked event.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup dev_link

# Build a fake source tree representing a developer checkout.
devrepo="$LAUNCHER_ROOT/_devrepo"
mkdir -p "$devrepo/templates/hooks" "$devrepo/commands/edikt"
printf '0.5.0-dev\n' >"$devrepo/VERSION"
printf '# Changelog\n' >"$devrepo/CHANGELOG.md"
printf '#!/bin/sh\necho hi\n' >"$devrepo/templates/hooks/session-start.sh"
chmod +x "$devrepo/templates/hooks/session-start.sh"
printf '# context cmd\n' >"$devrepo/commands/edikt/context.md"

test_start "dev link creates symlinks and activates"

run_launcher dev link "$devrepo" >/dev/null 2>&1
rc=$?
assert_rc "$rc" "0" "dev link exits 0"

# versions/dev/ exists.
assert_dir_exists "$LAUNCHER_ROOT/versions/dev" "versions/dev created"

# Symlinks point to the right places.
if [ -L "$LAUNCHER_ROOT/versions/dev/VERSION" ]; then
    tgt=$(readlink "$LAUNCHER_ROOT/versions/dev/VERSION")
    if [ "$tgt" = "$devrepo/VERSION" ]; then
        pass "dev/VERSION symlink correct"
    else
        fail "dev/VERSION symlink correct" "got: $tgt"
    fi
else
    fail "dev/VERSION is a symlink"
fi

if [ -L "$LAUNCHER_ROOT/versions/dev/hooks" ]; then
    tgt=$(readlink "$LAUNCHER_ROOT/versions/dev/hooks")
    if [ "$tgt" = "$devrepo/templates/hooks" ]; then
        pass "dev/hooks symlink correct"
    else
        fail "dev/hooks symlink correct" "got: $tgt"
    fi
else
    fail "dev/hooks is a symlink"
fi

if [ -L "$LAUNCHER_ROOT/versions/dev/templates" ]; then
    tgt=$(readlink "$LAUNCHER_ROOT/versions/dev/templates")
    if [ "$tgt" = "$devrepo/templates" ]; then
        pass "dev/templates symlink correct"
    else
        fail "dev/templates symlink correct" "got: $tgt"
    fi
else
    fail "dev/templates is a symlink"
fi

if [ -L "$LAUNCHER_ROOT/versions/dev/commands" ]; then
    tgt=$(readlink "$LAUNCHER_ROOT/versions/dev/commands")
    if [ "$tgt" = "$devrepo/commands" ]; then
        pass "dev/commands symlink correct"
    else
        fail "dev/commands symlink correct" "got: $tgt"
    fi
else
    fail "dev/commands is a symlink"
fi

# CHANGELOG.md symlink (present in devrepo).
if [ -L "$LAUNCHER_ROOT/versions/dev/CHANGELOG.md" ]; then
    pass "dev/CHANGELOG.md symlinked"
else
    fail "dev/CHANGELOG.md symlinked" "not a symlink"
fi

# DEV_SOURCE file contains the path.
if [ -f "$LAUNCHER_ROOT/versions/dev/DEV_SOURCE" ]; then
    src_path=$(cat "$LAUNCHER_ROOT/versions/dev/DEV_SOURCE")
    if [ "$src_path" = "$devrepo" ]; then
        pass "DEV_SOURCE contains source path"
    else
        fail "DEV_SOURCE contains source path" "got: $src_path"
    fi
else
    fail "DEV_SOURCE file created"
fi

# current → versions/dev
target=$(readlink "$LAUNCHER_ROOT/current" 2>/dev/null || echo "")
if [ "$target" = "versions/dev" ]; then
    pass "current points to versions/dev"
else
    fail "current points to versions/dev" "got: $target"
fi

# dev_linked event emitted.
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"dev_linked"' \
    "dev_linked event emitted"

test_start "dev link: fails when required path missing"

bad="$LAUNCHER_ROOT/_badrepo"
mkdir -p "$bad"
printf '0.5.0-dev\n' >"$bad/VERSION"
# Missing templates/ — should fail.
out=$(run_launcher dev link "$bad" 2>&1)
rc=$?
if [ "$rc" -ne 0 ]; then
    pass "dev link fails with missing templates/"
else
    fail "dev link fails with missing templates/" "exited 0 but should fail"
fi

test_summary
