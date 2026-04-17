#!/bin/bash
# upgrade: major-version jump → refuse with install.sh redirect, exit 1,
# no versions/ mutation.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup upgrade_major

# Seed current active version 0.5.0.
src="$LAUNCHER_ROOT/_src050"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

# Snapshot versions/ before the attempt.
versions_before=$(ls -1 "$LAUNCHER_ROOT/versions/" 2>/dev/null | sort)

test_start "upgrade major: refuses and prints install.sh command"

# Mock API to return v1.0.0 (major bump from 0.x).
out=$(EDIKT_RELEASE_TAG="v1.0.0" run_launcher upgrade --yes 2>&1)
rc=$?
assert_rc "$rc" "1" "major upgrade exits 1"

# Must print the install.sh redirect command verbatim, pinned to the
# target release tag per INV-008 (branch-tracking URLs are forbidden).
if echo "$out" | grep -q "curl -fsSL https://github.com/diktahq/edikt/releases/download/v1.0.0/install.sh | bash"; then
    pass "error message contains install.sh command"
else
    fail "error message contains install.sh command" "output: $out"
fi

# versions/ must be unchanged — no 1.0.0 directory created.
versions_after=$(ls -1 "$LAUNCHER_ROOT/versions/" 2>/dev/null | sort)
if [ "$versions_before" = "$versions_after" ]; then
    pass "versions/ not mutated"
else
    fail "versions/ not mutated" "before: $versions_before / after: $versions_after"
fi

test_start "upgrade major: dry-run also refuses"

out2=$(EDIKT_RELEASE_TAG="v1.0.0" run_launcher upgrade --dry-run 2>&1)
rc2=$?
assert_rc "$rc2" "1" "dry-run major upgrade exits 1"

if echo "$out2" | grep -q "install.sh"; then
    pass "dry-run output mentions install.sh"
else
    fail "dry-run output mentions install.sh" "output: $out2"
fi

test_summary
