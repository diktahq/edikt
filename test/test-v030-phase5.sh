#!/bin/bash
# Test: v0.3.0 Phase 5 — doctor + upgrade integration for project templates
#
# Guards the Phase 5 decisions from PROPOSAL-001 / file-changes.md:
#   1. doctor.md documents the Project templates check for all three types
#   2. doctor.md reports present/absent/broken states per ADR-005 contract
#   3. doctor.md reports on Reference templates shipped at ~/.edikt/templates/examples/
#   4. doctor.md documents the Directive sentinel schema check (v0.3.0 current,
#      partial, v0.2.x legacy, unrecognized classification)
#   5. doctor.md check order includes the new Phase 5 checks
#   6. doctor.md uses "Invariant Record" terminology (ADR-009)
#   7. upgrade.md never overwrites .edikt/templates/ — explicit ADR-005 contract
#   8. upgrade.md detects the grandfather flow (edikt_version < 0.3.0 + missing
#      templates) and surfaces the migration notice
#   9. upgrade.md detects the broken state (v0.3.0+ with missing templates) and
#      surfaces an error-grade notice
#  10. upgrade.md surfaces the post-upgrade "run /edikt:init" next-step prompt
#      when templates are missing
#  11. upgrade.md never auto-runs /edikt:init — advisory only
#  12. upgrade.md reports legacy directive sentinel schema as informational
#      and never auto-migrates
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

DOCTOR_MD="$PROJECT_ROOT/commands/doctor.md"
UPGRADE_MD="$PROJECT_ROOT/commands/upgrade.md"

# ============================================================
# Contract 1: doctor.md has the Project templates check
# ============================================================

assert_file_contains "$DOCTOR_MD" "Project templates" \
    "doctor.md has Project templates check"
assert_file_contains "$DOCTOR_MD" "ADR-009" \
    "doctor.md references ADR-009 in the Project templates check"
assert_file_contains "$DOCTOR_MD" "Phase 3 of v0.3.0" \
    "doctor.md cites Phase 3 of v0.3.0"

# ============================================================
# Contract 2: doctor checks all three artifact types
# ============================================================

for artifact in "adr.md" "invariant.md" "guideline.md"; do
    assert_file_contains "$DOCTOR_MD" ".edikt/templates/$artifact" \
        "doctor.md checks .edikt/templates/$artifact"
done

# ============================================================
# Contract 3: Template state classification
# ============================================================

assert_file_contains "$DOCTOR_MD" "Template present" \
    "doctor.md documents 'Template present' state"
assert_file_contains "$DOCTOR_MD" "Template absent AND project is v0.3.0+" \
    "doctor.md documents v0.3.0+ missing template state"
assert_file_contains "$DOCTOR_MD" "Template absent AND project is v0.2.x legacy" \
    "doctor.md documents v0.2.x legacy missing template state"
assert_file_contains "$DOCTOR_MD" "sentinel block" \
    "doctor.md verifies templates have the directives sentinel block"

# ============================================================
# Contract 4: Invariant Record terminology (ADR-009)
# ============================================================

assert_file_contains "$DOCTOR_MD" "Invariant Record template" \
    "doctor.md uses 'Invariant Record template' terminology"
assert_file_contains "$DOCTOR_MD" "Reinforces the coinage" \
    "doctor.md explicitly reinforces the Invariant Record coinage"

# ============================================================
# Contract 5: Template reference examples check
# ============================================================

assert_file_contains "$DOCTOR_MD" "Template reference examples" \
    "doctor.md has Template reference examples check"
assert_file_contains "$DOCTOR_MD" '~/.edikt/templates/examples/' \
    "doctor.md checks ~/.edikt/templates/examples/ directory"

for example in "adr-nygard-minimal" "adr-madr-extended" "invariant-minimal" "invariant-full" "guideline-minimal" "guideline-extended"; do
    assert_file_contains "$DOCTOR_MD" "$example" \
        "doctor.md checks for $example reference template"
done

# ============================================================
# Contract 6: Directive sentinel schema check (ADR-008)
# ============================================================

assert_file_contains "$DOCTOR_MD" "Directive sentinel schema" \
    "doctor.md has Directive sentinel schema check"
assert_file_contains "$DOCTOR_MD" "ADR-008" \
    "doctor.md references ADR-008 in the schema check"
assert_file_contains "$DOCTOR_MD" "v0.3.0 current" \
    "doctor.md classifies 'v0.3.0 current' state"
assert_file_contains "$DOCTOR_MD" "v0.3.0 partial" \
    "doctor.md classifies 'v0.3.0 partial' state"
assert_file_contains "$DOCTOR_MD" "v0.2.x legacy" \
    "doctor.md classifies 'v0.2.x legacy' state"
assert_file_contains "$DOCTOR_MD" "unrecognized" \
    "doctor.md classifies 'unrecognized' state"

assert_file_contains "$DOCTOR_MD" "Migrations are never urgent" \
    "doctor.md explicitly states migrations are never urgent"

# ============================================================
# Contract 7: doctor check order includes Phase 5 checks
# ============================================================

assert_file_contains "$DOCTOR_MD" "Project templates, Template reference examples" \
    "doctor.md check order lists Phase 5 checks"
assert_file_contains "$DOCTOR_MD" "Directive sentinel schema" \
    "doctor.md check order lists Directive sentinel schema"

# ============================================================
# Contract 8: upgrade.md Project templates check section
# ============================================================

assert_file_contains "$UPGRADE_MD" "2e-bis. Project templates check" \
    "upgrade.md has 2e-bis Project templates check section"
assert_file_contains "$UPGRADE_MD" "ADR-005" \
    "upgrade.md references ADR-005 in the project templates check"
assert_file_contains "$UPGRADE_MD" "ADR-009" \
    "upgrade.md references ADR-009 in the project templates check"

# ============================================================
# Contract 9: upgrade.md never overwrites templates
# ============================================================

assert_file_contains "$UPGRADE_MD" "Never overwrite project templates" \
    "upgrade.md explicitly states 'Never overwrite project templates'"
assert_file_contains "$UPGRADE_MD" "MUST NOT be touched" \
    "upgrade.md states templates MUST NOT be touched"
assert_file_contains "$UPGRADE_MD" "user-owned" \
    "upgrade.md documents templates as user-owned content"

# ============================================================
# Contract 10: upgrade.md classifies grandfather vs broken state
# ============================================================

assert_file_contains "$UPGRADE_MD" "grandfather flow" \
    "upgrade.md identifies the grandfather flow"
assert_file_contains "$UPGRADE_MD" "Broken state" \
    "upgrade.md identifies the broken state"
assert_file_contains "$UPGRADE_MD" "Partially configured" \
    "upgrade.md identifies the partially configured state"

# ============================================================
# Contract 11: upgrade.md never auto-runs init
# ============================================================

if grep -qE "Never auto-run init|Do not run .*/edikt:init.* automatically|Do NOT auto-run" "$UPGRADE_MD"; then
    pass "upgrade.md states it never auto-runs /edikt:init"
else
    fail "upgrade.md states it never auto-runs /edikt:init" \
        "Must explicitly say upgrade does not invoke init"
fi

assert_file_contains "$UPGRADE_MD" "Leave the user in control" \
    "upgrade.md explicitly leaves user in control"

# ============================================================
# Contract 12: Directive sentinel schema migration is informational
# ============================================================

assert_file_contains "$UPGRADE_MD" "Directive sentinel schema migration" \
    "upgrade.md has Directive sentinel schema migration section"
assert_file_contains "$UPGRADE_MD" "backward compatibility" \
    "upgrade.md notes backward compatibility for legacy schemas"

if grep -qE "never auto-migrate|Do NOT run .*--regenerate.* automatically|migrate at (your|the user)" "$UPGRADE_MD"; then
    pass "upgrade.md states legacy schema migration is user-paced"
else
    fail "upgrade.md states legacy schema migration is user-paced" \
        "Must explicitly say migration is at user's pace, not automatic"
fi

# ============================================================
# Contract 13: Post-upgrade next-step prompt for /edikt:init
# ============================================================

assert_file_contains "$UPGRADE_MD" "Project templates — /edikt:init required" \
    "upgrade.md has post-upgrade project templates next-step prompt"
assert_file_contains "$UPGRADE_MD" "HARD REFUSE" \
    "upgrade.md warns about HARD REFUSE after version bump if templates missing"
assert_file_contains "$UPGRADE_MD" "advisory only" \
    "upgrade.md marks the template prompt as advisory only"

# ============================================================
# Contract 14: edikt_version bump happens after templates check
# ============================================================

# Order matters: templates check happens in Section 2, version bump in Section 6.1,
# template prompt in Section 6.3 (after bump). Verify the flow.
assert_file_contains "$UPGRADE_MD" "Bump \`edikt_version\` in config" \
    "upgrade.md bumps edikt_version in config after the upgrade"

# The "after the version bump" phrase should appear in the post-upgrade section
if grep -qF "This notice fires AFTER the" "$UPGRADE_MD"; then
    pass "upgrade.md sequences template prompt after version bump"
else
    fail "upgrade.md sequences template prompt after version bump" \
        "Template prompt must fire after the version bump so the message has the correct version context"
fi

test_summary
