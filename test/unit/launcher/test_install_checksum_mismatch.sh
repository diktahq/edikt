#!/bin/bash
# Install with a wrong checksum must exit 2 and leave nothing behind.
# Covers two paths:
#   1. Explicit EDIKT_INSTALL_SHA256 mismatch.
#   2. Sibling .sha256 sidecar mismatch (local-tarball opportunistic verify,
#      which exercises the same verify_against_sidecar helper used by the
#      network branch).

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_checksum

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
tarball="$LAUNCHER_ROOT/_payload.tar.gz"
tar -czf "$tarball" -C "$src" .
actual_sha=$(sha256sum "$tarball" 2>/dev/null | awk '{print $1}' \
              || shasum -a 256 "$tarball" | awk '{print $1}')

test_start "install rejects checksum mismatch"

# 1. Explicit EDIKT_INSTALL_SHA256 mismatch.
EDIKT_INSTALL_SOURCE="$tarball" EDIKT_INSTALL_SHA256="deadbeef" \
    run_launcher install 0.5.0 >/dev/null 2>&1
assert_rc "$?" 2 "install exits 2 on explicit sha256 mismatch"
assert_file_not_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"

# 2. Sibling .sha256 sidecar mismatch — write a bogus hash to the sidecar
#    and verify the launcher refuses. This exercises verify_against_sidecar
#    functionally (same helper used by the network branch).
printf '%s\n' "ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff" \
    >"$tarball.sha256"
out=$(EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 2>&1)
rc=$?
assert_rc "$rc" 2 "install exits 2 on sidecar sha256 mismatch"
assert_grep 'checksum mismatch' "$out" "error message mentions checksum mismatch"
assert_file_not_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"

# 3. Sibling .sha256 sidecar match — install succeeds. Proves the helper
#    doesn't falsely reject.
printf '%s\n' "$actual_sha" >"$tarball.sha256"
EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 >/dev/null 2>&1
assert_rc "$?" 0 "install succeeds when sidecar sha256 matches"
assert_file_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"

# 4. Sidecar in "<hash>  <filename>" format (standard sha256sum output)
#    is also accepted.
rm -rf "$LAUNCHER_ROOT/versions/0.5.0"
printf '%s  payload.tar.gz\n' "$actual_sha" >"$tarball.sha256"
EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 >/dev/null 2>&1
assert_rc "$?" 0 "install accepts 'hash  filename' sidecar format"

test_summary
