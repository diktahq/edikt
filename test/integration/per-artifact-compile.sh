#!/usr/bin/env bash
# Integration test for Phase 4b — per-artifact :compile commands.
#
# What this test asserts (today):
#   1. The three :compile command files exist.
#   2. Each command file dispatches the sidecar-extractor agent.
#   3. None of the three command files contain the legacy in-body sentinel
#      writing flow (the [edikt:directives:start] / [edikt:directives:end]
#      output that the ADR-008 schema produced). Per ADR-027, sidecar
#      regeneration replaces that flow.
#   4. The sidecar-extractor agent template is present and locked
#      (tools: Read, Write only — no Edit, Bash, Agent, or Task).
#
# What this test asserts (gated on Phase 8 — canonical YAML serialization):
#   - Run /edikt:adr:compile against a fixture; assert the sidecar appears.
#   - Run again; assert idempotency (canonical-equal output).
#   - Modify the source body; re-run; assert the sidecar changes.
#
# Phase 4b's prompt asked for the LLM-driven idempotency assertions. Those
# require a Claude Code session running the slash command — they live in the
# end-to-end integration test suite (test/integration/test_e2e_*.py),
# not in this shell file. This file gates the static contract: the commands
# are wired to dispatch the extractor and not to write in-body sentinels.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$PROJECT_ROOT"

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

echo "Phase 4b — per-artifact :compile static contract"

# 1. Command files exist.
for f in commands/adr/compile.md commands/invariant/compile.md commands/guideline/compile.md; do
    assert "exists: $f" "[ -f '$f' ]"
done

# 2. Each command dispatches the sidecar-extractor agent.
for f in commands/adr/compile.md commands/invariant/compile.md commands/guideline/compile.md; do
    assert "$f dispatches sidecar-extractor" "grep -q 'subagent_type: sidecar-extractor' '$f'"
done

# 3. None of the three contain the legacy in-body sentinel write flow.
#    Specifically: no instruction telling the command to write the literal
#    [edikt:directives:start] / [edikt:directives:end] markers as output.
#    (References to those markers in PROSE describing past behavior or
#    legacy-fixture documentation are tolerated; what's forbidden is an
#    output template that EMITS those markers.)
for f in commands/adr/compile.md commands/invariant/compile.md commands/guideline/compile.md; do
    # The legacy flow had a YAML code-block example showing:
    #   [edikt:directives:start]: #
    #   ...
    #   [edikt:directives:end]: #
    # as the artifact written to the parent .md. Detect that block as a
    # fenced YAML code section that contains both markers.
    if python3 - "$f" <<'PY'
import re, sys
body = open(sys.argv[1]).read()
# Match a fenced ```yaml or ``` block containing both sentinel markers.
fenced = re.findall(r'```(?:yaml)?\s*\n([\s\S]*?)```', body)
for block in fenced:
    if '[edikt:directives:start]' in block and '[edikt:directives:end]' in block:
        sys.exit(1)
sys.exit(0)
PY
    then
        assert "$f has no legacy in-body sentinel output template" "true"
    else
        assert "$f has no legacy in-body sentinel output template" "false"
    fi
done

# 4. The sidecar-extractor agent template is present and locked.
EXTRACTOR_TEMPLATE="templates/agents/sidecar-extractor.md"
assert "$EXTRACTOR_TEMPLATE exists" "[ -f '$EXTRACTOR_TEMPLATE' ]"
assert "$EXTRACTOR_TEMPLATE allows only Read + Write" \
    "python3 -c \"
import re,sys
body=open('$EXTRACTOR_TEMPLATE').read()
# Find the YAML frontmatter
m=re.match(r'---\n(.*?\n)---', body, re.DOTALL)
if not m: sys.exit(1)
fm=m.group(1)
# tools: list under frontmatter
tools_block=re.search(r'^tools:\n((?:  - .+\n)+)', fm, re.MULTILINE)
if not tools_block: sys.exit(1)
tools=set(re.findall(r'  - (\w+)', tools_block.group(1)))
sys.exit(0 if tools=={'Read','Write'} else 1)
\""
assert "$EXTRACTOR_TEMPLATE explicitly disallows Edit/Bash/Agent/Task" \
    "python3 -c \"
import re,sys
body=open('$EXTRACTOR_TEMPLATE').read()
m=re.match(r'---\n(.*?\n)---', body, re.DOTALL)
if not m: sys.exit(1)
fm=m.group(1)
da=re.search(r'^disallowedTools:\n((?:  - .+\n)+)', fm, re.MULTILINE)
if not da: sys.exit(1)
disallowed=set(re.findall(r'  - (\w+)', da.group(1)))
required={'Edit','Bash','Agent','Task'}
sys.exit(0 if required.issubset(disallowed) else 1)
\""

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
echo -e "${DIM}E2E idempotency check (LLM dispatch + canonical compare) lives in test/integration/test_e2e_*.py — gated on Phase 8.${RESET}"
exit "$fail_count"
