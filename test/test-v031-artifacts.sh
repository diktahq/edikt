#!/bin/bash
# Test: v0.3.1 artifact generation changes
# Verifies JSONB storage strategy detection, domain class diagram (model.mmd),
# three entity modes in data-model.mmd, and configurable spec versions.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

ARTIFACTS_CMD="$PROJECT_ROOT/commands/sdlc/artifacts.md"
INIT_CMD="$PROJECT_ROOT/commands/init.md"
WEBSITE_ARTIFACTS="$PROJECT_ROOT/website/commands/sdlc/artifacts.md"
CHANGELOG="$PROJECT_ROOT/CHANGELOG.md"

echo ""

# ============================================================
# TEST 1: STORAGE_STRATEGY detection exists in artifacts command
# ============================================================

echo -e "${BOLD}TEST 1: Storage strategy detection${NC}"

assert_file_contains "$ARTIFACTS_CMD" "STORAGE_STRATEGY" "Artifacts command defines STORAGE_STRATEGY"
assert_file_contains "$ARTIFACTS_CMD" "jsonb-aggregate" "Artifacts command supports jsonb-aggregate strategy"
assert_file_contains "$ARTIFACTS_CMD" "normalized" "Artifacts command supports normalized strategy (default)"

# JSONB signal keywords must be documented
assert_file_contains "$ARTIFACTS_CMD" "jsonb" "JSONB signal: jsonb"
assert_file_contains "$ARTIFACTS_CMD" "json column" "JSONB signal: json column"
assert_file_contains "$ARTIFACTS_CMD" "aggregate storage" "JSONB signal: aggregate storage"
assert_file_contains "$ARTIFACTS_CMD" "embedded entity" "JSONB signal: embedded entity"
assert_file_contains "$ARTIFACTS_CMD" "nested entity" "JSONB signal: nested entity"

# Storage strategy in state checkpoint
assert_file_contains "$ARTIFACTS_CMD" "STORAGE_STRATEGY = {normalized | jsonb-aggregate | n/a}" "State checkpoint includes STORAGE_STRATEGY"

# ============================================================
# TEST 2: Domain class diagram (model.mmd) artifact
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Domain class diagram (model.mmd)${NC}"

# model.mmd is a new artifact type
assert_file_contains "$ARTIFACTS_CMD" "model.mmd" "Artifacts command defines model.mmd"
assert_file_contains "$ARTIFACTS_CMD" "classDiagram" "model.mmd uses Mermaid classDiagram"
assert_file_contains "$ARTIFACTS_CMD" "domain-model" "model.mmd artifact type is domain-model"

# Auto-triggers with data-model
assert_file_contains "$ARTIFACTS_CMD" "auto-triggers when data-model is generated" "model.mmd auto-triggers with data-model"

# Domain model stereotypes
assert_file_contains "$ARTIFACTS_CMD" "aggregate root" "model.mmd supports aggregate root stereotype"
assert_file_contains "$ARTIFACTS_CMD" "value object" "model.mmd supports value object stereotype"

# Reviewed by architect
assert_file_contains "$ARTIFACTS_CMD" "architect" "model.mmd reviewed by architect agent"

# ============================================================
# TEST 3: Three entity modes in data-model.mmd
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: Three entity modes in data-model.mmd${NC}"

# Physical table mode
assert_file_contains "$ARTIFACTS_CMD" "Physical table" "ERD supports physical table mode"

# JSONB-embedded mode
assert_file_contains "$ARTIFACTS_CMD" "JSONB-embedded" "ERD supports JSONB-embedded mode"
assert_file_contains "$ARTIFACTS_CMD" "contains jsonb" "JSONB-embedded uses 'contains jsonb' relationship label"

# Reference-only mode
assert_file_contains "$ARTIFACTS_CMD" "Reference-only" "ERD supports reference-only mode"
assert_file_contains "$ARTIFACTS_CMD" "references ref" "Reference-only uses 'references ref' relationship label"

# storage_strategy comment in template
assert_file_contains "$ARTIFACTS_CMD" "storage_strategy=" "ERD template includes storage_strategy metadata"

# ============================================================
# TEST 4: Configurable artifact spec versions
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Configurable artifact spec versions${NC}"

# Version resolution documented
assert_file_contains "$ARTIFACTS_CMD" "OPENAPI_VERSION" "Artifacts command uses OPENAPI_VERSION variable"
assert_file_contains "$ARTIFACTS_CMD" "ASYNCAPI_VERSION" "Artifacts command uses ASYNCAPI_VERSION variable"
assert_file_contains "$ARTIFACTS_CMD" "JSON_SCHEMA_URI" "Artifacts command uses JSON_SCHEMA_URI variable"

# Defaults are latest stable
assert_file_contains "$ARTIFACTS_CMD" '`3.1.0`' "Default OpenAPI version is 3.1.0"
assert_file_contains "$ARTIFACTS_CMD" '`3.0.0`' "Default AsyncAPI version is 3.0.0"
assert_file_contains "$ARTIFACTS_CMD" "2020-12" "Default JSON Schema is 2020-12"

# Templates use variables, not hardcoded versions
assert_file_contains "$ARTIFACTS_CMD" "{OPENAPI_VERSION}" "OpenAPI template uses variable"
assert_file_contains "$ARTIFACTS_CMD" "{ASYNCAPI_VERSION}" "AsyncAPI template uses variable"
assert_file_contains "$ARTIFACTS_CMD" "{JSON_SCHEMA_URI}" "JSON Schema template uses variable"

# No hardcoded old versions remain in templates
assert_file_not_contains "$ARTIFACTS_CMD" 'openapi: "3.0.0"' "No hardcoded OpenAPI 3.0.0 in templates"
assert_file_not_contains "$ARTIFACTS_CMD" 'asyncapi: "2.6.0"' "No hardcoded AsyncAPI 2.6.0 in templates"
assert_file_not_contains "$ARTIFACTS_CMD" "draft/07" "No hardcoded JSON Schema draft-07 in templates"

# Config key documented
assert_file_contains "$ARTIFACTS_CMD" "artifacts.versions" "Config key artifacts.versions documented"

# ============================================================
# TEST 5: AsyncAPI 3.0 template structure
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: AsyncAPI 3.0 template structure${NC}"

# AsyncAPI 3.0 uses channels + operations (not publish/subscribe)
assert_file_contains "$ARTIFACTS_CMD" "operations:" "AsyncAPI template has operations block"
assert_file_contains "$ARTIFACTS_CMD" "action: send" "AsyncAPI template uses action: send (3.0)"
assert_file_contains "$ARTIFACTS_CMD" 'address:' "AsyncAPI template has channel address (3.0)"
assert_file_not_contains "$ARTIFACTS_CMD" "publish:" "AsyncAPI template does not use publish (2.x pattern)"

# ============================================================
# TEST 6: Init command includes versions config
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Init command generates versions config${NC}"

assert_file_contains "$INIT_CMD" "versions:" "Init command generates versions block"
assert_file_contains "$INIT_CMD" "openapi:" "Init documents openapi version config"
assert_file_contains "$INIT_CMD" "asyncapi:" "Init documents asyncapi version config"
assert_file_contains "$INIT_CMD" "json_schema:" "Init documents json_schema version config"

# ============================================================
# TEST 7: Website documentation updated
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Website documentation${NC}"

assert_file_exists "$WEBSITE_ARTIFACTS" "Website artifacts page exists"
assert_file_contains "$WEBSITE_ARTIFACTS" "model.mmd" "Website documents model.mmd"
assert_file_contains "$WEBSITE_ARTIFACTS" "JSONB" "Website documents JSONB support"
assert_file_contains "$WEBSITE_ARTIFACTS" "jsonb-aggregate" "Website documents jsonb-aggregate strategy"
assert_file_contains "$WEBSITE_ARTIFACTS" "Storage strategy" "Website documents storage strategy"
assert_file_contains "$WEBSITE_ARTIFACTS" "domain class diagram" "Website documents domain class diagram"
assert_file_contains "$WEBSITE_ARTIFACTS" "artifacts.versions" "Website documents version config"
assert_file_contains "$WEBSITE_ARTIFACTS" "3.1.0" "Website shows OpenAPI 3.1.0 default"
assert_file_contains "$WEBSITE_ARTIFACTS" "3.0.0" "Website shows AsyncAPI 3.0.0 default"
assert_file_contains "$WEBSITE_ARTIFACTS" "2020-12" "Website shows JSON Schema 2020-12 default"

# ============================================================
# TEST 8: Changelog updated
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Changelog${NC}"

assert_file_contains "$CHANGELOG" "v0.3.1" "Changelog has v0.3.1 entry"
assert_file_contains "$CHANGELOG" "JSONB" "Changelog documents JSONB support"
assert_file_contains "$CHANGELOG" "model.mmd" "Changelog documents model.mmd"
assert_file_contains "$CHANGELOG" "Configurable artifact spec versions" "Changelog documents version config"
assert_file_contains "$CHANGELOG" "Storage strategy detection" "Changelog documents storage strategy"

# ============================================================
# TEST 9: Parity — model.mmd in detection table and routing
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: model.mmd parity across command sections${NC}"

# model.mmd must appear in: detection table, routing example, confirmation output, lookup table
assert_file_contains "$ARTIFACTS_CMD" "routing to architect.*model.mmd" "model.mmd in routing example"

# model.mmd in confirmation section
# Use grep with context to verify it's in the confirmation block
if grep -A2 "model.mmd" "$ARTIFACTS_CMD" | grep -q "domain class diagram"; then
    pass "model.mmd labeled as domain class diagram"
else
    fail "model.mmd not labeled as domain class diagram"
fi

# model.mmd in reference lookup table
if grep -B2 -A2 "model.mmd" "$ARTIFACTS_CMD" | grep -qi "any"; then
    pass "model.mmd in lookup table (any DB_TYPE)"
else
    fail "model.mmd not in lookup table for any DB_TYPE"
fi

test_summary
