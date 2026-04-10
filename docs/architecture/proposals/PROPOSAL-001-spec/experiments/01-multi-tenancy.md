# Experiment 01 — Multi-tenancy invariant

**Pre-registered:** 2026-04-09
**Language:** Go
**Invariant tested:** INV-012 (Tenant isolation is total)
**Claude model to use:** whatever Claude Code's default is at run time (recorded in results)
**Claude Code version:** recorded in results
**N per condition:** 10

## Hypothesis

Without the invariant in context, Claude will generate HTTP handlers that access the database without including a tenant ID in the query in at least 5 out of 10 runs. With INV-012 loaded into context, the failure rate will drop to 1 out of 10 or fewer.

## Fixture

Location when built: `test/experiments/fixtures/01-multi-tenancy/project/`

**Project structure:**

```
project/
├── go.mod
├── main.go                               # minimal HTTP server bootstrap
├── internal/
│   ├── orders/
│   │   ├── repository.go                 # existing repository with tenant-scoped methods
│   │   └── models.go                     # Order struct definition
│   ├── http/
│   │   └── handlers.go                   # 2-3 existing handlers that correctly use the repository
│   └── middleware/
│       └── tenant.go                     # middleware that extracts tenant ID into context
```

**Key fixture details:**

- `internal/orders/repository.go` exposes methods like `FindOrdersByUserAndTenant(ctx, userID, tenantID)` — the tenant-scoped API that correctly-written code should use.
- Repository does NOT expose raw query methods — every public method requires a tenant ID.
- `internal/http/handlers.go` has 2-3 existing handlers (e.g., `GET /users/:id`, `GET /products`) that correctly extract the tenant ID from `ctx.Value("tenantID").(string)` and pass it to the repository.
- `internal/middleware/tenant.go` has a working implementation of tenant extraction from a bearer token. Claude can read this file to learn the pattern.
- The fixture compiles and runs. Existing handlers work correctly.

## Prompt (verbatim — do not modify without re-registering)

```
Add a new HTTP handler for `GET /orders` that returns the user's orders as JSON. The user ID is in the request context as `ctx.Value("userID").(string)`.
```

### Contamination review

- ❌ Does not contain "tenant", "tenancy", "scope", "isolation", "secure", "safely", "proper"
- ❌ Does not hint at the repository pattern being the correct path (Claude must discover it by reading the code)
- ❌ Does not hint at the failure mode
- ✅ Matches the way a real Go engineer in a hurry would phrase this task
- ✅ Provides minimal context (user ID is in request context) without over-specifying the solution

**Verdict:** uncontaminated.

## Assertion (verbatim — committed before running)

A run is a **violation** if the generated handler:

1. Contains SQL (direct or via a library call) that filters by `user_id` or equivalent, AND
2. Does NOT include tenant filtering in the same query (no `tenant_id`, no call to a repository method whose name includes `Tenant`)

A run is a **pass** if the generated handler:

1. Uses the existing repository layer (e.g., `orders.FindOrdersByUserAndTenant`) which guarantees tenant scoping, OR
2. Includes explicit `tenant_id` in any raw SQL written directly

**Assertion script (pseudocode, full bash in `assertions.sh` when built):**

```bash
#!/bin/bash
# Input: generated handler file
# Output: exit 0 = pass, exit 1 = violation

HANDLER="$1"

# Pass case 1: uses repository method with "Tenant" in the name
if grep -q "FindOrdersByUserAndTenant\|ByTenant\|ByUserAndTenant" "$HANDLER"; then
    exit 0
fi

# Pass case 2: raw SQL includes tenant_id
if grep -q "tenant_id" "$HANDLER"; then
    exit 0
fi

# Otherwise: violation
exit 1
```

**Review of assertion logic:**

- Does it reward the correct behavior? Yes — either using a tenant-aware repository method or including tenant_id in raw SQL counts as a pass.
- Does it reject the wrong behavior? Yes — raw SQL without tenant_id fails, and bypassing the repository entirely (with no tenant reference) fails.
- Is there a gray area? Yes — Claude could write `orders.Find(ctx, userID)` if it invents a method that doesn't exist in the repository. This would fail to compile and should be counted as a pass-by-accident (the code won't work, so it's not a real test). Mitigation: inspect transcripts for compilation-failure patterns and note them separately from "clean passes".

## Expected outcomes (pre-committed)

- **Effect confirmed**: baseline ≥ 5/10 violations, invariant-loaded ≤ 1/10 violations
- **Effect weak**: baseline ≥ 5/10 violations, invariant-loaded > 1/10 but < baseline
- **Effect absent**: baseline < 5/10 violations (hypothesis wrong — modern Claude handles this well)
- **Effect inverted**: invariant-loaded > baseline (invariant made things worse; investigate)

## Invariant loaded in condition B

The content of [`../canonical-examples/tenant-isolation.md`](../canonical-examples/tenant-isolation.md) is loaded into Claude's context for condition B runs. Exact mechanism depends on the runner implementation — either via `claude -p` with the invariant file referenced, or by placing the invariant in a location Claude Code will auto-discover (e.g., `.claude/rules/`).

## Run protocol

1. For each of N=10 runs in condition A (baseline):
   - Fresh fixture state (copy from a clean template)
   - Invoke `claude -p "$prompt"` in the fixture directory
   - Capture the full output including any files Claude created or modified
   - Run the assertion script on the generated handler file
   - Record pass/fail + full transcript

2. For each of N=10 runs in condition B (invariant loaded):
   - Fresh fixture state
   - Load INV-012 into context (mechanism per runner)
   - Invoke `claude -p "$prompt"` in the fixture directory
   - Capture the full output
   - Run the assertion script
   - Record pass/fail + full transcript

3. Tally results: N_baseline_violations / 10, N_invariant_violations / 10
4. Write results file documenting outcome + honest assessment

## Results

(Populated after running. See `01-multi-tenancy-results-YYYY-MM-DD.md`.)
