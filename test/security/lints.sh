#!/usr/bin/env bash
# Pins INV-003, INV-004, INV-008 — grep-based linters that catch regressions
# in the JSON-emission, agent-text-in-shell, and branch-tracking-URL rules.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── INV-003: hooks MUST NOT build JSON via shell concatenation.
# Forbidden: echo '{'..., echo "{\"k\":\"${V}\"}", printf '{...}'.
# Allowed: python3 -c 'json.dumps(...)'.
# Exception: static JSON like '{"continue": true}' is fine — no interpolation.
#
# Grep for `echo "{\"` (escaped-quote pattern) and `echo "{" + ${VAR}` patterns.
hits=$(grep -nE 'echo[[:space:]]+"\{\\"' templates/hooks/*.sh install.sh 2>/dev/null || true)
if [ -n "$hits" ]; then
    echo "[INV-003] shell-concatenated JSON with escaped quotes:" >&2
    echo "$hits" >&2
    fail=1
fi

# Printf with interpolated %s into JSON-shaped template — usually unsafe.
hits=$(grep -nE "printf[[:space:]]+'\{[^']*\"%s\"" templates/hooks/*.sh install.sh 2>/dev/null || true)
if [ -n "$hits" ]; then
    # Filter out known-safe static-interpolation patterns (continue:true, etc)
    hits=$(echo "$hits" | grep -vE 'continue.*true|tool_use_id' || true)
    if [ -n "$hits" ]; then
        echo "[INV-003] printf %s into JSON template (likely unsafe):" >&2
        echo "$hits" >&2
        fail=1
    fi
fi

# ── INV-004: hooks MUST NOT embed attacker-derived ${VAR} into systemMessage
# or additionalContext that contains a bash code fence.
hits=$(grep -nE '```bash' templates/hooks/*.sh 2>/dev/null || true)
if [ -n "$hits" ]; then
    echo "[INV-004] bash code fence inside hook script (likely tells Claude to execute shell):" >&2
    echo "$hits" >&2
    fail=1
fi

# ── INV-008: no branch-tracking install URLs in USER-FACING docs and CI.
# Scope (the files a user reads or CI runs):
#   - README.md
#   - .github/workflows/
#   - website/ (documentation site)
#   - docs/guides/ (user-facing guides)
# Out of scope (historical / quoted / self-referential):
#   - docs/plans/, docs/product/, docs/architecture/, docs/reports/ — historical
#     records, ADRs that quote the forbidden pattern as an example of what's
#     forbidden, spec prose describing prior design state. INV-008 protects
#     what users COPY, not what we document about the past.
#   - docs/internal/ — gitignored.
_scope_dirs="README.md .github/workflows website docs/guides"
hits=""
for d in $_scope_dirs; do
    [ -e "$d" ] || continue
    # Skip build output under website/.vitepress/dist/ and cache/
    h=$(grep -rnE 'raw\.githubusercontent\.com/diktahq/edikt/(main|HEAD|master)/' "$d" \
        --exclude-dir='.vitepress' \
        --exclude-dir='dist' \
        --exclude-dir='cache' \
        --exclude-dir='node_modules' \
        2>/dev/null || true)
    [ -n "$h" ] && hits="${hits}${h}
"
done
if [ -n "$hits" ]; then
    echo "[INV-008] raw.githubusercontent.com tracking a moving ref in user-facing scope:" >&2
    echo "$hits" >&2
    fail=1
fi

hits=""
for d in $_scope_dirs; do
    [ -e "$d" ] || continue
    h=$(grep -rnE 'github\.com/diktahq/edikt/releases/latest/download/' "$d" \
        --exclude-dir='.vitepress' \
        --exclude-dir='dist' \
        --exclude-dir='cache' \
        --exclude-dir='node_modules' \
        2>/dev/null || true)
    [ -n "$h" ] && hits="${hits}${h}
"
done
if [ -n "$hits" ]; then
    echo "[INV-008] releases/latest/download/ in user-facing scope:" >&2
    echo "$hits" >&2
    fail=1
fi

exit $fail
