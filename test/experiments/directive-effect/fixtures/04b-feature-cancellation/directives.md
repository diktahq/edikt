# Governance Directives

Follow these directives in every file you write or edit.

## Non-Negotiable Constraints

These are invariants. Violation is never acceptable.

- Repository methods take `tenantID` as an explicit parameter. The repository does not read context. Service-layer callers are responsible for passing the authoritative tenant from the verified session. (ref: INV-012)
- Every structured log call written in a tenant-scoped flow includes a `"tenant_id"` field. The logger does not add it automatically — the caller passes it explicitly on every `log.Info` / `log.Warn` / `log.Error`. (ref: INV-012)
- Every `events.Publish` call sets `TenantID` on the `events.Event` literal. Downstream consumers filter on it; omission silently breaks the pipeline. (ref: INV-012)
- Every `audit.Record` call sets `TenantID` on the `audit.Entry`. (ref: INV-012)
- Tenant identity is read only from the verified session on `ctx`. Never from the request body, URL path parameter, or query string. (ref: INV-012)
- Raw SQL that touches a tenant-scoped table outside `internal/repository/` is forbidden. All database access goes through the repository. (ref: INV-012)

## Reminder

If you are about to write a log line, publish an event, record an audit entry, or call a repository method, and you cannot name the tenant scope on the spot — stop and read the context. Do not guess. Do not omit the field.
