#!/usr/bin/env bash
# Phase 8 of PLAN-sidecar-review-fixes: migrate sidecars on a corpus of
# partial-v0.5.x sentinel blocks (source_hash present; topic/signals
# absent). Asserts dry-run plans every artifact as `dry-llm-resync` —
# none should appear as SKIPPED unrecognized-schema rows.
set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t migrate-partial-v05x-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

mkdir -p "$WORK/.edikt/state" "$WORK/docs/architecture/decisions"
cat > "$WORK/.edikt/config.yaml" <<EOF
edikt_version: "0.6.0"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
EOF

# Build sentinel markers via printf so this test file does not itself
# contain a literal in-body managed region (managed-region guard).
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"
DIRHASHKEY="dir""ectives_hash"

# Stage 5 partial-v0.5.x ADRs: each has source_hash + directives_hash but
# NO topic, NO signals. This is the dogfood-project shape and the most
# common v0.5.x dev-branch state per governance/tooling.md line 6.
for i in 1 2 3 4 5; do
    id=$(printf "ADR-10%d" "$i")
    cat > "$WORK/docs/architecture/decisions/${id}-bench.md" <<EOF
# ${id} — partial fixture

## Decision

Hooks must emit JSON for ${id}. (ref: INV-003)

## Sentinel

${OPEN}
source_hash: hash-${i}
${DIRHASHKEY}: dh-${i}
compiler_version: "0.5.0"
${DIRKEY}:
  - "Hooks must emit JSON for ${id}. (ref: INV-003)"
${CLOSE}
EOF
done

cd "$WORK"
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1 || {
    cat "$WORK/dry.out"; exit 1;
}

# All 5 must appear as dry-llm-resync rows.
resync_count=$(grep -c "v0.5.x partial, needs LLM topic-resync" "$WORK/dry.out" || true)
if [ "$resync_count" -ne 5 ]; then
    echo "expected 5 dry-llm-resync rows; got $resync_count"
    cat "$WORK/dry.out"
    exit 1
fi

# No artifact in the corpus should be skipped as unrecognized-schema.
if grep -q "unrecognized schema" "$WORK/dry.out"; then
    echo "found unrecognized-schema rows; partial detection regressed"
    cat "$WORK/dry.out"
    exit 1
fi

# Plan summary line MUST report 5 to-create.
if ! grep -qE "^5 sidecars to create" "$WORK/dry.out"; then
    echo "expected '5 sidecars to create' summary"
    cat "$WORK/dry.out"
    exit 1
fi

# Dry-run state file MUST exist (writeDryRunState side effect).
test -f "$WORK/.edikt/state/migration-dry-run.json" || {
    echo "missing migration-dry-run.json"
    ls "$WORK/.edikt/state/"
    exit 1
}

echo "migrate-partial-v05x: OK"
