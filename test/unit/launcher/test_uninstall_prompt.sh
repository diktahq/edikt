#!/bin/bash
# uninstall without --yes and with no tty must not delete (default = abort).

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup uninstall_prompt

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

test_start "uninstall without --yes prompts and aborts non-interactively"
# Pipe empty stdin → not a tty → reply is empty → aborts.
echo "" | run_launcher uninstall >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "uninstall exits 0 on abort" || fail "uninstall exits 0" "got $rc"
[ -d "$LAUNCHER_ROOT" ] && pass "EDIKT_ROOT preserved on abort" || fail "EDIKT_ROOT preserved"

# Reply 'y' on stdin must proceed.
echo "y" | run_launcher uninstall >/dev/null 2>&1
rc=$?
# stdin is a pipe (not tty) so our launcher won't read it; this becomes
# a no-op — assert behavior matches our "no tty → abort" contract.
[ -d "$LAUNCHER_ROOT" ] && pass "non-tty 'y' input still aborts (safe default)" || fail "non-tty safe abort"

test_summary
