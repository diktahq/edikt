#!/bin/bash
# upgrade --yes: minor version bump → install + activate. Events written.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup upgrade_minor

# Seed current active version 0.5.0.
src="$LAUNCHER_ROOT/_src050"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

# Prepare a "remote" 0.5.1 payload.
src051="$LAUNCHER_ROOT/_src051"
make_payload "$src051" "0.5.1"

test_start "upgrade minor: install + activate newer version"

# Point the API mock at 0.5.1 and the install source at our local payload.
# EDIKT_RELEASE_TAG bypasses GitHub API.
# EDIKT_INSTALL_SOURCE bypasses network fetch.
EDIKT_RELEASE_TAG="v0.5.1" \
EDIKT_INSTALL_SOURCE="$src051" \
EDIKT_INSTALL_INSECURE=1 \
    run_launcher upgrade --yes >/dev/null 2>&1
rc=$?
assert_rc "$rc" "0" "upgrade exits 0"

# versions/0.5.1 exists
assert_dir_exists "$LAUNCHER_ROOT/versions/0.5.1" "0.5.1 installed"

# current → 0.5.1
target=$(readlink "$LAUNCHER_ROOT/current" 2>/dev/null || echo "")
if [ "$target" = "versions/0.5.1" ]; then
    pass "current points to 0.5.1"
else
    fail "current points to 0.5.1" "got: $target"
fi

# lock.yaml active = 0.5.1
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'active: "0.5.1"' \
    "lock.yaml active is 0.5.1"

# Events: version_installed and version_activated
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_installed"' \
    "version_installed event"
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_activated"' \
    "version_activated event"

test_start "upgrade minor: already up to date → exit 0"
EDIKT_RELEASE_TAG="v0.5.1" run_launcher upgrade --yes >/dev/null 2>&1
rc=$?
assert_rc "$rc" "0" "already up to date exits 0"

test_summary
