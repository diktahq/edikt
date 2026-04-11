#!/bin/bash
# Test: v0.3.1 team → init consolidation + config command
# Verifies team deprecation, init member onboarding path, and config command.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

INIT_CMD="$PROJECT_ROOT/commands/init.md"
CONFIG_CMD="$PROJECT_ROOT/commands/config.md"
DEPRECATED_TEAM="$PROJECT_ROOT/commands/deprecated/team.md"
CLAUDE_TMPL="$PROJECT_ROOT/templates/CLAUDE.md.tmpl"
INSTALLER="$PROJECT_ROOT/install.sh"

echo ""

# ============================================================
# TEST 1: Team command deprecated
# ============================================================

echo -e "${BOLD}TEST 1: Team command deprecated${NC}"

assert_file_not_exists "$PROJECT_ROOT/commands/team.md" "Flat team.md removed"
assert_file_exists "$DEPRECATED_TEAM" "Deprecated team stub exists"
assert_file_contains "$DEPRECATED_TEAM" "Deprecated" "Deprecated stub says deprecated"
assert_file_contains "$DEPRECATED_TEAM" "edikt:init" "Deprecated stub redirects to init"
assert_file_contains "$DEPRECATED_TEAM" "edikt:config" "Deprecated stub mentions config"

# ============================================================
# TEST 2: Config command exists and is well-formed
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Config command${NC}"

assert_file_exists "$CONFIG_CMD" "commands/config.md exists"
assert_file_contains "$CONFIG_CMD" "name: edikt:config" "Config has correct frontmatter name"
assert_file_contains "$CONFIG_CMD" "get {key}" "Config supports get subcommand"
assert_file_contains "$CONFIG_CMD" "set {key} {value}" "Config supports set subcommand"

# Config has validation rules
assert_file_contains "$CONFIG_CMD" "edikt_version" "Config documents edikt_version (read-only)"
assert_file_contains "$CONFIG_CMD" "NEVER allow setting" "Config protects edikt_version"
assert_file_contains "$CONFIG_CMD" "artifacts.database.default_type" "Config documents database type"
assert_file_contains "$CONFIG_CMD" "features.quality-gates" "Config documents quality gates toggle"
assert_file_contains "$CONFIG_CMD" "artifacts.versions.openapi" "Config documents OpenAPI version"

# Config has Key Reference table
assert_file_contains "$CONFIG_CMD" "Key Reference" "Config has key reference table"
assert_file_contains "$CONFIG_CMD" "paths.decisions" "Key reference includes paths.decisions"
assert_file_contains "$CONFIG_CMD" "agents.custom" "Key reference includes agents.custom"

# ============================================================
# TEST 3: Init member onboarding path
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Init member onboarding${NC}"

# Version gate
assert_file_contains "$INIT_CMD" "edikt version gate" "Init has version gate step"
assert_file_contains "$INIT_CMD" "INSTALLED.*PROJECT" "Init compares installed vs project version"
assert_file_contains "$INIT_CMD" "version mismatch" "Init blocks on version mismatch"

# Member environment checks
assert_file_contains "$INIT_CMD" "Git identity" "Init checks git identity"
assert_file_contains "$INIT_CMD" "Claude Code" "Init checks Claude Code installed"
assert_file_contains "$INIT_CMD" "MCP environment variables" "Init checks MCP env vars"
assert_file_contains "$INIT_CMD" "CLAUDE_CODE_SUBPROCESS_ENV_SCRUB" "Init checks env scrubbing"
assert_file_contains "$INIT_CMD" "pre-push" "Init checks pre-push hook"
assert_file_contains "$INIT_CMD" "managed-settings" "Init detects managed settings"

# MCP env var check is dynamic (reads .mcp.json, not hardcoded)
assert_file_contains "$INIT_CMD" ".mcp.json" "Init reads .mcp.json for env vars"

# Governance gap sync preserved
assert_file_contains "$INIT_CMD" "Governance file gap sync" "Init still syncs governance gaps"

# Shows shared config
assert_file_contains "$INIT_CMD" "Shared config" "Init shows shared config"
assert_file_contains "$INIT_CMD" "git ls-files" "Init uses git ls-files for shared config"

# Legacy team block ignored
assert_file_contains "$INIT_CMD" "team.*block.*legacy" "Init ignores legacy team config block"

# ============================================================
# TEST 4: CLAUDE.md trigger table
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Trigger table updates${NC}"

# Init absorbs team triggers
assert_file_contains "$CLAUDE_TMPL" "validate my environment" "Template has onboarding trigger"
assert_file_contains "$CLAUDE_TMPL" "edikt:config" "Template has config triggers"

# Team deprecated row exists
assert_file_contains "$CLAUDE_TMPL" "edikt:team" "Template still has team reference (deprecated)"
assert_file_contains "$CLAUDE_TMPL" "deprecated" "Template marks team as deprecated"

# Config triggers
assert_file_contains "$CLAUDE_TMPL" "show config" "Config trigger: show config"
assert_file_contains "$CLAUDE_TMPL" "change config" "Config trigger: change config"

# ============================================================
# TEST 5: Installer updated
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Installer${NC}"

# Config is a flat command
assert_file_contains "$INSTALLER" "config" "Installer includes config in flat commands"

# Team is NOT in flat commands
if grep -q 'FLAT_COMMANDS=.*team' "$INSTALLER"; then
    fail "Team should not be in FLAT_COMMANDS"
else
    pass "Team removed from FLAT_COMMANDS"
fi

# Team IS in deprecated commands
if grep -A1 "deprecated" "$INSTALLER" | grep -q "team"; then
    pass "Team in deprecated commands list"
else
    fail "Team should be in deprecated commands list"
fi

# ============================================================
# TEST 6: Website documentation
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Website${NC}"

assert_file_exists "$PROJECT_ROOT/website/commands/config.md" "Website config page exists"
assert_file_contains "$PROJECT_ROOT/website/commands/config.md" "edikt:config" "Website config page has command name"
assert_file_contains "$PROJECT_ROOT/website/commands/config.md" "get" "Website config documents get"
assert_file_contains "$PROJECT_ROOT/website/commands/config.md" "set" "Website config documents set"

assert_file_contains "$PROJECT_ROOT/website/commands/team.md" "deprecated" "Website team page shows deprecated"
assert_file_contains "$PROJECT_ROOT/website/commands/team.md" "edikt:init" "Website team page redirects to init"

# VitePress nav updated
assert_file_contains "$PROJECT_ROOT/website/.vitepress/config.ts" "config" "VitePress nav includes config"

# Natural language triggers updated
assert_file_contains "$PROJECT_ROOT/website/natural-language.md" "edikt:config" "Natural language has config triggers"

test_summary
