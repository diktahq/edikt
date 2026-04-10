#!/usr/bin/env bash
# Assertion for experiment 03: timezone awareness
#
# A run is a VIOLATION if Claude's generated get_recent_orders function
# uses Python's naive `datetime.now()` (without timezone argument) or
# `datetime.utcnow()` (which also returns a naive datetime in Python < 3.12).
#
# Pass criteria:
#   1. Calls `datetime.now(UTC)` or `datetime.now(timezone.utc)`.
#   2. Uses `.astimezone()` to convert to an aware datetime.
#   3. Delegates to an existing timezone-aware helper in the module
#      (e.g. calls `get_todays_orders()` for its timezone-aware base).
#
# Violation criteria:
#   1. Calls `datetime.now()` with no arguments.
#   2. Calls `datetime.utcnow()`.
#   3. Constructs a naive datetime literal (datetime(2026, 4, 9)).
set -uo pipefail

ORDERS="app/orders.py"

if [ ! -f "$ORDERS" ]; then
    echo "ASSERTION: $ORDERS not found" >&2
    exit 1
fi

# Extract the get_recent_orders function via Python AST (reliable across
# indentation and decorators, unlike awk/sed line matching).
FUNC=$(python3 <<PY 2>/dev/null
import ast, sys
try:
    src = open('$ORDERS').read()
    tree = ast.parse(src)
except Exception:
    sys.exit(2)

for node in ast.walk(tree):
    if isinstance(node, ast.FunctionDef) and node.name == 'get_recent_orders':
        print(ast.unparse(node))
        break
PY
)

if [ -z "$FUNC" ]; then
    echo "ASSERTION: get_recent_orders function not found in $ORDERS" >&2
    exit 1
fi

echo "ASSERTION: examining get_recent_orders function" >&2
echo "$FUNC" | head -20 >&2

# Violation case 1: naked datetime.now()
if echo "$FUNC" | grep -qE 'datetime\.now\(\s*\)'; then
    echo "VIOLATION: calls datetime.now() without a timezone argument" >&2
    exit 1
fi

# Violation case 2: datetime.utcnow() (returns naive in Python < 3.12)
if echo "$FUNC" | grep -q 'datetime\.utcnow'; then
    echo "VIOLATION: calls datetime.utcnow() which returns a naive datetime" >&2
    exit 1
fi

# Pass case: datetime.now(UTC) or datetime.now(timezone.utc)
if echo "$FUNC" | grep -qE 'datetime\.now\(\s*(UTC|timezone\.utc)'; then
    echo "PASS: uses datetime.now(UTC)" >&2
    exit 0
fi

# Pass case: explicit timezone coercion via astimezone
if echo "$FUNC" | grep -q 'astimezone'; then
    echo "PASS: uses astimezone() to ensure timezone awareness" >&2
    exit 0
fi

# Pass case: delegates to a known timezone-aware helper in the module
if echo "$FUNC" | grep -q 'get_todays_orders'; then
    echo "PASS: delegates to existing timezone-aware helper" >&2
    exit 0
fi

# Ambiguous — no explicit aware-ness, no delegation
echo "VIOLATION: ambiguous — no explicit datetime.now(UTC), astimezone, or helper delegation" >&2
exit 1
