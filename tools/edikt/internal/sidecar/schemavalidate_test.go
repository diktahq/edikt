package sidecar

import (
	"os"
	"path/filepath"
	"testing"
)

// THE GATE THAT MAKES THE MIRROR SAFE.
//
// The embedded schema is a byte-identical copy of the authoritative file. A
// copy with no gate is a second definition; a copy with this gate is a mirror.
// If they ever diverge, the build fails here rather than the product silently
// validating against a stale contract.
func TestSchemaMirrorIsByteIdentical(t *testing.T) {
	authoritative := filepath.Join("..", "..", "..", "..",
		"templates", "schemas", "gov-sidecar.v1.schema.json")
	want, err := os.ReadFile(authoritative)
	if err != nil {
		t.Fatalf("cannot read the authoritative schema at %s: %v", authoritative, err)
	}
	if string(want) != string(govSidecarSchema) {
		t.Fatalf("embedded schema mirror has DRIFTED from %s.\n"+
			"The authoritative file is templates/schemas/gov-sidecar.v1.schema.json.\n"+
			"Re-copy it:\n"+
			"  cp templates/schemas/gov-sidecar.v1.schema.json tools/edikt/internal/sidecar/schema/",
			authoritative)
	}
}

// Sensitivity: the schema must actually reject something. A validator that
// accepts everything passes every "valid input is accepted" test ever written.
func TestValidateAgainstSchema_RejectsUnknownField(t *testing.T) {
	sch, err := schemaValidator()
	if err != nil {
		t.Fatalf("schema unavailable: %v", err)
	}
	bad := map[string]any{
		"schema_version":   1,
		"topic":            "compile",
		"path":             "docs/x.md",
		"source_hash":      "forbidden-by-additionalProperties-false",
		"directives":       []any{},
	}
	if err := sch.Validate(bad); err == nil {
		t.Fatal("schema accepted a sidecar carrying source_hash — additionalProperties:false is not being enforced, so this validator proves nothing")
	}
}

// Isolation: a well-formed sidecar must pass, or the check above could be
// satisfied by a validator that rejects everything.
func TestValidateAgainstSchema_AcceptsMinimalValid(t *testing.T) {
	sch, err := schemaValidator()
	if err != nil {
		t.Fatalf("schema unavailable: %v", err)
	}
	// `signals` is REQUIRED by the schema and was NOT required by the
	// hand-written Validate() — a concrete divergence between the two
	// definitions, found by this test on its first run. The schema is
	// stricter; that is the point of making it the one that runs.
	ok := map[string]any{
		"schema_version": 1,
		"topic":          "compile",
		"path":           "docs/architecture/decisions/ADR-001-x.md",
		"signals":        []any{"compile"},
		"directives":     []any{},
	}
	if err := sch.Validate(ok); err != nil {
		t.Fatalf("schema rejected a minimal valid sidecar: %v", err)
	}
}
