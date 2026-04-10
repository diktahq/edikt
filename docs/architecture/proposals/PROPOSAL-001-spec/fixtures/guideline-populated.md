# Guideline: Function length and extraction

## Summary

When a function grows beyond ~50 lines OR contains more than two distinct responsibilities, extract helper functions to separate concerns and keep each unit testable in isolation.

## Rationale

Long functions combine multiple ideas at the same scope, making them hard to read, hard to test, and hard to modify without regressions. Short, single-purpose functions are mechanically easier to reason about and form natural test boundaries.

## Examples

**Before (too long, too many responsibilities):**

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    // 80 lines of: parse request, validate, compute totals, persist, send email, return response
}
```

**After (split):**

```go
func (h *Handler) CreateOrder(w http.ResponseWriter, r *http.Request) {
    req, err := parseOrderRequest(r)
    if err != nil { ... }

    if err := validateOrder(req); err != nil { ... }

    total := computeOrderTotal(req.Items)
    order := buildOrder(req, total)

    if err := h.repo.Save(order); err != nil { ... }

    h.mailer.SendOrderConfirmation(order)

    writeOrderResponse(w, order)
}
```

## When NOT to apply

Don't extract helpers when the resulting function would only be called from one place AND the extraction makes the main function harder to follow. Helper explosion is worse than a moderately long function.

[edikt:directives:start]: #
source_hash: 9876543210abcdef9876543210abcdef9876543210abcdef9876543210abcdef
directives_hash: 1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef
compiler_version: "0.3.0"
paths:
  - "**/*.go"
  - "**/*.ts"
  - "**/*.py"
scope:
  - implementation
  - review
directives:
  - "Extract helper functions when a function exceeds ~50 lines or handles more than two responsibilities (ref: guidelines/function-length.md)"
  - "Prefer splitting parse/validate/compute/persist/notify/respond into separate functions (ref: guidelines/function-length.md)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

<!--
This fixture demonstrates the NEW /edikt:guideline:compile command producing
a block identical in structure to ADRs and invariants. In v0.2.x, guidelines
did not have a compile command. v0.3.0 adds parity.
-->
