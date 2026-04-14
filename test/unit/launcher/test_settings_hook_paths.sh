#!/bin/bash
# Phase 2 hook paths in templates/settings.json.tmpl point at
# $HOME/.edikt/hooks/*.sh. After Phase 3 install + use, every one of
# those paths must resolve through the new symlink chain.

set -uo pipefail
. "$(dirname "$0")/_lib.sh"

launcher_setup hook_paths

# Build a payload that includes EVERY hook referenced in settings.json.tmpl.
src="$LAUNCHER_ROOT/_src"
mkdir -p "$src/templates" "$src/commands/edikt" "$src/hooks"
printf '0.5.0\n' >"$src/VERSION"
printf '# x\n' >"$src/commands/edikt/context.md"

# Extract all hook script names from the real settings template.
HOOKS_TMPL="$PROJECT_ROOT/templates/settings.json.tmpl"
HOOK_NAMES=$(grep -oE '\.edikt/hooks/[a-z-]+\.sh' "$HOOKS_TMPL" | sed 's|.*/||' | sort -u)

[ -n "$HOOK_NAMES" ] && pass "found hook names in settings.json.tmpl" || fail "no hook names found"

for h in $HOOK_NAMES; do
    printf '#!/bin/sh\necho %s\n' "$h" >"$src/hooks/$h"
    chmod +x "$src/hooks/$h"
done

EDIKT_INSTALL_SOURCE="$src" run_launcher install 0.5.0 >/dev/null 2>&1
run_launcher use 0.5.0 >/dev/null 2>&1

test_start "every settings.json hook path resolves through symlink chain"

# This test runs in the sandbox. EDIKT_ROOT is our launcher root; the
# settings.json template uses $HOME/.edikt — equivalent only when
# EDIKT_ROOT == $HOME/.edikt. Validate via the launcher's chain instead
# of the literal $HOME path.
fail_n=0
for h in $HOOK_NAMES; do
    p="$LAUNCHER_ROOT/hooks/$h"
    if [ ! -x "$p" ]; then
        fail "hook resolves: $h" "$p not executable through symlink chain"
        fail_n=$((fail_n + 1))
    fi
done
[ "$fail_n" -eq 0 ] && pass "all $(echo "$HOOK_NAMES" | wc -w | tr -d ' ') hook paths resolve"

test_summary
