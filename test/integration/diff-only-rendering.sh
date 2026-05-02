#!/usr/bin/env bash
# Phase 8 integration: diff-only topic rendering via fingerprint.
#
# Asserts that `edikt gov compile` over a sidecar corpus rerenders only the
# topic file(s) whose contributing sidecars changed. Untouched topics keep
# their on-disk mtime AND their committed `_fingerprint:` frontmatter line.
#
# The fixture lays down two ADRs in two different topics:
#   ADR-001 → topic alpha
#   ADR-002 → topic beta
# After a baseline compile we mutate ADR-001's sidecar (regenerating its
# directives) and re-run compile. Only `alpha.md` is expected to be
# rewritten; `beta.md` must be byte-equal to its baseline.

set -uo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"

EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .) || {
        echo "build failed"; exit 1
    }
    trap 'rm -f "$EDIKT_BIN"' EXIT
fi

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

# ─── Fixture ────────────────────────────────────────────────────────────────

WORK="$(mktemp -d -t diff-only-XXXXXX)"
trap 'rm -rf "$WORK"; rm -f "${EDIKT_BIN}"' EXIT

mkdir -p "$WORK/.edikt" \
         "$WORK/docs/architecture/decisions" \
         "$WORK/docs/architecture/invariants" \
         "$WORK/docs/guidelines"

cat > "$WORK/.edikt/config.yaml" <<'YAML'
edikt_version: "0.6.0-dev"
base: docs
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
YAML

# ADR-001 → topic alpha
cat > "$WORK/docs/architecture/decisions/ADR-001-test.md" <<'MD'
---
type: adr
id: ADR-001
status: accepted
---
# ADR-001 — fixture

## Decision

Alpha rule must hold.
MD
cat > "$WORK/docs/architecture/decisions/ADR-001-test.edikt.yaml" <<'YAML'
schema_version: 1
topic: alpha
path: docs/architecture/decisions/ADR-001-test.md
signals:
  - alpha
directives:
  - text: "Alpha rule must hold. (ref: ADR-001)"
    source_excerpt:
      line_start: 8
      line_end: 8
      quote: "Alpha rule must hold."
YAML

# ADR-002 → topic beta
cat > "$WORK/docs/architecture/decisions/ADR-002-test.md" <<'MD'
---
type: adr
id: ADR-002
status: accepted
---
# ADR-002 — fixture

## Decision

Beta rule must hold.
MD
cat > "$WORK/docs/architecture/decisions/ADR-002-test.edikt.yaml" <<'YAML'
schema_version: 1
topic: beta
path: docs/architecture/decisions/ADR-002-test.md
signals:
  - beta
directives:
  - text: "Beta rule must hold. (ref: ADR-002)"
    source_excerpt:
      line_start: 8
      line_end: 8
      quote: "Beta rule must hold."
YAML

# ─── Run 1: baseline compile ────────────────────────────────────────────────

cd "$WORK"
"$EDIKT_BIN" gov compile "$WORK" >/dev/null 2>&1 || {
    echo -e "${RED}baseline compile failed${RESET}"
    "$EDIKT_BIN" gov compile "$WORK"
    exit 1
}

ALPHA="$WORK/.claude/rules/governance/alpha.md"
BETA="$WORK/.claude/rules/governance/beta.md"

assert "alpha topic file exists after baseline" "[ -f '$ALPHA' ]"
assert "beta topic file exists after baseline"  "[ -f '$BETA'  ]"
assert "alpha frontmatter carries _fingerprint" "grep -q '^_fingerprint:' '$ALPHA'"
assert "beta  frontmatter carries _fingerprint" "grep -q '^_fingerprint:' '$BETA'"

ALPHA_FP_BASE=$(grep '^_fingerprint:' "$ALPHA" | head -1)
BETA_FP_BASE=$(grep  '^_fingerprint:' "$BETA"  | head -1)
ALPHA_HASH_BASE=$(shasum -a 256 "$ALPHA" | awk '{print $1}')
BETA_HASH_BASE=$(shasum  -a 256 "$BETA"  | awk '{print $1}')

# ─── Run 2: no-op re-compile — both topics must stay byte-equal ─────────────

# Sleep 1s so a missed cache hit would yield a different mtime / timestamp
# in the file body (compiled_at is stamped per run).
sleep 1
"$EDIKT_BIN" gov compile "$WORK" >/dev/null 2>&1 || {
    echo -e "${RED}no-op compile failed${RESET}"
    exit 1
}

ALPHA_HASH_NOOP=$(shasum -a 256 "$ALPHA" | awk '{print $1}')
BETA_HASH_NOOP=$(shasum  -a 256 "$BETA"  | awk '{print $1}')
assert "no-op compile leaves alpha byte-equal" "[ '$ALPHA_HASH_BASE' = '$ALPHA_HASH_NOOP' ]"
assert "no-op compile leaves beta  byte-equal" "[ '$BETA_HASH_BASE'  = '$BETA_HASH_NOOP'  ]"

# ─── Run 3: mutate ADR-001 sidecar; only alpha must rerender ────────────────

cat > "$WORK/docs/architecture/decisions/ADR-001-test.edikt.yaml" <<'YAML'
schema_version: 1
topic: alpha
path: docs/architecture/decisions/ADR-001-test.md
signals:
  - alpha
directives:
  - text: "Alpha rule must hold AND be enforced. (ref: ADR-001)"
    source_excerpt:
      line_start: 8
      line_end: 8
      quote: "Alpha rule must hold."
YAML

# Recompute the parent .md hash so the sidecar's source-hash doesn't drift —
# Phase A would otherwise dispatch a (non-existent) Claude session and fail.
# Easiest: keep parent .md unchanged. The sidecar drift detector compares the
# parent .md hash to the value implied by the sidecar; since sidecars don't
# carry a committed source_hash field, this stays valid.

sleep 1
"$EDIKT_BIN" gov compile "$WORK" >/dev/null 2>&1 || {
    echo -e "${RED}post-mutation compile failed${RESET}"
    "$EDIKT_BIN" gov compile "$WORK"
    exit 1
}

ALPHA_HASH_AFTER=$(shasum -a 256 "$ALPHA" | awk '{print $1}')
BETA_HASH_AFTER=$(shasum  -a 256 "$BETA"  | awk '{print $1}')
ALPHA_FP_AFTER=$(grep '^_fingerprint:' "$ALPHA" | head -1)
BETA_FP_AFTER=$(grep  '^_fingerprint:' "$BETA"  | head -1)

assert "alpha rerendered after sidecar mutation" "[ '$ALPHA_HASH_BASE' != '$ALPHA_HASH_AFTER' ]"
assert "alpha fingerprint changed after mutation" "[ '$ALPHA_FP_BASE' != '$ALPHA_FP_AFTER' ]"
assert "beta untouched after alpha-only mutation" "[ '$BETA_HASH_BASE' = '$BETA_HASH_AFTER' ]"
assert "beta fingerprint unchanged" "[ '$BETA_FP_BASE' = '$BETA_FP_AFTER' ]"
assert "alpha body reflects new directive text" \
    "grep -q 'Alpha rule must hold AND be enforced' '$ALPHA'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
