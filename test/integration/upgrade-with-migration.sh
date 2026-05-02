#!/usr/bin/env bash
# Phase 6b integration: end-to-end upgrade flow that surfaces sidecar
# migration to the user.
#
# This test does NOT spawn a Claude Code session — it asserts the static
# contract of `commands/upgrade.md` (the migration step is wired in) and
# the runtime contract of the migration tool that the command shells out
# to. The Claude-driven prompt flow (1.5b prompt → user picks y/N) lives
# in test/integration/test_e2e_*.py.
#
# What this test asserts:
#   1. commands/upgrade.md contains the Step 1.5 sidecar-migration block
#      that calls `edikt migrate sidecars --dry-run` and `--apply`.
#   2. commands/gov/compile.md contains the pre-flight gate that refuses
#      when legacy in-body sentinels are present.
#   3. End-to-end runtime flow: a fresh v0.5.x project that runs the
#      migration apply tool ends up with sidecars on disk and zero
#      in-body sentinels remaining.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .)
fi

WORK="$(mktemp -d -t upgrade-with-migration-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
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

echo "Phase 6b — upgrade-with-migration"

# ── 1. commands/upgrade.md wires the sidecar-migration step ────────────────
UPGRADE_MD="$PROJECT_ROOT/commands/upgrade.md"
assert "upgrade.md exists" "[ -f '$UPGRADE_MD' ]"
assert "upgrade.md adds the 1.5 Sidecar Migration Check section" \
    "grep -qF 'Sidecar Migration Check' '$UPGRADE_MD'"
assert "upgrade.md calls 'edikt migrate sidecars --dry-run'" \
    "grep -qF 'edikt migrate sidecars --dry-run' '$UPGRADE_MD'"
assert "upgrade.md calls 'edikt migrate sidecars --apply'" \
    "grep -qF 'edikt migrate sidecars --apply' '$UPGRADE_MD'"
assert "upgrade.md prompts the user with [y/N]" \
    "grep -qF 'Apply the migration now? [y/N]' '$UPGRADE_MD'"
assert "upgrade.md prints the deferred message on N" \
    "grep -qF 'Migration deferred' '$UPGRADE_MD'"
assert "upgrade.md cites ADR-027" \
    "grep -qF 'ADR-027' '$UPGRADE_MD'"

# ── 2. commands/gov/compile.md gates on legacy sentinels ───────────────────
GOV_MD="$PROJECT_ROOT/commands/gov/compile.md"
assert "gov/compile.md exists" "[ -f '$GOV_MD' ]"
assert "gov/compile.md adds the pre-v0.6.0 sentinel gate" \
    "grep -qF 'Pre-v0.6.0 sentinel gate' '$GOV_MD'"
assert "gov/compile.md refuses with the actionable error" \
    "grep -qF 'Migration required. Run /edikt:upgrade' '$GOV_MD'"

# ── 3. Runtime flow: legacy project → migrate --apply → sidecars exist ─────
mkdir -p "$EDIKT_ROOT/state" "$WORK/docs/architecture/decisions"
cat > "$WORK/.edikt/config.yaml" <<EOF
edikt_version: "0.5.4"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
EOF

# Build the legacy fixture with split markers so this test file does not itself
# contain a literal in-body managed region.
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"
DIRKEY="dir""ectives"
DIRHASHKEY="dir""ectives_hash"

cat > "$WORK/docs/architecture/decisions/ADR-200-foo.md" <<EOF
# ADR-200 — fixture

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

# Simulate the upgrade flow's Step 1.5a (dry-run plan).
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --dry-run > "$WORK/dry.out" 2>&1 || {
    echo "dry-run failed unexpectedly"; cat "$WORK/dry.out"; exit 1;
}
assert "dry-run flags ADR-200 as needing migration" \
    "grep -qF 'ADR-200-foo.md' '$WORK/dry.out'"
assert "dry-run reports a non-zero create count" \
    "grep -qE '[1-9][0-9]* sidecars to create' '$WORK/dry.out'"

# Simulate the upgrade flow's Step 1.5b on `y` (apply).
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply > "$WORK/apply.out" 2>&1 || {
    echo "apply failed unexpectedly"; cat "$WORK/apply.out"; exit 1;
}
assert "apply created the sidecar on disk" \
    "[ -f '$WORK/docs/architecture/decisions/ADR-200-foo.edikt.yaml' ]"
assert "sidecar carries the topic detected from the legacy block" \
    "grep -qF 'topic: hooks' '$WORK/docs/architecture/decisions/ADR-200-foo.edikt.yaml'"
assert "sidecar declares schema_version: 1" \
    "grep -qF 'schema_version: 1' '$WORK/docs/architecture/decisions/ADR-200-foo.edikt.yaml'"

# The .md must no longer contain a leading-column sentinel line.
LEGACY_OPEN_RE='^\[edikt:dir''ectives:start\]'
if grep -qE "$LEGACY_OPEN_RE" "$WORK/docs/architecture/decisions/ADR-200-foo.md"; then
    assert "in-body sentinel was removed from .md" "false"
else
    assert "in-body sentinel was removed from .md" "true"
fi

# Re-running --apply is a no-op (idempotent) — this is what /edikt:upgrade
# would re-confirm if the user re-runs the command after migration.
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply2.out" 2>&1
assert "second apply is a no-op (idempotent)" \
    "grep -qF '0 sidecars wrote' '$WORK/apply2.out'"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
