#!/bin/bash
# Dry-run: empty sandbox, --dry-run --global → exit 0, would-run commands
# printed, zero disk mutation. The sandbox tree sha256 is identical before
# and after (the --ref env override short-circuits the GitHub API call so
# the only potentially-mutating network read is also skipped).

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup dry_run
trap install_teardown EXIT

test_start "install.sh --dry-run --global"

# Snapshot the sandbox tree BEFORE.
before=$(tree_sha256 "$TEST_HOME")

# Payload must exist so the (non-executed) would-run line can reference it,
# but install.sh in dry-run never reads the payload itself.
PAYLOAD="$TEST_HOME/_payload"
make_payload "$PAYLOAD" "0.5.0"

# Re-snapshot now that the payload is present — this becomes the reference
# baseline for the no-mutation assertion (the payload is outside the
# EDIKT_ROOT tree we care about).
baseline=$(tree_sha256 "$TEST_HOME")

# Write the log OUTSIDE TEST_HOME so the tree-sha256 assertion measures
# only install.sh's disk impact, not the test harness's own logging.
OUT_LOG="$(mktemp -t edikt-dryrun-XXXXXX.log)"
trap 'install_teardown; rm -f "$OUT_LOG"' EXIT

EDIKT_RELEASE_TAG="v0.5.0" \
EDIKT_INSTALL_SOURCE="$PAYLOAD" \
  run_install --dry-run --global --ref v0.5.0 >"$OUT_LOG" 2>&1
rc=$?

[ "$rc" -eq 0 ] && pass "dry-run exits 0" || fail "dry-run exits 0" "got $rc"

# would-run commands are in the output.
if grep -qF "would-run:" "$OUT_LOG"; then
  pass "stdout contains would-run commands"
else
  fail "stdout contains would-run commands" "no would-run lines"
  cat "$OUT_LOG"
fi

# Expect would-run entries for launcher install, use, and rc append.
for needle in \
  "bin/edikt" \
  "install v0.5.0" \
  "use v0.5.0"; do
  if grep -qF "$needle" "$OUT_LOG"; then
    pass "mentions '$needle'"
  else
    fail "mentions '$needle'" "not found in dry-run output"
  fi
done

# Zero disk mutation: sandbox tree sha must match the baseline taken
# after payload creation.
after=$(tree_sha256 "$TEST_HOME")
if [ "$baseline" = "$after" ]; then
  pass "sandbox tree unchanged"
else
  fail "sandbox tree unchanged" "baseline=$baseline after=$after"
fi

# EDIKT_ROOT should not exist at all after dry-run.
if [ ! -e "$TEST_EDIKT_ROOT" ]; then
  pass "EDIKT_ROOT not created"
else
  fail "EDIKT_ROOT not created" "$TEST_EDIKT_ROOT was created"
fi

test_summary
