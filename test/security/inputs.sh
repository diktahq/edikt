#!/usr/bin/env bash
# Pins INV-006 — externally-controlled input validators reject forbidden shapes.

set -eu
cd "$(dirname "$0")/../.."

fail=0

# ── --ref validator in install.sh rejects path-traversal and whitespace.
run_install() {
    bash -c "$(sed -n '1,100p' install.sh); exit 0" 2>&1 || true
}

# Extract just the argv-parsing section and run the validator by invoking install.sh
# with --ref <bad>. --dry-run exits before any network fetch.
for bad_ref in '../../../etc/passwd' 'main' 'v0.5.0; rm -rf /' 'v0.5.0'$'\n''evil' 'v0.5.0/extra'; do
    out=$(bash install.sh --ref "$bad_ref" --dry-run 2>&1 || true)
    if ! echo "$out" | grep -qE 'invalid|forbidden|error:'; then
        echo "[INV-006] install.sh --ref=$bad_ref should have been rejected" >&2
        echo "  output: $out" >&2
        fail=1
    fi
done

# Good refs should NOT trigger the shape-validator abort. (They may fail later
# on network, but they should pass the regex.)
for good_ref in 'v0.5.0' '0.5.0' 'v0.5.0-rc1' 'v1.2.3-alpha.1'; do
    out=$(bash install.sh --ref "$good_ref" --dry-run 2>&1 || true)
    if echo "$out" | grep -qE 'must match|contains forbidden'; then
        echo "[INV-006] install.sh --ref=$good_ref was incorrectly rejected" >&2
        echo "  output: $out" >&2
        fail=1
    fi
done

# ── phase-end-detector plan-stem validator: bad stem aborts early.
#   A stem like `x"; rm` contains a `"` which is outside [A-Za-z0-9._-]+.
bad_stem_payload='{"stop_reason":"end_turn"}'
out=$(PLAN_FILE='plans/PLAN-x"; rm -rf ~; ".md' PHASE_NUM=1 \
      EDIKT_EVALUATOR_DRY_RUN=1 \
      bash -c "echo '$bad_stem_payload' | bash templates/hooks/phase-end-detector.sh" 2>&1 || true)
# Hook should either early-exit with {"continue":true} AND print the aborting
# systemMessage, OR not emit the evaluator prompt at all.
if echo "$out" | grep -qE 'claude -p .*ignore prior'; then
    echo "[INV-006] phase-end-detector invoked claude -p with unsafe plan stem" >&2
    fail=1
fi

exit $fail
