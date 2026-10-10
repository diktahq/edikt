package sidecar

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// planCriteriaSchema is a BYTE-IDENTICAL MIRROR of
// templates/schemas/plan-criteria.v1.schema.json, held here for the same
// reason the two gov-sidecar mirrors are: `go:embed` patterns cannot contain
// `..`, the authoritative schema lives outside this Go module, and a consumer
// project installs the binary with no `templates/` directory to resolve it
// from at runtime.
//
// It is a mirror, not a second definition. Nothing here re-expresses the
// schema's rules in Go — that is the route where drift is silent and
// unfixable by inspection. These are the authoritative bytes, and two gates
// hold them: TestPlanCriteriaSchemaMirrorIsByteIdentical in this package, and
// test/test-schema-copies.sh over all three on-disk copies.
//
//go:embed schema/plan-criteria.v1.schema.json
var planCriteriaSchema []byte

var (
	compiledPlanCriteriaSchema *jsonschema.Schema
	compilePlanCriteriaOnce    sync.Once
	compilePlanCriteriaErr     error
)

func planCriteriaValidator() (*jsonschema.Schema, error) {
	compilePlanCriteriaOnce.Do(func() {
		var doc any
		if err := json.Unmarshal(planCriteriaSchema, &doc); err != nil {
			compilePlanCriteriaErr = fmt.Errorf("embedded plan-criteria schema is not valid JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("plan-criteria.v1.schema.json", doc); err != nil {
			compilePlanCriteriaErr = fmt.Errorf("add plan-criteria schema: %w", err)
			return
		}
		compiledPlanCriteriaSchema, compilePlanCriteriaErr = c.Compile("plan-criteria.v1.schema.json")
	})
	return compiledPlanCriteriaSchema, compilePlanCriteriaErr
}

var (
	compiledPlanCriteriaRequirementsSchema *jsonschema.Schema
	compilePlanCriteriaRequirementsOnce    sync.Once
	compilePlanCriteriaRequirementsErr     error
)

// planCriteriaRequirementsFragment is the JSON-pointer location, WITHIN the
// embedded plan-criteria.v1 schema, of one criterion's `requirements:`
// property. Compiling this fragment rather than the whole document is what
// keeps ValidateRawPlanCriteriaRequirements scoped to the one property this
// phase adds — see that function's doc comment for why scope matters here.
const planCriteriaRequirementsFragment = "plan-criteria.v1.schema.json" +
	"#/properties/phases/items/properties/criteria/items/properties/requirements"

func planCriteriaRequirementsValidator() (*jsonschema.Schema, error) {
	compilePlanCriteriaRequirementsOnce.Do(func() {
		var doc any
		if err := json.Unmarshal(planCriteriaSchema, &doc); err != nil {
			compilePlanCriteriaRequirementsErr = fmt.Errorf("embedded plan-criteria schema is not valid JSON: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		if err := c.AddResource("plan-criteria.v1.schema.json", doc); err != nil {
			compilePlanCriteriaRequirementsErr = fmt.Errorf("add plan-criteria schema: %w", err)
			return
		}
		compiledPlanCriteriaRequirementsSchema, compilePlanCriteriaRequirementsErr =
			c.Compile(planCriteriaRequirementsFragment)
	})
	return compiledPlanCriteriaRequirementsSchema, compilePlanCriteriaRequirementsErr
}

// ValidateRawPlanCriteria checks an on-disk PLAN-<id>-criteria.yaml document
// against the authoritative plan-criteria.v1 schema.
//
// It takes the RAW BYTES, never a decoded struct, for the reason recorded on
// ValidateRawAgainstSchema: the schema describes the on-disk contract, and a
// struct carries runtime-only fields that `additionalProperties: false`
// rejects. Marshalling the struct would measure one document and gate on
// another.
func ValidateRawPlanCriteria(raw []byte) error {
	sch, err := planCriteriaValidator()
	if err != nil {
		// INV-011: a validator that could not run must not report a pass.  // edikt-guard:allow
		return fmt.Errorf("plan-criteria schema validation unavailable: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode for plan-criteria schema validation: %s", formatYAMLError(err))
	}
	j, err := json.Marshal(doc)
	if err != nil {
		return fmt.Errorf("normalise for plan-criteria schema validation: %w", err)
	}
	var norm any
	if err := json.Unmarshal(j, &norm); err != nil {
		return fmt.Errorf("normalise for plan-criteria schema validation: %w", err)
	}
	if err := sch.Validate(norm); err != nil {
		return fmt.Errorf("plan-criteria v1 schema: %s", formatSchemaErrors(err))
	}
	return nil
}

// ValidateRawPlanCriteriaRequirements checks ONLY each criterion's
// `requirements:` property against the plan-criteria.v1 schema's fragment for
// that property (an array of unique, non-empty string ids) — never the rest
// of the document.
//
// This is the production caller LoadCriteria uses, and it is deliberately
// narrower than ValidateRawPlanCriteria. Running the FULL document validator
// at load (D390, 2026-09-08) enforces every OTHER constraint the schema
// states — e.g. `timeout_s: {minimum: 1}` — against files nothing ever
// checked against it: a real, pre-existing criteria file carrying
// `timeout_s: 0` loads today and would be refused on upgrade for a reason
// that has nothing to do with `requirements:`. SSP-004's letter (the field is
// optional) would still hold, and its purpose — that this coverage
// improvement must not become a breaking change for a project that has not
// backfilled — would not.
//
// Scoping to the one property this phase adds closes AC-1.5 (the third
// mirrored schema copy's `uniqueItems` constraint gets a real production
// caller, not only its own test) without that collateral breakage. It still
// compiles from the SAME embedded schema bytes ValidateRawPlanCriteria uses —
// via a JSON-pointer fragment into planCriteriaSchema, not a hand-rolled Go
// re-expression of the rule — so a change to the authoritative
// `requirements:` sub-schema is picked up here automatically rather than
// needing a second edit.
//
// D391 (2026-09-09): the FIRST version of this scoping still ran
// json.Marshal over the WHOLE decoded document to normalise it before
// navigating to each criterion's `requirements:` — the same shape of
// over-reach AC-1.5's own doc comment warns about for schema breadth,
// one level down in code. gopkg.in/yaml.v3 decodes a mapping with a
// non-string key (an unquoted `true:`, `1.5:`, or similar — legitimate YAML,
// and a spelling this schema's freeform phase_metadata: does not forbid)
// into map[interface{}]interface{}, which encoding/json cannot marshal at
// all. That failure aborted THIS validator — and so LoadCriteria — for any
// document carrying such a key ANYWHERE, including inside phase_metadata:,
// which this check never reads. Traversal below walks the YAML-native
// structure directly and normalises (json.Marshal + Unmarshal, to convert
// YAML's native int/uint64 into the float64 shape jsonschema expects) only
// the `requirements:` leaf value itself — always a list of scalar ids by
// construction, so it can never carry a non-string-keyed submap regardless
// of what else the document contains.
func ValidateRawPlanCriteriaRequirements(raw []byte) error {
	sch, err := planCriteriaRequirementsValidator()
	if err != nil {
		// INV-011: a validator that could not run must not report a pass.  // edikt-guard:allow
		return fmt.Errorf("plan-criteria requirements schema validation unavailable: %w", err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return fmt.Errorf("decode for plan-criteria requirements validation: %s", formatYAMLError(err))
	}
	top, _ := doc.(map[string]any)
	phases, _ := top["phases"].([]any)
	for pi, rawPhase := range phases {
		phase, ok := rawPhase.(map[string]any)
		if !ok {
			continue
		}
		criteria, _ := phase["criteria"].([]any)
		for ci, rawCrit := range criteria {
			crit, ok := rawCrit.(map[string]any)
			if !ok {
				continue
			}
			reqs, present := crit["requirements"]
			if !present {
				continue
			}
			norm, err := normalizePlanCriteriaValue(reqs)
			if err != nil {
				return fmt.Errorf("phases[%d] (id %v) criteria[%d] (id %v): requirements: %w",
					pi, phase["id"], ci, crit["id"], err)
			}
			if err := sch.Validate(norm); err != nil {
				return fmt.Errorf("phases[%d] (id %v) criteria[%d] (id %v): requirements: %s",
					pi, phase["id"], ci, crit["id"], formatSchemaErrors(err))
			}
		}
	}
	return nil
}

// normalizePlanCriteriaValue round-trips a YAML-decoded value through JSON so
// the jsonschema library sees the same shapes a real JSON document would
// (float64 for numbers, etc.) rather than YAML's native Go types.
//
// Deliberately called on ONE leaf value, never the whole document — see
// ValidateRawPlanCriteriaRequirements's doc comment (D391).
func normalizePlanCriteriaValue(v any) (any, error) {
	j, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("normalise for plan-criteria requirements validation: %w", err)
	}
	var norm any
	if err := json.Unmarshal(j, &norm); err != nil {
		return nil, fmt.Errorf("normalise for plan-criteria requirements validation: %w", err)
	}
	return norm, nil
}
