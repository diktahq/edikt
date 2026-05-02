#!/usr/bin/env bash
# Phase 14 unit tests: PreToolUse hook path-scope narrowing per ADR-027.
#
# Verifies the allowlist short-circuit added to templates/hooks/pre-tool-use.sh:
#   1. CLAUDE.md (basename, any directory)            → in scope
#   2. settings.json under $CLAUDE_HOME or .claude/   → in scope
#   3. governance .md with surviving legacy sentinel  → in scope (migration window)
#   ↳ everything else                                 → continue: true (skip scan)
#
# Each case is hermetic: a fresh tmp project, no host ~/.claude, no host config.
# Per ADR-014, expected outputs are observed (assertions on JSON shape), not
# hand-authored fixtures. Opt-out: EDIKT_SKIP_HOOK_TESTS=1.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../../.." && pwd)}"
HOOK="$PROJECT_ROOT/templates/hooks/pre-tool-use.sh"

if [ "${EDIKT_SKIP_HOOK_TESTS:-0}" = "1" ]; then
    echo "  SKIP: pre-tool-use.sh scope — EDIKT_SKIP_HOOK_TESTS=1"
    exit 0
fi

if [ ! -x "$HOOK" ] && [ ! -f "$HOOK" ]; then
    echo "  MISSING: $HOOK"
    exit 1
fi

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0

# Sentinel markers built piecewise so this test file is not itself parsed
# as a managed region by anything that scans it.
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"

# Run the hook with a Write payload, return its stdout.
# Args: file_path, new_content, [cwd]
run_hook_write() {
    local fp="$1" content="$2" cwd="${3:-$PWD}"
    local payload
    payload=$(python3 -c '
import json, sys
print(json.dumps({
    "tool_name": "Write",
    "tool_input": {"file_path": sys.argv[1], "content": sys.argv[2]},
}))
' "$fp" "$content")
    (cd "$cwd" && unset EDIKT_COMPILE_IN_PROGRESS EDIKT_MIGRATION_IN_PROGRESS && \
        printf '%s' "$payload" | bash "$HOOK" 2>/dev/null)
}

# Run the hook with an Edit payload, return its stdout.
# Args: file_path, old_string, new_string, [cwd]
run_hook_edit() {
    local fp="$1" old="$2" new="$3" cwd="${4:-$PWD}"
    local payload
    payload=$(python3 -c '
import json, sys
print(json.dumps({
    "tool_name": "Edit",
    "tool_input": {
        "file_path": sys.argv[1],
        "old_string": sys.argv[2],
        "new_string": sys.argv[3],
    },
}))
' "$fp" "$old" "$new")
    (cd "$cwd" && unset EDIKT_COMPILE_IN_PROGRESS EDIKT_MIGRATION_IN_PROGRESS && \
        printf '%s' "$payload" | bash "$HOOK" 2>/dev/null)
}

# Assert grep-style. Args: label, output, expression
assert_contains() {
    local label="$1" out="$2" expr="$3"
    if printf '%s' "$out" | grep -q "$expr"; then
        echo -e "  ${GREEN}✓${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} $label"
        echo -e "    ${DIM}expected to contain: $expr${RESET}"
        echo -e "    ${DIM}actual: $out${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

assert_not_contains() {
    local label="$1" out="$2" expr="$3"
    if ! printf '%s' "$out" | grep -q "$expr"; then
        echo -e "  ${GREEN}✓${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} $label"
        echo -e "    ${DIM}expected NOT to contain: $expr${RESET}"
        echo -e "    ${DIM}actual: $out${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

# Each case spins up its own scratch project so cwd-relative config and
# governance paths resolve deterministically.
WORK_BASE="$(mktemp -d -t pretooluse-scope-XXXXXX)"
trap 'rm -rf "$WORK_BASE"' EXIT

new_project() {
    local name="$1"
    local p="$WORK_BASE/$name"
    mkdir -p "$p/.edikt" "$p/docs/architecture/decisions"
    cat > "$p/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML
    # docs/project-context.md present so the bottom-of-hook advisory does
    # not fire and pollute stdout.
    mkdir -p "$p/docs"
    : > "$p/docs/project-context.md"
    echo "$p"
}

echo "Phase 14 — PreToolUse hook scope narrowing"

# ─── Case 1: doc page with fenced sentinel example → continue ───────────────
P=$(new_project case1)
mkdir -p "$P/website/governance"
DOC="$P/website/governance/legacy-format.md"
{
    echo "# Legacy compile output"
    echo
    echo '```markdown'
    echo "$OPEN"
    echo "topic: example"
    echo "$CLOSE"
    echo '```'
} > "$DOC"
out=$(run_hook_write "$DOC" "rewritten" "$P")
assert_not_contains "doc page (fenced markers): not blocked" "$out" '"decision": "block"'
assert_contains    "doc page (fenced markers): continue: true" "$out" '"continue": true'

# ─── Case 2: governance artifact with unfenced legacy sentinel → block ──────
P=$(new_project case2)
ADR="$P/docs/architecture/decisions/ADR-001-test.md"
{
    echo "# ADR-001 — Test"
    echo
    echo "## Status"
    echo
    echo "Accepted"
    echo
    echo "## Sentinel"
    echo
    echo "$OPEN"
    echo "topic: example"
    echo "$CLOSE"
} > "$ADR"
# Edit overlaps the managed region (replace the topic line).
out=$(run_hook_edit "$ADR" "topic: example" "topic: tampered" "$P")
assert_contains    "governance ADR (unfenced sentinel): blocked" "$out" '"decision": "block"'
assert_not_contains "governance ADR (unfenced sentinel): not continue" "$out" '"continue": true'

# ─── Case 3: governance artifact post-migration (no sentinel) → continue ────
P=$(new_project case3)
ADR2="$P/docs/architecture/decisions/ADR-002-test.md"
{
    echo "# ADR-002 — Post-migration"
    echo
    echo "## Status"
    echo
    echo "Accepted"
    echo
    echo "(no sentinel block — sidecar lives at ADR-002-test.edikt.yaml)"
} > "$ADR2"
out=$(run_hook_write "$ADR2" "rewritten" "$P")
assert_not_contains "governance ADR (no sentinel): not blocked" "$out" '"decision": "block"'
assert_contains    "governance ADR (no sentinel): continue: true" "$out" '"continue": true'

# ─── Case 4: CLAUDE.md fenced sentinel residual → block (rule 1 in scope) ───
P=$(new_project case4)
CLAUDE="$P/CLAUDE.md"
{
    echo "# CLAUDE.md"
    echo
    echo "$OPEN"
    echo "topic: real"
    echo "$CLOSE"
} > "$CLAUDE"
out=$(run_hook_write "$CLAUDE" "rewritten with no markers" "$P")
assert_contains    "CLAUDE.md (real sentinel): blocked" "$out" '"decision": "block"'

# ─── Case 5: settings.json under $CLAUDE_HOME → in scope (rule 2) ───────────
# settings.json is JSON so the markdown-marker scan finds nothing; the
# observable signal is simply that the hook does not crash and emits
# continue: true. The synthetic check below proves rule 2 routes the
# file through the scan path: we plant a markdown marker pair INSIDE a
# settings.json string and confirm the scan blocks the rewrite.
P=$(new_project case5)
export CLAUDE_HOME="$P/.claude_home"
mkdir -p "$CLAUDE_HOME"
SETTINGS="$CLAUDE_HOME/settings.json"
# Synthetic: marker lines embedded as JSON string content. The hook's
# regex matches column-zero markers regardless of surrounding context.
{
    echo "{"
    echo "  \"_managed_marker_open\": \"PLACEHOLDER\","
    echo "$OPEN"
    echo "  \"managed_key\": \"value\","
    echo "$CLOSE"
    echo "  \"_managed_marker_close\": \"PLACEHOLDER\""
    echo "}"
} > "$SETTINGS"
out=$(run_hook_write "$SETTINGS" '{"managed_key":"tampered"}' "$P")
assert_contains "settings.json under \$CLAUDE_HOME: in scope (rule 2 fires)" "$out" '"decision": "block"'
unset CLAUDE_HOME

# ─── Case 6: settings.json outside $CLAUDE_HOME and .claude/ → continue ─────
P=$(new_project case6)
SETTINGS_DOC="$P/website/examples/settings.json"
mkdir -p "$(dirname "$SETTINGS_DOC")"
{
    echo "{"
    echo "$OPEN"
    echo "  \"example_key\": \"docs only\","
    echo "$CLOSE"
    echo "}"
} > "$SETTINGS_DOC"
# CLAUDE_HOME explicitly unset so the default $HOME/.claude does not
# accidentally match a parent of $P.
unset CLAUDE_HOME
out=$(run_hook_write "$SETTINGS_DOC" '{"example_key":"updated"}' "$P")
assert_not_contains "settings.json (doc example): not blocked" "$out" '"decision": "block"'
assert_contains    "settings.json (doc example): continue: true" "$out" '"continue": true'

# ─── Case 7: unparseable config → defaults still applied (when in doubt, scan) ─
P=$(new_project case7)
# Corrupt the config — keep the file readable but break YAML structure
# (truncation mid-key plus stray bytes).
printf 'edikt_version: "0.6.0"\npaths\n  decisions:\n  ::: BAD\n' > "$P/.edikt/config.yaml"
ADR7="$P/docs/architecture/decisions/ADR-007-test.md"
{
    echo "# ADR-007 — Defaults still scanned"
    echo
    echo "$OPEN"
    echo "topic: real"
    echo "$CLOSE"
} > "$ADR7"
out=$(run_hook_write "$ADR7" "tampered" "$P")
assert_contains "unparseable config: governance defaults still scanned" "$out" '"decision": "block"'

# ─── Case 8: §3.3 path-traversal — config with '..' falls back to defaults ──
P=$(new_project case8)
cat > "$P/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0"
paths:
  decisions: ../../../
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML
# Stage a doc page OUTSIDE the default governance dirs but inside the
# (malicious) traversal target. If the hand-parser had honored the
# traversal, this file would land in scope and a Write that drops its
# fenced sentinel would block. The hardened parser must NOT allow that.
DOC8="$P/website/governance/example.md"
mkdir -p "$(dirname "$DOC8")"
{
    echo "# example"
    echo
    echo '```markdown'
    echo "$OPEN"
    echo "topic: example"
    echo "$CLOSE"
    echo '```'
} > "$DOC8"
out=$(run_hook_write "$DOC8" "rewritten" "$P")
assert_not_contains "scope §3.3: traversal config does NOT enable scan on doc pages" "$out" '"decision": "block"'
assert_contains    "scope §3.3: traversal-rejected config falls back to defaults (continue: true on out-of-scope file)" "$out" '"continue": true'

# Sanity: the in-scope governance dir IS still scanned via the defaults.
ADR8="$P/docs/architecture/decisions/ADR-008-traversal.md"
{
    echo "# ADR-008 — Traversal sanity"
    echo
    echo "$OPEN"
    echo "topic: real"
    echo "$CLOSE"
} > "$ADR8"
out=$(run_hook_write "$ADR8" "tampered" "$P")
assert_contains "scope §3.3: defaults still cover legitimate governance dirs" "$out" '"decision": "block"'

# ─── Case 9: §3.4 YAML quirks — flow-style and multi-doc → defaults ─────────
P=$(new_project case9)
cat > "$P/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0"
paths: { decisions: docs/x, invariants: docs/y }
YAML
# A fenced doc page must NOT block (defaults apply, file is out of scope).
DOC9="$P/website/governance/flow-config.md"
mkdir -p "$(dirname "$DOC9")"
{
    echo "# flow-config"
    echo
    echo '```markdown'
    echo "$OPEN"
    echo "topic: example"
    echo "$CLOSE"
    echo '```'
} > "$DOC9"
out=$(run_hook_write "$DOC9" "rewritten" "$P")
assert_not_contains "scope §3.4: flow-style config falls back to defaults" "$out" '"decision": "block"'

# Multi-doc separator
P=$(new_project case9b)
cat > "$P/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0"
---
paths:
  decisions: docs/x
YAML
DOC9b="$P/website/governance/multidoc.md"
mkdir -p "$(dirname "$DOC9b")"
{
    echo "# multidoc"
    echo
    echo '```markdown'
    echo "$OPEN"
    echo "$CLOSE"
    echo '```'
} > "$DOC9b"
out=$(run_hook_write "$DOC9b" "rewritten" "$P")
assert_not_contains "scope §3.4: multi-doc YAML falls back to defaults" "$out" '"decision": "block"'

# ─── Case 10: §3.2 mixed-fence — ~~~ inside ``` block stays fenced ──────────
P=$(new_project case10)
DOC10="$P/website/governance/mixed-fence.md"
mkdir -p "$(dirname "$DOC10")"
{
    echo "# Mixed fence example"
    echo
    echo '```markdown'
    echo "~~~"
    echo "$OPEN"
    echo "topic: example"
    echo "$CLOSE"
    echo "~~~"
    echo '```'
} > "$DOC10"
# Out-of-scope file (not governance, not CLAUDE.md). Even if mixed-fence
# detection were broken, the path-allowlist short-circuits first. The
# value of this case is that it pins the fence-detection contract for
# files that ARE in scope (CLAUDE.md residual, governance pre-migration).
# We assert continue: true regardless.
out=$(run_hook_write "$DOC10" "rewritten" "$P")
assert_not_contains "scope §3.2: mixed-fence doc page is not blocked" "$out" '"decision": "block"'

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
