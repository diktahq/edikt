#!/usr/bin/env bash
# Meta-test: fixtures must not contain absolute user paths or timestamps.
# Enforces the portability invariant from SPEC-004 §9.1.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
# Phase 2 scope: hook payload fixtures only. Integration/spec fixtures
# elsewhere under test/fixtures/ are governed by their own generators
# and legitimately include authored timestamps.
FIXTURES="$PROJECT_ROOT/test/fixtures/hook-payloads"

FAIL=0

# Absolute user paths (macOS /Users/..., Linux /home/...).
# ${HOME} as a literal placeholder is allowed.
if grep -RE '/Users/[a-zA-Z]|/home/[a-zA-Z]' "$FIXTURES" 2>/dev/null; then
    echo "FAIL: hardcoded user path found in fixtures above"
    FAIL=1
fi

# ISO timestamps (YYYY-MM-DDTHH:MM:SS) and epoch-looking 10-digit integers
# in timestamp positions are banned — use placeholders instead.
if grep -RE '[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}' "$FIXTURES" 2>/dev/null; then
    echo "FAIL: hardcoded ISO timestamp found in fixtures above"
    FAIL=1
fi

if [ "$FAIL" -eq 0 ]; then
    echo "PASS: no hardcoded paths or timestamps in $FIXTURES"
fi

exit "$FAIL"
