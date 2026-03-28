#!/bin/bash
# Test: template quality, rule pack quality, extensibility, configurable paths
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Rule pack quality — v0.1.0 standards
# ============================================================

RULES_DIR="$PROJECT_ROOT/templates/rules"

# All rule packs should be v0.1.0
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    if grep -q 'version: "0.1.0"' "$rule" 2>/dev/null; then
        pass "Rule pack version 0.1.0: $name"
    else
        fail "Rule pack version 0.1.0: $name"
    fi
done

# All rule packs should have edikt:generated marker
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    assert_file_contains "$rule" "edikt:generated" "Has edikt:generated marker: $name"
done

# All rule packs must have governance checkpoint
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    assert_file_contains "$rule" "<governance_checkpoint>" "Has governance checkpoint: $name"
done

# Governance checkpoint must appear before the first heading (correct position)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    cp_line=$(grep -n "<governance_checkpoint>" "$rule" 2>/dev/null | head -1 | cut -d: -f1)
    h1_line=$(grep -n "^# " "$rule" 2>/dev/null | head -1 | cut -d: -f1)
    if [ -n "$cp_line" ] && [ -n "$h1_line" ] && [ "$cp_line" -lt "$h1_line" ]; then
        pass "Checkpoint before title: $name"
    else
        fail "Checkpoint before title: $name"
    fi
done

# Base packs should NOT use paths: "**/*" (should scope to code files)
for rule in "$RULES_DIR"/base/*.md; do
    name=$(basename "$rule")
    if grep -q 'paths: "\*\*/\*"' "$rule" 2>/dev/null; then
        fail "Base pack should scope to code files, not **/*: $name"
    else
        pass "Base pack properly scoped: $name"
    fi
done

# All packs should use four-tier phrasing (at least NEVER or MUST)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    if grep -qE "^- NEVER |^- MUST " "$rule" 2>/dev/null; then
        pass "Uses NEVER/MUST phrasing: $name"
    else
        fail "Missing NEVER/MUST phrasing: $name"
    fi
done

# Rule count should be 7-25 per pack (lower bound reduced after cross-pack dedup)
for rule in "$RULES_DIR"/base/*.md "$RULES_DIR"/lang/*.md "$RULES_DIR"/framework/*.md; do
    name=$(echo "$rule" | sed "s|$RULES_DIR/||")
    count=$(grep -cE "^- " "$rule" 2>/dev/null)
    if [ "$count" -ge 7 ] && [ "$count" -le 25 ]; then
        pass "Rule count in range (${count}): $name"
    else
        fail "Rule count out of range (${count}, want 7-25): $name"
    fi
done

# New packs exist
assert_file_exists "$RULES_DIR/base/api.md" "New pack exists: api.md"
assert_file_exists "$RULES_DIR/base/database.md" "New pack exists: database.md"
assert_file_exists "$RULES_DIR/base/observability.md" "New pack exists: observability.md"
assert_file_exists "$RULES_DIR/base/seo.md" "New pack exists: seo.md"

# ============================================================
# Template quality — new sections in artifact templates
# ============================================================

# PRD template has numbered requirements
assert_file_contains "$PROJECT_ROOT/commands/prd.md" "FR-001" "PRD has numbered requirements (FR-001)"
assert_file_contains "$PROJECT_ROOT/commands/prd.md" "AC-001" "PRD has numbered acceptance criteria (AC-001)"
assert_file_contains "$PROJECT_ROOT/commands/prd.md" "NEEDS CLARIFICATION" "PRD has NEEDS CLARIFICATION markers"
assert_file_contains "$PROJECT_ROOT/commands/prd.md" "Verify:" "PRD acceptance criteria have verification methods"
assert_file_contains "$PROJECT_ROOT/commands/prd.md" "stakeholders" "PRD has stakeholders in frontmatter"

# ADR template has MADR sections
assert_file_contains "$PROJECT_ROOT/commands/adr.md" "Confirmation" "ADR has Confirmation section"
assert_file_contains "$PROJECT_ROOT/commands/adr.md" "Decision Drivers" "ADR has Decision Drivers section"
assert_file_contains "$PROJECT_ROOT/commands/adr.md" "decision-makers" "ADR has decision-makers in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/adr.md" "supersedes" "ADR has supersedes in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/adr.md" "Rejected because" "ADR alternatives have rejection reasons"

# Invariant template has new sections
assert_file_contains "$PROJECT_ROOT/commands/invariant.md" "severity:" "Invariant has severity in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/invariant.md" "scope:" "Invariant has scope in frontmatter"
assert_file_contains "$PROJECT_ROOT/commands/invariant.md" "Violation Consequences" "Invariant has Violation Consequences"
assert_file_contains "$PROJECT_ROOT/commands/invariant.md" "Verification" "Invariant has Verification section"
assert_file_contains "$PROJECT_ROOT/commands/invariant.md" "Exceptions" "Invariant has Exceptions section"

# Spec template has new sections
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "Non-Goals" "Spec has Non-Goals section"
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "Alternatives Considered" "Spec has Alternatives Considered"
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "Risks" "Spec has Risks section"
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "AC-001" "Spec has numbered acceptance criteria"
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "NEEDS CLARIFICATION" "Spec has NEEDS CLARIFICATION markers"
assert_file_contains "$PROJECT_ROOT/commands/spec.md" "implements:" "Spec uses implements: (not source_prd:)"

# Compile output has primacy + recency
assert_file_contains "$PROJECT_ROOT/commands/compile.md" "Non-Negotiable Constraints" "Compile has constraints at top"
assert_file_contains "$PROJECT_ROOT/commands/compile.md" "Reminder:" "Compile has reminder at bottom (recency)"
assert_file_contains "$PROJECT_ROOT/commands/compile.md" "directives:" "Compile output has directive count"

# ============================================================
# Command quality — CRITICAL + REMEMBER blocks
# ============================================================

for cmd in doctor audit drift plan compile review brainstorm; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd}.md" "CRITICAL:" "${cmd} has CRITICAL statement"
done

for cmd in prd adr invariant spec spec-artifacts compile drift; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd}.md" "REMEMBER:" "${cmd} has REMEMBER block"
done

# Commands use paths: config (not hardcoded)
for cmd in prd adr invariant spec drift compile; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd}.md" "paths:" "${cmd} references paths: config"
done

# Interactive commands have plan mode guard
for cmd in init prd spec spec-artifacts adr invariant intake plan brainstorm; do
    assert_file_contains "$PROJECT_ROOT/commands/${cmd}.md" "plan mode" "${cmd} has plan mode guard"
done

# Brainstorm command has required phases and features
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Open Exploration" "brainstorm has open exploration phase"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Guided Narrowing" "brainstorm has guided narrowing phase"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "Formalize" "brainstorm has formalize step"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "BRAIN-" "brainstorm uses BRAIN- prefix"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "fresh" "brainstorm supports --fresh flag"
assert_file_contains "$PROJECT_ROOT/commands/brainstorm.md" "unconstrained" "brainstorm has unconstrained mode"

# Plan command accepts flexible input
assert_file_contains "$PROJECT_ROOT/commands/plan.md" "SPEC identifier" "plan accepts SPEC input"
assert_file_contains "$PROJECT_ROOT/commands/plan.md" "Ticket ID" "plan accepts ticket input"
assert_file_contains "$PROJECT_ROOT/commands/plan.md" "Existing plan name" "plan accepts PLAN input"
assert_file_contains "$PROJECT_ROOT/commands/plan.md" "Natural language" "plan accepts natural language"
assert_file_contains "$PROJECT_ROOT/commands/plan.md" "Quick plan" "plan offers disambiguation"

# Upgrade command has remote version check
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "Check for Updates" "upgrade has remote version check"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "offline" "upgrade supports --offline flag"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "raw.githubusercontent.com/diktahq/edikt/main/VERSION" "upgrade fetches VERSION from GitHub"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "max-time 5" "upgrade has 5s curl timeout"

# Website plan page matches command behavior
assert_file_contains "$PROJECT_ROOT/website/commands/plan.md" "PLAN-NNN" "website plan page documents PLAN input"
assert_file_contains "$PROJECT_ROOT/website/commands/plan.md" "Quick plan" "website plan page documents disambiguation"

# ============================================================
# Natural language triggers
# ============================================================

# CLAUDE.md template has intent-based trigger table
assert_file_contains "$PROJECT_ROOT/templates/CLAUDE.md.tmpl" "Match the user" "Template has intent-matching instruction"

# All 24 commands have a trigger in the template
for cmd in status context plan adr invariant prd spec spec-artifacts drift compile review-governance review audit docs doctor init intake rules-update sync session upgrade agents mcp team brainstorm; do
    assert_file_contains "$PROJECT_ROOT/templates/CLAUDE.md.tmpl" "edikt:${cmd}" "Template has trigger for ${cmd}"
done

# ============================================================
# Configurable paths
# ============================================================

# Config has paths section
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "paths:" "Config has paths: section"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "decisions:" "Config has decisions path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "invariants:" "Config has invariants path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "plans:" "Config has plans path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "specs:" "Config has specs path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "prds:" "Config has prds path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "guidelines:" "Config has guidelines path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "reports:" "Config has reports path"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "soul:" "Config has soul path"

# Config has features section
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "features:" "Config has features: section"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "auto-format:" "Config has auto-format feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "session-summary:" "Config has session-summary feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "signal-detection:" "Config has signal-detection feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "plan-injection:" "Config has plan-injection feature"
assert_file_contains "$PROJECT_ROOT/.edikt/config.yaml" "quality-gates:" "Config has quality-gates feature"

# Hooks respect feature config
assert_file_contains "$PROJECT_ROOT/templates/hooks/post-tool-use.sh" "auto-format: false" "PostToolUse checks auto-format config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/session-start.sh" "session-summary: false" "SessionStart checks session-summary config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/stop-hook.sh" "signal-detection: false" "Stop checks signal-detection config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/user-prompt-submit.sh" "plan-injection: false" "UserPromptSubmit checks plan-injection config"
assert_file_contains "$PROJECT_ROOT/templates/hooks/subagent-stop.sh" "quality-gates: false" "SubagentStop checks quality-gates config"

# Reports directory exists
assert_dir_exists "$PROJECT_ROOT/docs/reports" "Reports directory exists"

# Init creates reports directory
assert_file_contains "$PROJECT_ROOT/commands/init.md" "reports" "Init creates reports directory"

# ============================================================
# Extensibility (ADR-006)
# ============================================================

# Init supports template overrides
assert_file_contains "$PROJECT_ROOT/commands/init.md" ".edikt/templates" "Init checks for template overrides"

# Upgrade respects custom agents
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt:custom" "Upgrade respects custom agent marker"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "agents.custom" "Upgrade respects custom agent config"

# Rules-update handles overrides and extensions
assert_file_contains "$PROJECT_ROOT/commands/rules-update.md" "Overridden" "Rules-update detects overridden packs"
assert_file_contains "$PROJECT_ROOT/commands/rules-update.md" "extend" "Rules-update handles extensions"

# Doctor reports extensibility state
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Template override" "Doctor reports template overrides"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "Rule override" "Doctor reports rule overrides"

# ============================================================
# Version consistency
# ============================================================

# VERSION file
FILE_VER=$(cat "$PROJECT_ROOT/VERSION" | tr -d '[:space:]')
if echo "$FILE_VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+(-[a-zA-Z0-9.]+)?$'; then
    pass "VERSION file is valid semver ($FILE_VER)"
else
    fail "VERSION file is valid semver" "Got: $FILE_VER"
fi

# Config version matches
CONFIG_VER=$(grep 'edikt_version:' "$PROJECT_ROOT/.edikt/config.yaml" | awk '{print $2}' | tr -d '"')
if [ "$CONFIG_VER" = "$FILE_VER" ]; then
    pass "Config edikt_version matches VERSION ($FILE_VER)"
else
    fail "Config edikt_version matches VERSION" "Config=$CONFIG_VER, File=$FILE_VER"
fi

# No old version references in active code (excluding historical docs)
OLD_REFS=$(grep -rn '"4\.0"\|"3\.9"\|"3\.8"' "$PROJECT_ROOT/commands/" "$PROJECT_ROOT/templates/" "$PROJECT_ROOT/install.sh" "$PROJECT_ROOT/README.md" 2>/dev/null | wc -l | tr -d ' ')
if [ "$OLD_REFS" -eq 0 ]; then
    pass "No old version references in active code"
else
    fail "Old version references found in active code" "$OLD_REFS occurrences"
fi

# ============================================================
# No old agent names in active code
# ============================================================

OLD_AGENTS=$(grep -rn "principal-\|staff-\|senior-" "$PROJECT_ROOT/commands/" "$PROJECT_ROOT/templates/" "$PROJECT_ROOT/website/" --include="*.md" --include="*.yaml" --include="*.sh" --include="*.ts" 2>/dev/null | grep -v node_modules | grep -v ".vitepress/dist" | wc -l | tr -d ' ')
if [ "$OLD_AGENTS" -eq 0 ]; then
    pass "No old agent names (principal-/staff-/senior-) in codebase"
else
    fail "Old agent names found" "$OLD_AGENTS occurrences"
fi

# ============================================================
# Website builds
# ============================================================

if command -v npx >/dev/null 2>&1 && [ -d "$PROJECT_ROOT/website" ]; then
    BUILD_OUTPUT=$(cd "$PROJECT_ROOT/website" && npx vitepress build 2>&1)
    if echo "$BUILD_OUTPUT" | grep -q "build complete"; then
        pass "VitePress website builds successfully"
    else
        fail "VitePress website build failed"
    fi
else
    echo "  SKIP  VitePress build (npx not available)"
fi

# ============================================================
# spec-artifacts output contracts
# ============================================================

SPECS_DIR="$PROJECT_ROOT/test/fixtures/specs"

# Test 1: SQL path - spec with Postgres keyword → data-model.mmd
assert_file_exists "$SPECS_DIR/spec-sql-postgres.md" "Test fixture exists: spec-sql-postgres.md"
assert_file_contains "$SPECS_DIR/spec-sql-postgres.md" "Postgres" "Fixture contains Postgres keyword"
assert_file_contains "$SPECS_DIR/spec-sql-postgres.md" "status: accepted" "Fixture has accepted status"

# Test 2: Document-mongo path - spec with MongoDB keyword → data-model.schema.yaml
assert_file_exists "$SPECS_DIR/spec-doc-mongodb.md" "Test fixture exists: spec-doc-mongodb.md"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb.md" "MongoDB" "Fixture contains MongoDB keyword"

# Test 3: Document-dynamo path - spec with DynamoDB keyword → data-model.md with Access Patterns
assert_file_exists "$SPECS_DIR/spec-doc-dynamodb.md" "Test fixture exists: spec-doc-dynamodb.md"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb.md" "DynamoDB" "Fixture contains DynamoDB keyword"

# Test 4: Key-value path - spec with Redis keyword → data-model.md with key schema
assert_file_exists "$SPECS_DIR/spec-kv-redis.md" "Test fixture exists: spec-kv-redis.md"
assert_file_contains "$SPECS_DIR/spec-kv-redis.md" "Redis" "Fixture contains Redis keyword"

# Test 5: Mixed path - spec with Postgres and Redis → both data-model-sql.mmd and data-model-kv.md
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis.md" "Test fixture exists: spec-mixed-postgres-redis.md"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis.md" "Postgres" "Mixed fixture contains Postgres"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis.md" "Redis" "Mixed fixture contains Redis"

# Test 6: Config fallback - spec with no keywords + config default_type: sql → data-model.mmd exists
assert_file_exists "$SPECS_DIR/spec-no-keywords.md" "Test fixture exists: spec-no-keywords.md"
assert_file_contains "$SPECS_DIR/spec-no-keywords.md" "Data Model" "No-keyword fixture has Data Model section"

# Test 7: Config auto + no keywords - spec with no keywords + config auto → warning/prompt in output
assert_file_exists "$SPECS_DIR/spec-auto-fallback.md" "Test fixture exists: spec-auto-fallback.md"
assert_file_contains "$SPECS_DIR/spec-auto-fallback.md" "status: accepted" "Auto-fallback fixture has accepted status"

# Test 8: Active constraints injected - spec with active invariant → "active constraints applied" in routing
assert_file_exists "$SPECS_DIR/spec-with-constraints.md" "Test fixture exists: spec-with-constraints.md"
assert_file_contains "$SPECS_DIR/spec-with-constraints.md" "Constrained Feature" "Constraint fixture has title"

# Test 9: Empty invariant body warning - spec with empty invariant → "body is empty" in output
assert_file_exists "$SPECS_DIR/spec-empty-constraint.md" "Test fixture exists: spec-empty-constraint.md"
assert_file_contains "$SPECS_DIR/spec-empty-constraint.md" "Empty Constraint" "Empty constraint fixture exists"

# Test 10: Superseded invariant excluded - spec with Superseded invariant → constraint count is 0
assert_file_exists "$SPECS_DIR/spec-superseded-invariant.md" "Test fixture exists: spec-superseded-invariant.md"
assert_file_contains "$SPECS_DIR/spec-superseded-invariant.md" "Superseded" "Superseded fixture exists"

# Test 11: Spec-frontmatter override - config sql + spec database_type: document-mongo → data-model.schema.yaml exists
assert_file_exists "$SPECS_DIR/spec-override-frontmatter.md" "Test fixture exists: spec-override-frontmatter.md"
assert_file_contains "$SPECS_DIR/spec-override-frontmatter.md" "database_type: document-mongo" "Override fixture has frontmatter override"

# Test 12: Design blueprint header - any spec with data model → artifact contains "Design blueprint" comment
assert_file_exists "$SPECS_DIR/spec-blueprint-check.md" "Test fixture exists: spec-blueprint-check.md"
assert_file_contains "$SPECS_DIR/spec-blueprint-check.md" "Data Model" "Blueprint fixture has Data Model section"

# Verify spec-artifacts command has design blueprint framing
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "Design blueprint" "spec-artifacts mentions design blueprint framing"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "design blueprints" "spec-artifacts uses design blueprint language"

# Verify spec-artifacts command has constraint injection logic
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "ACTIVE CONSTRAINTS" "spec-artifacts injects active constraints"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "Resolve Context" "spec-artifacts has resolve context step"

# Verify spec-artifacts command has database type resolution
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "database_type:" "spec-artifacts reads spec frontmatter database_type"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "artifacts.database.default_type" "spec-artifacts reads config default_type"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "Keyword scan" "spec-artifacts performs keyword scanning"

# Verify data model lookup tables are referenced
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "data-model.mmd" "spec-artifacts generates .mmd files"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "data-model.schema.yaml" "spec-artifacts generates schema.yaml files"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "erDiagram" "spec-artifacts uses Mermaid erDiagram format"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "\$schema" "spec-artifacts uses JSON Schema"

# Verify multi-database suffix naming
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "data-model-sql.mmd" "spec-artifacts uses sql suffix for mixed"
assert_file_contains "$PROJECT_ROOT/commands/spec-artifacts.md" "data-model-kv.md" "spec-artifacts uses kv suffix for key-value"

# ============================================================
# Golden artifact validation
# ============================================================

# SQL golden — Mermaid ERD
assert_file_exists "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "Golden: SQL data-model.mmd exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "erDiagram" "Golden: SQL has erDiagram"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "Design blueprint" "Golden: SQL has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "edikt:artifact" "Golden: SQL has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "status=draft" "Golden: SQL has draft status"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/data-model.mmd" "%% Index:" "Golden: SQL has index comments"

# Document-mongo golden — JSON Schema YAML
assert_file_exists "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "Golden: Mongo data-model.schema.yaml exists"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "\$schema" "Golden: Mongo has \$schema"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "collection:" "Golden: Mongo has collection"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "indexes:" "Golden: Mongo has indexes"
assert_file_contains "$SPECS_DIR/spec-doc-mongodb/data-model.schema.yaml" "Design blueprint" "Golden: Mongo has blueprint header"

# Document-dynamo golden — Access patterns
assert_file_exists "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Golden: DynamoDB data-model.md exists"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Access Patterns" "Golden: DynamoDB has Access Patterns"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "GSI" "Golden: DynamoDB has GSI design"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Entity Prefixes" "Golden: DynamoDB has entity prefixes"
assert_file_contains "$SPECS_DIR/spec-doc-dynamodb/data-model.md" "Design blueprint" "Golden: DynamoDB has blueprint header"

# Key-value golden — Key schema
assert_file_exists "$SPECS_DIR/spec-kv-redis/data-model.md" "Golden: Redis data-model.md exists"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Key Schema" "Golden: Redis has Key Schema"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "TTL" "Golden: Redis has TTL column"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Namespace" "Golden: Redis has Namespace"
assert_file_contains "$SPECS_DIR/spec-kv-redis/data-model.md" "Design blueprint" "Golden: Redis has blueprint header"

# Mixed golden — suffix naming with both artifacts
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis/data-model-sql.mmd" "Golden: Mixed SQL data-model-sql.mmd exists"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-sql.mmd" "erDiagram" "Golden: Mixed SQL has erDiagram"
assert_file_exists "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "Golden: Mixed KV data-model-kv.md exists"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "Key Schema" "Golden: Mixed KV has Key Schema"
assert_file_contains "$SPECS_DIR/spec-mixed-postgres-redis/data-model-kv.md" "## Notes" "Golden: Mixed KV has Notes section"

# API contract golden — OpenAPI 3.0
assert_file_exists "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "Golden: API contract exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "openapi:" "Golden: API has openapi field"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "Design blueprint" "Golden: API has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "edikt:artifact" "Golden: API has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "paths:" "Golden: API has paths section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "components:" "Golden: API has components"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/api.yaml" "securitySchemes:" "Golden: API has security schemes"

# Event contract golden — AsyncAPI 2.6
assert_file_exists "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "Golden: Event contract exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "asyncapi:" "Golden: Events has asyncapi field"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "Design blueprint" "Golden: Events has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "channels:" "Golden: Events has channels"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "x-producer:" "Golden: Events has producer"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/contracts/events.yaml" "x-idempotency:" "Golden: Events has idempotency"

# Migration golden — SQL with UP/DOWN/BACKFILL/RISK
assert_file_exists "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "Golden: Migration exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "Design blueprint" "Golden: Migration has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "edikt:artifact" "Golden: Migration has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== UP ===" "Golden: Migration has UP section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== DOWN ===" "Golden: Migration has DOWN section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== BACKFILL ===" "Golden: Migration has BACKFILL section"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/migrations/001_create_users.sql" "=== RISK ===" "Golden: Migration has RISK section"

# Fixtures golden — YAML seed data
assert_file_exists "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "Golden: Fixtures exist"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "Design blueprint" "Golden: Fixtures has blueprint header"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "edikt:artifact" "Golden: Fixtures has artifact marker"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "scenarios:" "Golden: Fixtures has scenarios"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "_note:" "Golden: Fixtures has _note fields"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/fixtures.yaml" "relationships:" "Golden: Fixtures has relationships"

# Test strategy golden — markdown with frontmatter
assert_file_exists "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "Golden: Test strategy exists"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "artifact_type: test-strategy" "Golden: Test strategy has artifact_type"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "reviewed_by: qa" "Golden: Test strategy reviewed by qa"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Unit Tests" "Golden: Test strategy has Unit Tests"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Integration Tests" "Golden: Test strategy has Integration Tests"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Edge Cases" "Golden: Test strategy has Edge Cases"
assert_file_contains "$SPECS_DIR/spec-sql-postgres/test-strategy.md" "## Coverage Target" "Golden: Test strategy has Coverage Target"

# ============================================================
# Compile golden validation
# ============================================================

COMPILE_DIR="$PROJECT_ROOT/test/fixtures/compile"

# Input fixtures exist
assert_file_exists "$COMPILE_DIR/decisions/ADR-001-test.md" "Compile fixture: ADR-001 exists"
assert_file_contains "$COMPILE_DIR/decisions/ADR-001-test.md" "Accepted" "Compile fixture: ADR-001 is Accepted"
assert_file_exists "$COMPILE_DIR/decisions/ADR-002-test.md" "Compile fixture: ADR-002 exists"
assert_file_contains "$COMPILE_DIR/decisions/ADR-002-test.md" "Superseded" "Compile fixture: ADR-002 is Superseded"
assert_file_exists "$COMPILE_DIR/invariants/INV-001-test.md" "Compile fixture: INV-001 exists"
assert_file_contains "$COMPILE_DIR/invariants/INV-001-test.md" "Active" "Compile fixture: INV-001 is Active"

# Expected output structure
assert_file_exists "$COMPILE_DIR/expected-governance.md" "Compile golden: expected-governance.md exists"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "edikt:compiled" "Compile golden: has compiled marker"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "1 accepted, 1 superseded" "Compile golden: correct ADR counts"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Non-Negotiable Constraints" "Compile golden: has constraints section"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Architecture Decisions" "Compile golden: has architecture section"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "Reminder:" "Compile golden: has reminder section (recency)"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "ref: INV-001" "Compile golden: INV-001 referenced"
assert_file_contains "$COMPILE_DIR/expected-governance.md" "ref: ADR-001" "Compile golden: ADR-001 referenced"

# Superseded ADR must NOT appear in compiled output
if grep -q "kebab-case" "$COMPILE_DIR/expected-governance.md" 2>/dev/null; then
    fail "Compile golden: ADR-002 (superseded) excluded from output"
else
    pass "Compile golden: ADR-002 (superseded) excluded from output"
fi

test_summary
