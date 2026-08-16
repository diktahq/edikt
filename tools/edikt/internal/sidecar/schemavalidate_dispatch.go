package sidecar

// schemavalidate_dispatch.go — ONE place decides which schema a sidecar is
// judged against.
//
// Two schemas are live during and after the v1→v2 migration, and two call
// sites validate: the generation boundary (phasea/runner.go, per dispatch) and
// the corpus-wide check (cmd/gov/schema_corpus.go). Before this existed, both
// hardcoded the v1 validator. Flipping the corpus to v2 therefore made 70 of 82
// sidecars "fail the authoritative schema" — every one of them valid, all of
// them judged against a schema that no longer described them.
//
// That is the failure mode GL-002 names: two things that must AGREE must be
// unified. The schema a sidecar is validated against is not a per-call-site
// choice; it is a property of the document, and the document declares it.
//
// Fail-closed on an undeclared or unknown version (INV-011): a validator that  edikt-guard:allow
// cannot determine which contract applies has not validated anything, and must
// say so rather than defaulting to whichever schema happens to be older.

import (
	"fmt"

	"gopkg.in/yaml.v3"
)

// ValidateRawAgainstDeclaredSchema validates a sidecar's on-disk bytes against
// the schema its own `schema_version` names.
//
// This is the entry point every caller should use. The version-specific
// functions remain exported for callers that genuinely need to assert one
// particular contract (fixture tests pinning v1 or v2 behaviour); using them to
// validate real corpus content is how the two versions come to be judged by the
// wrong rules.
func ValidateRawAgainstDeclaredSchema(raw []byte) error {
	var probe struct {
		SchemaVersion *int `yaml:"schema_version"`
	}
	// Loose decode on purpose: this asks ONE question — which contract applies.
	// A strict decode would fail on any unknown field and report a version
	// problem for something that is not one.
	if err := yaml.Unmarshal(raw, &probe); err != nil {
		return fmt.Errorf("decode to determine schema_version: %w", err)
	}
	if probe.SchemaVersion == nil {
		return fmt.Errorf(
			"sidecar declares no schema_version — cannot determine which schema applies "+
				"(expected %d or %d); an undeclared version is unvalidated, not valid",
			SchemaVersion, SchemaVersionV2)
	}
	switch *probe.SchemaVersion {
	case SchemaVersion:
		return ValidateRawAgainstSchema(raw)
	case SchemaVersionV2:
		return ValidateRawAgainstSchemaV2(raw)
	default:
		return fmt.Errorf(
			"sidecar declares schema_version %d, which this binary does not know (expected %d or %d) — "+
				"a newer sidecar than this edikt understands; upgrade edikt rather than downgrading the sidecar",
			*probe.SchemaVersion, SchemaVersion, SchemaVersionV2)
	}
}
