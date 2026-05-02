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
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
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
}

// Criterion is one acceptance criterion under a phase.
type Criterion struct {
	ID        string `yaml:"id"`
	Statement string `yaml:"statement"`
	Verify    string `yaml:"verify,omitempty"`
}

// Valid classification values per Phase 12 spec.
const (
	ClassTestable      = "testable"
	ClassOperational   = "operational"
	ClassInformational = "informational"
)

// LoadCriteria strictly decodes the criteria sidecar at path and validates
// the v1 shape. Returns an error wrapped with the path for caller context.
func LoadCriteria(path string) (*CriteriaFile, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	dec := yaml.NewDecoder(f)
	dec.KnownFields(true)

	var c CriteriaFile
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if err := c.Validate(); err != nil {
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
		case ClassTestable, ClassOperational, ClassInformational:
		case "":
			return fmt.Errorf("phases[%s].classification: required", p.ID)
		default:
			return fmt.Errorf("phases[%s].classification: %q (must be testable|operational|informational)",
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
			if p.Classification == ClassTestable && cr.Verify == "" {
				return fmt.Errorf("phases[%s].criteria[%s]: testable phases require verify:",
					p.ID, cr.ID)
			}
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
