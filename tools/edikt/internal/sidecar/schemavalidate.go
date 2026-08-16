package sidecar

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// govSidecarSchema is a BYTE-IDENTICAL MIRROR of
// templates/schemas/gov-sidecar.v1.schema.json.
//
// WHY A MIRROR AND NOT A SECOND DEFINITION. `go:embed` patterns cannot contain
// `..`, and the authoritative schema lives outside this Go module. The options
// were: copy the schema's CONTENT into Go (route 2 — rejected, it leaves the
// schema unreachable while duplicating its meaning), resolve the file from the
// installed payload at runtime (breaks `go test` and any dev tree), or mirror
// the FILE verbatim and gate the copy.
//
// This is the third. The distinction from route 2 is the whole point: route 2
// would have re-expressed the schema's rules as Go code, where drift is silent
// and unfixable by inspection. This copies the bytes, and
// TestSchemaMirrorIsByteIdentical fails the build the moment they differ. One
// authoritative artifact, one gated mirror — not two definitions.
//
//go:embed schema/gov-sidecar.v1.schema.json
var govSidecarSchema []byte

var (
	compiledSchema *jsonschema.Schema
	compileOnce    sync.Once
	compileErr     error
)

// schemaValidator compiles the embedded schema once. Measured at ~0 ms to
// compile and 0.30 ms per sidecar to validate (63 sidecars in 18.8 ms), which
// is what settled route 1 over route 2 — under 4% of Phase B's no-op SLO.
func schemaValidator() (*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		var doc any
		if err := json.Unmarshal(govSidecarSchema, &doc); err != nil {
			compileErr = fmt.Errorf("embedded gov-sidecar schema is not valid JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("gov-sidecar.v1.schema.json", doc); err != nil {
			compileErr = fmt.Errorf("add gov-sidecar schema: %w", err)
			return
		}
		compiledSchema, compileErr = c.Compile("gov-sidecar.v1.schema.json")
	})
	return compiledSchema, compileErr
}

// ValidateRawAgainstSchema checks the ON-DISK YAML document against the
// AUTHORITATIVE JSON schema.
//
// It takes the raw bytes, NOT the decoded Go struct. That distinction cost a
// debugging cycle and is worth stating: the schema describes the on-disk
// contract, and the struct carries runtime-only fields (SourcePath) that have
// no YAML or JSON tag excluding them. Marshalling the struct therefore produced
// a document the schema rejected under `additionalProperties: false` — every
// sidecar failed, for a reason that had nothing to do with the sidecar.
//
// The corpus conformance measurement (0 of 63 failing) validated raw YAML.
// Validating the struct would have been measuring one document and gating on
// another.
//
// D22: before this, no JSON-schema library was a dependency of this module, so
// the authoritative schema was applied only by bash tests over fixtures and had
// NEVER seen a freshly written sidecar.
func ValidateRawAgainstSchema(raw []byte) error {
	sch, err := schemaValidator()
	if err != nil {
		// INV-011: a validator that could not run must not report a pass.  // edikt-guard:allow
		return fmt.Errorf("schema validation unavailable: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode for schema validation: %w", err)
	}
	// yaml.v3 yields map[string]any for mappings, which the validator accepts;
	// round-tripping through JSON normalises numeric kinds.
	j, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("normalise for schema validation: %w", err)
	}
	var norm any
	if err := json.Unmarshal(j, &norm); err != nil {
		return fmt.Errorf("normalise for schema validation: %w", err)
	}
	if err := sch.Validate(norm); err != nil {
		return fmt.Errorf("schema: %s", compactSchemaError(err))
	}
	return nil
}

// compactSchemaError flattens a jsonschema error to ONE actionable line.
//
// Load's error contract is single-line and bounded — asserted by
// TestAdversarialCorpus, which checks every Load failure is <500 chars with no
// newline, because these strings are what a user sees when a sidecar will not
// load. The library's default renders a multi-line tree headed by a file:// URL
// of the schema, which is noise to that reader: they need the failing property
// and why, not where the schema lives.
func compactSchemaError(err error) string {
	// Use the library's own rendering and flatten it. Walking ErrorKind
	// internals panicked on a nil localiser; the public string is stable and
	// this needs no library internals.
	raw := err.Error()
	// Drop the leading "jsonschema validation failed with 'file://...#'" line:
	// it names where the schema lives, which is noise to someone whose sidecar
	// will not load. Keep the causes.
	lines := strings.Split(raw, "\n")
	var keep []string
	for _, ln := range lines {
		ln = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(ln), "- "))
		if ln == "" || strings.HasPrefix(ln, "jsonschema validation failed") {
			continue
		}
		keep = append(keep, ln)
	}
	msg := strings.Join(keep, "; ")
	if msg == "" {
		msg = "validation failed"
	}
	if len(msg) > 140 {
		msg = msg[:137] + "..."
	}
	return msg
}
