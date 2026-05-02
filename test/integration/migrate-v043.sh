#!/usr/bin/env bash
# Phase 6 integration: migrate sidecars on a v0.4.3 legacy fixture.
# Stubs `claude` so the LLM dispatch path is exercised but doesn't need claude.
set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t migrate-v043-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

mkdir -p "$WORK/.edikt/state" "$WORK/docs/architecture/decisions" "$WORK/fakebin"
cat > "$WORK/.edikt/config.yaml" <<EOF
edikt_version: "0.6.0"
EOF

# Fake claude that exits 0 but writes nothing.
cat > "$WORK/fakebin/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$WORK/fakebin/claude"

OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"

cat > "$WORK/docs/architecture/decisions/ADR-200-bar.md" <<EOF
# ADR-200 — legacy fixture

## Decision

Some legacy directive sentence here.

## Sentinel

$OPEN
content_hash: deadbeefcafe
$DIRKEY:
  - "Some legacy directive sentence here."
$CLOSE
EOF

cd "$WORK"
PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1
grep -q "v0.4.3 legacy" "$WORK/dry.out" || { echo "expected v0.4.3 legacy in dry-run"; cat "$WORK/dry.out"; exit 1; }

PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply.out" 2>&1 || {
    cat "$WORK/apply.out"; exit 1;
}

SC="$WORK/docs/architecture/decisions/ADR-200-bar.edikt.yaml"
test -f "$SC" || { echo "sidecar not written"; cat "$WORK/apply.out"; exit 1; }
grep -q "topic: needs-review" "$SC" || { echo "expected topic: needs-review (fake claude wrote nothing)"; cat "$SC"; exit 1; }
grep -q "schema_version: 1" "$SC"

if grep -q "^\[edikt:dir""ectives:start\]" "$WORK/docs/architecture/decisions/ADR-200-bar.md"; then
    echo "sentinel still in md"; exit 1
fi

echo "migrate-v043: OK"
