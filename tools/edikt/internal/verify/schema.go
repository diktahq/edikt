// Package verify implements the plan-criteria verification runner described
// in Phase 12 of PLAN-sidecar-architecture. It loads a plan's criteria
// sidecar (PLAN-<id>-criteria.yaml), executes the per-criterion `verify:`
// shell commands under bash with a 30s timeout, and emits a JSON+text
// report under .edikt/state/verify/.
//
// The runner deliberately reads the criteria sidecar with a strict YAML
// decoder (KnownFields=true) so any drift from the v1 shape produces a
// hard error instead of silently dropping fields. The shape mirrors the
// in-tree PLAN-sidecar-architecture-criteria.yaml format, which is the
// canonical reference for v0.6.0.
package verify

import (
	"bytes"
	"fmt"
	"os"

	"gopkg.in/yaml.v3"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// SchemaVersion is the on-disk shape this package understands.
const SchemaVersion = 1

// CriteriaFile is the top-level shape of PLAN-<id>-criteria.yaml.
type CriteriaFile struct {
	Plan          string  `yaml:"plan"`
	TargetRelease string  `yaml:"target_release"`
	Created       string  `yaml:"created"`
	SchemaVersion int     `yaml:"schema_version"`
	Phases        []Phase `yaml:"phases"`

	// Optional sections that some criteria files include. The runner does
	// not act on them but accepts them so KnownFields-strict decoding does
	// not reject the canonical format.
	Dependencies map[string]Dependency `yaml:"dependencies,omitempty"`
	Risks        []Risk                `yaml:"risks,omitempty"`
}

// Dependency captures the DAG edges between phases.
type Dependency struct {
	Blocks []string `yaml:"blocks"`
}

// Risk is one entry in the risk register.
type Risk struct {
	ID              string `yaml:"id"`
	Severity        string `yaml:"severity"`
	MitigationPhase string `yaml:"mitigation_phase"`
}

// Phase is one phase in the plan.
type Phase struct {
	ID                string      `yaml:"id"`
	Name              string      `yaml:"name"`
	Classification    string      `yaml:"classification"`
	CompletionPromise string      `yaml:"completion_promise,omitempty"`
	Criteria          []Criterion `yaml:"criteria"`

	// PhaseMetadata is a freeform, runner-ignored object. The decoder is
	// KnownFields(true), so before this existed a plan carrying any concept
	// the schema had not anticipated could not be loaded at all — the author's
	// only options were to delete the text or mislabel the phase.
	//
	// Nothing in the runner reads it. It exists so the NEXT unanticipated
	// concept has somewhere to go that does not force that choice, and so the
	// text survives in the file where its author put it.
	PhaseMetadata map[string]any `yaml:"phase_metadata,omitempty"`
}

// Criterion is one acceptance criterion under a phase.
type Criterion struct {
	ID        string `yaml:"id"`
	Statement string `yaml:"statement"`
	Verify    string `yaml:"verify,omitempty"`

	// Requirements lists the spec ids this criterion CLAIMS to satisfy —
	// requirement ids (SR-*) or protection ids (SSP-*) drawn from the spec
	// the plan implements.
	//
	// OPTIONAL here, and required by the coverage gate rather than by this
	// decoder. That direction is the whole design and getting it backwards is
	// a breaking change: this file is read with KnownFields(true), so a
	// REQUIRED field would stop every criteria file in every downstream
	// project that has not backfilled from loading at all on upgrade.
	//
	// A declaration records that a requirement is CLAIMED. It is NOT evidence
	// that the criterion asserts the property — a criterion may name an id and
	// assert nothing about it, and nothing mechanical can tell. The ids
	// themselves are checked (LoadCriteria rejects one the spec does not
	// define, by name); whether the claim holds is a reading.
	Requirements []string `yaml:"requirements,omitempty"`

	// TimeoutS overrides the runner's per-criterion timeout (30 s) when a
	// criterion legitimately runs longer — a release gate that runs the
	// whole suite, an install-and-upgrade path. Optional; absent means the
	// default. Without it the runner recorded such a criterion as `timeout`
	// and doctor counted it as not-passing, so a release plan could never
	// flip its own gate (PLAN-v090-release pre-flight, performance finding 2).
	TimeoutS int `yaml:"timeout_s,omitempty"`

	// SPEC-009 Plan G — cheat-rate scoring metadata. Optional;  // edikt-guard:allow
	// populated only when this criterion carries a behavioral verify
	// the cheat-rate benchmark should score. Plan-level criteria
	// sidecars typically use this for AC entries that gate cheat-rate
	// runs, not as a per-criterion property — adopters can still mark
	// individual ACs behavioral when needed.
	VerifyKind            string `yaml:"verify_kind,omitempty"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	PositiveFixturePath   string `yaml:"positive_fixture_path,omitempty"`
	NegativeFixturePath   string `yaml:"negative_fixture_path,omitempty"`
}

// Valid classification values per Phase 12 spec.
const (
	ClassTestable      = "testable"
	ClassOperational   = "operational"
	ClassInformational = "informational"

	// ClassOwner marks a phase whose completion is a HUMAN DECISION — an
	// owner ruling, a sign-off, a go/no-go. Its criteria are never executed.
	//
	// It is deliberately not the same as informational. Informational means
	// "there is nothing to check here"; owner means "a person must act here
	// and has not necessarily done so". Collapsing them would lose exactly
	// the distinction the classification exists to record, and a reader
	// scanning a report for outstanding human work would find nothing.
	//
	// Added after a consumer project reported that its one owner-gated phase
	// of ten had no expressible shape: the only options were deleting the
	// owner's gate text or mislabelling the phase, so the plan was left
	// unverifiable and `edikt verify` silently covered nine of ten.
	ClassOwner = "owner"
)

// LoadCriteria strictly decodes the criteria sidecar at path and validates
// the v1 shape. Returns an error wrapped with the path for caller context.
func LoadCriteria(path string) (*CriteriaFile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)

	var c CriteriaFile
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	// AC-1.5: gives the third mirrored schema copy
	// (internal/sidecar/schema/plan-criteria.v1.schema.json) a real
	// production caller, not only its own test. The Go decoder's
	// KnownFields(true) mirrors additionalProperties:false, but a
	// constraint like uniqueItems on a criterion's `requirements:` list
	// exists ONLY in the schema document — nothing on the Go side
	// re-expresses it, so before this call a duplicate declared id
	// ([SR-001, SR-001]) decoded cleanly and nothing here ever noticed it
	// was the same claim made twice. Runs on the RAW bytes, never the
	// decoded struct — see ValidateRawPlanCriteriaRequirements's own doc
	// comment for why: the struct carries runtime-only fields
	// additionalProperties:false would reject, so marshalling it would
	// validate a different document than the one on disk.
	//
	// Deliberately the SCOPED validator, not ValidateRawPlanCriteria (the
	// whole document). D390 (2026-09-08): running the full validator here
	// enforces every OTHER schema constraint — e.g. timeout_s's minimum —
	// against files nothing ever checked against it, turning THIS coverage
	// improvement into the exact breaking-upgrade change SSP-004 forbids, by
	// a route SSP-004's text does not name (the optional field stays
	// optional; the harm arrives through an unrelated constraint instead).
	// ValidateRawPlanCriteriaRequirements checks only the `requirements:`
	// property this phase adds.
	if err := sidecar.ValidateRawPlanCriteriaRequirements(raw); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	// AC-1.4: the file's `plan:` field must agree with the plan id its own
	// filename asserts, and a declared requirement id that names nothing is
	// the same defect one level up — a declaration nothing validates. Both
	// are rejected here, at load, rather than carried into a report that
	// would then read as coverage.
	//
	// The filename check runs INSIDE validateDeclaredRequirements now, not
	// as a separate unconditional call — D391 (2026-09-09): calling it here,
	// ahead of the no-op-when-nothing-declared guard, made a `plan:` field
	// that has always disagreed with its filename (harmless, since nothing
	// was ever resolved from it) a NEW load failure for files that declare
	// no requirements at all, which is exactly the breaking-upgrade shape
	// this phase exists not to introduce.
	if err := c.validateDeclaredRequirements(path); err != nil {
		return nil, fmt.Errorf("validate %s: %w", path, err)
	}
	return &c, nil
}

// Validate enforces invariants not captured by structural decode.
func (c *CriteriaFile) Validate() error {
	if c.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version: got %d, want %d", c.SchemaVersion, SchemaVersion)
	}
	if c.Plan == "" {
		return fmt.Errorf("plan: required")
	}
	if len(c.Phases) == 0 {
		return fmt.Errorf("phases: at least one phase required")
	}
	seenPhase := make(map[string]bool, len(c.Phases))
	for i, p := range c.Phases {
		if p.ID == "" {
			return fmt.Errorf("phases[%d].id: required", i)
		}
		if seenPhase[p.ID] {
			return fmt.Errorf("phases[%d].id: duplicate %q", i, p.ID)
		}
		seenPhase[p.ID] = true
		switch p.Classification {
		case ClassTestable, ClassOperational, ClassInformational, ClassOwner:
		case "":
			return fmt.Errorf("phases[%s].classification: required", p.ID)
		default:
			return fmt.Errorf("phases[%s].classification: %q (must be testable|operational|informational|owner)",
				p.ID, p.Classification)
		}
		if len(p.Criteria) == 0 {
			return fmt.Errorf("phases[%s].criteria: at least one required", p.ID)
		}
		seenCrit := make(map[string]bool, len(p.Criteria))
		for j, cr := range p.Criteria {
			if cr.ID == "" {
				return fmt.Errorf("phases[%s].criteria[%d].id: required", p.ID, j)
			}
			if seenCrit[cr.ID] {
				return fmt.Errorf("phases[%s].criteria: duplicate id %q", p.ID, cr.ID)
			}
			seenCrit[cr.ID] = true
			if cr.Statement == "" {
				return fmt.Errorf("phases[%s].criteria[%s].statement: required", p.ID, cr.ID)
			}
			// A testable phase does NOT require a verify on every criterion.
			// Testable means every criterion carries a falsifier and a declared
			// evidence kind (mechanical, observed or judged); only mechanical
			// evidence has a command. Requiring one here made a plan written as
			// commands/sdlc/plan.md instructs unloadable, so authors either
			// invented a command or relabelled the phase. RunCriterion reports
			// a criterion with no verify as unmeasured, which gates the phase.
		}
	}
	return nil
}

// FindPhase returns the phase with id, or nil.
func (c *CriteriaFile) FindPhase(id string) *Phase {
	for i := range c.Phases {
		if c.Phases[i].ID == id {
			return &c.Phases[i]
		}
	}
	return nil
}
