#!/bin/bash
# project-pin warn:
# - 'list' is EXEMPT → no warning even when pin differs from active
# - 'install' is NOT exempt → warning appears on stderr
# - when pinned == active → no warning
# - when no project config → no warning

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup pin_warn

# Seed active version 0.5.1.
src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.1"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.1 >/dev/null 2>&1
run_launcher use 0.5.1 >/dev/null 2>&1

# Create a project that pins 0.5.0 (different from active 0.5.1).
proj="$LAUNCHER_ROOT/_project"
mkdir -p "$proj/.edikt"
cat >"$proj/.edikt/config.yaml" <<'EOF'
# project config
edikt_version: "0.5.0"
stack:
  type: shell
EOF

test_start "pin warn: list is exempt (no warning)"

stderr_list=$(
    cd "$proj" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher list 2>&1 >/dev/null
)
if echo "$stderr_list" | grep -q "pins edikt"; then
    fail "list is exempt from pin warn" "warning appeared: $stderr_list"
else
    pass "list is exempt from pin warn"
fi

test_start "pin warn: install emits warning on stderr when pin differs"

# We'll run 'install' on a non-existent tag to get an early error — what
# matters is that the pin-warn fires on stderr first.
# Use a tag that doesn't exist so install fails fast.
stderr_install=$(
    cd "$proj" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher install 0.5.999 2>&1 >/dev/null
)
if echo "$stderr_install" | grep -q "pins edikt"; then
    pass "pin warn appears on stderr for install"
else
    fail "pin warn appears on stderr for install" "stderr: $stderr_install"
fi

# The warning must reference both pinned and active versions.
if echo "$stderr_install" | grep -q "0.5.0" && echo "$stderr_install" | grep -q "0.5.1"; then
    pass "warning references both pinned (0.5.0) and active (0.5.1)"
else
    fail "warning references both versions" "stderr: $stderr_install"
fi

test_start "pin warn: no warning when pinned == active"

# Update the project config to pin 0.5.1 (same as active).
cat >"$proj/.edikt/config.yaml" <<'EOF'
edikt_version: "0.5.1"
stack:
  type: shell
EOF

stderr_match=$(
    cd "$proj" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher install 0.5.999 2>&1 >/dev/null
)
if echo "$stderr_match" | grep -q "pins edikt"; then
    fail "no warning when pin matches active" "warning appeared: $stderr_match"
else
    pass "no warning when pin matches active"
fi

test_start "pin warn: no warning when no project config (outside project)"

stderr_noproj=$(
    cd "$HOME" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher install 0.5.999 2>&1 >/dev/null
)
if echo "$stderr_noproj" | grep -q "pins edikt"; then
    fail "no warning outside project" "warning appeared: $stderr_noproj"
else
    pass "no warning outside project"
fi

test_summary
