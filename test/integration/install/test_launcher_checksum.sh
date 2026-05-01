#!/bin/bash
# Launcher sidecar checksum verification — install.sh trust-chain tests.
#
# Network-fetch paths tested via EDIKT_LAUNCHER_SOURCE overrides; real curl
# paths deferred to Phase 12 HTTP fixture harness.
#
# Covers finding #1 from v0.5.0 Phase 5 hardening review:
#   - Local launcher + matching .sha256 sidecar → install succeeds.
#   - Local launcher + mismatched .sha256 sidecar → refuses, exits 1, no launcher placed.
#   - EDIKT_LAUNCHER_SHA256=<correct hex> without a sidecar file → succeeds.
#   - EDIKT_LAUNCHER_SHA256=deadbeef mismatched → refuses, exits 1.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

# ─── helpers ────────────────────────────────────────────────────────────────

sha256_of() {
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$1" | awk '{print $1}'
  else
    shasum -a 256 "$1" | awk '{print $1}'
  fi
}

# ─── Test 1: matching .sha256 sidecar → success ─────────────────────────────

install_setup launcher-checksum-ok
trap install_teardown EXIT

PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

GOOD_HASH=$(sha256_of "$LAUNCHER_SRC")
printf '%s  bin/edikt\n' "$GOOD_HASH" > "${LAUNCHER_SRC}.sha256"

test_start "launcher sidecar — matching .sha256 → install succeeds"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
EDIKT_LAUNCHER_SOURCE="$LAUNCHER_SRC" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out1.log" 2>&1
rc=$?

if [ "$rc" -eq 0 ]; then
  pass "install exits 0 with matching sidecar"
else
  echo "--- output ---"; cat "$TEST_HOME/out1.log"; echo "--------------"
  fail "install exits 0 with matching sidecar" "got $rc"
fi

assert_file_exists "$TEST_EDIKT_ROOT/bin/edikt" "launcher placed at EDIKT_ROOT/bin/edikt"

install_teardown
# Cleanup the temporary sidecar we created alongside the real bin/edikt.
rm -f "${LAUNCHER_SRC}.sha256"

# ─── Test 2: mismatched .sha256 sidecar → refuses, launcher NOT placed ──────

install_setup launcher-checksum-mismatch
trap install_teardown EXIT

PAYLOAD2="$TEST_HOME/_payload"
make_payload "$PAYLOAD2" "0.5.0"

printf 'deadbeef0000000000000000000000000000000000000000000000000000dead  bin/edikt\n' \
  > "${LAUNCHER_SRC}.sha256"

test_start "launcher sidecar — mismatched .sha256 → refuses"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD2" \
EDIKT_LAUNCHER_SOURCE="$LAUNCHER_SRC" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out2.log" 2>&1
rc2=$?

if [ "$rc2" -ne 0 ]; then
  pass "install refuses with mismatched sidecar (exit $rc2)"
else
  echo "--- output ---"; cat "$TEST_HOME/out2.log"; echo "--------------"
  fail "install should refuse with mismatched sidecar" "exited 0"
fi

if [ ! -f "$TEST_EDIKT_ROOT/bin/edikt" ]; then
  pass "launcher NOT placed after mismatch"
else
  fail "launcher NOT placed after mismatch" "file exists: $TEST_EDIKT_ROOT/bin/edikt"
fi

if grep -q 'mismatch\|checksum' "$TEST_HOME/out2.log" 2>/dev/null; then
  pass "error message mentions checksum mismatch"
else
  fail "error message mentions checksum mismatch" "got: $(cat "$TEST_HOME/out2.log")"
fi

install_teardown
rm -f "${LAUNCHER_SRC}.sha256"

# ─── Test 3: EDIKT_LAUNCHER_SHA256=<correct hex> → succeeds ─────────────────

install_setup launcher-sha256-env-ok
trap install_teardown EXIT

PAYLOAD3="$TEST_HOME/_payload"
make_payload "$PAYLOAD3" "0.5.0"

CORRECT_HASH=$(sha256_of "$LAUNCHER_SRC")

test_start "EDIKT_LAUNCHER_SHA256 correct hex (no sidecar file) → succeeds"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD3" \
EDIKT_LAUNCHER_SOURCE="$LAUNCHER_SRC" \
EDIKT_LAUNCHER_SHA256="$CORRECT_HASH" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out3.log" 2>&1
rc3=$?

if [ "$rc3" -eq 0 ]; then
  pass "install exits 0 with correct EDIKT_LAUNCHER_SHA256 env"
else
  echo "--- output ---"; cat "$TEST_HOME/out3.log"; echo "--------------"
  fail "install exits 0 with correct EDIKT_LAUNCHER_SHA256 env" "got $rc3"
fi

assert_file_exists "$TEST_EDIKT_ROOT/bin/edikt" "launcher placed with env SHA256 override"

install_teardown

# ─── Test 4: EDIKT_LAUNCHER_SHA256=deadbeef → refuses ───────────────────────

install_setup launcher-sha256-env-bad
trap install_teardown EXIT

PAYLOAD4="$TEST_HOME/_payload"
make_payload "$PAYLOAD4" "0.5.0"

test_start "EDIKT_LAUNCHER_SHA256=deadbeef → refuses"

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD4" \
EDIKT_LAUNCHER_SOURCE="$LAUNCHER_SRC" \
EDIKT_LAUNCHER_SHA256="deadbeef" \
  run_install --global --ref v0.5.0 >"$TEST_HOME/out4.log" 2>&1
rc4=$?

if [ "$rc4" -ne 0 ]; then
  pass "install refuses with wrong EDIKT_LAUNCHER_SHA256 (exit $rc4)"
else
  echo "--- output ---"; cat "$TEST_HOME/out4.log"; echo "--------------"
  fail "install should refuse with wrong EDIKT_LAUNCHER_SHA256" "exited 0"
fi

if [ ! -f "$TEST_EDIKT_ROOT/bin/edikt" ]; then
  pass "launcher NOT placed after env SHA256 mismatch"
else
  fail "launcher NOT placed after env SHA256 mismatch" "file exists: $TEST_EDIKT_ROOT/bin/edikt"
fi

install_teardown

test_summary
