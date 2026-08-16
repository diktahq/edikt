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
LIST_EXEMPTIONS=0
case "${1:-}" in
    --quiet) QUIET=1 ;;
    # Publish the parsed exemption set as data. INV-014: a guard must be
    # able to assert this gate's resolved outcome without reimplementing
    # the comment-stripping rule and drifting from it — which is exactly
    # how the summary line below came to disagree with is_exempt.
    --list-exemptions) LIST_EXEMPTIONS=1 ;;
esac

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

if [[ $LIST_EXEMPTIONS -eq 1 ]]; then
    # One path per line, nothing else — no header, no summary. An empty
    # exempt set prints nothing and exits 0, which is a truthful "no
    # exemptions", not a failure to look.
    printf '%s\n' ${exempt[@]+"${exempt[@]}"}
    exit 0
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
        # -H forces the filename, and the prefix is then stripped by pattern
        # rather than by field index.
        #
        # This used to be `awk -F: '{ line=$3 }'`, which assumed grep always
        # emits `path:line:content`. BSD grep does for a single-file -R; GNU
        # grep OMITS the filename, shifting every field left by one. On Linux
        # $3 became a fragment of the matched text instead of the line, the
        # `substr(line,1,2) != "//"` comment filter never matched, and the
        # gate flagged ordinary comments — including the case this script's
        # own header says it must not flag.
        #
        # It had therefore only ever worked on macOS, and worked there by
        # accident. Field indexing was the wrong tool regardless of platform:
        # the matched CONTENT contains colons (`exec: "claude": not found`),
        # so -F: cannot locate the boundary in the general case. Stripping a
        # `path:line:` prefix by regex is deterministic on both.
        hits=$(grep -REHn --include='*.go' "$pat" "$src" 2>/dev/null \
            | awk '{ line=$0
                     sub(/^[^:]*:[0-9]+:/, "", line)
                     sub(/^[ \t]+/, "", line)
                     if (substr(line,1,2) != "//") print }' \
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

# ── Non-Go tier-2 files ──────────────────────────────────────────────────
# The .go sweep above is not sufficient. A shell helper, fixture, or template
# shipped under tools/edikt/ can dispatch an LLM just as effectively as Go
# code, and `find -name '*.go'` never sees it. This was not hypothetical: the
# cheat-rate package carried testdata/fake-claude-*.sh, which only
# sidecar-checks.yml's inline scan caught — this gate missed them entirely.
# That inline scan is being retired as a weaker duplicate, so its one genuine
# advantage is absorbed here first. Deleting a duplicate is only safe when the
# survivor is a strict superset.
#
# Excluded, deliberately:
#   .claude/ , .verikt/  host-agent config directories — references there are
#                        to the agent's own config, not a tier-2 dispatch
#   check/               these gates themselves, which must name the patterns
#                        they hunt for
while IFS= read -r src; do
    rel="${src#$ROOT/}"
    case "$rel" in
        */.claude/*|*/.verikt/*|tools/edikt/check/*) continue ;;
    esac
    if is_exempt "$rel"; then
        continue
    fi
    # Text files only — a compiled artifact would match on byte coincidence.
    if ! grep -Iq . "$src" 2>/dev/null; then
        continue
    fi
    hits=$(grep -En '\bclaude\b' "$src" 2>/dev/null || true)
    if [[ -n "$hits" ]]; then
        if [[ $QUIET -eq 0 ]]; then
            echo "VIOLATION: non-Go tier-2 file $rel references an LLM CLI:" >&2
            echo "$hits" >&2
        fi
        violations=$((violations + 1))
    fi
done < <(find "$TOOLS_DIR" -type f -not -name '*.go' -not -name 'go.sum' -not -name 'go.mod')

if [[ $violations -gt 0 ]]; then
    echo "no-llm-in-tier-2: $violations violation(s) — tier-2 binary must remain LLM-agnostic (ADR-030)" >&2
    exit 1
fi

# Report the exemptions the gate HONOURS, not the lines of the file it
# reads. `wc -l` counted the header and the per-entry ADR citations that
# INV-012 mandates above every path, so a list with one exempt file
# announced "13 exemption entries" — overstating the hole in the tier-2
# boundary twelvefold, using a derivation that had already diverged from
# the array is_exempt actually consults.
exempt_count=${#exempt[@]}
exempt_noun="exemption entries"
[[ $exempt_count -eq 1 ]] && exempt_noun="exemption entry"
[[ $QUIET -eq 0 ]] && echo "no-llm-in-tier-2: tier-2 sources are LLM-agnostic (per ADR-030, $exempt_count $exempt_noun)"
exit 0
