#!/usr/bin/env bash
# Integration test for Phase 4c — :review sidecar cross-check.
#
# What this test asserts (today):
#   1. Each :review command file (adr/invariant/guideline) contains the
#      Sidecar Cross-Check section (header + numbered steps).
#   2. The cross-check is read-only — no Write/Edit instructions inside
#      the cross-check section.
#   3. The fixture corpus is well-formed:
#       - synced/   — sidecar quotes match the prose body exactly
#       - drifted/  — sidecar quote does NOT match the prose body
#   4. A reference Python implementation of the cross-check produces:
#       - "in sync" for the synced fixture
#       - drift findings (extra rules + quote mismatches) for the drifted
#         fixture
#
# What this test does NOT do:
#   - Run the actual /edikt:adr:review slash command (that requires a
#     Claude Code session and lives in test/integration/test_e2e_*.py).
#     The reference Python implementation here mirrors what the LLM is
#     instructed to do; if the reference behavior holds, the LLM has a
#     well-defined target to match.

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

echo "Phase 4c — :review sidecar cross-check"

# 1. Each :review command contains the Sidecar Cross-Check section.
for f in commands/adr/review.md commands/invariant/review.md commands/guideline/review.md; do
    assert "$f has 'Sidecar Cross-Check' section" \
        "grep -q '## Sidecar Cross-Check' '$f'"
done

# 2. The cross-check is read-only (no Write/Edit in section body).
for f in commands/adr/review.md commands/invariant/review.md commands/guideline/review.md; do
    assert "$f cross-check is read-only (no Write/Edit instruction in section)" \
        "python3 -c \"
import re,sys
body=open('$f').read()
m=re.search(r'## Sidecar Cross-Check.*?(?=\n## |\nREMEMBER:|\Z)', body, re.DOTALL)
if not m: sys.exit(1)
section=m.group(0)
# 'Write' or 'Edit' as a verb instructing modification (case-insensitive 'write to'
# / 'edit the' / 'modify') would be a violation. Mentions in 'do NOT modify' are fine.
for forbidden in ['write to', 'edit the file', 'edit the sidecar', 'modify the file']:
    if forbidden in section.lower():
        sys.exit(2)
sys.exit(0)
\""
done

# 3. Fixture corpus is well-formed.
assert "synced/source.md exists" "[ -f test/fixtures/sidecar-drift/synced/source.md ]"
assert "synced/source.edikt.yaml exists" "[ -f test/fixtures/sidecar-drift/synced/source.edikt.yaml ]"
assert "drifted/source.md exists" "[ -f test/fixtures/sidecar-drift/drifted/source.md ]"
assert "drifted/source.edikt.yaml exists" "[ -f test/fixtures/sidecar-drift/drifted/source.edikt.yaml ]"

# 4. Reference Python cross-check produces expected verdict per fixture.
echo ""
echo "  Reference cross-check on fixtures:"

run_check() {
    python3 - "$1" "$2" <<'PY'
import sys, re, yaml

source_md, sidecar_yaml = sys.argv[1], sys.argv[2]
body_text = open(source_md, encoding='utf-8').read()
body_lines = body_text.splitlines()
sidecar = yaml.safe_load(open(sidecar_yaml, encoding='utf-8').read())

findings = []

# Step 2: quote drift
for i, d in enumerate(sidecar.get('directives', []), start=1):
    excerpt = d.get('source_excerpt', {})
    quote = excerpt.get('quote', '')
    if quote and quote not in body_text:
        findings.append(f"drift: directive #{i} quote not found in body")

# Step 3: missing directives — sentence-level token-overlap match.
def tokens(s):
    return set(re.findall(r"[a-zA-Z]{3,}", s.lower()))

# Walk the body, find each sentence containing an imperative verb. A "sentence"
# ends at a period, exclamation, question mark, or end-of-line — whichever
# comes first. This matches the .md instruction "imperative sentences" rather
# than treating a multi-directive line as a single unit.
NORMATIVE = re.compile(r'\b(MUST(?: NOT)?|NEVER|ALWAYS|SHOULD(?: NOT)?)\b')
sentence_re = re.compile(r'[^.!?\n]+[.!?]?')
prose_imperatives = []
for ln, line in enumerate(body_lines, start=1):
    for sent in sentence_re.findall(line):
        if NORMATIVE.search(sent):
            prose_imperatives.append((ln, sent.strip()))

sidecar_directive_token_sets = [
    tokens(d.get('text', '')) for d in sidecar.get('directives', [])
]

for ln, sentence in prose_imperatives:
    sentence_tokens = tokens(sentence)
    if not sentence_tokens:
        continue
    represented = False
    for d_tokens in sidecar_directive_token_sets:
        overlap = len(sentence_tokens & d_tokens) / max(len(sentence_tokens), 1)
        if overlap >= 0.6:
            represented = True
            break
    if not represented:
        findings.append(f"missing: line {ln} '{sentence[:60]}…' not in sidecar")

if findings:
    for f in findings:
        print(f)
    sys.exit(1)

print("in sync")
sys.exit(0)
PY
}

# Synced fixture must report "in sync".
synced_out=$(run_check test/fixtures/sidecar-drift/synced/source.md \
                       test/fixtures/sidecar-drift/synced/source.edikt.yaml 2>&1) && synced_status=0 || synced_status=$?
if [ "$synced_status" -eq 0 ] && [ "$synced_out" = "in sync" ]; then
    echo -e "  ${GREEN}✓${RESET} synced fixture reports 'in sync'"
    pass_count=$((pass_count + 1))
else
    echo -e "  ${RED}✗${RESET} synced fixture: expected 'in sync', got status=$synced_status output=$synced_out"
    fail_count=$((fail_count + 1))
fi

# Drifted fixture must report drift.
drifted_out=$(run_check test/fixtures/sidecar-drift/drifted/source.md \
                        test/fixtures/sidecar-drift/drifted/source.edikt.yaml 2>&1) && drifted_status=0 || drifted_status=$?
if [ "$drifted_status" -ne 0 ]; then
    echo -e "  ${GREEN}✓${RESET} drifted fixture reports drift findings"
    pass_count=$((pass_count + 1))
    # Verify both kinds of drift surface (extra-in-sidecar + missing-in-sidecar).
    if echo "$drifted_out" | grep -q '^drift:'; then
        echo -e "  ${GREEN}✓${RESET} drifted fixture surfaces quote-drift findings"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} drifted fixture missing 'drift:' findings"
        echo "$drifted_out" | sed 's/^/      /'
        fail_count=$((fail_count + 1))
    fi
    if echo "$drifted_out" | grep -q '^missing:'; then
        echo -e "  ${GREEN}✓${RESET} drifted fixture surfaces missing-directive findings"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}✗${RESET} drifted fixture missing 'missing:' findings"
        echo "$drifted_out" | sed 's/^/      /'
        fail_count=$((fail_count + 1))
    fi
else
    echo -e "  ${RED}✗${RESET} drifted fixture: expected drift, got 'in sync'"
    fail_count=$((fail_count + 1))
fi

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
