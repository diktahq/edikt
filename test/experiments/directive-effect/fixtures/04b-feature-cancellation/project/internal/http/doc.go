// Package http exposes the HTTP transport. The router wires middleware
// and handlers; response helpers live in internal/web so handler
// packages can depend on them without an import cycle.
package http
