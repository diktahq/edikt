#!/bin/bash
# list + version on empty and populated trees.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup list_version

test_start "list + version on empty tree"
out=$(run_launcher list 2>&1)
echo "$out" | grep -q "no versions installed" && pass "list reports empty tree" || fail "list empty" "$out"

out=$(run_launcher version 2>&1)
echo "$out" | grep -q "no version installed" && pass "version reports empty tree" || fail "version empty" "$out"

# Now install two and check active marker
s1="$LAUNCHER_ROOT/_s1"
s2="$LAUNCHER_ROOT/_s2"
make_payload "$s1" "0.5.0"
make_payload "$s2" "0.5.1"
EDIKT_INSTALL_SOURCE="$s1" run_launcher install 0.5.0 >/dev/null 2>&1
EDIKT_INSTALL_SOURCE="$s2" run_launcher install 0.5.1 >/dev/null 2>&1
run_launcher use 0.5.1 >/dev/null 2>&1

out=$(run_launcher list)
echo "$out" | grep -q "^\* 0.5.1" && pass "list marks active version" || fail "list marks active" "$out"
echo "$out" | grep -q "^  0.5.0" && pass "list shows non-active version" || fail "list non-active" "$out"

# verbose includes install date
out=$(run_launcher list --verbose)
echo "$out" | grep -q "0.5.1" && pass "list --verbose runs" || fail "list --verbose"

test_summary
