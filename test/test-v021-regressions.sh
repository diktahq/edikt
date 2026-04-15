#!/bin/bash
# Test: v0.2.1 bug regressions
# Guards against the 4 bugs fixed in v0.2.1:
#   1. Website content files referencing old flat command paths (dead links)
#   2. install.sh not cleaning up old flat commands from v0.1.x
#   3. install.sh not checking curl exit status
#   4. init.md not adopting detected ADR path into config
#   5. commands/sdlc/review.md containing seniority prefixes
set -uo pipefail

PROJECT_ROOT="${1:-.}"
source "$(dirname "$0")/helpers.sh"

echo ""

# ============================================================
# Regression 1: website content has no dead links to old flat paths
# ============================================================
# The v0.2.0 namespacing refactor moved these commands. Any surviving
# `/commands/<old>` link in website markdown will break the VitePress build.

WEBSITE_CONTENT_DEAD_LINKS=$(grep -rEn '\]\(/commands/(prd|spec|spec-artifacts|plan|compile|drift|audit|review-governance|rules-update|sync|intake)(\)|/)' \
    "$PROJECT_ROOT/website" \
    --include="*.md" \
    --exclude-dir=".vitepress" \
    2>/dev/null || true)

if [ -z "$WEBSITE_CONTENT_DEAD_LINKS" ]; then
    pass "Website content has no dead links to old flat command paths"
else
    fail "Website content has dead links to old flat command paths" "$WEBSITE_CONTENT_DEAD_LINKS"
fi

# Specifically check the 4 files that broke the v0.2.0 deploy pipeline
for file in website/commands/brainstorm.md website/governance/chain.md website/governance/compile.md website/governance/drift.md; do
    full_path="$PROJECT_ROOT/$file"
    if [ -f "$full_path" ]; then
        # Must NOT contain these broken links (must use new namespace paths)
        BROKEN=$(grep -E '\]\(/commands/(prd|spec|plan|compile|drift|spec-artifacts)(\)|/)' "$full_path" 2>/dev/null || true)
        if [ -z "$BROKEN" ]; then
            pass "No flat-path dead links: $file"
        else
            fail "Flat-path dead links still present: $file" "$BROKEN"
        fi
    fi
done

# ============================================================
# Regression 2: install.sh cleans up old flat commands from v0.1.x
# ============================================================
# On upgrade from v0.1.x → v0.2.x, the installer must remove the old
# flat command files at the top level of ~/.claude/commands/edikt/
# (not the deprecated stubs, which live under deprecated/).

INSTALL_SH="$PROJECT_ROOT/install.sh"

# install.sh-internal assertions removed in v0.5.0 Phase 5 hardening — the
# bootstrap delegates to bin/edikt; coverage now lives under
# test/unit/launcher/ and test/integration/install/.

# ============================================================
# Regression 4: init.md adopts detected ADR path into config
# ============================================================
# When init detects ADRs in a non-default folder, it must prompt the user
# to either Adopt (configure edikt to use that path) or Migrate (move files
# to edikt's default). The generated .edikt/config.yaml must reflect the
# chosen location — never leave the default when ADRs live elsewhere.

INIT_MD="$PROJECT_ROOT/commands/init.md"

assert_file_contains "$INIT_MD" 'DETECTED_DECISIONS_PATH' \
    "init.md captures detected ADR folder path"

assert_file_contains "$INIT_MD" 'DETECTED_INVARIANTS_PATH' \
    "init.md captures detected invariants folder path"

assert_file_contains "$INIT_MD" 'Adopt' \
    "init.md offers Adopt option for detected ADRs"

assert_file_contains "$INIT_MD" 'Migrate' \
    "init.md offers Migrate option for detected ADRs"

# The config generation section must reference the captured paths
if grep -A3 '^paths:' "$INIT_MD" | grep -q 'DETECTED_DECISIONS_PATH'; then
    pass "init.md config template uses detected paths when Adopt chosen"
else
    fail "init.md config template uses detected paths when Adopt chosen" \
        "paths.decisions should reference \$DETECTED_DECISIONS_PATH"
fi

# Must explicitly warn about never leaving default when ADRs elsewhere
assert_file_contains "$INIT_MD" 'Never leave the default path' \
    "init.md documents the criticality of matching config to ADR location"

# ============================================================
# Regression 5: no seniority prefixes in command files
# ============================================================
# Agent role names were standardized to remove Senior/Principal/Staff
# prefixes. This must apply to command documentation too, not just
# agent templates.

SENIORITY_IN_COMMANDS=$(grep -rEn '\b(Senior|Principal|Staff)\s+(DBA|SRE|Security|API|Architect|Performance|Backend|Frontend|Platform|QA|Data|Docs|PM|UX|Compliance|Mobile|SEO|GTM|Evaluator)' \
    "$PROJECT_ROOT/commands" \
    "$PROJECT_ROOT/templates/agents" \
    --include="*.md" \
    2>/dev/null || true)

if [ -z "$SENIORITY_IN_COMMANDS" ]; then
    pass "No seniority prefixes (Senior/Principal/Staff) in commands or agents"
else
    fail "Seniority prefixes found in commands or agents" "$SENIORITY_IN_COMMANDS"
fi

test_summary
