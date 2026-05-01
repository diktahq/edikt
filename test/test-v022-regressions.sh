#!/bin/bash
# Test: v0.2.2 bug regressions
# Guards against:
#   1. install.sh: ((BACKUP_COUNT++)) under set -e silently killed the
#      upgrade path from v0.1.x, leaving users with both old flat files
#      and no new namespaced files.
#   2. /edikt:upgrade not rewriting v0.1.x command references in CLAUDE.md
#      and generated rule packs after the namespacing refactor.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Regression 1: install.sh has no (VAR++) under set -e
# ============================================================
# Postfix ((VAR++)) returns the pre-increment value. When VAR=0, the
# expression exits 1, and `set -e` kills the script. Use $((VAR + 1)).

INSTALL_SH="$PROJECT_ROOT/install.sh"

if grep -nE '\(\([a-zA-Z_]+\+\+\)\)|\(\([a-zA-Z_]+\-\-\)\)' "$INSTALL_SH" >/dev/null 2>&1; then
    MATCHES=$(grep -nE '\(\([a-zA-Z_]+\+\+\)\)|\(\([a-zA-Z_]+\-\-\)\)' "$INSTALL_SH")
    fail "install.sh has no (VAR++) under set -e" "$MATCHES"
else
    pass "install.sh has no (VAR++) under set -e"
fi

# install.sh BACKUP_COUNT assertion removed in v0.5.0 Phase 5 hardening — the
# bootstrap delegates to bin/edikt; coverage now lives under
# test/unit/launcher/ and test/integration/install/.

# ============================================================
# Regression 2: upgrade.md documents the v0.1.x → v0.2.x command mapping
# ============================================================

UPGRADE_MD="$PROJECT_ROOT/commands/upgrade.md"

assert_file_contains "$UPGRADE_MD" 'Command reference migration' \
    "upgrade.md has a Command reference migration section"

# All 15 old → new mappings must be present
_assert_mapping() {
    local old="$1" new="$2"
    if grep -F "\`$old\`" "$UPGRADE_MD" | grep -qF "\`$new\`"; then
        pass "upgrade.md maps $old → $new"
    else
        fail "upgrade.md maps $old → $new" \
            "Mapping row not found in command migration table"
    fi
}

_assert_mapping "/edikt:adr"             "/edikt:adr:new"
_assert_mapping "/edikt:invariant"       "/edikt:invariant:new"
_assert_mapping "/edikt:compile"         "/edikt:gov:compile"
_assert_mapping "/edikt:review-governance" "/edikt:gov:review"
_assert_mapping "/edikt:rules-update"    "/edikt:gov:rules-update"
_assert_mapping "/edikt:sync"            "/edikt:gov:sync"
_assert_mapping "/edikt:prd"             "/edikt:sdlc:prd"
_assert_mapping "/edikt:spec"            "/edikt:sdlc:spec"
_assert_mapping "/edikt:spec-artifacts"  "/edikt:sdlc:artifacts"
_assert_mapping "/edikt:plan"            "/edikt:sdlc:plan"
_assert_mapping "/edikt:review"          "/edikt:sdlc:review"
_assert_mapping "/edikt:drift"           "/edikt:sdlc:drift"
_assert_mapping "/edikt:audit"           "/edikt:sdlc:audit"
_assert_mapping "/edikt:docs"            "/edikt:docs:review"
_assert_mapping "/edikt:intake"          "/edikt:docs:intake"

# ============================================================
# Regression 2b: upgrade.md documents safety rules for the string replace
# ============================================================
# The instructions must tell Claude to be idempotent (not replace
# /edikt:adr when it's followed by :), target only edikt-managed files,
# and use Edit with context.

assert_file_contains "$UPGRADE_MD" 'Idempotency is critical' \
    "upgrade.md warns about idempotency"

assert_file_contains "$UPGRADE_MD" 'already followed by' \
    "upgrade.md documents the 'already followed by :' safety rule"

assert_file_contains "$UPGRADE_MD" 'CLAUDE.md managed block' \
    "upgrade.md scopes migration to CLAUDE.md managed block"

assert_file_contains "$UPGRADE_MD" 'edikt:generated' \
    "upgrade.md scopes rule pack migration to edikt:generated files"

assert_file_contains "$UPGRADE_MD" 'never touch user content' \
    "upgrade.md explicitly bounds migration to edikt-owned files"

# ============================================================
# Regression 2c: the CLAUDE.md template itself doesn't still reference
# old flat commands (otherwise new projects would inherit the bug)
# ============================================================

CLAUDE_TMPL="$PROJECT_ROOT/templates/CLAUDE.md.tmpl"

# Find any backtick-wrapped old command NOT followed by :
STALE_REFS=$(grep -nE '\`/edikt:(adr|invariant|compile|review-governance|rules-update|sync|prd|spec|spec-artifacts|plan|review|drift|audit|docs|intake)\`' "$CLAUDE_TMPL" 2>/dev/null || true)

if [ -z "$STALE_REFS" ]; then
    pass "CLAUDE.md template uses only namespaced command references"
else
    fail "CLAUDE.md template uses only namespaced command references" "$STALE_REFS"
fi

test_summary
