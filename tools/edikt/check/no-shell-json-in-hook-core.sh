#!/usr/bin/env bash
# no-shell-json-in-hook-core.sh — INV-003 for the Go hook tier.
#
# INV-003 says JSON is built with a serializer, never by string concatenation.
# The bash-era enforcement greps templates/hooks/*.sh for echo/printf JSON. As
# the hook tier moves to Go, the same rule needs the same enforcement in the
# new language — otherwise the invariant quietly stops applying to the code
# that replaced the code it was written for.
#
# The Go shape is stricter than the python3 heredoc it replaces: a struct
# cannot express the unterminated string an f-string typo can. What it CANNOT
# prevent is someone deciding a fmt.Sprintf is easier than a struct for "just
# this one small reply". That is what this greps for.
#
# WHAT COUNTS AS A VIOLATION
#
#   a Go string literal containing a JSON object opener  {"  or  {\"
#   a Go string literal containing a quoted JSON key     \"key\":
#   fmt.Sprintf / Fprintf / Printf whose format string opens a JSON object
#
# Struct tags (`json:"file_path"`) are NOT violations — they are how the
# serializer is told what to emit, which is the opposite of hand-building. The
# patterns are written to require a brace or an escaped quote so a tag cannot
# match.
#
# POSITIVE ASSERTION, NOT ONLY A NEGATIVE ONE. A package that emits no JSON at
# all would pass a pure absence grep while providing no serializer discipline
# whatsoever. So this also requires that the hook tier actually imports
# encoding/json somewhere — the check has to find the right answer present, not
# only the wrong answer absent.
#
# INV-013: zero files scanned exits non-zero. A grep over an empty tree finds
# no violations, and that is not a clean result.
#
# Usage:  tools/edikt/check/no-shell-json-in-hook-core.sh [--quiet]

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
HOOK_DIR="$ROOT/tools/edikt/internal/hook"

QUIET=0
[ "${1:-}" = "--quiet" ] && QUIET=1

if [ ! -d "$HOOK_DIR" ]; then
    echo "no-shell-json: $HOOK_DIR does not exist — NOTHING WAS SCANNED, which is not a pass." >&2
    exit 2
fi

# Each pattern is an ERE matched against non-comment source lines.
PATTERNS=(
    '\{\\"'
    '"\{"'
    '\\"[A-Za-z_][A-Za-z0-9_]*\\"[[:space:]]*:'
    'fmt\.(Sprintf|Fprintf|Printf)\([^)]*\{"'
)

files_scanned=0
violations=0
json_importers=0

while IFS= read -r src; do
    case "$src" in
        *_test.go) continue ;;
    esac
    files_scanned=$((files_scanned + 1))
    rel="${src#"$ROOT"/}"

    if grep -q '"encoding/json"' "$src"; then
        json_importers=$((json_importers + 1))
    fi

    body=$(awk '{ line=$0; sub(/^[ \t]+/, "", line); if (substr(line,1,2) != "//") print NR": "$0 }' "$src")
    for pat in "${PATTERNS[@]}"; do
        hits=$(printf '%s\n' "$body" | grep -E "$pat" || true)
        if [ -n "$hits" ]; then
            violations=$((violations + 1))
            if [ "$QUIET" -eq 0 ]; then
                echo "VIOLATION: $rel hand-builds JSON (pattern $pat):" >&2
                printf '%s\n' "$hits" >&2
            fi
        fi
    done
done < <(find "$HOOK_DIR" -type f -name '*.go' | sort)

if [ "$files_scanned" -eq 0 ]; then
    echo "no-shell-json: 0 Go files scanned under $HOOK_DIR — NOTHING WAS MEASURED (INV-013)." >&2
    exit 2
fi

if [ "$json_importers" -eq 0 ]; then
    echo "no-shell-json: $files_scanned file(s) scanned and NONE imports encoding/json." >&2
    echo "  The hook tier must serialize its replies through a serializer. Zero" >&2
    echo "  violations here would mean the tier emits no JSON at all, not that it" >&2
    echo "  emits JSON correctly." >&2
    exit 1
fi

if [ "$violations" -gt 0 ]; then
    echo "no-shell-json: $violations violation(s) across $files_scanned file(s) — INV-003 requires" >&2
    echo "  every hook reply to be built with encoding/json, never string concatenation." >&2
    exit 1
fi

[ "$QUIET" -eq 0 ] && echo "no-shell-json: $files_scanned Go file(s) scanned against ${#PATTERNS[@]} hand-built-JSON pattern(s); 0 violations, $json_importers file(s) use encoding/json."
exit 0
