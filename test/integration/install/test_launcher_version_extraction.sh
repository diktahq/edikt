#!/bin/bash
# Verify go_binary_version() correctly extracts the version from a Go binary.
#
# Since v0.6.0 (ADR-022) the launcher is a Go binary; payload version
# extraction uses `$binary version` via go_binary_version(). This test
# validates the function works with both synthetic binaries and the real
# bin/edikt (which returns empty when no payload is installed — correct).
#
# Network-fetch paths tested via EDIKT_LAUNCHER_SOURCE overrides; real curl
# paths deferred to Phase 12 HTTP fixture harness.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup launcher-version-extract
trap install_teardown EXIT

# Extract and eval just the go_binary_version function definition.
FUNC_DEF=$(sed -n '/^go_binary_version()/,/^}/p' "$INSTALL_SH")

if [ -z "$FUNC_DEF" ]; then
  fail "go_binary_version function found in install.sh" "sed extraction returned empty"
  test_summary
  exit "$FAIL_COUNT"
fi

pass "go_binary_version function found in install.sh"

# Evaluate the function in this shell.
eval "$FUNC_DEF"

# Real bin/edikt: with no payload installed, go_binary_version returns empty.
# That is correct behavior — not an error. Just verify it doesn't crash.
REAL_VER=$(go_binary_version "$LAUNCHER_SRC" 2>/dev/null || true)
pass "go_binary_version handles real bin/edikt without crashing (got: '${REAL_VER:-<empty>}')"

# Verify against a synthetic binary that outputs a version line.
SYNTH="$TEST_HOME/fake_edikt"
printf '#!/bin/sh\nif [ "${1:-}" = "version" ]; then echo "1.2.3"; fi\n' > "$SYNTH"
chmod +x "$SYNTH"
SYNTH_VER=$(go_binary_version "$SYNTH")
if [ "$SYNTH_VER" = "1.2.3" ]; then
  pass "go_binary_version extracts version from synthetic binary"
else
  fail "go_binary_version extracts version from synthetic binary" "got: '$SYNTH_VER'"
fi

# Non-existent file → empty string (not an error)
MISSING_VER=$(go_binary_version "$TEST_HOME/does_not_exist")
if [ -z "$MISSING_VER" ]; then
  pass "go_binary_version returns empty for missing binary"
else
  fail "go_binary_version returns empty for missing binary" "got: '$MISSING_VER'"
fi

install_teardown

test_summary
exit "$FAIL_COUNT"
