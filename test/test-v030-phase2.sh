#!/bin/bash
# Test: v0.3.0 Phase 2 — three-list schema + hash-based caching (ADR-008)
#
# Guards the Phase 2 decisions from PRD-001 / file-changes.md + ADR-008:
#   1. All three <artifact>:compile.md commands document the three-list schema
#   2. All three <artifact>:compile.md commands document the hash algorithm
#      (SHA-256, normalization steps, block exclusion)
#   3. All three commands document the fast path / slow path / interview path
#   4. All three commands document the --strategy= headless override flags
#   5. All three commands document backward compatibility with legacy
#      content_hash blocks
#   6. All three commands auto-chain from new.md to compile.md
#   7. gov/compile.md reads all three lists and applies the ADR-008 merge formula
#   8. gov/compile.md documents within-artifact contradiction detection
#   9. gov/compile.md handles legacy v0.2.x blocks without manual/suppressed lists
#
# This test operates at the documentation level — it verifies that the
# markdown instructions for Claude correctly describe the ADR-008 contract.
# It does NOT execute the compile commands (which would require headless
# Claude Code).
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

ADR_COMPILE="$PROJECT_ROOT/commands/adr/compile.md"
INVARIANT_COMPILE="$PROJECT_ROOT/commands/invariant/compile.md"
GUIDELINE_COMPILE="$PROJECT_ROOT/commands/guideline/compile.md"
GOV_COMPILE="$PROJECT_ROOT/commands/gov/compile.md"
ADR_NEW="$PROJECT_ROOT/commands/adr/new.md"
INVARIANT_NEW="$PROJECT_ROOT/commands/invariant/new.md"
GUIDELINE_NEW="$PROJECT_ROOT/commands/guideline/new.md"

# ============================================================
# Contract 1: Three-list schema documented in all three <artifact>:compile
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "three-list schema" \
        "$name/compile.md documents 'three-list schema'"
    assert_file_contains "$file" "manual_directives" \
        "$name/compile.md documents manual_directives list"
    assert_file_contains "$file" "suppressed_directives" \
        "$name/compile.md documents suppressed_directives list"
    assert_file_contains "$file" "ADR-008" \
        "$name/compile.md references ADR-008"
done

# ============================================================
# Contract 2: Hash algorithm documented in all three compile commands
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "source_hash" \
        "$name/compile.md documents source_hash field"
    assert_file_contains "$file" "directives_hash" \
        "$name/compile.md documents directives_hash field"
    assert_file_contains "$file" "compiler_version" \
        "$name/compile.md documents compiler_version field"
    assert_file_contains "$file" "SHA-256" \
        "$name/compile.md specifies SHA-256 hash algorithm"
    assert_file_contains "$file" "directives block excluded" \
        "$name/compile.md documents 'directives block excluded' for source_hash"
    if grep -qF "r\\n" "$file"; then
        pass "$name/compile.md documents CRLF→LF normalization"
    else
        fail "$name/compile.md documents CRLF→LF normalization" \
            "Missing normalization step documentation"
    fi
    assert_file_contains "$file" "trailing whitespace" \
        "$name/compile.md documents trailing whitespace stripping"
done

# ============================================================
# Contract 3: Fast path / slow path / interview path documented
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "FAST PATH" \
        "$name/compile.md documents the fast path"
    assert_file_contains "$file" "SLOW PATH" \
        "$name/compile.md documents the slow path"
    assert_file_contains "$file" "Hand-edited" \
        "$name/compile.md documents hand-edited state"
    assert_file_contains "$file" "Interview" \
        "$name/compile.md documents interview path"
done

# ============================================================
# Contract 4: Interview has 5 options for hand-added, 3 for hand-deleted
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    # Hand-added line options
    for option_label in "Move to manual_directives" "Add to suppressed_directives" "Delete entirely" "Skip for now"; do
        if grep -qF "$option_label" "$file"; then
            pass "$name/compile.md interview has option: $option_label"
        else
            fail "$name/compile.md interview has option: $option_label" \
                "Missing interview option"
        fi
    done
    assert_file_contains "$file" "Let compile regenerate" \
        "$name/compile.md interview has 'Let compile regenerate' option for hand-deleted"
done

# ============================================================
# Contract 5: --strategy= headless override flags documented
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "strategy=regenerate" \
        "$name/compile.md documents --strategy=regenerate"
    assert_file_contains "$file" "strategy=preserve" \
        "$name/compile.md documents --strategy=preserve"
    assert_file_contains "$file" "headless" \
        "$name/compile.md documents headless mode"
    assert_file_contains "$file" "EDIKT_HEADLESS" \
        "$name/compile.md references EDIKT_HEADLESS environment variable"
done

# ============================================================
# Contract 6: Backward compatibility with legacy v0.2.x blocks
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "Legacy v0.2.x" \
        "$name/compile.md documents Legacy v0.2.x state"
    assert_file_contains "$file" "content_hash" \
        "$name/compile.md handles legacy content_hash field"
    assert_file_contains "$file" "Silent migration" \
        "$name/compile.md documents silent migration for legacy blocks"
done

# ============================================================
# Contract 7: "NEVER reads manual/suppressed" safety rule enforced in docs
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    # The rule is stated multiple times in different phrasings; any of these counts
    if grep -qE "NEVER (reads?|touch|write).*(manual_directives|suppressed_directives)" "$file"; then
        pass "$name/compile.md enforces 'never read/touch/write manual/suppressed'"
    else
        fail "$name/compile.md enforces 'never read/touch/write manual/suppressed'" \
            "Safety rule must be explicit — grep for 'NEVER reads/writes/touches manual/suppressed'"
    fi
done

# ============================================================
# Contract 8: Auto-chain from new.md to compile.md
# ============================================================

for pair in "adr:adr:new.md:adr:compile" "invariant:invariant:new.md:invariant:compile" "guideline:guideline:new.md:guideline:compile"; do
    artifact=$(echo "$pair" | cut -d: -f1)
    new_file="$PROJECT_ROOT/commands/$artifact/new.md"
    compile_cmd=$(echo "$pair" | cut -d: -f4-5)

    assert_file_contains "$new_file" "Auto-chain" \
        "$artifact/new.md documents Auto-chain section"
    assert_file_contains "$new_file" "/edikt:$compile_cmd" \
        "$artifact/new.md references /edikt:$compile_cmd in auto-chain"
    assert_file_contains "$new_file" "ADR-008" \
        "$artifact/new.md references ADR-008 for auto-chain"
done

# ============================================================
# Contract 9: gov/compile.md reads all three lists and applies merge formula
# ============================================================

assert_file_contains "$GOV_COMPILE" "three-list schema" \
    "gov/compile.md references three-list schema"
assert_file_contains "$GOV_COMPILE" "manual_directives" \
    "gov/compile.md reads manual_directives"
assert_file_contains "$GOV_COMPILE" "suppressed_directives" \
    "gov/compile.md reads suppressed_directives"
assert_file_contains "$GOV_COMPILE" "effective_rules" \
    "gov/compile.md computes effective_rules"
assert_file_contains "$GOV_COMPILE" "set difference by exact string match" \
    "gov/compile.md documents set difference semantics"
assert_file_contains "$GOV_COMPILE" "set union preserving document order" \
    "gov/compile.md documents set union semantics"

# The canonical formula must be present
if grep -qF "(directives - suppressed_directives) ∪ manual_directives" "$GOV_COMPILE"; then
    pass "gov/compile.md contains the canonical merge formula"
else
    fail "gov/compile.md contains the canonical merge formula" \
        "Expected '(directives - suppressed_directives) ∪ manual_directives' verbatim"
fi

# ============================================================
# Contract 10: gov/compile.md handles legacy blocks + contradiction detection
# ============================================================

assert_file_contains "$GOV_COMPILE" "Backward compatibility" \
    "gov/compile.md documents backward compatibility for legacy blocks"
assert_file_contains "$GOV_COMPILE" "Within-artifact contradiction detection" \
    "gov/compile.md documents within-artifact contradiction detection"
assert_file_contains "$GOV_COMPILE" "silent de-dup" \
    "gov/compile.md documents silent de-duplication between directives and manual_directives"

# ============================================================
# Contract 11: Fast-path skip means no Claude call
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    if grep -q "no Claude call" "$file"; then
        pass "$name/compile.md explicitly states fast path has no Claude call"
    else
        fail "$name/compile.md explicitly states fast path has no Claude call" \
            "Phrase 'no Claude call' must appear in the fast path description"
    fi
done

# ============================================================
# Contract 12: Report results mentions new outcomes
# ============================================================

for file in "$ADR_COMPILE" "$INVARIANT_COMPILE" "$GUIDELINE_COMPILE"; do
    name=$(basename "$(dirname "$file")")
    assert_file_contains "$file" "hash match, skipped" \
        "$name/compile.md report includes 'hash match, skipped' outcome"
    assert_file_contains "$file" "Legacy migrated" \
        "$name/compile.md report includes 'Legacy migrated' outcome"
    assert_file_contains "$file" "Strategy=regenerate" \
        "$name/compile.md report mentions Strategy=regenerate outcome"
    assert_file_contains "$file" "Strategy=preserve" \
        "$name/compile.md report mentions Strategy=preserve outcome"
done

test_summary
