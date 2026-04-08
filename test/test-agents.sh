#!/bin/bash
# Test: agent templates and registry
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

AGENTS_DIR="$PROJECT_ROOT/templates/agents"
REGISTRY="$AGENTS_DIR/_registry.yaml"

# ============================================================
# Registry checks
# ============================================================

assert_file_exists "$REGISTRY" "Agent registry exists"

# All slugs in registry must have a corresponding .md file
_registry_slugs_ok=true
while IFS= read -r slug; do
    [ -z "$slug" ] && continue
    if [ ! -f "$AGENTS_DIR/${slug}.md" ]; then
        fail "All registry slugs resolve to existing agent templates" "Missing: ${slug}.md"
        _registry_slugs_ok=false
        break
    fi
done < <(grep -E '^\s*-\s+\w' "$REGISTRY" | sed 's/^[[:space:]]*-[[:space:]]*//')
$_registry_slugs_ok && pass "All registry slugs resolve to existing agent templates"

# Required categories exist
if grep -q "^always:" "$REGISTRY" && grep -q "^common:" "$REGISTRY"; then
    _always_ok=true
    for _slug in architect qa; do
        if ! grep -A50 "^always:" "$REGISTRY" | grep -q "^\s*-\s*${_slug}$"; then
            fail "Registry has required categories and always-install agents" "always missing: ${_slug}"
            _always_ok=false
        fi
    done
    $_always_ok && pass "Registry has required categories and always-install agents"
else
    fail "Registry has required categories and always-install agents" "Missing always: or common: category"
fi

# ============================================================
# Per-agent template checks
# ============================================================

EXPECTED_AGENTS=(
    architect
    dba
    security
    api
    backend
    frontend
    qa
    sre
    platform
    docs
    pm
    ux
    data
    performance
    compliance
    mobile
    seo
    gtm
    evaluator
)

FOUND=0
for slug in "${EXPECTED_AGENTS[@]}"; do
    FILE="$AGENTS_DIR/${slug}.md"
    if [ ! -f "$FILE" ]; then
        fail "Agent template exists: ${slug}.md"
        continue
    fi
    FOUND=$((FOUND + 1))

    # Has YAML frontmatter
    head -1 "$FILE" | grep -q "^---$" \
        && pass "Has frontmatter: ${slug}.md" \
        || fail "Has frontmatter: ${slug}.md"

    # Has required frontmatter fields (no model: field — ADR-007)
    _fm_ok=true
    for _field in name: description: tools:; do
        grep -q "^${_field}" "$FILE" || { _fm_ok=false; break; }
    done
    grep -q "^model:" "$FILE" && _fm_ok=false
    $_fm_ok \
        && pass "Has required frontmatter (no model): ${slug}.md" \
        || fail "Has required frontmatter (no model): ${slug}.md"

    # Body contains role identity
    grep -q "specialist\|You are\|expert" "$FILE" \
        && pass "Has role identity: ${slug}.md" \
        || fail "Has role identity: ${slug}.md"

    # Body has REMEMBER block (recency reinforcement)
    grep -q "REMEMBER:" "$FILE" \
        && pass "Has REMEMBER block: ${slug}.md" \
        || fail "Has REMEMBER block: ${slug}.md"
done

pass "Found $FOUND/${#EXPECTED_AGENTS[@]} expected agent templates"

# ============================================================
# Specific agent checks
# ============================================================

# dba and security should have memory: project
assert_file_contains "$AGENTS_DIR/dba.md" "memory: project" "dba.md has memory: project"
assert_file_contains "$AGENTS_DIR/security.md" "memory: project" "security.md has memory: project"

# Write-capable agents should have Write and Edit tools
for writer in backend frontend qa mobile; do
    assert_file_contains "$AGENTS_DIR/${writer}.md" "Write" "${writer}.md has Write tool"
    assert_file_contains "$AGENTS_DIR/${writer}.md" "Edit" "${writer}.md has Edit tool"
done

# Write-capable agents should have File Formatting section
for writer in backend frontend qa mobile; do
    assert_file_contains "$AGENTS_DIR/${writer}.md" "File Formatting" "${writer}.md has File Formatting section"
done

# Read-only agents should NOT have Write in tools: section (may appear in disallowedTools:)
for reader in architect dba security api sre platform docs ux data performance compliance seo gtm; do
    # Extract only the tools: block (between "tools:" and the next frontmatter key or ---)
    tools_block=$(awk '/^tools:/{found=1; next} found && /^[a-zA-Z]/{exit} found && /^---/{exit} found{print}' "$AGENTS_DIR/${reader}.md" 2>/dev/null)
    if echo "$tools_block" | grep -q "Write"; then
        fail "${reader}.md should be read-only but has Write in tools:"
    else
        pass "${reader}.md is read-only (no Write in tools:)"
    fi
done

# Description includes "proactively" for auto-routing (evaluator excluded — invoked at phase-end)
for slug in "${EXPECTED_AGENTS[@]}"; do
    if [ "$slug" = "evaluator" ]; then
        assert_file_contains "$AGENTS_DIR/${slug}.md" "phase-end\|phase boundaries" "${slug}.md has phase-end trigger"
        continue
    fi
    assert_file_contains "$AGENTS_DIR/${slug}.md" "proactively\|Use proactively" "${slug}.md has proactive routing trigger"
done

# Old agent files should NOT exist
for old in principal-architect principal-dba principal-ux principal-data staff-engineer staff-security staff-frontend staff-qa staff-sre staff-docs senior-api senior-backend senior-pm senior-performance reviewer debugger; do
    if [ -f "$AGENTS_DIR/${old}.md" ]; then
        fail "Old agent file should be removed: ${old}.md"
    else
        pass "Old agent removed: ${old}.md"
    fi
done

test_summary
