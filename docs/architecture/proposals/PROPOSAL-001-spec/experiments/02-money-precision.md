# Experiment 02 — Money precision invariant

**Pre-registered:** 2026-04-09
**Language:** Python
**Invariant tested:** INV-008 (Monetary values are fixed-point, never floating-point)
**Claude model to use:** whatever Claude Code's default is at run time (recorded in results)
**Claude Code version:** recorded in results
**N per condition:** 10

## Hypothesis

Without the invariant in context, Claude will generate a discount calculation function using Python's native `float` type (either in parameter type hints, return type, or arithmetic operations) in at least 5 out of 10 runs. With INV-008 loaded into context, the failure rate will drop to 1 out of 10 or fewer.

## Fixture

Location when built: `test/experiments/fixtures/02-money-precision/project/`

**Project structure:**

```
project/
├── pyproject.toml              # minimal Python project with decimal available (stdlib)
├── app/
│   ├── __init__.py
│   ├── pricing.py              # existing module with Decimal-based functions
│   └── models.py               # Product dataclass with Decimal price field
└── tests/
    └── test_pricing.py         # existing tests using Decimal
```

**Key fixture details:**

- `app/pricing.py` has 2-3 existing functions using `Decimal`:
  - `calculate_tax(subtotal: Decimal, rate: Decimal) -> Decimal`
  - `apply_shipping(subtotal: Decimal, shipping: Decimal) -> Decimal`
  - Imports `from decimal import Decimal` at the top
- `app/models.py` has a `Product` dataclass with `price: Decimal`
- `tests/test_pricing.py` imports and uses `Decimal` consistently
- The fixture is valid Python (runs, tests pass) and demonstrates Decimal usage throughout

## Prompt (verbatim — do not modify without re-registering)

```
Add a function `calculate_discount(price, percentage)` to `app/pricing.py` that returns the discounted amount. For example, calculate_discount(100, 10) should return 90.
```

### Contamination review

- ❌ Does not contain "decimal", "precision", "float", "accurate", "exact", "safely"
- ❌ Does not hint at the type system
- ❌ Example uses integers (100, 10, 90) to avoid hinting at the type choice (floats would be more obvious if example was 99.99 or 10.5)
- ✅ Matches how a Python engineer in a hurry would phrase this task
- ✅ Points Claude at a specific file (so it has to read the existing code) without telling Claude what types that file uses

**Verdict:** uncontaminated. The integer example (100, 10, 90) is an intentional choice — it neither hints at integers nor at floats; Claude has to decide based on context.

## Assertion (verbatim — committed before running)

A run is a **violation** if the generated `calculate_discount` function:

1. Has a type hint of `float` on any parameter (`price`, `percentage`) or the return type, OR
2. Calls `float()` or converts any value to a float type, OR
3. Uses a numeric literal that would produce a float (e.g., `100.0` instead of `Decimal("100")` or `100`) in arithmetic mixed with user inputs, OR
4. Imports nothing from `decimal` while performing arithmetic on the price

A run is a **pass** if the generated function:

1. Uses `Decimal` in type hints (`price: Decimal`, `percentage: Decimal`, or return annotation), OR
2. Converts inputs to `Decimal` explicitly (`Decimal(str(price))`) before arithmetic, OR
3. Already receives `Decimal` inputs and performs arithmetic using `Decimal` operations (no float coercion)

**Assertion script (pseudocode):**

```bash
#!/bin/bash
# Input: path to pricing.py after Claude's edit
# Output: exit 0 = pass, exit 1 = violation

PRICING="$1"

# Extract the calculate_discount function
FUNC=$(python3 -c "
import ast
tree = ast.parse(open('$PRICING').read())
for node in ast.walk(tree):
    if isinstance(node, ast.FunctionDef) and node.name == 'calculate_discount':
        print(ast.unparse(node))
        break
")

# Violation case 1: float in type hints
if echo "$FUNC" | grep -qE ': float\b|-> float\b'; then
    exit 1
fi

# Violation case 2: explicit float() coercion
if echo "$FUNC" | grep -q 'float('; then
    exit 1
fi

# Pass case 1: Decimal in type hints
if echo "$FUNC" | grep -qE ': Decimal\b|-> Decimal\b'; then
    exit 0
fi

# Pass case 2: Uses Decimal() constructor
if echo "$FUNC" | grep -q 'Decimal('; then
    exit 0
fi

# Ambiguous — no explicit float violations, but no explicit Decimal either
# (e.g., just does `return price - (price * percentage / 100)`)
# Check if inputs would inherit Decimal from context (they should if the rest of the module uses Decimal consistently)
# This is a gray area — inspect transcript
echo "AMBIGUOUS: no explicit float, no explicit Decimal" >&2
exit 1  # treat ambiguous as violation by default; manual review can promote to pass
```

**Review of assertion logic:**

- Violations are specific (float type hints, explicit float coercion)
- Passes are specific (Decimal type hints or Decimal constructor use)
- Ambiguous cases (function signature without types, arithmetic without explicit types) default to violation. This is a conservative bias — we're more likely to under-count the invariant's effectiveness than over-count.
- Gray-area runs get flagged in transcripts for manual review. If a significant fraction are ambiguous, we note this in the results and potentially re-run with a tightened prompt.

## Expected outcomes (pre-committed)

- **Effect confirmed**: baseline ≥ 5/10 violations, invariant-loaded ≤ 1/10 violations
- **Effect weak**: baseline ≥ 5/10, invariant-loaded > 1/10 but < baseline
- **Effect absent**: baseline < 5/10 (Claude already uses Decimal correctly in Python context — hypothesis wrong for this model)
- **Effect inverted**: invariant-loaded > baseline (investigate)

## Notes on expected baseline rate

Prior expectation: moderately high baseline violation rate (~60-70%). Python's `float` is the default numeric type that comes to mind first, and while `Decimal` exists in the standard library, Claude often uses `float` unless cued otherwise. However, the fixture has `Decimal` usage throughout, which might cue Claude via context pattern-matching. The baseline rate may be lower than "Claude writes Python from scratch" because the fixture actively demonstrates Decimal usage.

This is worth noting in results — if baseline is low (e.g., 2/10), it may be because the fixture cues Claude rather than because Claude "knows" to use Decimal in general.

## Invariant loaded in condition B

The content of [`../canonical-examples/money-precision.md`](../canonical-examples/money-precision.md) is loaded into Claude's context for condition B runs.

## Run protocol

Same as experiment 01. See [`01-multi-tenancy.md`](01-multi-tenancy.md) for the protocol template.

## Results

(Populated after running.)
