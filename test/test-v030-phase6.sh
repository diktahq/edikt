#!/bin/bash
# Test: v0.3.0 Phase 6 — Invariant Record canonical examples + website + experiments
#
# Guards the Phase 6 deliverables from PRD-001 / file-changes.md:
#   1. Canonical invariant examples shipped under templates/examples/invariants/
#   2. Condensed WRITING-GUIDE shipped alongside the canonical examples
#   3. README shipped in templates/examples/invariants/
#   4. install.sh ships all four files in templates/examples/invariants/
#   5. install.sh creates templates/examples/invariants/ directory
#   6. Website pages exist for Invariant Records landing, writing guide,
#      and both canonical examples
#   7. Vitepress sidebar includes the new Invariant Records section
#   8. Experiment infrastructure exists at test/experiments/
#   9. All three experiment fixtures have required files (project, prompt,
#      invariant, assertion)
#  10. Experiment runner script is executable and references all three IDs
#  11. Assertion scripts are executable
#  12. Experiment 01 (Go) project compiles (syntactic correctness)
#  13. Experiment 02 + 03 Python fixtures have valid module structure
#  14. Canonical examples are in the ADR-009 template shape (6 sections)
#  15. Website pages reference ADR-008 and ADR-009 via GitHub links
#      (not broken relative paths)
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Contract 1: Canonical invariant examples shipped
# ============================================================

INV_EXAMPLES_DIR="$PROJECT_ROOT/templates/examples/invariants"

assert_file_exists "$INV_EXAMPLES_DIR/tenant-isolation.md" \
    "templates/examples/invariants/tenant-isolation.md exists"
assert_file_exists "$INV_EXAMPLES_DIR/money-precision.md" \
    "templates/examples/invariants/money-precision.md exists"
assert_file_exists "$INV_EXAMPLES_DIR/README.md" \
    "templates/examples/invariants/README.md exists"
assert_file_exists "$INV_EXAMPLES_DIR/WRITING-GUIDE.md" \
    "templates/examples/invariants/WRITING-GUIDE.md exists"

# ============================================================
# Contract 2: Canonical examples follow the ADR-009 template shape
# ============================================================

for example in tenant-isolation.md money-precision.md; do
    file="$INV_EXAMPLES_DIR/$example"
    # Required sections per ADR-009 template
    assert_file_contains "$file" "## Statement" \
        "$example has ## Statement section"
    assert_file_contains "$file" "## Rationale" \
        "$example has ## Rationale section"
    assert_file_contains "$file" "## Consequences of violation" \
        "$example has ## Consequences of violation section"
    assert_file_contains "$file" "## Enforcement" \
        "$example has ## Enforcement section"
    # Optional but strongly encouraged
    assert_file_contains "$file" "## Implementation" \
        "$example has ## Implementation section"
    assert_file_contains "$file" "## Anti-patterns" \
        "$example has ## Anti-patterns section"
    # Directives sentinel block
    assert_file_contains "$file" "edikt:directives:start" \
        "$example has directives sentinel block"
    # Must reference ADR-009
    assert_file_contains "$file" "ADR-009" \
        "$example references ADR-009"
done

# ============================================================
# Contract 3: README and WRITING-GUIDE quality checks
# ============================================================

assert_file_contains "$INV_EXAMPLES_DIR/README.md" "Canonical Invariant Record examples" \
    "README has correct title"
assert_file_contains "$INV_EXAMPLES_DIR/README.md" "ADR-009" \
    "README references ADR-009"
assert_file_contains "$INV_EXAMPLES_DIR/README.md" "ADR-009" \
    "README references ADR-009"

assert_file_contains "$INV_EXAMPLES_DIR/WRITING-GUIDE.md" "Five qualities" \
    "WRITING-GUIDE has Five qualities section"
assert_file_contains "$INV_EXAMPLES_DIR/WRITING-GUIDE.md" "Seven traps" \
    "WRITING-GUIDE has Seven traps section"
assert_file_contains "$INV_EXAMPLES_DIR/WRITING-GUIDE.md" "seven-question self-test" \
    "WRITING-GUIDE has seven-question self-test"

# ============================================================
# Contract 4: install.sh ships invariants examples
# ============================================================

INSTALL_SH="$PROJECT_ROOT/install.sh"

assert_file_contains "$INSTALL_SH" "templates/examples/invariants" \
    "install.sh creates templates/examples/invariants directory"
assert_file_contains "$INSTALL_SH" "Canonical Invariant Record examples" \
    "install.sh has the canonical examples install block"

for example in tenant-isolation money-precision README WRITING-GUIDE; do
    if grep -qF "${example}.md" "$INSTALL_SH"; then
        pass "install.sh ships ${example}.md"
    else
        fail "install.sh ships ${example}.md" "Not found in install.sh"
    fi
done

# ============================================================
# Contract 5: Website pages exist
# ============================================================

WEBSITE_GOV="$PROJECT_ROOT/website/governance"

assert_file_exists "$WEBSITE_GOV/invariant-records.md" \
    "website/governance/invariant-records.md exists"
assert_file_exists "$WEBSITE_GOV/writing-invariants.md" \
    "website/governance/writing-invariants.md exists"
assert_file_exists "$WEBSITE_GOV/canonical-invariants/tenant-isolation.md" \
    "website/governance/canonical-invariants/tenant-isolation.md exists"
assert_file_exists "$WEBSITE_GOV/canonical-invariants/money-precision.md" \
    "website/governance/canonical-invariants/money-precision.md exists"

# ============================================================
# Contract 6: Website pages reference ADR-008 and ADR-009
# ============================================================

assert_file_contains "$WEBSITE_GOV/invariant-records.md" "Invariant Records" \
    "invariant-records.md has correct title"
assert_file_contains "$WEBSITE_GOV/invariant-records.md" "ADR-009" \
    "invariant-records.md references ADR-009"
assert_file_contains "$WEBSITE_GOV/invariant-records.md" "ADR-008" \
    "invariant-records.md references ADR-008"
assert_file_contains "$WEBSITE_GOV/invariant-records.md" "ADR-009" \
    "invariant-records.md references ADR-009"

# writing-invariants.md should have no broken relative links to decisions/
if grep -qF '../../decisions/' "$WEBSITE_GOV/writing-invariants.md"; then
    fail "writing-invariants.md has no broken ../../decisions/ links" \
        "Found relative paths that won't work from the website layout"
else
    pass "writing-invariants.md has no broken ../../decisions/ links"
fi

# writing-invariants.md should have no broken canonical-examples/ links
if grep -qF 'canonical-examples/' "$WEBSITE_GOV/writing-invariants.md"; then
    fail "writing-invariants.md uses website path canonical-invariants/" \
        "Found stale 'canonical-examples/' path — website uses canonical-invariants/"
else
    pass "writing-invariants.md uses website path canonical-invariants/"
fi

# ============================================================
# Contract 7: Vitepress config includes new sidebar
# ============================================================

VITEPRESS_CONFIG="$PROJECT_ROOT/website/.vitepress/config.ts"

assert_file_contains "$VITEPRESS_CONFIG" "Invariant Records" \
    "vitepress config has Invariant Records sidebar group"
assert_file_contains "$VITEPRESS_CONFIG" "/governance/invariant-records" \
    "vitepress config links to invariant-records landing page"
assert_file_contains "$VITEPRESS_CONFIG" "/governance/writing-invariants" \
    "vitepress config links to writing-invariants page"
assert_file_contains "$VITEPRESS_CONFIG" "canonical-invariants/tenant-isolation" \
    "vitepress config links to tenant-isolation canonical"
assert_file_contains "$VITEPRESS_CONFIG" "canonical-invariants/money-precision" \
    "vitepress config links to money-precision canonical"

# ============================================================
# Contract 8: Experiment infrastructure exists
# ============================================================

EXP_DIR="$PROJECT_ROOT/test/experiments"

assert_file_exists "$EXP_DIR/README.md" \
    "test/experiments/README.md exists"

# Experiment suites are gitignored (directive-effect/, long-running/)
# Only rule-compliance/ is tracked. Check the tracked structure:
assert_file_exists "$EXP_DIR/rule-compliance/README.md" \
    "test/experiments/rule-compliance/README.md exists"

# ============================================================
# Contract 9: Directive-effect experiment fixtures (gitignored — skip if absent)
# ============================================================
# Directive-effect experiments are gitignored (local development only).
# These checks run when the files are on disk but skip gracefully in CI.

DE_DIR="$EXP_DIR/directive-effect"
if [ -d "$DE_DIR" ]; then
    if [ -x "$DE_DIR/run.sh" ]; then
        pass "directive-effect/run.sh is executable"
    else
        fail "directive-effect/run.sh is executable" "Missing +x bit"
    fi

    for exp_id in 01-multi-tenancy 02-money-precision 03-timezone-awareness; do
        fixture="$DE_DIR/fixtures/$exp_id"
        if [ -d "$fixture" ]; then
            assert_file_exists "$fixture/prompt.txt" "$exp_id fixture has prompt.txt"
            assert_file_exists "$fixture/invariant.md" "$exp_id fixture has invariant.md"
            assert_file_exists "$fixture/assertion.sh" "$exp_id fixture has assertion.sh"
            if [ -d "$fixture/project" ]; then
                pass "$exp_id fixture has project/ directory"
            else
                fail "$exp_id fixture has project/ directory" "Missing"
            fi
        fi
    done
else
    echo "  SKIP  Directive-effect experiments not on disk (gitignored)"
fi

# ============================================================
# Contract 11: Experiment 01 Go fixture compiles (skip if gitignored)
# ============================================================

if [ ! -d "$DE_DIR/fixtures/01-multi-tenancy" ]; then
    echo "  SKIP  Experiment fixture checks (gitignored)"
elif command -v go >/dev/null 2>&1; then
    if (cd "$DE_DIR/fixtures/01-multi-tenancy/project" && go build ./... 2>&1) >/dev/null; then
        pass "Experiment 01 Go fixture compiles"
    else
        fail "Experiment 01 Go fixture compiles" \
            "Go build failed — fixture is syntactically invalid"
    fi
else
    echo "  SKIP  Experiment 01 Go compile check (go not installed)"
fi

# ============================================================
# Contract 12: Experiment 02 Python fixture has valid structure (skip if gitignored)
# ============================================================

if [ ! -d "$DE_DIR/fixtures/02-money-precision" ]; then
    echo "  SKIP  Experiment 02 fixtures (gitignored)"
elif command -v python3 >/dev/null 2>&1; then
    if (cd "$DE_DIR/fixtures/02-money-precision/project" && python3 -c "import ast; ast.parse(open('app/pricing.py').read())" 2>&1) >/dev/null; then
        pass "Experiment 02 pricing.py parses"
    else
        fail "Experiment 02 pricing.py parses" "Python syntax error"
    fi
    if (cd "$DE_DIR/fixtures/02-money-precision/project" && python3 -c "import ast; ast.parse(open('app/models.py').read())" 2>&1) >/dev/null; then
        pass "Experiment 02 models.py parses"
    else
        fail "Experiment 02 models.py parses" "Python syntax error"
    fi
else
    echo "  SKIP  Experiment 02 Python parse check (python3 not installed)"
fi

# ============================================================
# Contract 13: Experiment 03 Python fixture has valid structure (skip if gitignored)
# ============================================================

if [ ! -d "$DE_DIR/fixtures/03-timezone-awareness" ]; then
    echo "  SKIP  Experiment 03 fixtures (gitignored)"
elif command -v python3 >/dev/null 2>&1; then
    if (cd "$DE_DIR/fixtures/03-timezone-awareness/project" && python3 -c "import ast; ast.parse(open('app/orders.py').read())" 2>&1) >/dev/null; then
        pass "Experiment 03 orders.py parses"
    else
        fail "Experiment 03 orders.py parses" "Python syntax error"
    fi
    if (cd "$DE_DIR/fixtures/03-timezone-awareness/project" && python3 -c "import ast; ast.parse(open('app/db.py').read())" 2>&1) >/dev/null; then
        pass "Experiment 03 db.py parses"
    else
        fail "Experiment 03 db.py parses" "Python syntax error"
    fi
else
    echo "  SKIP  Experiment 03 Python parse check (python3 not installed)"
fi

# ============================================================
# Contract 14: Experiment prompts are contamination-free
# ============================================================

# The prompts MUST NOT contain words that hint at the invariant being tested.
# This check catches prompt drift that would invalidate the experiment.

PROMPT_01="$DE_DIR/fixtures/01-multi-tenancy/prompt.txt"
PROMPT_02="$DE_DIR/fixtures/02-money-precision/prompt.txt"
PROMPT_03="$DE_DIR/fixtures/03-timezone-awareness/prompt.txt"

if [ ! -f "$PROMPT_01" ]; then
    echo "  SKIP  Contamination checks (fixtures gitignored)"
else

for bad_word in tenant isolation scope secure; do
    if grep -qi "$bad_word" "$PROMPT_01"; then
        fail "01-multi-tenancy prompt is contamination-free ($bad_word)" \
            "Prompt contains hint word '$bad_word' — invalidates experiment"
    else
        pass "01-multi-tenancy prompt does not contain '$bad_word'"
    fi
done

for bad_word in decimal float precision precise; do
    if grep -qi "$bad_word" "$PROMPT_02"; then
        fail "02-money-precision prompt is contamination-free ($bad_word)" \
            "Prompt contains hint word '$bad_word' — invalidates experiment"
    else
        pass "02-money-precision prompt does not contain '$bad_word'"
    fi
done

for bad_word in timezone utc naive aware tzinfo; do
    if grep -qi "$bad_word" "$PROMPT_03"; then
        fail "03-timezone-awareness prompt is contamination-free ($bad_word)" \
            "Prompt contains hint word '$bad_word' — invalidates experiment"
    else
        pass "03-timezone-awareness prompt does not contain '$bad_word'"
    fi
done

fi  # end of contamination checks (skip if gitignored)

# ============================================================
# Contract 15: Website page links from invariant-records.md
# ============================================================

assert_file_contains "$WEBSITE_GOV/invariant-records.md" "canonical-invariants/tenant-isolation" \
    "invariant-records.md links to tenant-isolation canonical"
assert_file_contains "$WEBSITE_GOV/invariant-records.md" "canonical-invariants/money-precision" \
    "invariant-records.md links to money-precision canonical"
assert_file_contains "$WEBSITE_GOV/invariant-records.md" "writing-invariants" \
    "invariant-records.md links to writing-invariants guide"

test_summary
