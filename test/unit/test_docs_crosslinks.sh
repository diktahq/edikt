#!/usr/bin/env bash
# Cross-link validation — every new guide must be linked from at least one
# command page and from getting-started.md.
#
# Per Phase 14 AC: guides that exist in website/guides/ but have no inbound
# links are invisible to users. This test is the mechanized audit.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
. "$PROJECT_ROOT/test/helpers.sh"

WEBSITE="$PROJECT_ROOT/website"
GUIDES_DIR="$WEBSITE/guides"
GETTING_STARTED="$WEBSITE/getting-started.md"
COMMANDS_DIR="$WEBSITE/commands"

echo ""

# ─── Required guides ─────────────────────────────────────────────────────────
# Every guide in this list must be linked from at least one command page
# AND from getting-started.md.

REQUIRED_GUIDES=(
    "upgrade-and-rollback.md"
    "migrating-from-v0.4.md"
    "homebrew.md"
)

for guide in "${REQUIRED_GUIDES[@]}"; do
    guide_path="$GUIDES_DIR/$guide"

    test_start "guide exists: $guide"
    if [ -f "$guide_path" ]; then
        pass "guide exists: $guide"
    else
        fail "guide exists: $guide" "not found at $guide_path"
        continue
    fi

    # Check linked from getting-started.md.
    test_start "$guide linked from getting-started.md"
    if grep -q "$guide" "$GETTING_STARTED" 2>/dev/null; then
        pass "$guide linked from getting-started.md"
    else
        fail "$guide linked from getting-started.md" \
            "no reference to $guide found in $GETTING_STARTED"
    fi

    # Check linked from at least one command page.
    test_start "$guide linked from at least one command page"
    if grep -rq "$guide" "$COMMANDS_DIR" 2>/dev/null; then
        pass "$guide linked from a command page"
    else
        fail "$guide linked from a command page" \
            "no reference to $guide in $COMMANDS_DIR/**"
    fi
done

# ─── Internal link targets exist ─────────────────────────────────────────────
# Check that markdown links in guides pointing to other local files resolve.

test_start "guide internal links resolve"
broken=0
for guide in "$GUIDES_DIR"/*.md; do
    while IFS= read -r line; do
        # Extract [text](path) links — local relative paths only.
        echo "$line" | grep -oE '\]\([^)]+\.md[^)]*\)' | tr -d ')(' | while IFS= read -r target; do
            # Strip any fragment (#section).
            base="${target%%#*}"
            # Resolve relative to guides/ dir.
            resolved="$GUIDES_DIR/$base"
            if [ ! -f "$resolved" ]; then
                # Also try relative to website/.
                resolved2="$WEBSITE/$base"
                if [ ! -f "$resolved2" ]; then
                    echo "  BROKEN: $(basename $guide) → $base"
                    broken=1
                fi
            fi
        done
    done < "$guide"
done
[ "$broken" -eq 0 ] && pass "guide internal links resolve" || fail "guide internal links resolve"

test_summary
