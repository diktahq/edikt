package service

import "errors"

// ErrNoSession is returned when a service method is called without an
// authenticated tenant+user on the context. Every service entry point
// calls scope() first and surfaces this.
var ErrNoSession = errors.New("service: no authenticated session in context")

// ErrNotFound is the service-layer translation of repository.ErrNotFound.
// Handlers map it to HTTP 404.
var ErrNotFound = errors.New("service: not found")

// ErrForbidden is returned when the authenticated user is not allowed
// to perform the action on the subject. Handlers map it to HTTP 403 —
// though some endpoints prefer to return 404 to avoid leaking existence.
var ErrForbidden = errors.New("service: forbidden")
