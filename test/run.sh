#!/bin/bash
# edikt test runner — discovers and runs all test-*.sh files
#
# Sandboxed: $HOME, $EDIKT_HOME, $CLAUDE_HOME are redirected to a per-run
# temp tree so tests never touch the user's real ~/.edikt/ or ~/.claude/.
# This is Layer 3 isolation (SPEC-004 §9.3).

set -uo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

# ─── Layer 3: sandbox isolation ─────────────────────────────────────────────
# Redirect HOME and edikt/claude state dirs into a per-run temp tree.
# This eliminates shared-state flakiness and prevents tests from
# contaminating the developer's real ~/.edikt/ or ~/.claude/ — which is
# critical when edikt itself is installed on the dev machine and a live
# Claude Code session may be running in parallel.
TEST_SANDBOX="$(mktemp -d -t edikt-test-XXXXXX)"
export HOME="$TEST_SANDBOX/home"
export EDIKT_HOME="$HOME/.edikt"
export CLAUDE_HOME="$HOME/.claude"
mkdir -p "$EDIKT_HOME" "$CLAUDE_HOME"

cleanup() {
    rm -rf "$TEST_SANDBOX"
}
trap cleanup EXIT INT TERM

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[0;33m'
BOLD='\033[1m'
NC='\033[0m'

echo ""
echo -e "${BOLD}edikt Test Suite${NC}"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Sandbox: $TEST_SANDBOX"

SUITE_COUNT=0
FAILED_SUITES=0

# ─── Layer 1: unit tests (fixture-driven, no API) ───────────────────────────
# Discovers test/unit/**/test_*.sh. Runs before the shell suites so fast
# fixture checks fail early before any longer-running test.
if [ -d "$SCRIPT_DIR/unit" ]; then
    while IFS= read -r unit_test; do
        rel="${unit_test#$SCRIPT_DIR/}"
        suite_name="${rel%.sh}"
        echo ""
        echo -e "${BOLD}[$suite_name]${NC}"
        SUITE_COUNT=$((SUITE_COUNT + 1))

        chmod +x "$unit_test"
        if ! bash "$unit_test" "$PROJECT_ROOT"; then
            FAILED_SUITES=$((FAILED_SUITES + 1))
        fi
    done < <(find "$SCRIPT_DIR/unit" -type f -name 'test_*.sh' | sort)
fi

# ─── Existing shell suites ──────────────────────────────────────────────────
for test_file in "$SCRIPT_DIR"/test-*.sh; do
    if [ ! -f "$test_file" ]; then
        echo "No test files found."
        break
    fi

    suite_name=$(basename "$test_file" .sh | sed 's/^test-//')
    echo ""
    echo -e "${BOLD}[$suite_name]${NC}"
    SUITE_COUNT=$((SUITE_COUNT + 1))

    chmod +x "$test_file"
    if ! bash "$test_file" "$PROJECT_ROOT"; then
        FAILED_SUITES=$((FAILED_SUITES + 1))
    fi
done

# ─── Layer 2a: shell-based integration tests (install.sh bootstrap) ─────────
# Phase 5 owns test/integration/install/test_*.sh. Discovered the same way
# as Layer 1 unit tests. Runs before the pytest branch so install-bootstrap
# regressions fail loudly even when pytest isn't installed.
if [ -d "$SCRIPT_DIR/integration/install" ]; then
    while IFS= read -r it_test; do
        rel="${it_test#$SCRIPT_DIR/}"
        suite_name="${rel%.sh}"
        echo ""
        echo -e "${BOLD}[$suite_name]${NC}"
        SUITE_COUNT=$((SUITE_COUNT + 1))

        chmod +x "$it_test"
        if ! bash "$it_test" "$PROJECT_ROOT"; then
            FAILED_SUITES=$((FAILED_SUITES + 1))
        fi
    done < <(find "$SCRIPT_DIR/integration/install" -type f -name 'test_*.sh' | sort)
fi

# ─── Layer 2b: shell-based integration tests (init provenance) ───────────────
# Phase 9 owns test/integration/init/test_*.sh. Structural checks for
# _substitutions.yaml, stack markers, and provenance instructions in init.md.
#
# NOTE: test/integration/upgrade/ is pytest-only by design (no test_*.sh
# files are expected there). All upgrade tests are picked up by the pytest
# invocation in the Layer 2 block below. Adding .sh tests under upgrade/
# would require a matching discovery block here.
if [ -d "$SCRIPT_DIR/integration/init" ]; then
    while IFS= read -r it_test; do
        rel="${it_test#$SCRIPT_DIR/}"
        suite_name="${rel%.sh}"
        echo ""
        echo -e "${BOLD}[$suite_name]${NC}"
        SUITE_COUNT=$((SUITE_COUNT + 1))

        chmod +x "$it_test"
        if ! bash "$it_test" "$PROJECT_ROOT"; then
            FAILED_SUITES=$((FAILED_SUITES + 1))
        fi
    done < <(find "$SCRIPT_DIR/integration/init" -type f -name 'test_*.sh' | sort)
fi

# ─── Layer 2: Agent SDK integration tests (opt-in) ──────────────────────────
# Placeholder branch for Phase 12. When test/integration/ exists and
# SKIP_INTEGRATION != 1, the pytest suite runs here. Until then this is
# a no-op, but the branch is in place so CI wiring (Phase 14) works.
# Phase 12 will populate test/integration/ with pytest files; until then
# we gate on the presence of conftest.py or a pytest.ini to avoid pytest
# tripping over the shell-based install tests above.
if [ "${SKIP_INTEGRATION:-0}" != "1" ] && [ -d "$SCRIPT_DIR/integration" ] && \
   { [ -f "$SCRIPT_DIR/integration/conftest.py" ] || [ -f "$SCRIPT_DIR/integration/pytest.ini" ]; }; then
    echo ""
    echo -e "${BOLD}[integration]${NC}"
    SUITE_COUNT=$((SUITE_COUNT + 1))
    if ! (cd "$SCRIPT_DIR/integration" && pytest -v); then
        FAILED_SUITES=$((FAILED_SUITES + 1))
    fi
elif [ "${SKIP_INTEGRATION:-0}" = "1" ]; then
    echo ""
    echo -e "${YELLOW}Integration tests skipped (SKIP_INTEGRATION=1)${NC}"
fi

echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
if [ "$FAILED_SUITES" -eq 0 ]; then
    echo -e "${GREEN}${BOLD}All $SUITE_COUNT test suites passed${NC}"
else
    echo -e "${RED}${BOLD}$FAILED_SUITES of $SUITE_COUNT test suites had failures${NC}"
fi
echo ""

exit "$FAILED_SUITES"
