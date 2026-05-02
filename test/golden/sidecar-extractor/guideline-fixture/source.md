# Guideline: HTTP handler conventions

## Context

Every HTTP handler in this service is wired into the same router and shares the same observability pipeline. Inconsistent handler shape produces broken metrics, fragile retry behavior, and noisy logs.

## Rules

- Every handler MUST emit a structured log entry on entry and on exit, including the `request_id` from the inbound header.
- Handlers MUST NEVER block on unbounded I/O. Outbound HTTP calls MUST use a context with a 5-second deadline.
- All 4xx responses MUST include a JSON body with `error.code` and `error.message`. NEVER return a 4xx with an empty body.
- Authenticated endpoints MUST validate the JWT before any business logic runs. NEVER trust claims that haven't been verified.

## Examples

```go
func handleCreateOrder(w http.ResponseWriter, r *http.Request) {
    // ...
}
```
