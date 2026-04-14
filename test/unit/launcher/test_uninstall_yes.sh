#!/bin/bash
# uninstall --yes removes EDIKT_ROOT and the claude commands symlink
# without prompting.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup uninstall_yes

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

[ -L "$CLAUDE_HOME/commands/edikt" ] && pass "claude symlink exists pre-uninstall" || fail "pre-uninstall claude symlink"

test_start "uninstall --yes"
run_launcher uninstall --yes >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "uninstall exits 0" || fail "uninstall exits 0" "got $rc"

[ ! -d "$LAUNCHER_ROOT" ] && pass "EDIKT_ROOT removed" || fail "EDIKT_ROOT removed"
[ ! -e "$CLAUDE_HOME/commands/edikt" ] && pass "claude commands symlink removed" || fail "claude commands symlink removed"

test_summary
