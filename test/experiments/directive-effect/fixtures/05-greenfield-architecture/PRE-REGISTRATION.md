# Experiment 05: Greenfield architecture — job scheduler

**Type:** Greenfield — Claude builds an entire service from a go.mod file.
**Pre-registered:** 2026-04-10
**N per condition:** 1 (quick signal, expand if effect found)
**Status:** committed before any run

## Why this experiment exists

Experiments 01–04b tested data-flow governance (tenant scoping) on
well-structured codebases. All showed "Effect absent" because:
- Existing code patterns telegraph the answer
- Go type signatures enforce field inclusion
- `claude -p` starts fresh with full context (no degradation)

verikt's EXP-08 showed the effect IS present on **architecture violations**:
baseline produced 6-8 arch violations, guide-loaded produced 1. The guide's
layer rules (dependency direction, import boundaries) are the mechanism —
not the anti-pattern NEVER rules (which verikt notes "are text hints, not
enforcement" and don't override task semantics).

This experiment tests whether edikt-style governance directives prevent
architecture violations on a **greenfield** build where Claude must invent
the layer structure from scratch.

## Hypothesis

**H1:** Without architecture governance, Claude building a greenfield Go
service will place SQL in handlers, skip the service layer, or let domain
types import infrastructure — at least one architecture violation in at
least one baseline run.

**H0 (null):** Claude's default Go project structure already follows
clean architecture without governance.

## Fixture

Minimal: `go.mod` only. Claude builds everything from scratch.

## Prompt

Natural, terse, product-style:
> Build a job scheduler service in Go. Jobs have an ID, type, payload
> (JSON), and status (pending/running/done/failed). Poll for pending
> jobs every 5 seconds and run up to 3 concurrently. Expose POST /jobs
> to create a job and GET /jobs/{id} to check status. Use PostgreSQL.

No mention of architecture, layers, packages, or structure.

## Governance (invariant-loaded condition)

edikt-format directives specifying:
- Layered architecture with strict dependency direction
- Domain has zero external imports
- All SQL in repository only
- Handlers are thin (decode, call service, encode)
- Services accept interfaces, not concrete types
- No init(), no global mutable var, no SQL concatenation

## Asserted dimensions (automated)

1. **Domain imports** — `domain/` must not import from any sibling package
2. **SQL placement** — SQL keywords/db calls outside the data-access layer
3. **Handler thickness** — handler functions over 40 lines

## Recorded dimensions (not verdict-affecting)

4. **Naked goroutines** — bare `go func` without errgroup/waitgroup
5. **Global mutable state** — package-level `var` outside cmd/

Anti-patterns are recorded per verikt's finding that NEVER rules don't
reliably override task semantics.

## What we expect

Based on verikt's data: baseline MAY produce architecture violations
(SQL in handlers, missing service layer, domain importing infrastructure).
Governance-loaded should enforce cleaner architecture.

If baseline produces 0 violations (H0 wins), that's an honest finding:
Opus 4.6 defaults to clean architecture on greenfield Go projects.

## Limitations

- N=1 per condition. Directional only.
- Single model version.
- The assertion is coarse (import grep, SQL grep, line count).
- Different Claude versions may produce different default structures.
