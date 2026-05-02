#!/usr/bin/env bash
# Phase 6 integration: migrate sidecars on a v0.5.x in-body fixture.
set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t migrate-v05x-XXXXXX)"
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

# Build the fixture sentinel block via printf so this test file does not
# itself contain a literal in-body managed region.
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"
DIRHASHKEY="dir""ectives_hash"

cat > "$WORK/docs/architecture/decisions/ADR-100-foo.md" <<EOF
# ADR-100 — fixture

## Decision

Hooks must emit JSON. (ref: INV-003)

## Sentinel

$OPEN
source_hash: aaa
$DIRHASHKEY: bbb
topic: hooks
signals:
  - hook
  - posttooluse
$DIRKEY:
  - "Hooks must emit JSON. (ref: INV-003)"
$CLOSE
EOF

cd "$WORK"
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1 || {
    cat "$WORK/dry.out"; exit 1;
}
grep -q "ADR-100-foo.md" "$WORK/dry.out" || { echo "dry-run output missing ADR-100"; cat "$WORK/dry.out"; exit 1; }
grep -q "v0.5.x mechanical" "$WORK/dry.out" || { echo "dry-run did not detect v0.5.x"; cat "$WORK/dry.out"; exit 1; }

EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply.out" 2>&1 || {
    cat "$WORK/apply.out"; exit 1;
}
test -f "$WORK/docs/architecture/decisions/ADR-100-foo.edikt.yaml" || {
    echo "sidecar not written"; cat "$WORK/apply.out"; exit 1;
}
grep -q "topic: hooks" "$WORK/docs/architecture/decisions/ADR-100-foo.edikt.yaml"
grep -q "schema_version: 1" "$WORK/docs/architecture/decisions/ADR-100-foo.edikt.yaml"
# path: must be project-relative (the schema's documented shape) so doctor's
# PATH MISMATCH check (Phase 7) and IsStale() resolution work correctly.
grep -q "^path: docs/architecture/decisions/ADR-100-foo.md$" \
    "$WORK/docs/architecture/decisions/ADR-100-foo.edikt.yaml" || {
    echo "path: field is not project-relative"
    cat "$WORK/docs/architecture/decisions/ADR-100-foo.edikt.yaml"
    exit 1
}

# Sentinel removed from md.
if grep -q "^\[edikt:dir""ectives:start\]" "$WORK/docs/architecture/decisions/ADR-100-foo.md"; then
    echo "sentinel still in md"; exit 1
fi

# Idempotency: re-apply.
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply2.out" 2>&1
grep -q "0 sidecars wrote" "$WORK/apply2.out" || { echo "expected idempotent re-apply"; cat "$WORK/apply2.out"; exit 1; }

echo "migrate-v05x: OK"
