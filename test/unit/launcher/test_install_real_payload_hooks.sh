#!/bin/bash
# Smoke test: install a REAL payload (not stubs) and verify that hooks are
# executable and respond correctly to baseline inputs after activation.
#
# Gap covered: the happy-path installer tests use make_payload() which stubs
# hooks with `printf '#!/bin/sh\necho hi'`. A packaging error that leaves a
# hook non-executable or with a broken shebang passes the stub tests but
# would silently break in production. This test installs the actual hook
# scripts from templates/hooks/ and verifies they work end-to-end.
#
# Does NOT require ANTHROPIC_API_KEY or claude CLI auth. Hooks are bash
# scripts that never call external APIs.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

# ─── Helpers ─────────────────────────────────────────────────────────────────

# Build a payload using REAL hooks from $PROJECT_ROOT/templates/hooks/.
make_real_payload() {
    local dest="$1"
    local version="${2:-0.5.0}"
    local hooks_src="$PROJECT_ROOT/templates/hooks"

    rm -rf "$dest"
    mkdir -p "$dest/commands/edikt" "$dest/templates" "$dest/agents"
    printf '%s\n' "$version" > "$dest/VERSION"
    printf '# changelog\n' > "$dest/CHANGELOG.md"
    printf '# context\n' > "$dest/commands/edikt/context.md"

    # Copy real hook scripts — this is the key difference from make_payload().
    if [ ! -d "$hooks_src" ]; then
        echo "make_real_payload: $hooks_src not found" >&2
        return 1
    fi
    cp -R "$hooks_src" "$dest/hooks"
    chmod +x "$dest/hooks"/*.sh 2>/dev/null || true
}

# Write a minimal .edikt/config.yaml inside a project directory so hooks
# that check for config don't treat it as a non-edikt project.
make_project() {
    local dir="$1"
    mkdir -p "$dir/.edikt"
    cat > "$dir/.edikt/config.yaml" <<'YAML'
edikt_version: "0.5.0"
base: docs
stack: []
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
YAML
}

# ─── Test setup ──────────────────────────────────────────────────────────────

launcher_setup install_real_payload_hooks

src="$LAUNCHER_ROOT/_src"
make_real_payload "$src" "0.5.0"

# ─── 1. Install + activate ───────────────────────────────────────────────────

test_start "install real payload exits 0"
EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "install exits 0" || fail "install exits 0" "got rc=$rc"

test_start "activate real payload"
run_launcher use 0.5.0 >/dev/null 2>&1
rc=$?
[ "$rc" -eq 0 ] && pass "use 0.5.0 exits 0" || fail "use 0.5.0 exits 0" "got rc=$rc"

hooks_dir="$LAUNCHER_ROOT/versions/0.5.0/hooks"

# ─── 2. Hooks are present and executable ─────────────────────────────────────

test_start "hooks directory exists after install"
assert_dir_exists "$hooks_dir"

for hook in session-start.sh stop-hook.sh post-compact.sh pre-tool-use.sh post-tool-use.sh; do
    test_start "hook $hook is present"
    if [ -f "$hooks_dir/$hook" ]; then
        pass "hook present: $hook"
    else
        fail "hook present: $hook" "not found in $hooks_dir"
    fi

    test_start "hook $hook is executable"
    if [ -x "$hooks_dir/$hook" ]; then
        pass "hook executable: $hook"
    else
        fail "hook executable: $hook" "chmod +x was not applied"
    fi
done

# ─── 3. session-start.sh: no-edikt project exits 0, no output ───────────────
#
# When invoked in a directory with no .edikt/config.yaml, session-start
# MUST exit 0 silently. This is the "don't break non-edikt projects"
# guarantee (ADR-003, INV-001 opt-in invariant).

test_start "session-start.sh exits 0 in non-edikt dir"
no_edikt_dir="$LAUNCHER_ROOT/no-edikt-project"
mkdir -p "$no_edikt_dir"
payload='{"hook_event_name":"SessionStart","cwd":"'"$no_edikt_dir"'","session_id":"test-001"}'

out=$(echo "$payload" | bash "$hooks_dir/session-start.sh" 2>/dev/null)
rc=$?
[ "$rc" -eq 0 ] && pass "session-start exits 0 (non-edikt)" || \
    fail "session-start exits 0 (non-edikt)" "got rc=$rc"

# ─── 4. stop-hook.sh: clean stop, no crash ───────────────────────────────────
#
# A plain refactor message triggers no signal keywords. Hook MUST exit 0
# and not crash on a well-formed payload.

test_start "stop-hook.sh exits 0 on clean stop payload"
project_dir="$LAUNCHER_ROOT/edikt-project"
make_project "$project_dir"
stop_payload='{
  "hook_event_name": "Stop",
  "stop_hook_active": false,
  "last_assistant_message": "Refactored helper function to reduce duplication.",
  "cwd": "'"$project_dir"'"
}'

out=$(echo "$stop_payload" | bash "$hooks_dir/stop-hook.sh" 2>/dev/null)
rc=$?
[ "$rc" -eq 0 ] && pass "stop-hook exits 0 (clean stop)" || \
    fail "stop-hook exits 0 (clean stop)" "got rc=$rc"

# ─── 5. post-tool-use.sh: exits 0 on unknown extension (no formatter crash) ──
#
# Regression anchor: hook MUST NOT crash when asked about an unknown
# extension. It should pass through silently.

test_start "post-tool-use.sh exits 0 on unknown extension"
post_payload='{
  "hook_event_name": "PostToolUse",
  "tool_name": "Write",
  "tool_input": {
    "file_path": "notes/meeting.xyz",
    "content": "free-form text"
  },
  "tool_response": {"success": true},
  "cwd": "'"$project_dir"'"
}'

echo "$post_payload" | bash "$hooks_dir/post-tool-use.sh" 2>/dev/null
rc=$?
[ "$rc" -eq 0 ] && pass "post-tool-use exits 0 (unknown ext)" || \
    fail "post-tool-use exits 0 (unknown ext)" "got rc=$rc"

test_summary
