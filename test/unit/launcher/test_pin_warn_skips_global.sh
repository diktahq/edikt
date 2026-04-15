#!/bin/bash
# pin-warn must NOT fire when the only ancestor .edikt/config.yaml is the
# global edikt config ($EDIKT_ROOT itself). It MUST fire for a real project
# config found higher in the tree.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup pin_warn_skips_global

# For this test EDIKT_ROOT must equal the natural global path (EDIKT_HOME)
# so that find_project_config's global-config guard triggers.
# launcher_setup sets EDIKT_HOME to $HOME/.edikt; override EDIKT_ROOT to match.
EDIKT_ROOT="$EDIKT_HOME"
export EDIKT_ROOT
mkdir -p "$EDIKT_ROOT"

# Seed an active version 0.5.1 directly into EDIKT_ROOT.
src="$EDIKT_ROOT/_src_$$"
make_payload "$src" "0.5.1"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.1 >/dev/null 2>&1
run_launcher use 0.5.1 >/dev/null 2>&1

# Create the global config at $EDIKT_ROOT/config.yaml pinning 0.5.0.
cat >"$EDIKT_ROOT/config.yaml" <<'EOF'
# global edikt config — should never trigger project-pin warn
edikt_version: "0.5.0"
EOF

# ─── Part 1: global config must NOT trigger pin warn ─────────────────────────

test_start "pin warn: global config at EDIKT_ROOT is skipped (no warning)"

# Run from $HOME — find_project_config walks up from $HOME and finds
# $HOME/.edikt/config.yaml, which equals $EDIKT_ROOT/config.yaml.
# The guard introduced in fix #1 must skip it.
stderr_global=$(
    cd "$HOME" && EDIKT_ROOT="$EDIKT_ROOT" run_launcher install 0.5.999 2>&1 >/dev/null
)
if echo "$stderr_global" | grep -q "pins edikt"; then
    fail "global config skipped (no warning)" "warning appeared: $stderr_global"
else
    pass "global config skipped (no warning)"
fi

# ─── Part 2: real project config MUST trigger pin warn ───────────────────────

test_start "pin warn: real project config (non-global) does trigger warning"

# Create a project dir under HOME that is NOT the EDIKT_ROOT dir.
proj="$HOME/myproj_$$"
mkdir -p "$proj/.edikt"
cat >"$proj/.edikt/config.yaml" <<'EOF'
# real project config
edikt_version: "0.5.0"
stack:
  type: shell
EOF

stderr_proj=$(
    cd "$proj" && EDIKT_ROOT="$EDIKT_ROOT" run_launcher install 0.5.999 2>&1 >/dev/null
)
if echo "$stderr_proj" | grep -q "pins edikt"; then
    pass "real project pin triggers warning"
else
    fail "real project pin triggers warning" "no warning in: $stderr_proj"
fi

# Both versions must be referenced in the warning.
if echo "$stderr_proj" | grep -q "0.5.0" && echo "$stderr_proj" | grep -q "0.5.1"; then
    pass "warning references pinned (0.5.0) and active (0.5.1)"
else
    fail "warning references both versions" "stderr: $stderr_proj"
fi

test_summary
