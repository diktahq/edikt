#!/bin/bash
# upgrade-pin inside a project: updates edikt_version: to active version,
# preserves all other bytes, emits version_pinned event.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup upgrade_pin_inside

# Seed an active version 0.5.1.
src="$LAUNCHER_ROOT/_src"
make_payload "$src" "0.5.1"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.1 >/dev/null 2>&1
run_launcher use 0.5.1 >/dev/null 2>&1

# Create a fake project dir with .edikt/config.yaml pinning 0.5.0.
proj="$LAUNCHER_ROOT/_project"
mkdir -p "$proj/.edikt"
cat >"$proj/.edikt/config.yaml" <<'EOF'
# edikt project config
edikt_version: "0.5.0"
stack:
  type: shell
gates:
  quality: true
EOF

test_start "upgrade-pin updates edikt_version"

# Save the exact bytes of every OTHER line for comparison.
before_other=$(grep -v '^edikt_version:' "$proj/.edikt/config.yaml")

# Run upgrade-pin from inside the project dir.
# We use a subshell so the cd is scoped.
( cd "$proj" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher upgrade-pin >/dev/null 2>&1 )
rc=$?
assert_rc "$rc" "0" "upgrade-pin exits 0"

# edikt_version should now be 0.5.1
assert_file_contains "$proj/.edikt/config.yaml" 'edikt_version: "0.5.1"' \
    "edikt_version updated to 0.5.1"

# All other lines preserved byte-for-byte.
after_other=$(grep -v '^edikt_version:' "$proj/.edikt/config.yaml")
if [ "$before_other" = "$after_other" ]; then
    pass "other lines preserved byte-for-byte"
else
    fail "other lines preserved byte-for-byte" \
        "before: $(echo "$before_other" | head -5) / after: $(echo "$after_other" | head -5)"
fi

# version_pinned event.
assert_file_contains "$LAUNCHER_ROOT/events.jsonl" '"event":"version_pinned"' \
    "version_pinned event emitted"

test_start "upgrade-pin: config without edikt_version line appends it"

mkdir -p "$proj/.edikt2"
cat >"$proj/.edikt2/config.yaml" <<'EOF'
# minimal config
stack:
  type: shell
EOF
# Use upgrade-pin with a custom project dir by going there.
( cd "$proj" && \
    EDIKT_ROOT="$LAUNCHER_ROOT" \
    # Override find_project_config by providing a fake .edikt inside proj
    # We need to trick find_project_config to find .edikt2 — instead use a
    # second project subdir.
    true
)

# Simpler: create .edikt/config.yaml without edikt_version in a new dir.
proj2="$LAUNCHER_ROOT/_project2"
mkdir -p "$proj2/.edikt"
cat >"$proj2/.edikt/config.yaml" <<'EOF'
# no edikt_version key
stack:
  type: shell
EOF
( cd "$proj2" && EDIKT_ROOT="$LAUNCHER_ROOT" run_launcher upgrade-pin >/dev/null 2>&1 )
rc=$?
assert_rc "$rc" "0" "upgrade-pin exits 0 when appending"
assert_file_contains "$proj2/.edikt/config.yaml" 'edikt_version: "0.5.1"' \
    "edikt_version appended"

test_summary
