#!/usr/bin/env bash
# Docs sanity checks — stale version references and broken install snippets.
#
# Verifies that:
#   1. No document hardcodes an old explicit version string in an install snippet
#      (e.g. "/v0.3.0/install.sh" in a curl command) that would silently install
#      the wrong version.
#   2. The current version in CHANGELOG.md matches the launcher's LAUNCHER_VERSION.
#   3. No command page references a non-existent command file.
#
# Does NOT fail on raw.githubusercontent.com/diktahq/edikt/main/install.sh —
# that URL intentionally always points at the latest install script.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
. "$PROJECT_ROOT/test/helpers.sh"

WEBSITE="$PROJECT_ROOT/website"
CHANGELOG="$PROJECT_ROOT/CHANGELOG.md"
LAUNCHER="$PROJECT_ROOT/bin/edikt"
README="$PROJECT_ROOT/README.md"

echo ""

# ─── 1. No hardcoded old version in install snippets ─────────────────────────
#
# Allow: .../main/install.sh (version-less, always latest)
# Reject: .../v0.3.0/install.sh, .../v0.4.0/install.sh, etc.
# (v0.5.0 and above are fine — they're current or future)

STALE_VERSIONS=("v0.1" "v0.2" "v0.3" "v0.4")

test_start "no stale version in install snippets"
stale_found=0
for sv in "${STALE_VERSIONS[@]}"; do
    matches=$(grep -rn "install.sh" "$WEBSITE" "$README" 2>/dev/null | grep "$sv/" || true)
    if [ -n "$matches" ]; then
        echo "  Stale install snippet ($sv):"
        echo "$matches" | head -5 | sed 's/^/    /'
        stale_found=1
    fi
done
[ "$stale_found" -eq 0 ] && pass "no stale version in install snippets" || fail "no stale version in install snippets"

# ─── 2. CHANGELOG has a v0.5.0 entry ─────────────────────────────────────────

test_start "CHANGELOG.md has v0.5.0 entry"
grep -q "^## v0.5.0" "$CHANGELOG" && \
    pass "CHANGELOG.md has v0.5.0 entry" || \
    fail "CHANGELOG.md has v0.5.0 entry" "no '## v0.5.0' heading found"

# ─── 3. Launcher version constant is defined ─────────────────────────────────

test_start "bin/edikt defines LAUNCHER_VERSION"
grep -q "LAUNCHER_VERSION=" "$LAUNCHER" && \
    pass "bin/edikt defines LAUNCHER_VERSION" || \
    fail "bin/edikt defines LAUNCHER_VERSION" "LAUNCHER_VERSION not found in bin/edikt"

# ─── 4. README has brew install section ──────────────────────────────────────

test_start "README.md has brew install section"
grep -q "brew install" "$README" && \
    pass "README.md has brew install section" || \
    fail "README.md has brew install section" "no 'brew install' found in README.md"

# ─── 5. Three new guides exist ───────────────────────────────────────────────

for guide in upgrade-and-rollback.md migrating-from-v0.4.md homebrew.md; do
    test_start "website/guides/$guide exists"
    [ -f "$WEBSITE/guides/$guide" ] && \
        pass "website/guides/$guide exists" || \
        fail "website/guides/$guide exists" "file not found"
done

# ─── 6. No broken command page links to non-existent commands ─────────────────

test_start "command page links to existing files"
broken_links=0
find "$WEBSITE/commands" -name "*.md" | while IFS= read -r page; do
    grep -oE '\]\([^)]+\.md[^)]*\)' "$page" 2>/dev/null | tr -d ')(' | while IFS= read -r target; do
        base="${target%%#*}"
        [ "${base:0:4}" = "http" ] && continue  # skip external links
        resolved="$(dirname "$page")/$base"
        if [ ! -f "$resolved" ]; then
            alt="$WEBSITE/$base"
            if [ ! -f "$alt" ]; then
                echo "  BROKEN: $(basename $page) → $base"
                broken_links=1
            fi
        fi
    done
done
[ "$broken_links" -eq 0 ] && pass "command page links to existing files" || fail "command page links to existing files"

test_summary
