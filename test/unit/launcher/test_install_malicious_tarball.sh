#!/bin/bash
# Finding #2: a tarball containing a path-traversal entry (../evil.txt,
# absolute path, or an embedded ../ component) must be rejected before
# extraction. Exit code is EX_MALICIOUS=5.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup install_malicious_tarball

# Build a legitimate payload, then craft a tarball that includes an
# additional `../evil.txt` entry alongside the valid files.
src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"

stage="$LAUNCHER_ROOT/_stage"
mkdir -p "$stage/wrapper"
( cd "$src" && tar -cf - . ) | ( cd "$stage/wrapper" && tar -xf - )

# Create an evil file one level up from wrapper/ so tar can record it
# with a `../` prefix.
echo "pwned" > "$stage/evil.txt"

tarball="$LAUNCHER_ROOT/_malicious.tar.gz"
# Build the tarball with a traversal entry. Using relative paths from
# the `wrapper` directory so tar records `../evil.txt`.
( cd "$stage/wrapper" && tar -czf "$tarball" . ../evil.txt )

# Sanity check that the traversal entry is actually present.
if ! tar -tzf "$tarball" | grep -q '\.\./evil\.txt'; then
    fail "tarball crafted" "no ../evil.txt entry in $tarball"
    test_summary
    exit 1
fi

test_start "install rejects path-traversal tarball"

EDIKT_INSTALL_SOURCE="$tarball" run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 5 ] && pass "install exits 5 (EX_MALICIOUS) on traversal entry" \
                || fail "install exits 5" "got $rc"

assert_file_not_exists "$LAUNCHER_ROOT/versions/0.5.0/VERSION"

# And the evil file must not have been written anywhere under EDIKT_ROOT.
if [ -f "$LAUNCHER_ROOT/evil.txt" ] || [ -f "$LAUNCHER_ROOT/../evil.txt" ]; then
    fail "no traversal write" "evil.txt landed outside the extract dir"
else
    pass "no traversal write occurred"
fi

test_summary
