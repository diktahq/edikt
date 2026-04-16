#!/bin/bash
# Test: project directory structure is correct
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# Core directories exist
assert_dir_exists "$PROJECT_ROOT/commands" "commands/ exists"
assert_dir_exists "$PROJECT_ROOT/templates" "templates/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/rules" "templates/rules/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/rules/base" "templates/rules/base/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/rules/lang" "templates/rules/lang/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/rules/framework" "templates/rules/framework/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/agents" "templates/agents/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/sdlc" "templates/sdlc/ exists"
assert_dir_exists "$PROJECT_ROOT/templates/hooks" "templates/hooks/ exists"
assert_dir_exists "$PROJECT_ROOT/test" "test/ exists"
assert_dir_exists "$PROJECT_ROOT/docs" "docs/ exists"

# Core files exist
assert_file_exists "$PROJECT_ROOT/CLAUDE.md" "CLAUDE.md exists"
assert_file_exists "$PROJECT_ROOT/README.md" "README.md exists"
assert_file_exists "$PROJECT_ROOT/VERSION" "VERSION file exists"
assert_file_exists "$PROJECT_ROOT/CHANGELOG.md" "CHANGELOG.md exists"
assert_file_exists "$PROJECT_ROOT/.edikt/config.yaml" ".edikt/config.yaml exists"
assert_file_exists "$PROJECT_ROOT/docs/project-context.md" "docs/project-context.md exists"
assert_file_exists "$PROJECT_ROOT/templates/rules/_registry.yaml" "Registry exists"

# VERSION file contains a semver-like version
if grep -qE '^[0-9]+\.[0-9]+' "$PROJECT_ROOT/VERSION"; then
    pass "VERSION file contains a version number"
else
    fail "VERSION file contains a version number"
fi

# .edikt/config.yaml has edikt_version
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "edikt_version" ".edikt/config.yaml has edikt_version"

# edikt_version in config matches VERSION file
CONFIG_VER=$(grep '^edikt_version:' "$PROJECT_ROOT/.edikt/config.yaml" | awk '{print $2}' | tr -d '"')
FILE_VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
if [ "$CONFIG_VER" = "$FILE_VER" ]; then
    pass "edikt_version in config matches VERSION file ($FILE_VER)"
else
    fail "edikt_version in config matches VERSION file" "config=$CONFIG_VER, file=$FILE_VER"
fi

# No legacy tool references
assert_file_not_contains "$PROJECT_ROOT/CLAUDE.md" "conductor:context" "CLAUDE.md has no conductor: command references"
# Note: can't grep for "conductor" because the repo might be in a path containing it
assert_file_not_contains "$PROJECT_ROOT/README.md" "conductor:context" "README.md has no conductor: command references"

# No legacy directories
assert_file_not_exists "$PROJECT_ROOT/.conductor/config.yaml" "No .conductor/ directory"
assert_file_not_exists "$PROJECT_ROOT/.dof/config.yaml" "No .dof/ directory"

# settings template has all four hooks
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "SessionStart" "settings.json.tmpl has SessionStart hook"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "PreToolUse" "settings.json.tmpl has PreToolUse hook"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "Stop" "settings.json.tmpl has Stop hook"
# PreCompact removed in v0.5.0 per ADR-014 (hook deleted entirely)
assert_file_not_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "PreCompact" "settings.json.tmpl has no PreCompact hook (removed per ADR-014)"
# v0.5.0 new events MUST be wired
for event in SessionEnd SubagentStart TaskCompleted WorktreeCreate WorktreeRemove; do
    assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "$event" "settings.json.tmpl wires $event"
done
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "Write|Edit" "PreToolUse hook targets Write|Edit"
assert_file_contains "$PROJECT_ROOT/templates/settings.json.tmpl" "PostToolUse" "settings.json.tmpl has PostToolUse hook"
assert_file_contains "$PROJECT_ROOT/templates/hooks/session-start.sh" "git log" "SessionStart hook is git-aware"

# Flat commands exist
for cmd in init context status doctor upgrade agents mcp config session brainstorm capture; do
    assert_file_exists "$PROJECT_ROOT/commands/${cmd}.md" "commands/${cmd}.md exists"
done

# Namespaced commands exist
for cmd_path in adr/new invariant/new sdlc/prd sdlc/plan sdlc/spec sdlc/artifacts sdlc/review sdlc/drift sdlc/audit docs/review docs/intake gov/compile gov/review gov/rules-update gov/sync guideline/new guideline/review; do
    assert_file_exists "$PROJECT_ROOT/commands/${cmd_path}.md" "commands/${cmd_path}.md exists"
done

# Deprecated stubs exist
for cmd in adr invariant prd plan review drift audit docs intake rules-update sync compile review-governance spec spec-artifacts team; do
    assert_file_exists "$PROJECT_ROOT/commands/deprecated/${cmd}.md" "commands/deprecated/${cmd}.md exists"
done

# Shell preprocessing markers exist in artifact commands (namespaced paths)
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" '!`' "adr/new.md has shell preprocessing"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" '!`' "invariant/new.md has shell preprocessing"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" '!`' "sdlc/prd.md has shell preprocessing"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" '!`' "sdlc/plan.md has shell preprocessing"
assert_file_contains "$PROJECT_ROOT/commands/adr/new.md" "edikt:live" "adr/new.md injects live ADR number"
assert_file_contains "$PROJECT_ROOT/commands/invariant/new.md" "edikt:live" "invariant/new.md injects live INV number"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/prd.md" "edikt:live" "sdlc/prd.md injects live PRD number"

# Agent templates directory exists
assert_dir_exists "$PROJECT_ROOT/templates/agents" "templates/agents/ exists"
assert_file_exists "$PROJECT_ROOT/templates/agents/_registry.yaml" "Agent registry exists"
assert_file_exists "$PROJECT_ROOT/templates/agents/docs.md" "docs agent exists"

# Git hook template exists
assert_file_exists "$PROJECT_ROOT/templates/hooks/pre-push" "pre-push hook template exists"
assert_file_contains "$PROJECT_ROOT/templates/hooks/pre-push" "EDIKT_DOCS_SKIP" "pre-push hook has disable flag"
assert_file_contains "$PROJECT_ROOT/templates/hooks/pre-push" "exit 0" "pre-push hook never blocks (exits 0)"
assert_file_contains "$PROJECT_ROOT/templates/hooks/pre-push" "pre-push: false" "pre-push hook respects config disable"
assert_file_contains "$PROJECT_ROOT/templates/hooks/pre-push" "EDIKT_SECURITY_SKIP" "pre-push has security skip flag"

# init.md combined configuration view
assert_file_contains "$PROJECT_ROOT/commands/init.md" "single combined view" "init.md uses combined config view"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "One screen, one confirmation" "init.md confirms in one step"

# sdlc/plan.md pre-flight review checks
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "PRE-FLIGHT" "sdlc/plan.md has pre-flight review"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "no-review" "sdlc/plan.md supports --no-review flag"

# session.md checks
assert_file_contains "$PROJECT_ROOT/commands/session.md" "context: fork" "session.md has context: fork"
assert_file_contains "$PROJECT_ROOT/commands/session.md" "SESSION SUMMARY" "session.md has session summary output"
assert_file_contains "$PROJECT_ROOT/commands/session.md" "edikt:adr\|edikt:capture" "session.md suggests artifact capture"

# upgrade.md checks
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "EDIKT UPGRADE" "upgrade.md has upgrade output format"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "never overwrites" "upgrade.md documents safe upgrade behavior"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt:doctor" "upgrade.md suggests doctor after upgrade"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "inline bash" "upgrade.md detects inline bash migration"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "JSON validation" "upgrade.md detects Stop hook JSON bug"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "WHAT'S NEW" "upgrade.md shows release notes"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "CHANGELOG" "upgrade.md reads changelog"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt_version" "upgrade.md updates edikt_version after upgrade"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "VERSION" "upgrade.md reads installed VERSION"
assert_file_exists "$PROJECT_ROOT/website/commands/upgrade.md" "website/commands/upgrade.md exists"

# doctor.md version check
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "edikt_version" "doctor.md checks edikt_version in config"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "~/.edikt/VERSION" "doctor.md reads installed VERSION"

# init.md writes edikt_version
assert_file_contains "$PROJECT_ROOT/commands/init.md" "edikt_version" "init.md writes edikt_version to config"

# sdlc/audit.md checks
assert_file_contains "$PROJECT_ROOT/commands/sdlc/audit.md" "context: fork" "sdlc/audit.md has context: fork"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/audit.md" "OWASP" "sdlc/audit.md covers OWASP"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/audit.md" "security" "sdlc/audit.md routes to security agent"

# sdlc/review.md checks
assert_file_contains "$PROJECT_ROOT/commands/sdlc/review.md" "context: fork" "sdlc/review.md has context: fork"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/review.md" "IMPLEMENTATION REVIEW" "sdlc/review.md has review output format"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/review.md" "dba" "sdlc/review.md routes to dba"

# website pages exist for all v3 commands
assert_file_exists "$PROJECT_ROOT/website/commands/sdlc/review.md" "website/commands/sdlc/review.md exists"
assert_file_exists "$PROJECT_ROOT/website/commands/sdlc/audit.md" "website/commands/sdlc/audit.md exists"
assert_file_exists "$PROJECT_ROOT/website/commands/session.md" "website/commands/session.md exists"

test_summary
