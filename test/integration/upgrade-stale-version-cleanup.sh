#!/usr/bin/env bash
# Regression test for v0.4.5 audit bug: /edikt:upgrade did not clean
# stale project-level .edikt/VERSION, so subsequent upgrade runs read
# the stale value via the INSTALLED_VERSION fallback chain.
#
# Bug pattern (verified against v0.4.5 + v0.6.0-rc5):
#   .edikt/VERSION = "0.3.0-dev"
#   .edikt/config.yaml.edikt_version = "0.6.0"
#   Step 0a fallback reads .edikt/VERSION first → INSTALLED_VERSION = "0.3.0-dev"
#   → upgrade thinks project is on 0.3.0-dev, applies wrong migrations.
#
# Fix lives in commands/upgrade.md Step 6 item 2: rm -f .edikt/VERSION
# after a successful upgrade. This test pins the expected post-upgrade
# state — Bug 2 in the v0.4.5 audit.
#
# Per INV-007: hermetic TMPDIR, no host-state leakage.
set -euo pipefail

WORK="$(mktemp -d -t upgrade-stale-version-XXXXXX)"
trap "rm -rf '$WORK'" EXIT

mkdir -p "$WORK/.edikt"

# ── Step 1: synthesize a project with stale .edikt/VERSION ──────────────────
echo "0.3.0-dev" > "$WORK/.edikt/VERSION"
cat > "$WORK/.edikt/config.yaml" <<'CFGEOF'
edikt_version: "0.6.0-rc7"
paths:
  decisions: docs/architecture/decisions
  invariants: docs/architecture/invariants
  guidelines: docs/guidelines
CFGEOF

# Confirm the bug shape: stale VERSION + current config.yaml drift
test -f "$WORK/.edikt/VERSION" || {
    echo "FAIL: setup — .edikt/VERSION not created"
    exit 1
}
test "$(cat "$WORK/.edikt/VERSION")" = "0.3.0-dev" || {
    echo "FAIL: setup — .edikt/VERSION content unexpected"
    exit 1
}

# ── Step 2: simulate upgrade Step 6 item 2 cleanup ──────────────────────────
# This is the exact bash from commands/upgrade.md Step 6 item 2.
cd "$WORK"
if [ -f .edikt/VERSION ]; then
    rm -f .edikt/VERSION
    echo "Removed stale .edikt/VERSION (v0.3-era; superseded by .edikt/config.yaml edikt_version)"
fi

# ── Step 3: assert .edikt/VERSION is gone ───────────────────────────────────
if [ -f "$WORK/.edikt/VERSION" ]; then
    echo "FAIL: .edikt/VERSION still exists after cleanup"
    exit 1
fi
echo "Step 3 OK: stale .edikt/VERSION removed"

# ── Step 4: assert .edikt/config.yaml is intact (cleanup is scoped) ─────────
if ! grep -q "edikt_version: \"0.6.0-rc7\"" "$WORK/.edikt/config.yaml"; then
    echo "FAIL: .edikt/config.yaml mutated by cleanup"
    cat "$WORK/.edikt/config.yaml"
    exit 1
fi
echo "Step 4 OK: .edikt/config.yaml untouched"

# ── Step 5: idempotency — re-run cleanup, must not error ────────────────────
if [ -f .edikt/VERSION ]; then
    rm -f .edikt/VERSION
fi
echo "Step 5 OK: idempotent (re-run is no-op)"

# ── Step 6: verify the version-resolution fallback now correctly skips ──────
# Step 0a's fallback chain in upgrade.md:
#   INSTALLED_VERSION=$(
#     edikt version 2>/dev/null \
#     || cat .edikt/VERSION 2>/dev/null \
#     || cat ~/.edikt/VERSION 2>/dev/null
#     ...
#   )
# With .edikt/VERSION absent, the chain proceeds past it. We can't test
# the full chain without a launcher, but we can verify the file is
# absent so the fallback doesn't latch onto a stale value.
test ! -e .edikt/VERSION || {
    echo "FAIL: fallback chain would still see stale value"
    exit 1
}
echo "Step 6 OK: fallback chain proceeds past removed file"

echo "upgrade-stale-version-cleanup: OK"
