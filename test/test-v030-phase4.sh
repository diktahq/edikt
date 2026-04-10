#!/bin/bash
# Test: v0.3.0 Phase 4 — flexible prose input with reference extraction
#
# Guards the Phase 4 decisions from PROPOSAL-001 / file-changes.md:
#   1. All three <artifact>:new commands document the prose-first dispatch
#   2. Reference extraction for paths, identifiers, branches documented
#   3. Mixed input handling (prose with embedded refs) documented
#   4. Empty argument → conversation context fallback documented
#   5. Same pattern as /edikt:sdlc:plan from v0.1.3
#   6. "Do NOT classify into rigid types" principle stated
#   7. Primary sources (resolved refs) dominate; framing is secondary
#   8. Invariant prose extraction respects "constraint vs implementation" rule
#   9. Guideline prose extraction extracts MUST/NEVER rules from source pool
#  10. Examples table present in each command showing the dispatch matrix
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

ADR_NEW="$PROJECT_ROOT/commands/adr/new.md"
INVARIANT_NEW="$PROJECT_ROOT/commands/invariant/new.md"
GUIDELINE_NEW="$PROJECT_ROOT/commands/guideline/new.md"

# ============================================================
# Contract 1: Prose-first dispatch documented in all three new.md files
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "flexible prose input" \
        "$name/new.md documents 'flexible prose input'"
    assert_file_contains "$file" "reference extraction" \
        "$name/new.md documents reference extraction"
    assert_file_contains "$file" "prose first" \
        "$name/new.md states input is 'prose first'"
done

# guideline/new.md uses slightly different phrasing in the topic section
assert_file_contains "$GUIDELINE_NEW" "flexible prose input" \
    "guideline/new.md documents 'flexible prose input'"
assert_file_contains "$GUIDELINE_NEW" "reference extraction" \
    "guideline/new.md documents reference extraction"
assert_file_contains "$GUIDELINE_NEW" "prose first" \
    "guideline/new.md states input is 'prose first'"

# ============================================================
# Contract 2: Same-pattern claim to /edikt:sdlc:plan v0.1.3
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "/edikt:sdlc:plan" \
        "$name/new.md references /edikt:sdlc:plan as the established pattern"
    assert_file_contains "$file" "v0.1.3" \
        "$name/new.md references v0.1.3 as the pattern's origin"
done

# ============================================================
# Contract 3: Three reference kinds documented (paths, identifiers, branches)
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "Reference kind 1: file paths" \
        "$name/new.md documents Reference kind 1 (file paths)"
    assert_file_contains "$file" "Reference kind 2: identifiers" \
        "$name/new.md documents Reference kind 2 (identifiers)"
    assert_file_contains "$file" "Reference kind 3: branch names" \
        "$name/new.md documents Reference kind 3 (branch names)"
done

# ============================================================
# Contract 4: Identifier patterns enumerated (ADR, INV, SPEC, PRD, PLAN)
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    for id_pattern in "ADR-NNN" "INV-NNN" "SPEC-NNN" "PRD-NNN" "PLAN-NNN"; do
        if grep -qF "$id_pattern" "$file"; then
            pass "$name/new.md documents $id_pattern identifier"
        else
            fail "$name/new.md documents $id_pattern identifier" \
                "Pattern $id_pattern not found"
        fi
    done
done

# ============================================================
# Contract 5: Branch prefix enumeration
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    for branch_prefix in "feature" "fix" "hotfix" "refactor" "spike"; do
        if grep -qF "\`$branch_prefix\`" "$file"; then
            pass "$name/new.md recognizes $branch_prefix/ branch prefix"
        else
            fail "$name/new.md recognizes $branch_prefix/ branch prefix" \
                "Prefix '$branch_prefix' not documented"
        fi
    done
done

# ============================================================
# Contract 6: Git verification mechanism documented
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "git rev-parse --verify" \
        "$name/new.md documents git rev-parse for branch verification"
done

# ============================================================
# Contract 7: Empty argument → conversation context fallback
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "Empty argument" \
        "$name/new.md documents empty argument fallback"
    assert_file_contains "$file" "conversation" \
        "$name/new.md references conversation context"
done

# ============================================================
# Contract 8: Examples table shows the dispatch matrix
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "| Input |" \
        "$name/new.md has Examples table with Input column"
    assert_file_contains "$file" "| Behavior |" \
        "$name/new.md has Examples table with Behavior column"
done

# ============================================================
# Contract 9: "Do NOT classify into rigid types" principle stated
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    if grep -qE "Do NOT classify|do not classify" "$file"; then
        pass "$name/new.md states the 'do not classify into rigid types' principle"
    else
        fail "$name/new.md states the 'do not classify into rigid types' principle" \
            "Phrase missing — flexible prose input must be explicit about not type-dispatching"
    fi
done

# ============================================================
# Contract 10: Source pool and framing prose concepts introduced
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "source pool" \
        "$name/new.md introduces the 'source pool' concept"
    assert_file_contains "$file" "framing prose" \
        "$name/new.md introduces the 'framing prose' concept"
done

# ============================================================
# Contract 11: Primary sources dominate over framing
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW"; do
    name=$(basename "$(dirname "$file")")
    if grep -qE "[Pp]rimary sources?" "$file"; then
        pass "$name/new.md distinguishes primary sources from framing"
    else
        fail "$name/new.md distinguishes primary sources from framing" \
            "Missing 'primary source' distinction"
    fi
done

# ============================================================
# Contract 12: Interview for gaps, not full interview, when refs resolve
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "Interview for gaps" \
        "$name/new.md has 'Interview for gaps' section header"
    if grep -qE "Ask ONLY about|not present in the source pool|missing element" "$file"; then
        pass "$name/new.md states interview only fills gaps when source pool has content"
    else
        fail "$name/new.md states interview only fills gaps" \
            "Must explicitly say interview is for gaps, not full re-interview"
    fi
done

# ============================================================
# Contract 13: Invariant-specific — constraint-vs-implementation rule
# ============================================================

assert_file_contains "$INVARIANT_NEW" "constraint, not the implementation" \
    "invariant/new.md reiterates constraint-vs-implementation rule in prose extraction"
assert_file_contains "$INVARIANT_NEW" "ADR-009" \
    "invariant/new.md references ADR-009 from the prose extraction section"

# ============================================================
# Contract 14: Guideline-specific — extract MUST/NEVER rules from source pool
# ============================================================

if grep -qE "Extract(ed)? these rules from|extracted rules" "$GUIDELINE_NEW"; then
    pass "guideline/new.md extracts rules from source pool when references resolve"
else
    fail "guideline/new.md extracts rules from source pool" \
        "Missing 'extract rules from source' flow"
fi

if grep -qE "MUST/NEVER form|validate it uses MUST or NEVER" "$GUIDELINE_NEW"; then
    pass "guideline/new.md validates extracted rules use MUST/NEVER language"
else
    fail "guideline/new.md validates extracted rules use MUST/NEVER language" \
        "Missing validation step on extracted rules"
fi

# ============================================================
# Contract 15: Error handling — unresolved refs are NOT errors
# ============================================================

for file in "$ADR_NEW" "$INVARIANT_NEW" "$GUIDELINE_NEW"; do
    name=$(basename "$(dirname "$file")")
    if grep -qE "do NOT error|treat as plain prose|treat the token as plain prose" "$file"; then
        pass "$name/new.md treats unresolved references as plain prose (no error)"
    else
        fail "$name/new.md treats unresolved references as plain prose" \
            "Must explicitly not error on unresolved refs — they might be forward references"
    fi
done

test_summary
