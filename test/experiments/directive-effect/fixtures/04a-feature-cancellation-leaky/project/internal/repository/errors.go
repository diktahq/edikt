package repository

import "errors"

// ErrNoTenantContext is returned by every repository method when the
// request context is missing an authoritative tenant ID. The repository
// refuses to construct any query in that state.
var ErrNoTenantContext = errors.New("repository: no tenant in context")

// ErrNotFound is returned when a query has zero rows.
var ErrNotFound = errors.New("repository: not found")
