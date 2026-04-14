#!/bin/bash
# Install with no EDIKT_INSTALL_SOURCE and a fake tag attempts to fetch
# from GitHub; we point at a bad URL via a missing local source path so
# the launcher hits the EX_NETWORK code path without real DNS.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_network_fail

test_start "install fails with EX_NETWORK on missing source"
EDIKT_INSTALL_SOURCE="/nonexistent/path/that/does/not/exist" \
    run_launcher install never-exists-tag >/dev/null 2>&1
rc=$?
[ "$rc" -eq 1 ] && pass "install exits 1 (EX_NETWORK) on bad source" || fail "install exits 1" "got $rc"
assert_file_not_exists "$LAUNCHER_ROOT/versions/never-exists-tag/VERSION"

test_summary
