#!/bin/bash
# Test: edikt_version tracking — VERSION file, config field, upgrade logic
set -uo pipefail

PROJECT_ROOT="$(cd "${1:-.}" && pwd)"
source "$(dirname "$0")/helpers.sh"

echo ""

VERSION_FILE="$PROJECT_ROOT/VERSION"
CHANGELOG_FILE="$PROJECT_ROOT/CHANGELOG.md"

# ============================================================
# VERSION file
# ============================================================

# VERSION file exists
assert_file_exists "$VERSION_FILE" "VERSION file exists"

# VERSION file contains a semver-like string (X.Y or X.Y.Z)
if grep -qE '^[0-9]+\.[0-9]+' "$VERSION_FILE"; then
    pass "VERSION file contains a valid version number"
else
    fail "VERSION file contains a valid version number" "$(cat "$VERSION_FILE")"
fi

# VERSION file has no trailing garbage (just a version line)
LINE_COUNT=$(grep -c '[^[:space:]]' "$VERSION_FILE" || true)
if [ "$LINE_COUNT" -eq 1 ]; then
    pass "VERSION file has exactly one non-empty line"
else
    fail "VERSION file has exactly one non-empty line" "Found $LINE_COUNT non-empty lines"
fi

# ============================================================
# CHANGELOG
# ============================================================

assert_file_exists "$CHANGELOG_FILE" "CHANGELOG.md exists"

# CHANGELOG has at least one version section
if grep -qE '^## v[0-9]' "$CHANGELOG_FILE"; then
    pass "CHANGELOG.md has at least one version section"
else
    fail "CHANGELOG.md has at least one version section"
fi

# CHANGELOG version matches VERSION file (strip pre-release suffix for lookup)
FILE_VER=$(cat "$VERSION_FILE" | tr -d '[:space:]')
BASE_VER=$(echo "$FILE_VER" | sed 's/-.*//')
if grep -q "^## v${BASE_VER}" "$CHANGELOG_FILE"; then
    pass "CHANGELOG.md has an entry for version $FILE_VER"
else
    fail "CHANGELOG.md has an entry for version $FILE_VER" "No '## v${BASE_VER}' section found"
fi

# ============================================================
# .edikt/config.yaml edikt_version
# ============================================================

CONFIG_FILE="$PROJECT_ROOT/.edikt/config.yaml"

assert_file_contains "$CONFIG_FILE" "edikt_version" ".edikt/config.yaml has edikt_version field"

# edikt_version value matches VERSION file
CONFIG_VER=$(grep '^edikt_version:' "$CONFIG_FILE" | awk '{print $2}' | tr -d '"' 2>/dev/null || echo "")
if [ "$CONFIG_VER" = "$FILE_VER" ]; then
    pass "edikt_version in .edikt/config.yaml matches VERSION ($FILE_VER)"
else
    fail "edikt_version in .edikt/config.yaml matches VERSION" "config=$CONFIG_VER, file=$FILE_VER"
fi

# ============================================================
# install.sh downloads VERSION + CHANGELOG
# ============================================================

assert_file_contains "$PROJECT_ROOT/install.sh" "VERSION" "install.sh downloads VERSION file"
assert_file_contains "$PROJECT_ROOT/install.sh" "CHANGELOG.md" "install.sh downloads CHANGELOG.md"

# ============================================================
# commands/init.md writes edikt_version
# ============================================================

assert_file_contains "$PROJECT_ROOT/commands/init.md" "edikt_version" "init.md writes edikt_version to config"
assert_file_contains "$PROJECT_ROOT/commands/init.md" "~/.edikt/VERSION" "init.md reads installed VERSION"

# ============================================================
# commands/upgrade.md handles version correctly
# ============================================================

assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt_version" "upgrade.md updates edikt_version"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "~/.edikt/VERSION" "upgrade.md reads installed VERSION"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "Always update" "upgrade.md always writes edikt_version"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "predates versioning" "upgrade.md handles projects missing edikt_version"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "edikt_version.*is missing" "upgrade.md detects missing edikt_version as upgrade"

# ============================================================
# commands/doctor.md checks version
# ============================================================

assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "edikt_version" "doctor.md checks edikt_version"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "~/.edikt/VERSION" "doctor.md reads installed VERSION"
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "edikt_version not set" "doctor.md warns when edikt_version missing"

# doctor output template shows version
assert_file_contains "$PROJECT_ROOT/commands/doctor.md" "edikt {version}" "doctor.md output shows version"

# ============================================================
# upgrade shows release notes (WHAT'S NEW)
# ============================================================

assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "WHAT'S NEW" "upgrade.md shows WHAT'S NEW section"
assert_file_contains "$PROJECT_ROOT/commands/upgrade.md" "CHANGELOG" "upgrade.md reads CHANGELOG"

# ============================================================
# website/guides/upgrading.md reflects versioning
# ============================================================

assert_file_contains "$PROJECT_ROOT/website/guides/upgrading.md" "edikt_version" "upgrading guide explains edikt_version"
assert_file_contains "$PROJECT_ROOT/website/guides/upgrading.md" "~/.edikt/VERSION" "upgrading guide mentions VERSION file"
assert_file_contains "$PROJECT_ROOT/website/guides/upgrading.md" "edikt:doctor" "upgrading guide mentions doctor for version check"

test_summary
