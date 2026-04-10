#!/bin/bash
# Test: v0.3.0 Phase 3 — init style detection + Adapt mode
#
# Guards the Phase 3 decisions from PROPOSAL-001 / file-changes.md:
#   1. init.md documents the Adapt mode flow for all three artifact types
#   2. Three-choice prompt (Adapt / Start fresh / Write my own) documented per type
#   3. Inconsistent style fallback documented (team template / draft from majority / pick reference)
#   4. Re-run protection (skip if template exists, --reset-templates flag)
#   5. Grandfather flow for v0.2.x projects documented
#   6. All three new.md files enforce template-less refusal when edikt_version >= 0.3.0
#   7. All three new.md files fall back to inline template for v0.2.x legacy projects
#   8. All three new.md files reference /edikt:init in the refusal message
#   9. Summary of generated templates printed after all three sub-flows
#  10. Writing guidance from ADR-009 is always included in Adapt-mode invariant templates
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

INIT_MD="$PROJECT_ROOT/commands/init.md"
ADR_NEW="$PROJECT_ROOT/commands/adr/new.md"
INVARIANT_NEW="$PROJECT_ROOT/commands/invariant/new.md"
GUIDELINE_NEW="$PROJECT_ROOT/commands/guideline/new.md"

# ============================================================
# Contract 1: init.md has the Phase 3 template section
# ============================================================

assert_file_contains "$INIT_MD" "### 2b. Project Templates" \
    "init.md has section 2b Project Templates"
assert_file_contains "$INIT_MD" "Adapt mode" \
    "init.md references Adapt mode"
assert_file_contains "$INIT_MD" "ADR-005" \
    "init.md references ADR-005 for the extensibility model"
assert_file_contains "$INIT_MD" "ADR-009" \
    "init.md references ADR-009 for Invariant Record coinage"

# ============================================================
# Contract 2: Three sub-sections for ADR, Invariant Record, Guideline
# ============================================================

assert_file_contains "$INIT_MD" "#### 2b.1 ADR template" \
    "init.md has 2b.1 ADR template sub-section"
assert_file_contains "$INIT_MD" "#### 2b.2 Invariant Record template" \
    "init.md has 2b.2 Invariant Record template sub-section"
assert_file_contains "$INIT_MD" "#### 2b.3 Guideline template" \
    "init.md has 2b.3 Guideline template sub-section"
assert_file_contains "$INIT_MD" "#### 2b.4 Summary" \
    "init.md has 2b.4 Summary sub-section"

# ============================================================
# Contract 3: Three-choice prompt per artifact type
# ============================================================

# ADR three-choice prompt — brackets are regex metacharacters, use grep -F
if grep -qF '[1] Adapt' "$INIT_MD"; then
    pass "init.md documents [1] Adapt option in ADR template prompt"
else
    fail "init.md documents [1] Adapt option in ADR template prompt" \
        "Pattern '[1] Adapt' not found"
fi
if grep -qF '[2] Start fresh' "$INIT_MD"; then
    pass "init.md documents [2] Start fresh option"
else
    fail "init.md documents [2] Start fresh option" \
        "Pattern '[2] Start fresh' not found"
fi
if grep -qF '[3] Write my own' "$INIT_MD"; then
    pass "init.md documents [3] Write my own option"
else
    fail "init.md documents [3] Write my own option" \
        "Pattern '[3] Write my own' not found"
fi

# The three choices must be documented for each artifact type (ADR, Invariant, Guideline)
# Count occurrences of each option — should be at least 3 (one per artifact type)
# In practice the prompt text may only be shown in full for ADR with "(same pattern)" for others
# So we check for explicit mention of "Adapt" in each sub-section
ADR_SECTION=$(awk '/^#### 2b\.1 ADR template/,/^#### 2b\.2/' "$INIT_MD")
INV_SECTION=$(awk '/^#### 2b\.2 Invariant Record template/,/^#### 2b\.3/' "$INIT_MD")
GUIDE_SECTION=$(awk '/^#### 2b\.3 Guideline template/,/^#### 2b\.4/' "$INIT_MD")

if echo "$ADR_SECTION" | grep -qF 'Adapt'; then
    pass "ADR sub-section offers Adapt mode"
else
    fail "ADR sub-section offers Adapt mode" "Missing from 2b.1"
fi
if echo "$INV_SECTION" | grep -qF 'Adapt'; then
    pass "Invariant Record sub-section offers Adapt mode"
else
    fail "Invariant Record sub-section offers Adapt mode" "Missing from 2b.2"
fi
if echo "$GUIDE_SECTION" | grep -qF 'Adapt'; then
    pass "Guideline sub-section offers Adapt mode"
else
    fail "Guideline sub-section offers Adapt mode" "Missing from 2b.3"
fi

# ============================================================
# Contract 4: Structural analysis documented (frontmatter, sections, heading levels)
# ============================================================

assert_file_contains "$INIT_MD" "Analyze structural pattern" \
    "init.md documents structural pattern analysis"
assert_file_contains "$INIT_MD" "frontmatter" \
    "init.md analyzes frontmatter presence and fields"
assert_file_contains "$INIT_MD" "heading levels" \
    "init.md analyzes heading levels"
assert_file_contains "$INIT_MD" "section names" \
    "init.md analyzes section names"

# ============================================================
# Contract 5: Inconsistent style fallback documented
# ============================================================

assert_file_contains "$INIT_MD" "inconsistent styles" \
    "init.md documents inconsistent style detection"
assert_file_contains "$INIT_MD" "team template" \
    "init.md offers existing team template option"
assert_file_contains "$INIT_MD" "draft a template from the majority style" \
    "init.md offers 'draft from majority' option"

# ============================================================
# Contract 6: Re-run protection (--reset-templates flag)
# ============================================================

assert_file_contains "$INIT_MD" "Re-run protection" \
    "init.md has Re-run protection section"
if grep -qF -- "--reset-templates" "$INIT_MD"; then
    pass "init.md documents --reset-templates flag"
else
    fail "init.md documents --reset-templates flag" \
        "Pattern '--reset-templates' not found in init.md"
fi
assert_file_contains "$INIT_MD" "already exists" \
    "init.md explains skip-if-exists behavior"

# ============================================================
# Contract 7: Grandfather flow for v0.2.x projects
# ============================================================

assert_file_contains "$INIT_MD" "### 2c. Grandfather flow" \
    "init.md has grandfather flow section"
assert_file_contains "$INIT_MD" "v0.2.x" \
    "init.md references v0.2.x legacy projects"
assert_file_contains "$INIT_MD" "edikt_version" \
    "init.md reads edikt_version from config for grandfather detection"

# ============================================================
# Contract 8: Reference templates referenced correctly
# ============================================================

assert_file_contains "$INIT_MD" "adr-nygard-minimal" \
    "init.md references adr-nygard-minimal template"
assert_file_contains "$INIT_MD" "adr-madr-extended" \
    "init.md references adr-madr-extended template"
assert_file_contains "$INIT_MD" "invariant-minimal" \
    "init.md references invariant-minimal template"
assert_file_contains "$INIT_MD" "invariant-full" \
    "init.md references invariant-full template"
assert_file_contains "$INIT_MD" "guideline-minimal" \
    "init.md references guideline-minimal template"
assert_file_contains "$INIT_MD" "guideline-extended" \
    "init.md references guideline-extended template"

# ============================================================
# Contract 9: Sentinel block always present in generated templates
# ============================================================

if grep -A4 "Adapt mode" "$INIT_MD" | grep -qF 'edikt:directives:start'; then
    pass "init.md Adapt mode ensures directives sentinel block is present"
else
    # Alternative phrasing check
    assert_file_contains "$INIT_MD" "sentinel block at the end" \
        "init.md Adapt mode ensures sentinel block in generated templates"
fi

# ============================================================
# Contract 10: Invariant template always includes ADR-009 writing guidance
# ============================================================

if echo "$INV_SECTION" | grep -qF 'writing guidance'; then
    pass "Invariant Adapt mode always includes ADR-009 writing guidance"
else
    fail "Invariant Adapt mode always includes ADR-009 writing guidance" \
        "Sub-section 2b.2 must mention that writing guidance is added even if not in detected style"
fi

if echo "$GUIDE_SECTION" | grep -qF 'MUST or NEVER'; then
    pass "Guideline Adapt mode always requires MUST/NEVER language"
else
    fail "Guideline Adapt mode always requires MUST/NEVER language" \
        "Sub-section 2b.3 must mention that MUST/NEVER is the hard contract"
fi

# ============================================================
# Contract 11: Template-less refusal in new.md files (conditional on edikt_version)
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")

    assert_file_contains "$file" "Refuse (v0.3.0+" \
        "$name/new.md has Refuse section for v0.3.0+ projects"
    assert_file_contains "$file" "legacy inline fallback" \
        "$name/new.md documents legacy inline fallback for v0.2.x projects"
    assert_file_contains "$file" "/edikt:init" \
        "$name/new.md points at /edikt:init in the refusal message"
    # Use grep -F -- directly because assert_file_contains can't handle patterns
    # starting with `--` (grep treats them as flags).
    if grep -qF -- "--reset-templates" "$file"; then
        pass "$name/new.md mentions --reset-templates in the refusal message"
    else
        fail "$name/new.md mentions --reset-templates in the refusal message" \
            "Pattern '--reset-templates' not found"
    fi
    assert_file_contains "$file" "PROJECT_EDIKT_VERSION" \
        "$name/new.md documents how to check the project edikt_version"
    assert_file_contains "$file" "Compare to \`0.3.0\`" \
        "$name/new.md compares edikt_version against 0.3.0"
done

# ============================================================
# Contract 12: Refusal is a hard stop — does NOT fall back to inline
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    if grep -qF "Do NOT fall back to" "$file"; then
        pass "$name/new.md explicitly states 'Do NOT fall back to inline' in refusal"
    else
        fail "$name/new.md explicitly states 'Do NOT fall back to inline' in refusal" \
            "Refusal must be a hard stop, not a fallback"
    fi
done

# ============================================================
# Contract 13: Each new.md refusal message references the template contract
# ============================================================

assert_file_contains "$ADR_NEW" "sentinel block" \
    "adr/new.md refusal mentions sentinel block requirement"
assert_file_contains "$INVARIANT_NEW" "invariant-record-template.md" \
    "invariant/new.md refusal references authoritative template"
assert_file_contains "$INVARIANT_NEW" "writing-invariants-guide.md" \
    "invariant/new.md refusal references writing guide"
assert_file_contains "$GUIDELINE_NEW" "MUST/NEVER" \
    "guideline/new.md refusal mentions MUST/NEVER requirement"

test_summary
