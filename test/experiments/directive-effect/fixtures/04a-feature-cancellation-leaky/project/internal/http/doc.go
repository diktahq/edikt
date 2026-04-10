// Package http exposes the HTTP transport for the orders service. The
// router wires middleware and handlers; response helpers live in
// internal/web so handler packages can depend on them without an import
// cycle through this package.
package http
