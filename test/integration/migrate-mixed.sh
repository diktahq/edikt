#!/usr/bin/env bash
# Phase 6 integration: mixed v0.4.3 + v0.5.x fixtures in one project.
set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t migrate-mixed-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

mkdir -p "$WORK/.edikt/state" "$WORK/docs/architecture/decisions" "$WORK/docs/architecture/invariants" "$WORK/fakebin"
echo 'edikt_version: "0.6.0"' > "$WORK/.edikt/config.yaml"

cat > "$WORK/fakebin/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$WORK/fakebin/claude"

OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"
DIRHASHKEY="dir""ectives_hash"

# v0.5.x ADR
cat > "$WORK/docs/architecture/decisions/ADR-301-new.md" <<EOF
# ADR-301

A modern directive sentence.

$OPEN
source_hash: x
$DIRHASHKEY: y
topic: misc
signals:
  - alpha
$DIRKEY:
  - "A modern directive sentence."
$CLOSE
EOF

# v0.4.3 INV
cat > "$WORK/docs/architecture/invariants/INV-301-old.md" <<EOF
# INV-301

A legacy invariant directive.

$OPEN
content_hash: cafebabe
$DIRKEY:
  - "A legacy invariant directive."
$CLOSE
EOF

cd "$WORK"
PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1
grep -q "v0.5.x mechanical" "$WORK/dry.out"
grep -q "v0.4.3 legacy" "$WORK/dry.out"

PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply.out" 2>&1 || {
    cat "$WORK/apply.out"; exit 1;
}

test -f "$WORK/docs/architecture/decisions/ADR-301-new.edikt.yaml"
test -f "$WORK/docs/architecture/invariants/INV-301-old.edikt.yaml"
grep -q "topic: misc" "$WORK/docs/architecture/decisions/ADR-301-new.edikt.yaml"
grep -q "topic: needs-review" "$WORK/docs/architecture/invariants/INV-301-old.edikt.yaml"

echo "migrate-mixed: OK"
