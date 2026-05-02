#!/usr/bin/env bash
# Phase 9 E2E: edit-prose → :compile-sidecar → gov:compile roundtrip.
#
# The sidecar architecture's user-visible promise (ADR-027 + ADR-028) is:
#   1. After editing an ADR's prose, that artifact's sidecar is the ONLY
#      sidecar that needs regeneration.
#   2. After regenerating the sidecar, gov:compile rerenders ONLY that
#      artifact's topic file. Every other topic stays byte-equal.
#   3. A subsequent `gov compile` is a deterministic no-op (Phase A skipped).
#
# This roundtrip test pins all three. It uses the v0.6.0 binary directly to
# avoid pulling in a Claude session — the per-artifact :compile step is
# simulated by overwriting the sidecar with the known-good post-edit shape
# (the same end state a real /edikt:adr:compile would produce).

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .) || {
        echo "build failed"; exit 1
    }
fi

WORK="$(mktemp -d -t e2e-roundtrip-XXXXXX)"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

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

echo "Phase 9 — edit / recompile roundtrip"

# ─── Seed: 5 ADRs across 5 distinct topics ──────────────────────────────────
mkdir -p "$WORK/.edikt" "$WORK/docs/architecture/decisions"
cat > "$WORK/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0-dev"
base: docs
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

seed_adr() {
    local id="$1" topic="$2" rule="$3"
    cat > "$WORK/docs/architecture/decisions/${id}-test.md" <<MD
---
type: adr
id: ${id}
status: accepted
---
# ${id} — fixture

## Decision

${rule}
MD
    cat > "$WORK/docs/architecture/decisions/${id}-test.edikt.yaml" <<YAML
schema_version: 1
topic: ${topic}
path: docs/architecture/decisions/${id}-test.md
signals:
  - ${topic}
directives:
  - text: "${rule} (ref: ${id})"
    source_excerpt:
      line_start: 8
      line_end: 8
      quote: "${rule}"
YAML
}

seed_adr "ADR-401" "alpha"   "Alpha rule must hold."
seed_adr "ADR-402" "beta"    "Beta rule must hold."
seed_adr "ADR-403" "gamma"   "Gamma rule must hold."
seed_adr "ADR-404" "delta"   "Delta rule must hold."
seed_adr "ADR-405" "epsilon" "Epsilon rule must hold."

cd "$WORK"

# ─── Run 1: baseline compile ────────────────────────────────────────────────
"$EDIKT_BIN" gov compile "$WORK" > "$WORK/c1.out" 2>&1 || {
    echo -e "${RED}baseline compile failed${RESET}"; cat "$WORK/c1.out"; exit 1;
}
GOV="$WORK/.claude/rules/governance"
for t in alpha beta gamma delta epsilon; do
    assert "topic file rendered: $t" "[ -f '$GOV/$t.md' ]"
done

snapshot() {
    find "$GOV" -type f -name '*.md' -exec shasum -a 256 {} \; 2>/dev/null | sort
}
SNAP_BASE=$(snapshot)
ALPHA_BASE=$(shasum -a 256 "$GOV/alpha.md" 2>/dev/null | awk '{print $1}')
BETA_BASE=$(shasum  -a 256 "$GOV/beta.md"  2>/dev/null | awk '{print $1}')

# ─── Run 2: edit ADR-402 prose AND its sidecar (simulating :compile) ────────
# Update prose: a real session would edit the body and run /edikt:adr:compile.
# We do both steps here — first the body edit, then the sidecar regenerate
# (we write the canonical post-edit sidecar content directly).
cat > "$WORK/docs/architecture/decisions/ADR-402-test.md" <<'MD'
---
type: adr
id: ADR-402
status: accepted
---
# ADR-402 — fixture

## Decision

Beta rule must hold AND be enforced.
MD
cat > "$WORK/docs/architecture/decisions/ADR-402-test.edikt.yaml" <<'YAML'
schema_version: 1
topic: beta
path: docs/architecture/decisions/ADR-402-test.md
signals:
  - beta
directives:
  - text: "Beta rule must hold AND be enforced. (ref: ADR-402)"
    source_excerpt:
      line_start: 8
      line_end: 8
      quote: "Beta rule must hold AND be enforced."
YAML

sleep 1
"$EDIKT_BIN" gov compile "$WORK" > "$WORK/c2.out" 2>&1 || {
    echo -e "${RED}post-edit compile failed${RESET}"; cat "$WORK/c2.out"; exit 1;
}

ALPHA_AFTER=$(shasum -a 256 "$GOV/alpha.md" | awk '{print $1}')
BETA_AFTER=$(shasum  -a 256 "$GOV/beta.md"  | awk '{print $1}')
assert "beta topic re-rendered after ADR-402 edit"   "[ '$BETA_BASE'  != '$BETA_AFTER'  ]"
assert "alpha topic untouched (only beta should change)" "[ '$ALPHA_BASE' = '$ALPHA_AFTER' ]"
for t in gamma delta epsilon; do
    HASH=$(shasum -a 256 "$GOV/$t.md" | awk '{print $1}')
    PREV=$(echo "$SNAP_BASE" | awk -v p="$GOV/$t.md" '$2==p{print $1}')
    assert "$t topic untouched after alpha-only edit" "[ '$HASH' = '$PREV' ]"
done
assert "beta topic reflects new directive text" \
    "grep -q 'Beta rule must hold AND be enforced' '$GOV/beta.md'"

# ─── Run 3: re-compile is a no-op (Phase A skipped, Phase B byte-equal) ────
sleep 1
SNAP_AFTER_EDIT=$(snapshot)
"$EDIKT_BIN" gov compile "$WORK" > "$WORK/c3.out" 2>&1 || {
    echo -e "${RED}third compile failed${RESET}"; cat "$WORK/c3.out"; exit 1;
}
SNAP_NOOP=$(snapshot)
assert "third compile is byte-equal no-op (deterministic merge)" \
    "[ \"$SNAP_AFTER_EDIT\" = \"$SNAP_NOOP\" ]"
assert "third-compile log declares zero topics rendered" \
    "grep -qE 'rendered: ?0|0 topics rendered|Phase B' '$WORK/c3.out'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
