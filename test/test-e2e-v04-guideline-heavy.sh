#!/usr/bin/env bash
# E2E regression test for v0.6.0 release.
#
# Single fixture that catches both bug families that shipped in rc≤7:
#
#   Family 1 (silent guideline reminders/verification drop) — migrating a
#   v0.x guideline with populated reminders:/verification: in its legacy
#   sentinel block must preserve those arrays in the new sidecar, AND the
#   subsequent compile must include them in governance output.
#
#   Family 2 (silent gov compile no-op) — a project with custom paths.*
#   in .edikt/config.yaml must NOT silently no-op edikt gov compile. The
#   dispatcher (compile.go) must read configured paths, not hardcoded
#   defaults. --check must emit a verdict. --json must always populate
#   phase_b (never null).
#
# Failure of ANY assertion here means a rc-class regression is shipping.
# This test is the safety net superpowers' verification-before-completion
# rule depends on for the gov compile path.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/.." && pwd)}"
EDIKT_BIN="${EDIKT_BIN:-}"
if [ -z "$EDIKT_BIN" ]; then
    EDIKT_BIN="$(mktemp -t edikt-bin-XXXXXX)"
    if ! (cd "$PROJECT_ROOT/tools/edikt" && go build -o "$EDIKT_BIN" .); then
        echo "FAIL: could not build edikt binary at $PROJECT_ROOT/tools/edikt"
        exit 1
    fi
fi
if [ ! -x "$EDIKT_BIN" ] || [ ! -s "$EDIKT_BIN" ]; then
    echo "FAIL: EDIKT_BIN ($EDIKT_BIN) is missing, not executable, or empty"
    exit 1
fi

WORK="$(mktemp -d -t e2e-guideline-XXXXXX)"
EDIKT_ROOT="$WORK/.edikt"
trap 'rm -rf "$WORK" 2>/dev/null || true; [ -z "${EDIKT_BIN_PRESERVE:-}" ] && rm -f "$EDIKT_BIN" 2>/dev/null || true' EXIT

# CUSTOM paths.guidelines exercises the governanceDirs config fix (#11).
# A project using the hardcoded default (docs/guidelines) would not surface
# the regression; the bug only fires when the user customizes paths.*.
mkdir -p "$WORK/.edikt/state" "$WORK/memory-bank" "$WORK/fakebin"
cat > "$WORK/.edikt/config.yaml" <<'EOF'
edikt_version: "0.6.0"
paths:
  guidelines: memory-bank
EOF

# Fake claude — migration doesn't need a real LLM for v0.5.x-full schema
# (mechanical lift). Some code paths probe for `claude` on PATH; keep it
# present and inert so EDIKT_INSECURE et al don't trip.
cat > "$WORK/fakebin/claude" <<'EOF'
#!/usr/bin/env bash
exit 0
EOF
chmod +x "$WORK/fakebin/claude"

# Sentinel tokens — built up to avoid the file itself being detected as
# carrying a managed region by edikt's own pre-tool-use hook on the
# repo this test lives in.
OPEN="[edikt:dir""ectives:start]: #"
CLOSE="[edikt:dir""ectives:end]: #"

cat > "$WORK/memory-bank/guideline-authorization.md" <<EOF
# Authorization Guideline

You MUST always use OPA for authorization decisions. Handlers MUST NOT
contain if/else authz logic.

$OPEN
schema_version: 2
source_hash: aaaaaaaaaaaa
topic: authorization
signals: ["auth", "opa", "authorization"]
directives:
  - "You MUST always use OPA for authorization decisions."
  - "Handlers MUST NOT contain if/else authz logic."
reminders:
  - "Before writing authz logic → put it in OPA, never in handlers as if/else"
  - "Before importing OPA SDK → only adapters/opa/authorizer.go may import opa/v1/sdk"
verification:
  - "Every authenticated route is mounted inside AuthzMiddleware"
  - "policy.rego declares package keystone, imports rego.v1, starts with default allow := false"
$CLOSE
EOF

cd "$WORK"

# ── Step 1: migrate sidecars --apply preserves reminders/verification ────────
PATH="$WORK/fakebin:$PATH" EDIKT_ROOT="$EDIKT_ROOT" \
  "$EDIKT_BIN" migrate sidecars --apply --force > "$WORK/apply.out" 2>&1 || {
    echo "FAIL: migrate sidecars --apply exited non-zero"
    cat "$WORK/apply.out"
    exit 1
}

SC="$WORK/memory-bank/guideline-authorization.edikt.yaml"
test -f "$SC" || { echo "FAIL: sidecar not written at $SC"; exit 1; }

# Lift preservation — Family 1 (silent drop) guard
grep -q "Before writing authz logic" "$SC" || {
    echo "FAIL: reminder 1 lost in migration lift"
    echo "--- sidecar:"; cat "$SC"; exit 1
}
grep -q "Before importing OPA SDK" "$SC" || {
    echo "FAIL: reminder 2 lost in migration lift"
    echo "--- sidecar:"; cat "$SC"; exit 1
}
grep -q "Every authenticated route" "$SC" || {
    echo "FAIL: verification 1 lost in migration lift"
    echo "--- sidecar:"; cat "$SC"; exit 1
}
grep -q "policy.rego declares package" "$SC" || {
    echo "FAIL: verification 2 lost in migration lift"
    echo "--- sidecar:"; cat "$SC"; exit 1
}

# Migration informational notice (#7)
grep -q "lifting 2 reminder" "$WORK/apply.out" || {
    echo "FAIL: missing migration notice (lifting N reminders + M verification items)"
    cat "$WORK/apply.out"; exit 1
}

# ── Step 2: gov compile honors custom paths.guidelines (#11) ─────────────────
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile > "$WORK/compile.out" 2>&1 || {
    echo "FAIL: gov compile errored"
    cat "$WORK/compile.out"; exit 1
}

# Family 2 (silent no-op) guard — output must be non-empty when sidecars exist
[ -s "$WORK/compile.out" ] || {
    echo "FAIL: gov compile produced empty output (governanceDirs config-respect regression)"
    exit 1
}
grep -q "Phase B" "$WORK/compile.out" || {
    echo "FAIL: gov compile didn't reach Phase B (early-return regression)"
    cat "$WORK/compile.out"; exit 1
}

# governance.md must exist and be non-trivial
test -f "$WORK/.claude/rules/governance.md" || {
    echo "FAIL: governance.md not written"; exit 1
}
gov_size=$(wc -c < "$WORK/.claude/rules/governance.md")
[ "$gov_size" -gt 100 ] || {
    echo "FAIL: governance.md is suspiciously small ($gov_size bytes)"
    cat "$WORK/.claude/rules/governance.md"; exit 1
}

# ── Step 3: --check emits a verdict, never silent (#3) ───────────────────────
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile --check 2> "$WORK/check.err" 1> "$WORK/check.out" || {
    echo "FAIL: gov compile --check errored"
    cat "$WORK/check.err"; exit 1
}
grep -qE "(up-to-date|stale)" "$WORK/check.err" || {
    echo "FAIL: --check produced no verdict (silent regression)"
    echo "--- check.err:"; cat "$WORK/check.err"
    echo "--- check.out:"; cat "$WORK/check.out"
    exit 1
}

# ── Step 4: --json contract — phase_b always populated, never null (#3) ──────
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile --json > "$WORK/json.out" 2>/dev/null || true
grep -qE '"phase_b":\s*\{' "$WORK/json.out" || {
    echo "FAIL: --json phase_b not a populated object (null-contract regression)"
    cat "$WORK/json.out"; exit 1
}
grep -q '"ran":' "$WORK/json.out" || {
    echo "FAIL: --json phase_b missing 'ran' field"
    cat "$WORK/json.out"; exit 1
}

# --check --json also populates phase_b (with ran: false)
EDIKT_ROOT="$EDIKT_ROOT" "$EDIKT_BIN" gov compile --check --json > "$WORK/check.json" 2>/dev/null || true
grep -qE '"phase_b":\s*\{' "$WORK/check.json" || {
    echo "FAIL: --check --json phase_b not populated"
    cat "$WORK/check.json"; exit 1
}
grep -q '"ran":\s*false' "$WORK/check.json" || {
    echo "FAIL: --check --json phase_b 'ran' should be false"
    cat "$WORK/check.json"; exit 1
}

# ── Step 5: version stamp invariant — binary --version never disagrees ───────
# Catches the rc4-stamped-as-rc7 drift class. ldflag injection at build
# time means binary version must equal whatever we passed.
binary_ver=$("$EDIKT_BIN" version --binary 2>&1 | head -1)
if [ "$binary_ver" = "dev" ]; then
    : # locally-built binary without ldflags — acceptable for the test runner
elif ! echo "$binary_ver" | grep -qE '^v?[0-9]+\.[0-9]+\.[0-9]'; then
    echo "FAIL: binary --version returned malformed string: '$binary_ver'"
    exit 1
fi

echo "e2e-v04-guideline-heavy: OK"
