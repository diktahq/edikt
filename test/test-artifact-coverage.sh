#!/bin/bash
# Test: spec-artifacts → plan artifact coverage flow
# Verifies the plan command's artifact coverage check (step 6b) handles
# all artifact types, edge cases, and the original bug scenario.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

FIXTURES_DIR="/tmp/edikt-artifact-coverage-$$"
trap 'rm -rf "$FIXTURES_DIR"' EXIT

echo ""

# ============================================================
# Fixture setup — simulate a spec directory with artifacts
# ============================================================

setup_spec_dir() {
    local dir="$1"
    rm -rf "$dir"
    mkdir -p "$dir/contracts" "$dir/migrations"
}

# ============================================================
# TEST 1: Full artifact set — the original bug scenario
# All artifact types present, plan must cover each
# ============================================================

echo -e "${BOLD}TEST 1: Full artifact set (original bug scenario)${NC}"

SPEC_DIR="$FIXTURES_DIR/test1/docs/product/specs/SPEC-003-kollekta"
setup_spec_dir "$SPEC_DIR"

# Create spec
cat > "$SPEC_DIR/spec.md" << 'MD'
---
type: spec
id: SPEC-003
title: Kollekta Redesign
status: accepted
---

# SPEC-003: Kollekta Redesign

## Acceptance Criteria

- AC-001: Problem space API returns collections
- AC-002: AI endpoint processes natural language queries
MD

# Create all artifact types
cat > "$SPEC_DIR/fixtures.yaml" << 'YAML'
# Design blueprint
# edikt:artifact type=fixtures spec=SPEC-003 status=accepted
scenarios:
  - name: basic_collection
    tables:
      - users: [{id: 1, name: "Test"}]
YAML

cat > "$SPEC_DIR/fixtures-solution.yaml" << 'YAML'
# Design blueprint
# edikt:artifact type=fixtures spec=SPEC-003 status=accepted
scenarios:
  - name: solution_data
    tables:
      - solutions: [{id: 1, title: "Test Solution"}]
YAML

cat > "$SPEC_DIR/test-strategy.md" << 'MD'
---
artifact_type: test-strategy
status: accepted
reviewed_by: qa
---

# Test Strategy

## Unit Tests
- Repository layer: test CRUD operations
- Service layer: test business logic

## Integration Tests
- API endpoints: test full request/response cycle
- Database: test migrations and seed data

## Edge Cases
- Empty collections
- AI endpoint with malformed input
MD

cat > "$SPEC_DIR/contracts/api.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/collections:
    get:
      summary: List collections
    post:
      summary: Create collection
  /api/v1/collections/{id}:
    get:
      summary: Get collection
    put:
      summary: Update collection
    delete:
      summary: Delete collection
YAML

cat > "$SPEC_DIR/contracts/api-ai.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/ai/ask:
    post:
      summary: Process AI query
YAML

cat > "$SPEC_DIR/contracts/api-solution.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/solutions:
    get:
      summary: List solutions
    post:
      summary: Create solution
YAML

cat > "$SPEC_DIR/contracts/events.yaml" << 'YAML'
asyncapi: "2.6.0"
channels:
  collection.created:
    publish:
      summary: Collection created event
  solution.evaluated:
    publish:
      summary: Solution evaluated event
YAML

cat > "$SPEC_DIR/migrations/001_problem_space.sql" << 'SQL'
-- Design blueprint
-- UP
CREATE TABLE collections (id UUID PRIMARY KEY, name TEXT);
-- DOWN
DROP TABLE collections;
SQL

cat > "$SPEC_DIR/migrations/002_solution_space.sql" << 'SQL'
-- Design blueprint
-- UP
CREATE TABLE solutions (id UUID PRIMARY KEY, title TEXT);
-- DOWN
DROP TABLE solutions;
SQL

cat > "$SPEC_DIR/data-model.mmd" << 'MMD'
%% Design blueprint
erDiagram
    COLLECTION {
        uuid id PK
        text name
    }
MMD

# Verify all artifacts exist
ARTIFACT_COUNT=$(find "$SPEC_DIR" -type f ! -name "spec.md" | wc -l | tr -d ' ')
if [ "$ARTIFACT_COUNT" -eq 10 ]; then
    pass "Full artifact set: 10 files created"
else
    fail "Expected 10 artifacts, found $ARTIFACT_COUNT"
fi

# Verify the plan command has coverage instructions for each type
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "fixtures" "Plan covers fixtures type"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "test-strategy" "Plan covers test-strategy type"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "contracts/api" "Plan covers API contracts"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "contracts/events" "Plan covers event contracts"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "migrations" "Plan covers migrations"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "data-model" "Plan covers data models"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "config-spec" "Plan covers config spec"

# ============================================================
# TEST 2: Fixtures only — no API contracts
# Plan should still add seeding phase
# ============================================================

echo ""
echo -e "${BOLD}TEST 2: Fixtures only (no contracts)${NC}"

SPEC_DIR2="$FIXTURES_DIR/test2/docs/product/specs/SPEC-004-simple"
setup_spec_dir "$SPEC_DIR2"

cat > "$SPEC_DIR2/spec.md" << 'MD'
---
type: spec
id: SPEC-004
status: accepted
---
# SPEC-004: Simple Feature
MD

cat > "$SPEC_DIR2/fixtures.yaml" << 'YAML'
# edikt:artifact type=fixtures
scenarios:
  - name: seed
    tables:
      - users: [{id: 1}]
YAML

ARTIFACT_COUNT=$(find "$SPEC_DIR2" -type f ! -name "spec.md" | wc -l | tr -d ' ')
if [ "$ARTIFACT_COUNT" -eq 1 ]; then
    pass "Fixtures-only: 1 artifact"
else
    fail "Expected 1 artifact, found $ARTIFACT_COUNT"
fi

# Plan instruction covers this case
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Database seeding" "Plan adds seeding phase for standalone fixtures"

# ============================================================
# TEST 3: API contracts only — no fixtures, no tests
# Plan should still verify endpoint coverage
# ============================================================

echo ""
echo -e "${BOLD}TEST 3: API contracts only${NC}"

SPEC_DIR3="$FIXTURES_DIR/test3/docs/product/specs/SPEC-005-api-only"
setup_spec_dir "$SPEC_DIR3"

cat > "$SPEC_DIR3/spec.md" << 'MD'
---
type: spec
id: SPEC-005
status: accepted
---
# SPEC-005: API Only
MD

cat > "$SPEC_DIR3/contracts/api.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/items:
    get:
      summary: List items
    post:
      summary: Create item
  /api/v1/items/{id}:
    get:
      summary: Get item
YAML

ENDPOINT_COUNT=$(grep -c "summary:" "$SPEC_DIR3/contracts/api.yaml")
if [ "$ENDPOINT_COUNT" -eq 3 ]; then
    pass "API-only: 3 endpoints defined"
else
    fail "Expected 3 endpoints, found $ENDPOINT_COUNT"
fi

# Plan instruction covers endpoint verification
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "path + method" "Plan checks path+method pairs"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Uncovered API endpoints" "Plan warns on uncovered endpoints"

# ============================================================
# TEST 4: Multiple API contract files — all must be checked
# ============================================================

echo ""
echo -e "${BOLD}TEST 4: Multiple API contract files${NC}"

SPEC_DIR4="$FIXTURES_DIR/test4/docs/product/specs/SPEC-006-multi-api"
setup_spec_dir "$SPEC_DIR4"

cat > "$SPEC_DIR4/spec.md" << 'MD'
---
type: spec
id: SPEC-006
status: accepted
---
# SPEC-006: Multi-API
MD

cat > "$SPEC_DIR4/contracts/api.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/users:
    get:
      summary: List users
YAML

cat > "$SPEC_DIR4/contracts/api-admin.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/admin/users:
    get:
      summary: List users (admin)
    delete:
      summary: Delete user (admin)
YAML

cat > "$SPEC_DIR4/contracts/api-webhooks.yaml" << 'YAML'
openapi: "3.0.0"
paths:
  /api/v1/webhooks:
    post:
      summary: Register webhook
  /api/v1/webhooks/{id}:
    delete:
      summary: Remove webhook
YAML

CONTRACT_COUNT=$(ls "$SPEC_DIR4/contracts/api"*.yaml 2>/dev/null | wc -l | tr -d ' ')
if [ "$CONTRACT_COUNT" -eq 3 ]; then
    pass "Multi-API: 3 contract files"
else
    fail "Expected 3 contract files, found $CONTRACT_COUNT"
fi

TOTAL_ENDPOINTS=$(grep -h "summary:" "$SPEC_DIR4"/contracts/api*.yaml | wc -l | tr -d ' ')
if [ "$TOTAL_ENDPOINTS" -eq 5 ]; then
    pass "Multi-API: 5 total endpoints across files"
else
    fail "Expected 5 total endpoints, found $TOTAL_ENDPOINTS"
fi

# Plan instruction uses glob pattern for multiple api files
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "contracts/api\*.yaml" "Plan uses glob for multiple API contracts"

# ============================================================
# TEST 5: Test strategy with multiple categories
# Each category must map to a phase
# ============================================================

echo ""
echo -e "${BOLD}TEST 5: Test strategy categories${NC}"

SPEC_DIR5="$FIXTURES_DIR/test5/docs/product/specs/SPEC-007-tests"
setup_spec_dir "$SPEC_DIR5"

cat > "$SPEC_DIR5/spec.md" << 'MD'
---
type: spec
id: SPEC-007
status: accepted
---
# SPEC-007: Test Coverage
MD

cat > "$SPEC_DIR5/test-strategy.md" << 'MD'
---
artifact_type: test-strategy
status: accepted
reviewed_by: qa
---

# Test Strategy

## Unit Tests
- Model validation
- Service methods

## Integration Tests
- API round-trips
- Database queries

## E2E Tests
- Full user workflow
- Error scenarios

## Edge Cases
- Empty inputs
- Rate limiting
- Concurrent requests

## Performance Tests
- Load testing at 1000 RPS
- Response time P99 < 200ms
MD

CATEGORY_COUNT=$(grep -c "^## " "$SPEC_DIR5/test-strategy.md")
if [ "$CATEGORY_COUNT" -eq 5 ]; then
    pass "Test strategy: 5 categories"
else
    fail "Expected 5 test categories, found $CATEGORY_COUNT"
fi

# Plan instruction requires each category to map
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "Every test category must map to at least one phase" "Plan requires per-category mapping"

# ============================================================
# TEST 6: Data model only — reference, no phase needed
# ============================================================

echo ""
echo -e "${BOLD}TEST 6: Data model only (reference artifact)${NC}"

SPEC_DIR6="$FIXTURES_DIR/test6/docs/product/specs/SPEC-008-diagram"
setup_spec_dir "$SPEC_DIR6"

cat > "$SPEC_DIR6/spec.md" << 'MD'
---
type: spec
id: SPEC-008
status: accepted
---
# SPEC-008: Diagram Only
MD

cat > "$SPEC_DIR6/data-model.mmd" << 'MMD'
erDiagram
    USER {
        uuid id PK
        text name
    }
MMD

# Plan explicitly says data-model is reference only
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "reference only" "Plan marks data-model as reference only"

# ============================================================
# TEST 7: Empty spec directory — no artifacts, no coverage check
# ============================================================

echo ""
echo -e "${BOLD}TEST 7: Empty spec directory (no artifacts)${NC}"

SPEC_DIR7="$FIXTURES_DIR/test7/docs/product/specs/SPEC-009-empty"
setup_spec_dir "$SPEC_DIR7"

cat > "$SPEC_DIR7/spec.md" << 'MD'
---
type: spec
id: SPEC-009
status: accepted
---
# SPEC-009: No Artifacts
MD

ARTIFACT_COUNT=$(find "$SPEC_DIR7" -type f ! -name "spec.md" | wc -l | tr -d ' ')
if [ "$ARTIFACT_COUNT" -eq 0 ]; then
    pass "Empty spec: 0 artifacts"
else
    fail "Expected 0 artifacts, found $ARTIFACT_COUNT"
fi

# Plan only runs coverage check when artifacts exist (step 3 inventory)
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "only when a SPEC was resolved and artifacts were inventoried" "Plan skips coverage when no artifacts"

# ============================================================
# TEST 8: Events contract — producer and consumer phases
# ============================================================

echo ""
echo -e "${BOLD}TEST 8: Events contract (producer + consumer)${NC}"

SPEC_DIR8="$FIXTURES_DIR/test8/docs/product/specs/SPEC-010-events"
setup_spec_dir "$SPEC_DIR8"

cat > "$SPEC_DIR8/spec.md" << 'MD'
---
type: spec
id: SPEC-010
status: accepted
---
# SPEC-010: Event-Driven
MD

cat > "$SPEC_DIR8/contracts/events.yaml" << 'YAML'
asyncapi: "2.6.0"
channels:
  order.created:
    publish:
      summary: Order created event
  order.shipped:
    publish:
      summary: Order shipped event
  payment.received:
    publish:
      summary: Payment received event
YAML

EVENT_COUNT=$(grep -c "summary:" "$SPEC_DIR8/contracts/events.yaml")
if [ "$EVENT_COUNT" -eq 3 ]; then
    pass "Events: 3 events defined"
else
    fail "Expected 3 events, found $EVENT_COUNT"
fi

# Plan requires producer and consumer for each event
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "producing phase and a consuming phase" "Plan requires producer+consumer for events"

# ============================================================
# TEST 9: Config spec artifact
# ============================================================

echo ""
echo -e "${BOLD}TEST 9: Config spec artifact${NC}"

SPEC_DIR9="$FIXTURES_DIR/test9/docs/product/specs/SPEC-011-config"
setup_spec_dir "$SPEC_DIR9"

cat > "$SPEC_DIR9/spec.md" << 'MD'
---
type: spec
id: SPEC-011
status: accepted
---
# SPEC-011: With Config
MD

cat > "$SPEC_DIR9/config-spec.md" << 'MD'
---
artifact_type: config-spec
status: accepted
---

# Configuration

## Environment Variables
- DATABASE_URL — PostgreSQL connection string
- REDIS_URL — Cache connection
- AI_API_KEY — AI service key

## Feature Flags
- ENABLE_AI — toggle AI features
MD

assert_file_exists "$SPEC_DIR9/config-spec.md" "Config spec artifact exists"
assert_file_contains "$PROJECT_ROOT/commands/sdlc/plan.md" "config-spec" "Plan covers config-spec artifacts"

# ============================================================
# TEST 10: Plan command structural checks
# ============================================================

echo ""
echo -e "${BOLD}TEST 10: Plan command structural integrity${NC}"

PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"

# Artifact inventory happens in step 3 (governance chain)
assert_file_contains "$PLAN_CMD" "Inventory all artifacts" "Step 3 inventories artifacts"

# Coverage check is step 6b (after phase generation)
assert_file_contains "$PLAN_CMD" "Artifact coverage check" "Step 6b checks coverage"

# Coverage runs only for SPEC-based plans
assert_file_contains "$PLAN_CMD" "only when a SPEC was resolved" "Coverage only for SPEC plans"

# Coverage table in Reference section
assert_file_contains "$PLAN_CMD" "Artifact Coverage Table" "Reference has coverage table"

# CRITICAL warning about silent failures
assert_file_contains "$PLAN_CMD" "silent failures" "Plan warns about silent failures from uncovered artifacts"

# Coverage report format
assert_file_contains "$PLAN_CMD" "Artifact coverage:" "Plan outputs coverage report"

# ============================================================
# TEST 11: Final validation gate
# ============================================================

echo ""
echo -e "${BOLD}TEST 11: Final validation gate${NC}"

# Plan has step 6c — final artifact validation
assert_file_contains "$PLAN_CMD" "Final artifact validation" "Plan has final validation step (6c)"
assert_file_contains "$PLAN_CMD" "ARTIFACT COVERAGE INCOMPLETE" "Plan blocks on incomplete coverage"
assert_file_contains "$PLAN_CMD" "cannot be written until all artifacts are covered" "Plan refuses to write with gaps"
assert_file_contains "$PLAN_CMD" "Deferred Artifacts" "Plan allows deferring artifacts explicitly"
assert_file_contains "$PLAN_CMD" "All spec artifacts have plan coverage" "Plan confirms full coverage"

# ============================================================
# TEST 12: The golden rule — no artifact left behind
# Verify every artifact type in spec-artifacts matches plan coverage
# ============================================================

echo ""
echo -e "${BOLD}TEST 11: Artifact type parity (spec-artifacts ↔ plan)${NC}"

SPEC_ARTIFACTS_CMD="$PROJECT_ROOT/commands/sdlc/artifacts.md"
PLAN_CMD="$PROJECT_ROOT/commands/sdlc/plan.md"

# Every artifact type that spec-artifacts generates must appear in plan's coverage table
for artifact_type in "fixtures" "test-strategy" "api.yaml" "events.yaml" "migrations" "data-model"; do
    if grep -q "$artifact_type" "$PLAN_CMD" 2>/dev/null; then
        pass "Plan covers artifact type: $artifact_type"
    else
        fail "Plan missing coverage for artifact type: $artifact_type"
    fi
done

# spec-artifacts mentions these artifacts — plan must too
for keyword in "fixtures" "test-strategy" "OpenAPI" "AsyncAPI" "migration" "data-model" "config-spec"; do
    if grep -qi "$keyword" "$SPEC_ARTIFACTS_CMD" 2>/dev/null; then
        if grep -qi "$keyword" "$PLAN_CMD" 2>/dev/null; then
            pass "Parity: $keyword in both spec-artifacts and plan"
        else
            fail "Parity gap: $keyword in spec-artifacts but missing in plan"
        fi
    fi
done

test_summary
