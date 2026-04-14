#!/bin/bash
# Install with EDIKT_INSTALL_SHA256 set to a wrong value must exit 2 and
# leave nothing behind in versions/.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_checksum

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
tarball="$LAUNCHER_ROOT/_payload.tar.gz"
tar -czf "$tarball" -C "$src" .

test_start "install rejects checksum mismatch"
EDIKT_INSTALL_SOURCE="$tarball" EDIKT_INSTALL_SHA256="deadbeef" \
    run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 2 ] && pass "install exits 2 on checksum mismatch" || fail "install exits 2" "got $rc"
assert_file_not_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"

test_summary
