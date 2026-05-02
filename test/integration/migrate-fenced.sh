#!/usr/bin/env bash
# Phase 6 integration: doc-mention sentinel inside ``` fence is preserved.
set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t migrate-fenced-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" "$EDIKT_BIN" 2>/dev/null || true' EXIT

mkdir -p "$WORK/.edikt/state" "$WORK/docs/architecture/decisions"
echo 'edikt_version: "0.6.0"' > "$WORK/.edikt/config.yaml"

OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"

cat > "$WORK/docs/architecture/decisions/ADR-400-fenced.md" <<EOF
# ADR-400 — fenced doc-mention only

The sentinel below is documentation, not a real block:

\`\`\`
$OPEN
$DIRKEY:
  - "Example only — must NOT be lifted."
$CLOSE
\`\`\`

End of file.
EOF

cd "$WORK"
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply.out" 2>&1

# Sidecar must NOT be created.
if [ -f "$WORK/docs/architecture/decisions/ADR-400-fenced.edikt.yaml" ]; then
    echo "fenced sentinel was incorrectly lifted"; exit 1
fi

# Original md unchanged: sentinel still present (inside the fence).
grep -q "Example only" "$WORK/docs/architecture/decisions/ADR-400-fenced.md"

echo "migrate-fenced: OK"
