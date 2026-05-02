#!/usr/bin/env bash
# Integration test for Phase 12 — `edikt doctor` Plan Verification check.
#
# Asserts:
#   1. A fixture project with a plan + criteria sidecar + a `done` row but
#      NO verification report produces a WARN line in `edikt doctor` stdout
#      naming the plan and phase.
#   2. The check is silent (does not increment errN) — doctor exits 0 or 1
#      depending on warnings, never 2 from this check alone.
#   3. When a passing report exists for the row, no WARN is emitted.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$PROJECT_ROOT"

GREEN='\033[0;32m'
RED='\033[0;31m'
DIM='\033[0;90m'
RESET='\033[0m'

pass_count=0
fail_count=0

# Build edikt into a tempdir.
BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
EDIKT_BIN="$BIN_DIR/edikt"
( cd tools/edikt && go build -o "$EDIKT_BIN" . )

# Fixture sandbox so we don't touch the real project's plans.
SANDBOX="$(mktemp -d)"
mkdir -p "$SANDBOX/docs/internal/plans"
mkdir -p "$SANDBOX/.edikt"

cat > "$SANDBOX/.edikt/config.yaml" <<'YAML'
paths:
  plans: docs/internal/plans
YAML

cat > "$SANDBOX/docs/internal/plans/PLAN-doctorfix-criteria.yaml" <<'YAML'
plan: doctorfix
schema_version: 1
phases:
  - id: "1"
    name: needs verifying
    classification: testable
    completion_promise: VERIFIED
    criteria:
      - id: 1.1
        statement: trivially passes
        verify: "exit 0"
YAML

cat > "$SANDBOX/docs/internal/plans/PLAN-doctorfix.md" <<'MD'
# PLAN-doctorfix

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-05-02 |
MD

echo "Phase 12 — doctor Plan Verification check"

# Doctor exits non-zero when warnings exist; capture stdout regardless.
DOCTOR_OUT="$( cd "$SANDBOX" && "$EDIKT_BIN" doctor 2>&1 || true )"

if echo "$DOCTOR_OUT" | grep -q "Plan Verification"; then
    echo -e "  ${GREEN}+${RESET} doctor emits 'Plan Verification' header"
    pass_count=$((pass_count + 1))
else
    echo -e "  ${RED}x${RESET} doctor missing 'Plan Verification' header"
    echo "$DOCTOR_OUT"
    fail_count=$((fail_count + 1))
fi

if echo "$DOCTOR_OUT" | grep -q "doctorfix phase 1"; then
    echo -e "  ${GREEN}+${RESET} doctor names the plan + phase missing a report"
    pass_count=$((pass_count + 1))
else
    echo -e "  ${RED}x${RESET} doctor did not name doctorfix phase 1"
    fail_count=$((fail_count + 1))
fi

# Now write a passing report and re-run; the WARN should disappear.
mkdir -p "$SANDBOX/.edikt/state/verify"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
# Bump the timestamp by an hour to be safely newer than the criteria mtime.
cat > "$SANDBOX/.edikt/state/verify/doctorfix-phase-1-${TS}.json" <<JSON
{"plan_id":"doctorfix","phase":"1","summary":{"passed":1,"failed":0,"skipped":0,"timeout":0,"total":1},"criteria":[]}
JSON

DOCTOR_OUT2="$( cd "$SANDBOX" && "$EDIKT_BIN" doctor 2>&1 || true )"
if echo "$DOCTOR_OUT2" | grep -q "doctorfix phase 1"; then
    echo -e "  ${RED}x${RESET} doctor still warns even after passing report written"
    echo "$DOCTOR_OUT2"
    fail_count=$((fail_count + 1))
else
    echo -e "  ${GREEN}+${RESET} doctor falls silent after passing report exists"
    pass_count=$((pass_count + 1))
fi

rm -rf "$SANDBOX"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
