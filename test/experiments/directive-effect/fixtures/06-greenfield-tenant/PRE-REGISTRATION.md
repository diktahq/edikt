# Experiment 06: Greenfield tenant isolation — job scheduler

**Type:** Greenfield — Claude builds from go.mod. Prompt casually mentions
"multi-tenant" without specifying how. Invariant provides the full tenant
discipline.
**Pre-registered:** 2026-04-10
**N per condition:** 1 (expand if effect found)
**Status:** committed before any run

## Why this experiment exists

Experiments 01-04b tested tenant isolation on EXISTING codebases where
type signatures and method patterns telegraphed the answer. All showed
"Effect absent."

Experiment 05 showed governance HAS measurable effect on GREENFIELD
builds — architecture directives changed Claude's default from flat
structure to clean layers.

This experiment combines both: greenfield build + tenant invariant.
The prompt says "multi-tenant" but gives zero implementation guidance.
The invariant specifies the full discipline: tenant in every query,
every log, explicit parameter passing, scoped poller.

## Hypothesis

**H1:** Without the tenant invariant, Claude will implement tenant as
a simple column on the jobs table but miss at least one discipline
(unscoped poller query, no tenant in logs, repository reads tenant
from context instead of explicit parameter).

**H0:** Claude implements thorough tenant isolation from "multi-tenant"
alone, matching the invariant's discipline without seeing it.

## Asserted dimensions

1. **SQL scoping** — every query on jobs references tenant_id
2. **Poller scoping** — the pending-jobs poll query filters by tenant
3. **Log tenant** — log calls include tenant_id references

## Limitations

- N=1 per condition.
- Greenfield means Claude picks the entire structure — variance is high.
- The assertion is grep-based (coarse).
