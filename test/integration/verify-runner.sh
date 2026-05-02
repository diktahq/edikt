#!/usr/bin/env bash
# Integration test for Phase 12 — `edikt verify` runner.
#
# Asserts:
#   1. `edikt verify sidecar-architecture --phase 1` against this repo
#      exits 0 (Phase 1 is already complete and its criteria are real).
#   2. A deliberately failing criterion produces exit 1 and the failure
#      is recorded in the JSON report.
#   3. `--allow-failures` suppresses the non-zero exit but still records
#      the failure in the report.
#   4. JSON + text reports are written under .edikt/state/verify/.
#
# The script builds the edikt binary in-place (`go build`) so it does not
# depend on a globally installed edikt or on the Phase 11 release pipeline.

set -euo pipefail

PROJECT_ROOT="${PROJECT_ROOT:-$(cd "$(dirname "$0")/../.." && pwd)}"
cd "$PROJECT_ROOT"

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
        echo -e "  ${GREEN}+${RESET} $label"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}x${RESET} $label"
        echo -e "    ${DIM}cmd: $cmd${RESET}"
        fail_count=$((fail_count + 1))
    fi
}

# Build the binary into a tempdir so the test does not pollute $PATH.
BIN_DIR="$(mktemp -d)"
trap 'rm -rf "$BIN_DIR"' EXIT
EDIKT_BIN="$BIN_DIR/edikt"
( cd tools/edikt && go build -o "$EDIKT_BIN" . )

echo "Phase 12 — verify runner"

# 1. Real Phase 1 criteria pass against this repo.
assert "edikt verify sidecar-architecture --phase 1 exits 0" \
    "'$EDIKT_BIN' verify sidecar-architecture --phase 1 >/dev/null"

# Pull a real report path for assertion 4 below.
LATEST_JSON="$(ls -t "$PROJECT_ROOT/.edikt/state/verify/sidecar-architecture-phase-1-"*.json 2>/dev/null | head -1 || true)"
assert "JSON report written under .edikt/state/verify/" "[ -n '$LATEST_JSON' ] && [ -f '$LATEST_JSON' ]"
assert "text report written alongside JSON" \
    "[ -f \"\${LATEST_JSON%.json}.txt\" ]"

# 2. Inject a deliberately failing criterion via a temporary plan + sidecar
# in an isolated workspace, so we never touch the real repo's plans.
SANDBOX="$(mktemp -d)"
mkdir -p "$SANDBOX/docs/internal/plans"
cat > "$SANDBOX/docs/internal/plans/PLAN-failtest-criteria.yaml" <<'YAML'
plan: failtest
schema_version: 1
phases:
  - id: "1"
    name: deliberately failing
    classification: testable
    completion_promise: NEVER COMPLETE
    criteria:
      - id: 1.1
        statement: this never passes
        verify: "exit 7"
YAML

if ( cd "$SANDBOX" && "$EDIKT_BIN" verify failtest --phase 1 >/dev/null 2>&1 ); then
    echo -e "  ${RED}x${RESET} failing criterion should produce exit 1"
    fail_count=$((fail_count + 1))
else
    rc=$?
    if [ "$rc" -eq 1 ]; then
        echo -e "  ${GREEN}+${RESET} failing criterion produces exit 1"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}x${RESET} failing criterion: exit $rc, want 1"
        fail_count=$((fail_count + 1))
    fi
fi

FAIL_JSON="$(ls -t "$SANDBOX/.edikt/state/verify/failtest-phase-1-"*.json 2>/dev/null | head -1 || true)"
assert "report written for failing run" "[ -n '$FAIL_JSON' ] && [ -f '$FAIL_JSON' ]"
if [ -n "$FAIL_JSON" ]; then
    if python3 -c "import json,sys; r=json.load(open('$FAIL_JSON')); sys.exit(0 if r['summary']['failed']==1 else 1)"; then
        echo -e "  ${GREEN}+${RESET} failing run records summary.failed=1"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}x${RESET} failing run did not record summary.failed=1"
        fail_count=$((fail_count + 1))
    fi
fi

# 3. --allow-failures suppresses exit code but still records failure.
assert "--allow-failures suppresses exit-1" \
    "( cd '$SANDBOX' && '$EDIKT_BIN' verify failtest --phase 1 --allow-failures >/dev/null )"

ALLOW_JSON="$(ls -t "$SANDBOX/.edikt/state/verify/failtest-phase-1-"*.json 2>/dev/null | head -1 || true)"
if [ -n "$ALLOW_JSON" ]; then
    if python3 -c "import json,sys; r=json.load(open('$ALLOW_JSON')); sys.exit(0 if r['summary']['failed']==1 else 1)"; then
        echo -e "  ${GREEN}+${RESET} --allow-failures still records the failure"
        pass_count=$((pass_count + 1))
    else
        echo -e "  ${RED}x${RESET} --allow-failures did not record the failure"
        fail_count=$((fail_count + 1))
    fi
fi

rm -rf "$SANDBOX"

echo
echo -e "${DIM}$pass_count passed, $fail_count failed.${RESET}"
exit "$fail_count"
