#!/bin/bash
# After install, every file under templates/hooks/ on disk MUST be exactly
# mode 0755. Catches a release tarball that was packed from a working tree
# where git modes were correct but on-disk modes got corrupted by a
# packaging step (umask drift, archive tool quirks).
#
# Coverage MUST match test/test-hook-modes.sh (the git-index gate). If you
# change the file selection rule here, change it there too — drift between
# the two gates is exactly the failure mode this Phase exists to prevent.
#
# Assertion targets:
# 1) The active runtime hook directory ($EDIKT_ROOT/hooks/, the symlink
#    settings.json actually invokes through). This is where the bug bites
#    the user — checking the source templates/hooks/ would test the wrong
#    surface.
# 2) The payload's templates/hooks/ as a secondary correctness check, since
#    the runtime symlink resolves through it.
#
# (ref: PLAN-v0.5.0-stability Phase 21)
#
# Entry point: invoke via test/run.sh — install_setup refuses to run
# outside the test/run.sh sandbox.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup hookmodes
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

test_start "templates/hooks/* are mode 0755 after install (runtime + payload paths)"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out.log" 2>&1
rc=$?

if [ "$rc" -ne 0 ]; then
  echo "--- install.sh output ---"
  cat "$TEST_HOME/out.log"
  echo "-------------------------"
  fail "install.sh exits 0" "got $rc — cannot check post-install hook modes"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "install.sh exits 0"

# Portable octal-mode read: GNU stat (-c %a) and BSD stat (-f %Lp) both
# return permission bits as octal. Try GNU first, fall back to BSD. Empty
# output (broken symlink, ENOENT, ACL denial) returns the sentinel
# STAT_FAILED so the failure message is diagnostic instead of saying "is 0".
get_mode() {
  _m=""
  if stat -c '%a' "$1" >/dev/null 2>&1; then
    _m=$(stat -c '%a' "$1" 2>/dev/null)
  else
    _m=$(stat -f '%Lp' "$1" 2>/dev/null)
  fi
  if [ -z "$_m" ]; then
    printf 'STAT_FAILED'
  else
    printf '%s' "$_m"
  fi
}

# Check both surfaces. The runtime path is what hooks actually fire from;
# the payload path is the source-of-truth the runtime symlink resolves to.
# A drift between them would indicate a launcher bug (ensure_external_symlinks
# pointing somewhere unexpected) — surface that explicitly.
PAYLOAD_HOOKS="$TEST_EDIKT_ROOT/current/templates/hooks"
RUNTIME_HOOKS="$TEST_EDIKT_ROOT/hooks"

if [ ! -d "$PAYLOAD_HOOKS" ]; then
  fail "payload templates/hooks/ exists post-install" "missing $PAYLOAD_HOOKS"
  test_summary
  exit "$FAIL_COUNT"
fi
if [ ! -d "$RUNTIME_HOOKS" ]; then
  fail "runtime hooks/ symlink exists post-install" \
       "missing $RUNTIME_HOOKS — ensure_external_symlinks did not run or failed"
  test_summary
  exit "$FAIL_COUNT"
fi
pass "payload + runtime hook dirs exist"

check_dir_modes() {
  _label="$1"
  _dir="$2"
  _total=0
  _wrong=0
  # Match test-hook-modes.sh's selection rule: every file under templates/hooks/.
  # NUL-delim find handles future paths with spaces; -type f follows the
  # symlinked runtime dir into the payload (intentional — runtime hooks live
  # behind a directory symlink) but `! -type l` rejects any individual file
  # that's a symlink, since hook files themselves should be regular files.
  while IFS= read -r -d '' hook; do
    _total=$((_total + 1))
    mode=$(get_mode "$hook")
    if [ "$mode" = "755" ]; then
      pass "[$_label] mode 0755: $(basename "$hook")"
    else
      _wrong=$((_wrong + 1))
      fail "[$_label] mode WRONG: $(basename "$hook") is 0$mode (expected exactly 0755)" \
           "find: $hook"
    fi
  done < <(find -L "$_dir" -type f ! -type l -print0 2>/dev/null | LC_ALL=C sort -z)
  if [ "$_total" -eq 0 ]; then
    fail "[$_label] found at least one hook file" "directory $_dir is empty"
  fi
}

check_dir_modes payload "$PAYLOAD_HOOKS"
check_dir_modes runtime "$RUNTIME_HOOKS"

test_summary
