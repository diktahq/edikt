#!/bin/bash
# Finding #1: install without any checksum reference.
#
# Policy: local tarballs are implicitly trusted (user chose the file); if
# a sibling .sha256 exists it is verified opportunistically. Network
# tarballs REFUSE when no sidecar is available, unless EDIKT_INSTALL_INSECURE=1.
#
# This test exercises the local path functionally and asserts the network
# refusal branch by reading the policy strings (the network branch needs
# a live HTTP server, deferred to Phase 12's integration harness).

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_missing_checksum

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
tarball="$LAUNCHER_ROOT/_payload.tar.gz"
tar -czf "$tarball" -C "$src" .

test_start "install without checksum reference"

# 1. Local tarball, no sha256, no sidecar → succeeds (local is trusted).
EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 >/dev/null 2>&1
assert_rc "$?" 0 "local tarball without sidecar installs successfully"

# 2. Local tarball with an EMPTY sidecar file → refuse. The helper must
#    treat an empty reference as an error, not silently pass.
rm -rf "$LAUNCHER_ROOT/versions/0.5.0"
: >"$tarball.sha256"
out=$(EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 2>&1)
assert_rc "$?" 2 "empty sidecar refuses install"
assert_grep 'empty' "$out" "error message mentions empty reference"

# 3. Network-branch refusal policy strings exist in the launcher. Can't
#    trigger the branch without a live HTTP server (Phase 12 work).
assert_grep 'no checksum reference at' "$(cat "$LAUNCHER")" \
    "launcher has sidecar-refusal branch"
assert_grep 'EDIKT_INSTALL_INSECURE' "$(cat "$LAUNCHER")" \
    "launcher honors EDIKT_INSTALL_INSECURE override"

test_summary
