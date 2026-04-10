package repository

import "errors"

// ErrNotFound is returned when a query has zero rows.
var ErrNotFound = errors.New("repository: not found")

// ErrEmptyTenant is returned when a repository method is called with
// an empty tenantID argument. The repository refuses to run in that
// state — callers must pass an authoritative tenant ID.
var ErrEmptyTenant = errors.New("repository: empty tenant id")
