package sidecar

import (
	"bytes"
	"strings"
)

// The IDE schema hint a sidecar template opens with. It is a YAML comment, so
// any writer that round-trips the document through a data model — Marshal
// here, PyYAML's safe_dump in a command body — drops it and nothing fails:
// the file still parses, still validates, and the editor silently loses
// autocomplete. Every writer of a sidecar goes through WithSchemaHeader so
// the property belongs to the writers rather than to whichever library each
// one happens to use.
const SchemaHeaderPrefix = "# yaml-language-server: $schema="

// SchemaHeaderOf returns the header line a sidecar's bytes open with, or ""
// when they do not open with one.
func SchemaHeaderOf(b []byte) string {
	first, _, _ := bytes.Cut(b, []byte("\n"))
	line := strings.TrimRight(string(first), "\r")
	if strings.HasPrefix(line, SchemaHeaderPrefix) {
		return line
	}
	return ""
}

// SchemaHeaderFinding reports why a sidecar's header is not acceptable —
// absent, or present with no path after `$schema=`. "" means acceptable.
func SchemaHeaderFinding(b []byte) string {
	h := SchemaHeaderOf(b)
	if h == "" {
		return "missing `" + SchemaHeaderPrefix + "<path>` header on line 1 — the sidecar lost its IDE schema hint (a writer that round-trips through plain data drops it)"
	}
	if strings.TrimSpace(strings.TrimPrefix(h, SchemaHeaderPrefix)) == "" {
		return "empty `" + SchemaHeaderPrefix + "` header on line 1 — the schema path was never rendered"
	}
	return ""
}

// WithSchemaHeader prepends header to out unless out already opens with one.
// A no-op when the file had no header to keep.
func WithSchemaHeader(header string, out []byte) []byte {
	if header == "" || SchemaHeaderOf(out) != "" {
		return out
	}
	return append([]byte(header+"\n"), out...)
}
