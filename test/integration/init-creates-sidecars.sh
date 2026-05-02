#!/usr/bin/env bash
# Phase 6b integration: init.md wires sidecar generation for seeded /
# adopted artifacts.
#
# What this test asserts:
#   1. commands/init.md contains the new step "4b. Generate Sidecars
#      for Seeded/Adopted Artifacts" wired to the per-artifact :compile
#      commands.
#   2. The step prints the "Created N sidecars" message.
#   3. The skip-list rule (ADR-008/ADR-009/SPEC) is honored at the
#      command-prose level.
#
# The end-to-end LLM-driven path (running /edikt:init in a Claude Code
# session and asserting that real sidecars get written) lives in
# test/integration/test_init_greenfield.py — that's the right scope for
# session-driven E2E because it needs a Claude session.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0
assert() {
    local label="$1"
    local cmd="$2"
    if eval "$cmd" >/dev/null 2>&1; then
        echo -e "  ${GREEN}✓${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} $label"
        echo -e "    ${DIM}cmd: $cmd${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

echo "Phase 6b — init-creates-sidecars (static contract)"

INIT_MD="$PROJECT_ROOT/commands/init.md"
assert "init.md exists" "[ -f '$INIT_MD' ]"
assert "init.md adds step 4b for sidecar generation" \
    "grep -qE '4b\. Generate Sidecars' '$INIT_MD'"
assert "init.md calls /edikt:adr:compile per artifact" \
    "grep -qF '/edikt:adr:compile' '$INIT_MD'"
assert "init.md calls /edikt:invariant:compile per artifact" \
    "grep -qF '/edikt:invariant:compile' '$INIT_MD'"
assert "init.md calls /edikt:guideline:compile per artifact" \
    "grep -qF '/edikt:guideline:compile' '$INIT_MD'"
assert "init.md prints 'Created N sidecars'" \
    "grep -qF 'Created {N} sidecars' '$INIT_MD'"
assert "init.md cites ADR-027 for the sidecar contract" \
    "grep -qF 'ADR-027' '$INIT_MD'"
assert "init.md honors the documentation skip-list" \
    "grep -qF 'ADR-008' '$INIT_MD' && grep -qF 'ADR-009' '$INIT_MD' && grep -qF 'SPEC-' '$INIT_MD'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
