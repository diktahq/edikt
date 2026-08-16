package sidecar

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"
)

// govSidecarSchemaV2 mirrors templates/schemas/gov-sidecar.v2.schema.json for the
// same reason the v1 mirror exists: go:embed cannot reach outside the module.
// TestSchemaMirrorV2IsByteIdentical fails the build the moment they diverge.
//
//go:embed schema/gov-sidecar.v2.schema.json
var govSidecarSchemaV2 []byte

var (
	compiledSchemaV2 *jsonschema.Schema
	compileOnceV2    sync.Once
	compileErrV2     error
)

func schemaValidatorV2() (*jsonschema.Schema, error) {
	compileOnceV2.Do(func() {
		var doc any
		if err := json.Unmarshal(govSidecarSchemaV2, &doc); err != nil {
			compileErrV2 = fmt.Errorf("embedded gov-sidecar v2 schema is not valid JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("gov-sidecar.v2.schema.json", doc); err != nil {
			compileErrV2 = fmt.Errorf("add gov-sidecar v2 schema: %w", err)
			return
		}
		compiledSchemaV2, compileErrV2 = c.Compile("gov-sidecar.v2.schema.json")
	})
	return compiledSchemaV2, compileErrV2
}

// ValidateRawAgainstSchemaV2 checks an on-disk sidecar document against the
// authoritative v2 schema.
//
// WHY THIS IS THE REJECTION POINT AND Load() IS NOT. `Load` is deliberately
// permissive: D22/D45 ruled that rejection belongs at a gate, and v12_test.go
// pins that with an explicit "expected nil error (rejection is at schema or
// compile layer, not Go-loader)". This function is that gate. A v2 requirement
// to reject the v1 singular `source_excerpt` is therefore satisfied here, by
// running the validator against the document — not by loosening Load, and not
// by grepping source for a field name (which would prove only that a string
// appears in a file, never that the code rejects anything).
func ValidateRawAgainstSchemaV2(raw []byte) error {
	sch, err := schemaValidatorV2()
	if err != nil {
		// INV-011: a validator that could not run must not report a pass.  // edikt-guard:allow
		return fmt.Errorf("v2 schema validation unavailable: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode for v2 schema validation: %w", err)
	}
	j, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("normalise for v2 schema validation: %w", err)
	}
	var norm any
	if err := json.Unmarshal(j, &norm); err != nil {
		return fmt.Errorf("normalise for v2 schema validation: %w", err)
	}
	if err := sch.Validate(norm); err != nil {
		return fmt.Errorf("gov-sidecar v2 schema: %w", err)
	}
	return nil
}
