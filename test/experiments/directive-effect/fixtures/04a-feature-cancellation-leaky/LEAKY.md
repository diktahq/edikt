# This fixture leaks the answer — preserved for audit

Experiment 04 was meant to test whether Claude preserves tenant isolation
when adding a multi-file feature. It produced "Effect absent" but the
fixture telegraphs the answer in four distinct ways, so the null result
does not falsify the hypothesis — it just confirms that a *heavily
helper-assisted* codebase makes governance redundant.

## The four leaks

1. **`logging.FromContext(ctx)` auto-populates tenant_id.** The contextual
   logger reads `ctxkeys.TenantID` from the context and stamps it on every
   log line automatically. Claude gets tenant-tagged logs for free. There
   is nothing to forget.

2. **The repository is the only database surface and auto-scopes.** Every
   method in `internal/repository/orders.go` calls `tenantFrom(ctx)` at
   the top. Claude just adds another method; the helper does the work.

3. **No service layer.** Handlers call the repository directly. The
   "passing tenant from handler → service → repository" failure surface
   does not exist in this fixture.

4. **No fan-out.** A real cancellation touches audit log, domain event
   bus, cache invalidation, sometimes search indices. This fixture has
   only database and log — both auto-scoped. Zero surfaces where Claude
   can forget to include tenant explicitly.

## Why this is preserved

Per the pre-registration methodology, failed or invalid experiments are
not silently deleted. This fixture remains so the honest audit chain
stays intact. Experiment 04b replaces it with a fixture that removes all
four leaks.

See `test/experiments/fixtures/04b-feature-cancellation/` for the redesign.
