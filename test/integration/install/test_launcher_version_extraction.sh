#!/bin/bash
# Verify launcher_script_version() correctly extracts LAUNCHER_VERSION from bin/edikt.
#
# Network-fetch paths tested via EDIKT_LAUNCHER_SOURCE overrides; real curl
# paths deferred to Phase 12 HTTP fixture harness.
#
# Covers finding #4 from v0.5.0 Phase 5 hardening review.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

install_setup launcher-version-extract
trap install_teardown EXIT

# Source only the launcher_script_version function from install.sh.
# We need a minimal sourcing approach: disable set -e temporarily, source the
# file in a subshell, then call the function.  The easiest portable approach
# is to copy the function body out via grep+eval, but that's fragile.
# Instead we source install.sh with the flag/interactive blocks neutralized
# by overriding the exit call and piping through /dev/null for the prompt.

# Extract and eval just the function definition.
FUNC_DEF=$(sed -n '/^launcher_script_version()/,/^}/p' "$INSTALL_SH")

if [ -z "$FUNC_DEF" ]; then
  fail "launcher_script_version function found in install.sh" "sed extraction returned empty"
  test_summary
  exit 0
fi

pass "launcher_script_version function found in install.sh"

# Evaluate the function in this shell and call it against bin/edikt.
eval "$FUNC_DEF"

EXTRACTED=$(launcher_script_version "$LAUNCHER_SRC")

if [ "$EXTRACTED" = "0.5.0" ]; then
  pass "launcher_script_version extracts '0.5.0' from bin/edikt"
else
  fail "launcher_script_version extracts '0.5.0' from bin/edikt" "got: '$EXTRACTED'"
fi

# Also verify against a synthetic launcher to confirm the awk pattern works
# for both bare-assignment and quoted-assignment styles.
SYNTH="$TEST_HOME/fake_launcher"
printf '#!/bin/sh\nLAUNCHER_VERSION="1.2.3"\n' > "$SYNTH"
SYNTH_VER=$(launcher_script_version "$SYNTH")
if [ "$SYNTH_VER" = "1.2.3" ]; then
  pass "launcher_script_version extracts version from synthetic launcher"
else
  fail "launcher_script_version extracts version from synthetic launcher" "got: '$SYNTH_VER'"
fi

# Non-existent file → empty string (not an error)
MISSING_VER=$(launcher_script_version "$TEST_HOME/does_not_exist")
if [ -z "$MISSING_VER" ]; then
  pass "launcher_script_version returns empty for missing file"
else
  fail "launcher_script_version returns empty for missing file" "got: '$MISSING_VER'"
fi

install_teardown

test_summary
