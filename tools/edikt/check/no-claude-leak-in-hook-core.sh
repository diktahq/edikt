#!/usr/bin/env bash
# no-claude-leak-in-hook-core.sh — SPEC-014 SP-004 / SAC-003 seam gate.
#
# WHAT THE SEAM IS, AND WHY A GREP ENFORCES IT
#
# tools/edikt/internal/hook/adapter/ is the ONLY place in the hook tier that
# may know a host harness's wire vocabulary — its JSON field spellings, its
# reply shape, its vendor name. Every handler receives a host-neutral
# adapter.Payload and returns a host-neutral adapter.Response, so a second
# harness is a new file in adapter/ and nothing else.
#
# That property is worth exactly as much as its enforcement. A seam nobody
# checks lasts until the first handler that finds it convenient to read the raw
# field, and by then the cost of the port is 22 rewrites — which is the whole
# thing SP-004 exists to prevent. So this runs in CI.
#
# WHAT IT CHECKS
#
#   1. No file under internal/hook/ OUTSIDE adapter/ names the host vendor
#      (case-insensitive) in non-comment code.
#   2. No such file names a host WIRE FIELD — the snake_case and camelCase JSON
#      keys the harness actually sends and reads. Handlers use Go field names
#      (FilePath, ToolName); the wire spellings (file_path, tool_name) belong
#      to the adapter alone, so the two vocabularies never collide.
#   3. adapter/ actually contains an implementation. "No leaks outside
#      adapter/" is vacuous when adapter/ is empty.
#
# COMMENTS ARE EXCLUDED, deliberately and with a known limitation. Explaining
# the seam requires naming the host it separates from, and a gate that forbids
# its own rationale gets deleted. Only lines whose trimmed form starts with //
# are stripped, so a wire field inside a /* */ block would be missed. Stated
# rather than hidden; every file in this package uses // comments.
#
# INV-013: a run that scanned ZERO handler files exits non-zero. "No violations
# found" over an empty file set is not a clean result, it is no result, and it
# is exactly what a moved or renamed package directory would produce.
#
# Usage:  tools/edikt/check/no-claude-leak-in-hook-core.sh [--quiet]

set -uo pipefail

ROOT="$(cd "$(dirname "$0")/../../.." && pwd)"
HOOK_DIR="$ROOT/tools/edikt/internal/hook"
ADAPTER_DIR="$HOOK_DIR/adapter"

QUIET=0
[ "${1:-}" = "--quiet" ] && QUIET=1

say() { [ "$QUIET" -eq 0 ] && echo "$@"; return 0; }

if [ ! -d "$HOOK_DIR" ]; then
    echo "no-claude-leak: $HOOK_DIR does not exist — NOTHING WAS SCANNED." >&2
    echo "  This is not a pass. The hook core is the subject of this gate; if the" >&2
    echo "  package moved, the gate must be repointed, not silently satisfied." >&2
    exit 2
fi
if [ ! -d "$ADAPTER_DIR" ]; then
    echo "no-claude-leak: $ADAPTER_DIR does not exist — the seam this gate enforces is absent." >&2
    exit 2
fi

# The host vendor token, matched case-insensitively as a bare substring: an
# import path, a type name, a variable, or a string literal all count.
VENDOR_PATTERN='[Cc][Ll][Aa][Uu][Dd][Ee]'

# Wire field denylist. Derived by reading every payload field access and every
# emitted JSON key across the current 22 bash hooks — not guessed. Each entry
# is a spelling only the host protocol uses; a handler naming one has reached
# past the adapter.
WIRE_FIELDS=(
    hook_event_name
    hookEventName
    hookSpecificOutput
    additionalContext
    systemMessage
    permissionDecision
    permissionDecisionReason
    stopReason
    stop_hook_active
    transcript_path
    last_assistant_message
    subagent_type
    session_id
    tool_name
    tool_input
    file_path
    old_string
    new_string
    agent_id
    agent_type
    task_id
    task_name
    worktree_path
)

files_scanned=0
violations=0
vendor_hits=0
field_hits=0

# Collect the handler files: every non-test .go under internal/hook/ that is
# not inside adapter/.
handler_files=""
while IFS= read -r src; do
    case "$src" in
        "$ADAPTER_DIR"/*) continue ;;
        *_test.go) continue ;;
    esac
    handler_files="$handler_files$src
"
done < <(find "$HOOK_DIR" -type f -name '*.go' | sort)

while IFS= read -r src; do
    [ -n "$src" ] || continue
    files_scanned=$((files_scanned + 1))
    rel="${src#"$ROOT"/}"

    # Strip pure // comment lines, keeping original line numbers.
    body=$(awk '{ line=$0; sub(/^[ \t]+/, "", line); if (substr(line,1,2) != "//") print NR": "$0 }' "$src")

    hits=$(printf '%s\n' "$body" | grep -E "$VENDOR_PATTERN" || true)
    if [ -n "$hits" ]; then
        vendor_hits=$((vendor_hits + 1))
        violations=$((violations + 1))
        if [ "$QUIET" -eq 0 ]; then
            echo "VIOLATION: $rel names the host vendor outside adapter/:" >&2
            printf '%s\n' "$hits" >&2
        fi
    fi

    for field in "${WIRE_FIELDS[@]}"; do
        fhits=$(printf '%s\n' "$body" | grep -F "$field" || true)
        if [ -n "$fhits" ]; then
            field_hits=$((field_hits + 1))
            violations=$((violations + 1))
            if [ "$QUIET" -eq 0 ]; then
                echo "VIOLATION: $rel names host wire field '$field' outside adapter/:" >&2
                printf '%s\n' "$fhits" >&2
            fi
        fi
    done
done <<EOF
$handler_files
EOF

if [ "$files_scanned" -eq 0 ]; then
    echo "no-claude-leak: 0 handler files scanned under $HOOK_DIR — NOTHING WAS MEASURED." >&2
    echo "  A seam gate with no subject must not report a clean seam (INV-013)." >&2
    exit 2
fi

adapter_files=$(find "$ADAPTER_DIR" -type f -name '*.go' -not -name '*_test.go' | wc -l | tr -d ' ')
if [ "$adapter_files" -eq 0 ]; then
    echo "no-claude-leak: adapter/ contains 0 non-test Go files — the seam is empty," >&2
    echo "  so 'no leaks outside adapter/' is vacuous." >&2
    exit 2
fi

if [ "$violations" -gt 0 ]; then
    echo "no-claude-leak: $violations violation(s) across $files_scanned handler file(s)" >&2
    echo "  ($vendor_hits vendor-name, $field_hits wire-field). SP-004 requires every host" >&2
    echo "  detail to live in internal/hook/adapter/ so a second harness is additive." >&2
    exit 1
fi

say "no-claude-leak: $files_scanned handler file(s) scanned against ${#WIRE_FIELDS[@]} wire field(s) plus the vendor token; 0 leaks. Seam implemented by $adapter_files adapter file(s)."
exit 0
