#!/bin/bash
# Test: v0.3.0 Phase 1 — extensibility plumbing + guideline:compile
#
# Guards the Phase 1 decisions from PRD-001 / file-changes.md:
#   1. commands/guideline/compile.md exists with parity to adr/invariant compile
#   2. Reference example templates exist under templates/examples/
#   3. Each reference template contains the directives sentinel block
#   4. Commands adr/new, invariant/new, guideline/new document the lookup chain
#   5. install.sh ships reference examples to ~/.edikt/templates/examples/
#   6. install.sh creates the templates/examples/ directory
#   7. install.sh includes guideline:compile in the guideline namespace loop
#   8. Commands reference ADR-009 terminology (Invariant Record)
#
# Phase 1 does NOT enforce template-less refusal — that's Phase 3 when init
# is ready to fix missing templates. This test accordingly checks documentation
# of the lookup chain, not the refusal path.
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Regression 1: commands/guideline/compile.md exists (new in v0.3.0)
# ============================================================

GUIDELINE_COMPILE="$PROJECT_ROOT/commands/guideline/compile.md"

assert_file_exists "$GUIDELINE_COMPILE" \
    "commands/guideline/compile.md exists (new in v0.3.0)"

assert_file_contains "$GUIDELINE_COMPILE" "name: edikt:guideline:compile" \
    "guideline/compile.md frontmatter has name: edikt:guideline:compile"

assert_file_contains "$GUIDELINE_COMPILE" "edikt:directives:start" \
    "guideline/compile.md documents directive sentinel generation"

assert_file_contains "$GUIDELINE_COMPILE" "content_hash" \
    "guideline/compile.md documents content hash for staleness detection"

assert_file_contains "$GUIDELINE_COMPILE" "MUST or NEVER" \
    "guideline/compile.md requires MUST/NEVER language in rules"

assert_file_contains "$GUIDELINE_COMPILE" "paths.guidelines" \
    "guideline/compile.md resolves paths.guidelines from config"

# Parity check: guideline/compile.md should have the same structural sections as adr/compile.md and invariant/compile.md
for section in "Config Guard" "Resolve Paths" "Determine Scope" "Process Each" "Report Results"; do
    assert_file_contains "$GUIDELINE_COMPILE" "$section" \
        "guideline/compile.md has section: $section (parity with adr/invariant compile)"
done

# ============================================================
# Regression 2: Reference example templates exist
# ============================================================

EXAMPLES_DIR="$PROJECT_ROOT/templates/examples"

assert_file_exists "$EXAMPLES_DIR/adr-nygard-minimal.md" \
    "templates/examples/adr-nygard-minimal.md exists"
assert_file_exists "$EXAMPLES_DIR/adr-madr-extended.md" \
    "templates/examples/adr-madr-extended.md exists"
assert_file_exists "$EXAMPLES_DIR/invariant-minimal.md" \
    "templates/examples/invariant-minimal.md exists"
assert_file_exists "$EXAMPLES_DIR/invariant-full.md" \
    "templates/examples/invariant-full.md exists"
assert_file_exists "$EXAMPLES_DIR/guideline-minimal.md" \
    "templates/examples/guideline-minimal.md exists"
assert_file_exists "$EXAMPLES_DIR/guideline-extended.md" \
    "templates/examples/guideline-extended.md exists"

# ============================================================
# Regression 3: Every reference template has the directives sentinel block
# ============================================================

for template in "$EXAMPLES_DIR"/*.md; do
    name=$(basename "$template")
    assert_file_contains "$template" "edikt:directives:start" \
        "Reference template has directives:start sentinel: $name"
    assert_file_contains "$template" "edikt:directives:end" \
        "Reference template has directives:end sentinel: $name"
done

# ============================================================
# Regression 4: ADR reference templates cite their source
# ============================================================

assert_file_contains "$EXAMPLES_DIR/adr-nygard-minimal.md" "Nygard" \
    "adr-nygard-minimal.md cites Michael Nygard as the source format"
assert_file_contains "$EXAMPLES_DIR/adr-madr-extended.md" "MADR" \
    "adr-madr-extended.md cites MADR as the source format"

# ============================================================
# Regression 5: Invariant and guideline templates reference ADR-009
# ============================================================

assert_file_contains "$EXAMPLES_DIR/invariant-minimal.md" "ADR-009" \
    "invariant-minimal.md references ADR-009"
assert_file_contains "$EXAMPLES_DIR/invariant-full.md" "ADR-009" \
    "invariant-full.md references ADR-009"
# Guideline templates don't reference ADR-009 (guidelines are not Invariant Records)
assert_file_exists "$EXAMPLES_DIR/guideline-minimal.md" \
    "guideline-minimal.md exists"
assert_file_exists "$EXAMPLES_DIR/guideline-extended.md" \
    "guideline-extended.md exists"

# ============================================================
# Regression 6: Invariant templates reference ADR-009 (Invariant Record template)
# ============================================================

assert_file_contains "$EXAMPLES_DIR/invariant-minimal.md" "ADR-009" \
    "invariant-minimal.md references ADR-009 template"
assert_file_contains "$EXAMPLES_DIR/invariant-full.md" "ADR-009" \
    "invariant-full.md references ADR-009 template"

# ============================================================
# Regression 7: The three `new.md` commands document the lookup chain
# ============================================================

ADR_NEW="$PROJECT_ROOT/commands/adr/new.md"
INVARIANT_NEW="$PROJECT_ROOT/commands/invariant/new.md"
GUIDELINE_NEW="$PROJECT_ROOT/commands/guideline/new.md"

assert_file_contains "$ADR_NEW" "Resolve Template" \
    "adr/new.md has 'Resolve Template' section"
assert_file_contains "$ADR_NEW" "Project template" \
    "adr/new.md documents 'Project template' precedence"
assert_file_contains "$ADR_NEW" "Inline fallback" \
    "adr/new.md documents 'Inline fallback' precedence"
assert_file_contains "$ADR_NEW" "No global default" \
    "adr/new.md explicitly states 'No global default'"

assert_file_contains "$INVARIANT_NEW" "Resolve Template" \
    "invariant/new.md has 'Resolve Template' section"
assert_file_contains "$INVARIANT_NEW" "Invariant Record" \
    "invariant/new.md references 'Invariant Record' terminology from ADR-009"
assert_file_contains "$INVARIANT_NEW" "No global default" \
    "invariant/new.md explicitly states 'No global default'"

assert_file_contains "$GUIDELINE_NEW" "Resolve Template" \
    "guideline/new.md has 'Resolve Template' section"
assert_file_contains "$GUIDELINE_NEW" "No global default" \
    "guideline/new.md explicitly states 'No global default'"

# ============================================================
# Regression 8: install.sh ships the reference examples
# ============================================================

INSTALL_SH="$PROJECT_ROOT/install.sh"

# install.sh-internal assertions removed in v0.5.0 Phase 5 hardening — the
# bootstrap delegates to bin/edikt; coverage now lives under
# test/unit/launcher/ and test/integration/install/.

# ============================================================
# Regression 10: Invariant templates reference ADR-009 for the template contract
# ============================================================
# Coinage language was dropped in v0.3.0. Templates now reference ADR-009
# for the template contract without claiming to coin a term.

assert_file_contains "$EXAMPLES_DIR/invariant-minimal.md" "ADR-009" \
    "invariant-minimal.md references ADR-009"
assert_file_contains "$EXAMPLES_DIR/invariant-full.md" "ADR-009" \
    "invariant-full.md references ADR-009"

test_summary
