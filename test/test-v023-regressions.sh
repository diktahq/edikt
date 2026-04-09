#!/bin/bash
# Test: v0.2.3 — compile schema version (ADR-007)
#
# Guards the decisions in ADR-007:
#   1. commands/gov/compile.md declares COMPILE_SCHEMA_VERSION as a constant
#   2. compile.md's output templates emit compile_schema_version in YAML
#      frontmatter (not the legacy 'version' field)
#   3. compiled_by and compiled_at stay as HTML comments (not frontmatter)
#   4. commands/doctor.md checks compile_schema_version, not edikt version
#   5. The dogfood .claude/rules/governance.md uses the new format
#   6. An ADR-007 document exists describing the decision
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

COMPILE_MD="$PROJECT_ROOT/commands/gov/compile.md"
DOCTOR_MD="$PROJECT_ROOT/commands/doctor.md"
GOV_DOGFOOD="$PROJECT_ROOT/.claude/rules/governance.md"
ADR_007="$PROJECT_ROOT/docs/architecture/decisions/ADR-007-compile-schema-version.md"

# ============================================================
# Regression 1: ADR-007 exists and is accepted
# ============================================================

assert_file_exists "$ADR_007" "ADR-007 exists"
if grep -qF 'Status:** Accepted' "$ADR_007"; then
    pass "ADR-007 is Accepted"
else
    fail "ADR-007 is Accepted" "Status line not 'Accepted'"
fi
assert_file_contains "$ADR_007" "compile_schema_version" \
    "ADR-007 describes compile_schema_version"

# ============================================================
# Regression 2: commands/gov/compile.md declares the constant
# ============================================================

assert_file_contains "$COMPILE_MD" "COMPILE_SCHEMA_VERSION" \
    "compile.md declares COMPILE_SCHEMA_VERSION constant"

# Must be a positive integer
if grep -oE 'COMPILE_SCHEMA_VERSION = [0-9]+' "$COMPILE_MD" | grep -qE '= [1-9][0-9]*$'; then
    pass "COMPILE_SCHEMA_VERSION is a positive integer"
else
    fail "COMPILE_SCHEMA_VERSION is a positive integer" \
        "$(grep 'COMPILE_SCHEMA_VERSION' "$COMPILE_MD" | head -1)"
fi

# ============================================================
# Regression 3: compile.md output templates use the new format
# ============================================================

# The index template (governance.md) must emit compile_schema_version
if grep -A5 'Write the governance index' "$COMPILE_MD" | grep -q 'compile_schema_version'; then
    pass "compile.md index template emits compile_schema_version in frontmatter"
else
    fail "compile.md index template emits compile_schema_version in frontmatter" \
        "Index template missing compile_schema_version field"
fi

# The topic file template must emit compile_schema_version
if grep -B2 -A10 'Each file follows this format' "$COMPILE_MD" | grep -q 'compile_schema_version'; then
    pass "compile.md topic template emits compile_schema_version in frontmatter"
else
    fail "compile.md topic template emits compile_schema_version in frontmatter" \
        "Topic template missing compile_schema_version field"
fi

# compiled_by MUST be in HTML comment, not YAML
if grep -q '<!-- compiled_by: edikt v' "$COMPILE_MD"; then
    pass "compile.md emits compiled_by as HTML comment"
else
    fail "compile.md emits compiled_by as HTML comment" \
        "compiled_by should be in <!-- --> not YAML frontmatter"
fi

# compiled_at MUST be in HTML comment, not YAML
if grep -q '<!-- compiled_at:' "$COMPILE_MD"; then
    pass "compile.md emits compiled_at as HTML comment"
else
    fail "compile.md emits compiled_at as HTML comment" \
        "compiled_at should be in <!-- --> not YAML frontmatter"
fi

# compile.md must NOT emit the legacy `version: "{edikt_version}"` field
if grep -q 'version: "{edikt_version}"' "$COMPILE_MD"; then
    fail "compile.md no longer emits legacy version: \"{edikt_version}\"" \
        "Legacy version field still in compile output template"
else
    pass "compile.md no longer emits legacy version: \"{edikt_version}\""
fi

# ============================================================
# Regression 4: doctor.md checks compile_schema_version
# ============================================================

assert_file_contains "$DOCTOR_MD" "compile_schema_version" \
    "doctor.md checks compile_schema_version"

assert_file_contains "$DOCTOR_MD" "ADR-007" \
    "doctor.md references ADR-007"

# doctor.md must document the three cases (missing, older, newer)
assert_file_contains "$DOCTOR_MD" "legacy version stamp" \
    "doctor.md handles missing compile_schema_version (legacy) case"

assert_file_contains "$DOCTOR_MD" "regenerate" \
    "doctor.md recommends regeneration on schema mismatch"

# doctor.md MUST document that compiled_by and compiled_at are informational
assert_file_contains "$DOCTOR_MD" "informational only" \
    "doctor.md documents that compiled_by/compiled_at are informational"

# ============================================================
# Regression 5: dogfood governance.md uses the new format
# ============================================================

assert_file_contains "$GOV_DOGFOOD" "compile_schema_version:" \
    "dogfood governance.md declares compile_schema_version"

# Must NOT have the legacy version field in YAML frontmatter
if awk '/^---$/{c++; next} c==1' "$GOV_DOGFOOD" | grep -qE '^version:'; then
    fail "dogfood governance.md has no legacy 'version:' in frontmatter" \
        "Legacy field still present"
else
    pass "dogfood governance.md has no legacy 'version:' in frontmatter"
fi

# compiled_by and compiled_at should be present as HTML comments
assert_file_contains "$GOV_DOGFOOD" "<!-- compiled_by:" \
    "dogfood governance.md has compiled_by HTML comment"

assert_file_contains "$GOV_DOGFOOD" "<!-- compiled_at:" \
    "dogfood governance.md has compiled_at HTML comment"

# Schema version must match the constant in compile.md
SCHEMA_CONST=$(grep -oE 'COMPILE_SCHEMA_VERSION = [0-9]+' "$COMPILE_MD" | awk '{print $3}' | head -1)
DOGFOOD_SCHEMA=$(grep -oE '^compile_schema_version: [0-9]+' "$GOV_DOGFOOD" | awk '{print $2}' | head -1)

if [ "$DOGFOOD_SCHEMA" = "$SCHEMA_CONST" ]; then
    pass "dogfood governance schema ($DOGFOOD_SCHEMA) matches compile.md constant ($SCHEMA_CONST)"
else
    fail "dogfood governance schema matches compile.md constant" \
        "dogfood=$DOGFOOD_SCHEMA, constant=$SCHEMA_CONST"
fi

# ============================================================
# Regression 6: upgrade.md handles the new schema version
# ============================================================

UPGRADE_MD="$PROJECT_ROOT/commands/upgrade.md"

# upgrade.md should recommend recompiling when schema is out of date
# (Loose check — this is documentation, not enforcement logic)
if grep -q 'compile_schema_version' "$UPGRADE_MD" 2>/dev/null; then
    pass "upgrade.md mentions compile_schema_version"
else
    fail "upgrade.md mentions compile_schema_version" \
        "Upgrade should guide users through schema migrations"
fi

test_summary
