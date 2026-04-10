# Experiment 03 — Timezone awareness invariant

**Pre-registered:** 2026-04-09
**Language:** Python
**Invariant tested:** INV-016 (Timestamps are timezone-aware; naive datetimes are forbidden) — see draft below
**Claude model to use:** whatever Claude Code's default is at run time (recorded in results)
**Claude Code version:** recorded in results
**N per condition:** 10

## Hypothesis

Without the invariant in context, Claude will generate a function that computes "orders from the last 24 hours" using Python's naive `datetime.now()` (without timezone information) in at least 5 out of 10 runs. When comparing this naive datetime against database columns that store timezone-aware datetimes, the result is silently incorrect (or raises a TypeError if the underlying layer is strict). With INV-016 loaded, the failure rate drops to 1 out of 10 or fewer.

## Invariant draft (INV-016)

This experiment tests an invariant we haven't written yet as a canonical example. Draft for use in the experiment:

```markdown
# INV-016: Timestamps are timezone-aware

**Status:** Active

## Statement

All datetime values in the system — stored, transmitted, computed,
compared, or logged — carry explicit timezone information. Naive
datetimes (Python `datetime` without `tzinfo`) are forbidden.

## Rationale

Naive datetimes are a silent bug factory. Python's `datetime.now()`
returns a naive datetime that assumes the local system's timezone —
which is usually set to UTC on servers but might be anything on a
developer's machine. Comparing a naive datetime against a
timezone-aware one raises TypeError (if you're lucky) or produces
silently wrong results in libraries that coerce (if you're not).

The constraint applies to all layers: storage uses timezone-aware
types (PostgreSQL `TIMESTAMP WITH TIME ZONE`), application code uses
`datetime.now(UTC)` or equivalent, API responses include timezone
information in serialized form (ISO 8601 with `Z` or offset).

## Consequences of violation

- **Silent wrong results on DST transitions.** A naive datetime is
  interpreted in local time, which changes twice a year in most
  regions. A "last 24 hours" query can return 23 or 25 hours of data
  without warning.
- **Silent wrong results across server timezone changes.** Move your
  server from UTC to a different tz and all naive datetime
  comparisons silently change meaning.
- **TypeError in strict comparison paths.** Python raises TypeError
  when comparing naive and aware datetimes. This surfaces as a
  500 error to users, but only on code paths that actually hit
  the mixed comparison — which can be deeply buried.
- **Off-by-hours bugs in reports.** "Last week" queries return
  slightly wrong data; aggregations drift.

## Implementation

- Use `datetime.now(UTC)` instead of `datetime.now()`. Import UTC
  from `datetime` (Python 3.11+) or use `pytz.UTC` / `timezone.utc`
  for older versions.
- Database columns use `TIMESTAMP WITH TIME ZONE` (`timestamptz` in
  Postgres). Never `TIMESTAMP WITHOUT TIME ZONE`.
- ORM configurations (SQLAlchemy, Django, etc.) have explicit
  `timezone=True` on datetime fields.
- API serialization uses ISO 8601 with timezone indicator (`Z` for
  UTC or an explicit offset like `+00:00`).

## Anti-patterns

- `datetime.now()` without a timezone argument.
- `datetime.utcnow()` — this returns a naive datetime in UTC,
  confusingly. Use `datetime.now(UTC)` instead.
- Storing datetimes as strings or Unix timestamps to "sidestep" the
  issue — this loses type safety and pushes the problem to
  deserialization.
- Comparing naive and aware datetimes with implicit coercion — if
  the library allows it, you get silently wrong results.

## Enforcement

- Linter rule: grep for `datetime.now()` without arguments and
  `datetime.utcnow()` fails pre-commit hook.
- ORM model schema validation: datetime fields must have
  `timezone=True`.
- Database schema linter: migrations with `TIMESTAMP WITHOUT TIME ZONE`
  fail pre-push.
- edikt directive: "All datetimes must be timezone-aware. Never use
  `datetime.now()` without a timezone argument."
- Code review checklist: datetime handling is a review checkpoint.
```

This invariant gets written as part of Phase 6 of v0.3.0 implementation. For the experiment, it's drafted here and used directly.

## Fixture

Location when built: `test/experiments/fixtures/03-timezone-awareness/project/`

**Project structure:**

```
project/
├── pyproject.toml
├── app/
│   ├── __init__.py
│   ├── orders.py          # existing module with timezone-aware functions
│   └── db.py              # mock database layer that uses timezone-aware datetimes
└── tests/
    └── test_orders.py
```

**Key fixture details:**

- `app/db.py` has a mock `query_orders_since(cutoff: datetime)` that expects a timezone-aware datetime and raises TypeError if passed a naive one.
- `app/orders.py` has one existing function that correctly uses `datetime.now(UTC)`:
  ```python
  def get_todays_orders():
      start_of_day = datetime.now(UTC).replace(hour=0, minute=0, second=0, microsecond=0)
      return db.query_orders_since(start_of_day)
  ```
- The import line at the top of `orders.py` is `from datetime import datetime, UTC, timedelta` — so Claude can see that `UTC` is already imported and available.
- The fixture runs and the existing function works correctly.

## Prompt (verbatim — do not modify without re-registering)

```
Add a function `get_recent_orders()` to `app/orders.py` that returns all orders created in the last 24 hours.
```

### Contamination review

- ❌ Does not contain "timezone", "UTC", "aware", "naive", "tz", "datetime"
- ❌ Does not hint at the failure mode
- ❌ "last 24 hours" is natural English and would trigger any Python developer to compute `now - timedelta(hours=24)` — the natural implementation tests exactly the naive-vs-aware question
- ✅ Human-natural phrasing — this is exactly how a product request would be phrased
- ✅ Points Claude at the existing file so it can read the pattern

**Verdict:** uncontaminated.

## Assertion (verbatim — committed before running)

A run is a **violation** if the generated `get_recent_orders` function:

1. Calls `datetime.now()` without a timezone argument, OR
2. Calls `datetime.utcnow()` (returns naive datetime), OR
3. Constructs a datetime literal without timezone info (e.g., `datetime(2026, 4, 9)` with no tz)

A run is a **pass** if the generated function:

1. Calls `datetime.now(UTC)` or `datetime.now(timezone.utc)`, OR
2. Calls `datetime.now().astimezone()` to get an aware datetime (technically correct, though unusual), OR
3. Uses a variable that was already assigned as timezone-aware (e.g., inheriting from a helper)

**Assertion script (pseudocode):**

```bash
#!/bin/bash
# Input: path to orders.py after Claude's edit
# Output: exit 0 = pass, exit 1 = violation

ORDERS="$1"

# Extract the get_recent_orders function
FUNC=$(python3 -c "
import ast
tree = ast.parse(open('$ORDERS').read())
for node in ast.walk(tree):
    if isinstance(node, ast.FunctionDef) and node.name == 'get_recent_orders':
        print(ast.unparse(node))
        break
")

# Violation case 1: naked datetime.now()
if echo "$FUNC" | grep -qE 'datetime\.now\(\s*\)'; then
    exit 1
fi

# Violation case 2: datetime.utcnow() (returns naive in most Python versions)
if echo "$FUNC" | grep -q 'datetime\.utcnow'; then
    exit 1
fi

# Pass case: datetime.now(UTC) or datetime.now(timezone.utc)
if echo "$FUNC" | grep -qE 'datetime\.now\(\s*(UTC|timezone\.utc)'; then
    exit 0
fi

# Pass case: explicit timezone coercion
if echo "$FUNC" | grep -q 'astimezone'; then
    exit 0
fi

# Ambiguous (neither explicit violation nor explicit pass) — default to violation
exit 1
```

## Expected outcomes (pre-committed)

- **Effect confirmed**: baseline ≥ 5/10 violations, invariant-loaded ≤ 1/10 violations
- **Effect weak**: baseline ≥ 5/10, invariant-loaded > 1/10 but < baseline
- **Effect absent**: baseline < 5/10 (Claude already uses timezone-aware datetimes — hypothesis wrong)
- **Effect inverted**: invariant-loaded > baseline (investigate)

## Notes on expected baseline rate

Prior expectation: this is the most "easily confirmed" of the three hypotheses. Python's `datetime.now()` without a timezone argument is the default that every tutorial shows, and Claude has likely been trained on vast amounts of Python code that uses naive datetimes. The fixture uses `datetime.now(UTC)` in the existing function, which provides some cue, but naive-datetime habits are strong in Python. A baseline rate of 7-9/10 violations would not surprise me.

## Invariant loaded in condition B

The draft INV-016 content above is loaded into Claude's context for condition B runs.

## Run protocol

Same as experiment 01.

## Results

(Populated after running.)
