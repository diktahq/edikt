#!/usr/bin/env bash
# Meta-test: every hook-payloads/*.json has a matching hook-outputs/*.expected.json.
# Prevents fixture rot as scenarios are added/removed.

set -uo pipefail

PROJECT_ROOT="${1:-$(cd "$(dirname "$0")/../.." && pwd)}"
PAYLOADS="$PROJECT_ROOT/test/fixtures/hook-payloads"
EXPECTED="$PROJECT_ROOT/test/expected/hook-outputs"

FAIL=0

for p in "$PAYLOADS"/*.json; do
    [ -e "$p" ] || { echo "FAIL: no payload fixtures found in $PAYLOADS"; exit 1; }
    name=$(basename "$p" .json)
    sibling="$EXPECTED/$name.expected.json"
    if [ ! -f "$sibling" ]; then
        echo "FAIL: payload $name.json has no matching $name.expected.json"
        FAIL=1
    fi
done

for e in "$EXPECTED"/*.expected.json; do
    [ -e "$e" ] || { echo "FAIL: no expected-output fixtures found in $EXPECTED"; exit 1; }
    name=$(basename "$e" .expected.json)
    sibling="$PAYLOADS/$name.json"
    if [ ! -f "$sibling" ]; then
        echo "FAIL: expected output $name.expected.json has no matching $name.json"
        FAIL=1
    fi
done

if [ "$FAIL" -eq 0 ]; then
    echo "PASS: fixture symmetry ($(ls "$PAYLOADS"/*.json | wc -l | tr -d ' ') pairs)"
fi

exit "$FAIL"
