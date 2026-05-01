#!/bin/bash
# use <tag> flips current symlink, writes lock.yaml atomically, emits
# version_activated event, repairs external symlinks.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup use_happy

src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1

test_start "use happy path"
run_launcher use 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "use exits 0" || fail "use exits 0" "got $rc"

# current is a symlink → versions/0.5.0
target=$(readlink "$LAUNCHER_ROOT/current")
[ "$target" = "versions/0.5.0" ] && pass "current points to versions/0.5.0" || fail "current target" "got $target"

# external symlinks resolve through chain
[ -e "$LAUNCHER_ROOT/hooks/session-start.sh" ] && pass "hooks symlink resolves" || fail "hooks symlink"
[ -d "$LAUNCHER_ROOT/templates" ] && pass "templates symlink resolves" || fail "templates symlink"
[ -e "$CLAUDE_HOME/commands/edikt/context.md" ] && pass "claude commands symlink resolves" || fail "claude commands symlink"

# lock.yaml has active and history
assert_file_exists "$LAUNCHER_ROOT/lock.yaml"
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'active: "0.5.0"'
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'installed_via: "launcher"'
assert_file_contains "$LAUNCHER_ROOT/lock.yaml" 'history:'

# version_activated event
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_activated"'

# version subcommand reads from current/VERSION
v=$(run_launcher version)
[ "$v" = "0.5.0" ] && pass "version returns 0.5.0" || fail "version" "got $v"

test_summary
