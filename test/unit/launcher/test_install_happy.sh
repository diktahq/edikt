#!/bin/bash
# Happy-path install: extract local source dir into versions/<tag>/, write
# manifest.yaml, emit version_installed event. Does NOT activate.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_happy

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"

test_start "install happy path"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "install exits 0" || fail "install exits 0" "got $rc"

assert_dir_exists "$LAUNCHER_ROOT/versions/0.5.0"
assert_file_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"
assert_file_exists "$LAUNCHER_ROOT/versions/0.5.0/manifest.yaml"
assert_file_contains "$LAUNCHER_ROOT/versions/0.5.0/manifest.yaml" 'version: "0.5.0"'
assert_file_contains "$LAUNCHER_ROOT/versions/0.5.0/manifest.yaml" 'sha256:'
assert_file_exists "$LAUNCHER_ROOT/events.jsonl"
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_installed"'

# Install does NOT flip current
if [ ! -L "$LAUNCHER_ROOT/current" ]; then
    pass "install does not activate"
else
    fail "install does not activate" "current symlink was created"
fi

# Re-install of same version should fail with EX_ALREADY=3
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 3 ] && pass "re-install exits 3 (EX_ALREADY)" || fail "re-install exits 3" "got $rc"

test_summary
