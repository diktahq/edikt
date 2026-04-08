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

assert_file_contains "$INSTALL_SH" "V01_MOVED_COMMANDS" \
    "install.sh declares V01_MOVED_COMMANDS cleanup list"

# Must list all 15 moved commands
for cmd in adr invariant compile review-governance rules-update sync prd spec spec-artifacts plan review drift audit docs intake; do
    if grep -E '^V01_MOVED_COMMANDS=.*\b'"$cmd"'\b' "$INSTALL_SH" >/dev/null 2>&1; then
        pass "V01_MOVED_COMMANDS includes '$cmd'"
    else
        fail "V01_MOVED_COMMANDS includes '$cmd'" "Missing from cleanup list in install.sh"
    fi
done

# Cleanup must run `rm -f` on old flat files
if grep -E 'rm -f.*edikt/\$\{cmd\}\.md|rm -f "\$old"' "$INSTALL_SH" >/dev/null 2>&1; then
    pass "install.sh removes old flat files after backup"
else
    fail "install.sh removes old flat files after backup" \
        "No 'rm -f \$old' (or similar) found in cleanup loop"
fi

# Cleanup must preserve user-customized files
assert_file_contains "$INSTALL_SH" 'keeping old edikt:' \
    "install.sh preserves user-customized old commands during cleanup"

# ============================================================
# Regression 3: install.sh checks curl exit status
# ============================================================
# Bare `curl -o` can silently fail and leave stale files. All downloads
# must go through a _fetch helper that exits on failure.

assert_file_contains "$INSTALL_SH" '_fetch()' \
    "install.sh defines _fetch helper"

assert_file_contains "$INSTALL_SH" 'if ! curl -fsSL --retry' \
    "install.sh _fetch uses --retry flag"

if grep -F -- '--max-time' "$INSTALL_SH" >/dev/null 2>&1; then
    pass "install.sh _fetch uses --max-time to avoid hangs"
else
    fail "install.sh _fetch uses --max-time to avoid hangs" \
        "No --max-time flag found in install.sh"
fi

# Must error out on failure, not silently continue
if grep -A3 '_fetch()' "$INSTALL_SH" | grep -q 'error "Failed to download'; then
    pass "install.sh _fetch aborts on download failure"
else
    fail "install.sh _fetch aborts on download failure" \
        "_fetch should call 'error' on curl failure"
fi

# Must detect empty downloads
if grep -A6 '_fetch()' "$INSTALL_SH" | grep -q '\-s "\$dest"'; then
    pass "install.sh _fetch detects empty downloads"
else
    fail "install.sh _fetch detects empty downloads" \
        "_fetch should check that downloaded file is non-empty"
fi

# No bare `curl -fsSL ... -o` calls should remain (outside of _fetch itself
# and header comments). Allow the comment example and the _fetch definition.
BARE_CURL=$(grep -nE 'curl -fsSL.*-o ' "$INSTALL_SH" \
    | grep -v '^[0-9]*:#' \
    | grep -v 'curl -fsSL --retry' \
    || true)
if [ -z "$BARE_CURL" ]; then
    pass "install.sh has no bare 'curl -o' calls outside _fetch"
else
    fail "install.sh has bare 'curl -o' calls outside _fetch" "$BARE_CURL"
fi

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
