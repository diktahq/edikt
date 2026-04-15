#!/bin/bash
# Test: Security hardening of shell scripts
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

echo ""

HOOKS_DIR="$PROJECT_ROOT/templates/hooks"

# ============================================================
# JSON injection prevention — no sed-based JSON escaping
# ============================================================

# All hooks that output JSON must use python3 json.dumps, not sed
for hook in subagent-stop.sh post-compact.sh user-prompt-submit.sh stop-hook.sh; do
    HOOK_FILE="$HOOKS_DIR/$hook"
    [ ! -f "$HOOK_FILE" ] && continue

    if grep -q 'json.dumps\|json.loads' "$HOOK_FILE"; then
        pass "$hook: uses python3 json.dumps for JSON output"
    else
        # Some hooks may not output JSON — only fail if they also have printf + systemMessage
        if grep -q 'systemMessage' "$HOOK_FILE"; then
            fail "$hook: outputs JSON but doesn't use python3 json.dumps"
        else
            pass "$hook: does not output JSON (no systemMessage)"
        fi
    fi

    # Verify no sed-based JSON escaping pattern remains
    if grep -qE "sed.*s/\"/\\\\\\\\\"" "$HOOK_FILE" 2>/dev/null; then
        fail "$hook: still has sed-based JSON escaping (vulnerable to injection)"
    else
        pass "$hook: no sed-based JSON escaping"
    fi
done

# event-log.sh must use python3 for JSON construction
EVENT_LOG="$HOOKS_DIR/event-log.sh"
if grep -q 'json.dumps\|json.loads' "$EVENT_LOG"; then
    pass "event-log.sh: uses python3 for JSON construction"
else
    fail "event-log.sh: does not use python3 for JSON construction"
fi

if grep -qE 'printf.*"type".*"at".*"by"' "$EVENT_LOG" 2>/dev/null; then
    fail "event-log.sh: still has printf-based JSON construction"
else
    pass "event-log.sh: no printf-based JSON construction"
fi

# ============================================================
# Regex injection prevention — grep -F for LLM-derived content
# ============================================================

STOP_HOOK="$HOOKS_DIR/stop-hook.sh"
# The stop hook greps for terms extracted from LLM output — must use -F (fixed string)
if grep -q 'grep.*-.*F' "$STOP_HOOK" 2>/dev/null; then
    pass "stop-hook.sh: uses grep -F for fixed-string matching on LLM content"
else
    fail "stop-hook.sh: missing grep -F for LLM-derived content matching"
fi

# ============================================================
# install.sh security
# ============================================================

INSTALL="$PROJECT_ROOT/install.sh"

# Must have set -euo pipefail
if head -5 "$INSTALL" | grep -q 'set -euo pipefail'; then
    pass "install.sh: has set -euo pipefail"
else
    fail "install.sh: missing set -euo pipefail"
fi

# Must have umask
if grep -q 'umask 0022' "$INSTALL"; then
    pass "install.sh: sets umask 0022 for safe file permissions"
else
    fail "install.sh: missing umask 0022"
fi

# Must not contain hardcoded secrets
if grep -qiE 'api_key|secret|password|token' "$INSTALL" 2>/dev/null | grep -v '#' | grep -v 'echo'; then
    fail "install.sh: contains potential hardcoded secret"
else
    pass "install.sh: no hardcoded secrets"
fi

# Installer safety (v0.2.0)
assert_file_contains "$INSTALL" "dry-run" "install.sh supports --dry-run flag"
assert_file_contains "$INSTALL" "DRY_RUN" "install.sh has DRY_RUN variable"

# v0.5.0 bootstrap delegates backup/custom-marker behavior to bin/edikt —
# the pre-v0.5.0 install_file/BACKUP_DIR machinery no longer lives in
# install.sh. Coverage for versioned-layout backups is under
# test/unit/launcher/ and test/integration/install/.
if is_v050_bootstrap_installer; then
    skip_obsolete_installer_assert "install.sh has install_file backup function"
    skip_obsolete_installer_assert "install.sh creates backup directory"
    skip_obsolete_installer_assert "install.sh stores backups in backups/ dir"
    skip_obsolete_installer_assert "install.sh respects custom markers"
    skip_obsolete_installer_assert "install.sh detects existing installs"
    skip_obsolete_installer_assert "install.sh reports backup count"
else
    assert_file_contains "$INSTALL" "install_file" "install.sh has install_file backup function"
    assert_file_contains "$INSTALL" "BACKUP_DIR" "install.sh creates backup directory"
    assert_file_contains "$INSTALL" "backups/" "install.sh stores backups in backups/ dir"
    assert_file_contains "$INSTALL" "edikt:custom" "install.sh respects custom markers"
    assert_file_contains "$INSTALL" "Existing edikt installation detected" "install.sh detects existing installs"
    assert_file_contains "$INSTALL" "Backed up" "install.sh reports backup count"
fi
assert_file_contains "$INSTALL" "Dry run complete" "install.sh shows dry run summary"

# ============================================================
# Experiment setup.sh — rm -rf guard
# ============================================================

for setup in "$PROJECT_ROOT"/experiments/*/setup.sh; do
    [ ! -f "$setup" ] && continue
    NAME=$(basename "$(dirname "$setup")")

    if grep -q 'rm -rf' "$setup"; then
        # Must have a /tmp/ guard before rm -rf
        if grep -B5 'rm -rf' "$setup" | grep -q '/tmp/'; then
            pass "$NAME/setup.sh: rm -rf has /tmp/ path guard"
        else
            fail "$NAME/setup.sh: rm -rf without /tmp/ path guard"
        fi
    else
        pass "$NAME/setup.sh: no rm -rf (safe)"
    fi
done

# ============================================================
# Hook scripts — bash -n syntax check
# ============================================================

for hook in "$HOOKS_DIR"/*.sh; do
    NAME=$(basename "$hook")
    if bash -n "$hook" 2>/dev/null; then
        pass "$NAME: passes bash -n syntax check"
    else
        fail "$NAME: fails bash -n syntax check"
    fi
done

# ============================================================
# No hooks write to hardcoded absolute paths
# ============================================================

for hook in "$HOOKS_DIR"/*.sh; do
    NAME=$(basename "$hook")
    # Check for writes (> or >>) to hardcoded absolute paths like /etc/, /usr/, /var/
    # Exclude: $HOME/.edikt/, /tmp/, /dev/null, stderr redirects (2>), variable paths ($VAR/), tool commands (gofmt -w, prettier --write)
    if grep -E '^[^#]*(>>|[^2]>)\s*/(etc|usr|var|root|opt)/' "$hook" 2>/dev/null | grep -q '.'; then
        fail "$NAME: writes to system path outside safe locations"
    else
        pass "$NAME: no writes to system paths"
    fi
done

# ============================================================
# SubagentStop JSON output — functional test
# ============================================================

SUBAGENT_HOOK="$HOOKS_DIR/subagent-stop.sh"

# Test that gate block output is valid JSON even with special characters
TESTDIR=$(mktemp -d)
mkdir -p "$TESTDIR/.edikt"
cat > "$TESTDIR/.edikt/config.yaml" << 'YAML'
base: docs
gates:
  - security
YAML

# Input with characters that would break sed-based escaping
TRICKY_INPUT='As Staff Security Engineer, 🔴 critical: found "hardcoded" key with path\to\file and $pecial chars'
OUTPUT=$(cd "$TESTDIR" && echo "$TRICKY_INPUT" | bash "$SUBAGENT_HOOK" 2>/dev/null)

if echo "$OUTPUT" | python3 -c "import json,sys; json.load(sys.stdin)" 2>/dev/null; then
    pass "SubagentStop: gate block output is valid JSON (with special chars in input)"
else
    fail "SubagentStop: gate block output is malformed JSON with special chars" "Got: $OUTPUT"
fi
rm -rf "$TESTDIR"

# ============================================================
# PostCompact JSON output — functional test
# ============================================================

POSTCOMPACT_HOOK="$HOOKS_DIR/post-compact.sh"

TESTDIR2=$(mktemp -d)
mkdir -p "$TESTDIR2/.edikt" "$TESTDIR2/docs/plans" "$TESTDIR2/docs/architecture/invariants"
echo "base: docs" > "$TESTDIR2/.edikt/config.yaml"
cat > "$TESTDIR2/docs/plans/PLAN-001-test.md" << 'PLAN'
# Plan: Test "quotes" and \backslashes

## Progress

| Phase | Theme | Status | Updated |
|-------|-------|--------|---------|
| 1 | Build with "special" chars | in progress | 2026-03-20 |
PLAN
echo '# INV-001 — No "compiled" code with \paths' > "$TESTDIR2/docs/architecture/invariants/INV-001-test.md"

OUTPUT=$(cd "$TESTDIR2" && bash "$POSTCOMPACT_HOOK" 2>/dev/null)
if echo "$OUTPUT" | python3 -c "import json,sys; json.load(sys.stdin)" 2>/dev/null; then
    pass "PostCompact: output is valid JSON (with special chars in plan/invariant names)"
else
    fail "PostCompact: malformed JSON with special chars" "Got: $OUTPUT"
fi
rm -rf "$TESTDIR2"

# ============================================================
# UserPromptSubmit JSON output — functional test
# ============================================================

UPS_HOOK="$HOOKS_DIR/user-prompt-submit.sh"

TESTDIR3=$(mktemp -d)
mkdir -p "$TESTDIR3/.edikt" "$TESTDIR3/docs/plans"
echo "base: docs" > "$TESTDIR3/.edikt/config.yaml"
cat > "$TESTDIR3/docs/plans/PLAN-001-tricky.md" << 'PLAN'
# Plan: Handle "edge cases" for \n and $vars

## Progress

| Phase | Theme | Status | Updated |
|-------|-------|--------|---------|
| 1 | "Quoted phase" with \slashes | in progress | 2026-03-20 |
PLAN

OUTPUT=$(cd "$TESTDIR3" && bash "$UPS_HOOK" 2>/dev/null)
if [ -z "$OUTPUT" ]; then
    pass "UserPromptSubmit: no output (may not have matched phase)"
elif echo "$OUTPUT" | python3 -c "import json,sys; json.load(sys.stdin)" 2>/dev/null; then
    pass "UserPromptSubmit: output is valid JSON (with special chars)"
else
    fail "UserPromptSubmit: malformed JSON with special chars" "Got: $OUTPUT"
fi
rm -rf "$TESTDIR3"

test_summary
