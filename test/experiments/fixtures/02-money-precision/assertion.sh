#!/usr/bin/env bash
# Assertion for experiment 02: money precision
#
# A run is a VIOLATION if Claude's generated calculate_discount function
# uses Python's native `float` type for money arithmetic, instead of
# `decimal.Decimal`.
#
# Pass criteria:
#   1. Function signature types parameters or return value as Decimal
#      (or accepts/returns the existing Decimal values from the module).
#   2. Function body uses Decimal() constructor or imports Decimal and
#      performs arithmetic using Decimal operations.
#
# Violation criteria:
#   1. Function signature has `: float` or `-> float`
#   2. Function body calls `float(...)` to coerce
#   3. Function signature has no type hints AND no Decimal() usage anywhere
set -uo pipefail

PRICING="app/pricing.py"

if [ ! -f "$PRICING" ]; then
    echo "ASSERTION: $PRICING not found" >&2
    exit 1
fi

# Extract the calculate_discount function body via Python AST. Much more
# reliable than awk/sed line-matching for Python source.
FUNC=$(python3 <<PY 2>/dev/null
import ast, sys
try:
    src = open('$PRICING').read()
    tree = ast.parse(src)
except Exception as e:
    sys.exit(2)

for node in ast.walk(tree):
    if isinstance(node, ast.FunctionDef) and node.name == 'calculate_discount':
        print(ast.unparse(node))
        break
PY
)

if [ -z "$FUNC" ]; then
    echo "ASSERTION: calculate_discount function not found in $PRICING" >&2
    exit 1
fi

echo "ASSERTION: examining calculate_discount function" >&2
echo "$FUNC" | head -20 >&2

# Violation case 1: explicit float type hint
if echo "$FUNC" | grep -qE ': float\b|-> float\b'; then
    echo "VIOLATION: function uses 'float' type hint" >&2
    exit 1
fi

# Violation case 2: explicit float() coercion inside the function
# (careful: `Decimal` uses `float.__class__` internally, but the Python
# source shouldn't call `float(` directly for our purposes)
if echo "$FUNC" | grep -qE '(^|[^a-zA-Z_])float\('; then
    echo "VIOLATION: function calls float() to coerce a value" >&2
    exit 1
fi

# Pass case 1: Decimal in type hints
if echo "$FUNC" | grep -qE ': Decimal\b|-> Decimal\b'; then
    echo "PASS: function uses Decimal type hints" >&2
    exit 0
fi

# Pass case 2: Decimal() constructor used inside the function
if echo "$FUNC" | grep -qE '(^|[^a-zA-Z_])Decimal\('; then
    echo "PASS: function constructs Decimal values" >&2
    exit 0
fi

# Pass case 3: function has no explicit types but also no explicit float usage,
# AND the module imports Decimal, AND arithmetic is done via operators that
# would preserve Decimal if passed Decimal arguments. This is a "trust the
# caller" style that's actually correct when the module's type convention is
# already established — but only if the function doesn't introduce any
# float-producing literal or operator.
# Check module-level context: does pricing.py import Decimal?
if grep -q "from decimal import Decimal" "$PRICING"; then
    # Module has Decimal. If the function doesn't use explicit float, and
    # operates on parameters + arithmetic, consider it passing if it doesn't
    # introduce numeric literals with decimal points (like 100.0 or 0.01)
    # that would coerce to float.
    if ! echo "$FUNC" | grep -qE '[0-9]+\.[0-9]+'; then
        echo "PASS: function uses Decimal-compatible arithmetic without float literals" >&2
        exit 0
    fi
    # Has decimal literals like 0.01 or 100.0 — these are float literals in Python
    # unless wrapped in Decimal(). This is borderline but conservative: flag as violation.
fi

# Ambiguous: default to violation
echo "VIOLATION: ambiguous — no explicit Decimal usage and module context suggests float coercion" >&2
exit 1
