#!/usr/bin/env bash
# no-llm-in-tier-2.sh — ADR-030 purity gate.
#
# Tier-2 Go binaries (tools/edikt/, tools/<name>/) MUST NOT spawn,
# invoke, or shell out to any LLM CLI. The host agent (Claude Code,
# Codex, Cursor, …) owns LLM dispatch via tier-1 markdown — the Go
# binary is structurally agent-agnostic so edikt can support multiple
# host agents from a single tier-2 release.
#
# This script greps every non-test .go file under tools/edikt/ for the
# canonical patterns that indicate an LLM shell-out:
#   * exec.Command(...claude...)
#   * exec.LookPath("claude")
#   * the literal string "claude" inside any non-test source
#
# An exemption file (no-llm-in-tier-2.exempt) lists the paths that are
# carved out per ADR-030 — currently only internal/phasea/runner.go
# until v0.7.0 ships the Phase A refactor. Adding a new entry to the
# exemption file requires amending ADR-030.
#
# Usage:  tools/edikt/check/no-llm-in-tier-2.sh [--quiet]

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
TOOLS_DIR="$ROOT/tools/edikt"
EXEMPT_FILE="$ROOT/tools/edikt/check/no-llm-in-tier-2.exempt"

QUIET=0
[[ "${1:-}" == "--quiet" ]] && QUIET=1

# Build the exempt-paths set (relative to repo root). Lines starting
# with `#` and blank lines are ignored. Every entry MUST cite an ADR
# and a removal deadline in a comment immediately above it.
exempt=()
if [[ -f "$EXEMPT_FILE" ]]; then
    while IFS= read -r line; do
        line="${line%%#*}"
        line="${line## }"
        line="${line%% }"
        [[ -z "$line" ]] && continue
        exempt+=("$line")
    done < "$EXEMPT_FILE"
fi

is_exempt () {
    local rel="$1"
    for e in "${exempt[@]}"; do
        [[ "$rel" == "$e" ]] && return 0
    done
    return 1
}

# Source-level patterns that indicate an LLM shell-out from Go.
patterns=(
    'exec\.Command[^)]*"claude"'
    'exec\.LookPath\("claude"\)'
    'exec\.LookPath[^)]*claude'
    '"claude"'
)

violations=0
while IFS= read -r src; do
    rel="${src#$ROOT/}"
    if is_exempt "$rel"; then
        continue
    fi
    for pat in "${patterns[@]}"; do
        # -E for ERE, -n for line numbers, --include filtered by find.
        # Filter pure comment lines so referencing the pattern in a
        # doc-comment (e.g., this very script's header) does not
        # trip the gate.
        hits=$(grep -REn --include='*.go' "$pat" "$src" 2>/dev/null \
            | awk -F: 'NF>=3 { line=$3; sub(/^[ \t]+/, "", line); if (substr(line,1,2) != "//") print }' \
            || true)
        if [[ -n "$hits" ]]; then
            if [[ $QUIET -eq 0 ]]; then
                echo "VIOLATION: tier-2 source $rel contains LLM shell-out pattern '$pat':" >&2
                echo "$hits" >&2
            fi
            violations=$((violations + 1))
        fi
    done
done < <(find "$TOOLS_DIR" -type f -name '*.go' -not -name '*_test.go')

if [[ $violations -gt 0 ]]; then
    echo "no-llm-in-tier-2: $violations violation(s) — tier-2 binary must remain LLM-agnostic (ADR-030)" >&2
    exit 1
fi

[[ $QUIET -eq 0 ]] && echo "no-llm-in-tier-2: tier-2 sources are LLM-agnostic (per ADR-030, $(wc -l < "$EXEMPT_FILE" 2>/dev/null || echo 0) exemption entries)"
exit 0
