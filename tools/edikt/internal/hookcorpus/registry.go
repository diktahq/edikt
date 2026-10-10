package hookcorpus

// registry.go — this package's own reader for templates/hooks/registry.yaml.
//
// WHY A SECOND READER EXISTS. internal/hook already parses this file. Importing
// it here is banned (SSP-002, see doc.go): the ban keeps hookcorpus out of
// `hook run`'s call graph, and an import edge would put it back regardless of
// which symbols were used.
//
// The duplication is bounded on purpose. What the two readers share is the
// FILE and its schema, not a struct — and the thing that keeps them honest is
// that they read the same bytes and are each asserted against them
// (TestRegistryReaderAgreesWithFile here, TestRegistryParity there). A shared
// Go type would look like tighter coupling and would buy nothing this pair of
// tests does not already buy.
//
// This reader also carries a fact internal/hook's does not need: which
// implementation a hook's name resolves to RIGHT NOW. That is the whole basis
// of the `oracle` field, so it lives with the recorder rather than with the
// dispatcher.

import (
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// Registry row status values, mirroring templates/schemas/hook-registry.v1.schema.json.
const (
	StatusPending  = "pending"
	StatusMigrated = "migrated"
)

// Registry row class values.
const (
	ClassGate      = "gate"
	ClassMultiCall = "multi-call"
	ClassInjector  = "injector"
	ClassLibrary   = "library"
)

// Oracle values, mirroring hook-corpus-recording.v1.schema.json's enum.
const (
	OracleBash = "bash"
	OracleGo   = "go"
	// OracleHostContract labels a recording captured from the CURRENT Go
	// implementation because the bash oracle encoded a reply the host does not
	// read — PostCompact's additionalContext, rejected by the 2.1.257 reply
	// union. Such a recording must name its contract and the date it was read
	// (Recording.Contract / ContractRecordedAt); without them it would be
	// oracle=go under another name, an implementation graded against itself.
	OracleHostContract = "host-contract"
)

// RegistryRow is one hook's declaration.
//
// Event is a pointer because `event: null` is the schema's way of saying "this
// row is a sourced library, not a hook". Decoding null and absent to the same
// "" would erase the one fact the field carries.
type RegistryRow struct {
	Name   string  `yaml:"name"`
	Event  *string `yaml:"event"`
	File   string  `yaml:"file"`
	Class  string  `yaml:"class"`
	Status string  `yaml:"status"`
	// Python3BaselineCount is read but unused here; declared so a strict
	// decode does not reject the file over a field this package ignores.
	Python3BaselineCount int `yaml:"python3_baseline_count"`
}

// IsLibrary reports whether this row is a sourced helper rather than a hook.
//
// A library row has no payload × config × environment × ordering surface to
// characterize (ADR-063 Decision §4), so it has no matrix, no recording and no
// migration gate. Asking for its coverage is a category error, not a gap.
func (r RegistryRow) IsLibrary() bool { return r.Class == ClassLibrary || r.Event == nil }

// RegistryFile is the parsed registry.
type RegistryFile struct {
	SchemaVersion int           `yaml:"schema_version"`
	Hooks         []RegistryRow `yaml:"hooks"`
	// Retired is accepted and ignored here: the recorder iterates hooks that
	// run, and a retired hook has no handler to record (ADR-083).  edikt-guard:allow
	Retired []map[string]any `yaml:"retired,omitempty"`

	// Path records where this was read from, so a report about the registry
	// can name its subject instead of asserting about an unnamed file.
	Path string `yaml:"-"`
}

// LoadRegistry reads and validates a registry file.
//
// A registry that parses to zero rows is an error, not an empty success:
// every caller here uses it as a denominator, and a denominator of zero would
// let "no hooks are uncovered" be true because no hooks were looked at
// (INV-013).
func LoadRegistry(path string) (*RegistryFile, error) {
	raw, err := os.ReadFile(path) //nolint:gosec // operator-supplied dev tooling path
	if err != nil {
		return nil, fmt.Errorf("hook registry: %w", err)
	}
	var f RegistryFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return nil, fmt.Errorf("hook registry %s: %w", path, err)
	}
	f.Path = path
	if len(f.Hooks) == 0 {
		return nil, fmt.Errorf("hook registry %s declares zero hooks — NOTHING WAS READ, "+
			"which is not an empty registry", path)
	}
	for i, r := range f.Hooks {
		switch {
		case r.Name == "":
			return nil, fmt.Errorf("hook registry %s: row %d has no name", path, i)
		case r.File == "":
			return nil, fmt.Errorf("hook registry %s: row %q has no file", path, r.Name)
		case r.Class == "":
			return nil, fmt.Errorf("hook registry %s: row %q has no class", path, r.Name)
		case r.Status != StatusPending && r.Status != StatusMigrated:
			return nil, fmt.Errorf("hook registry %s: row %q has status %q, want %q or %q",
				path, r.Name, r.Status, StatusPending, StatusMigrated)
		}
	}
	return &f, nil
}

// Row resolves one hook name.
//
// An unknown name is an error naming every name that IS present. A recorder
// asked about a hook that does not exist has measured nothing, and a typo'd
// name that returned "no scenarios, nothing to do" would be the exact shape
// INV-013 forbids.
func (f *RegistryFile) Row(name string) (RegistryRow, error) {
	for _, r := range f.Hooks {
		if r.Name == name {
			return r, nil
		}
	}
	return RegistryRow{}, fmt.Errorf("hook %q is not in %s (known: %v)", name, f.Path, f.Names())
}

// Names lists every declared hook name, sorted.
func (f *RegistryFile) Names() []string {
	out := make([]string, 0, len(f.Hooks))
	for _, r := range f.Hooks {
		out = append(out, r.Name)
	}
	sort.Strings(out)
	return out
}

// OracleFor reports which implementation the given row resolves to today.
//
// This is derived from the FILE, never from a caller's flag, and that is the
// single most important line in the package. SPEC-014's gate compares a Go
// replacement against a bash recording; if the operator could assert "this is
// a bash recording" while the Go handler was already registered, the corpus
// would compare Go against Go and report green for a migration nobody checked.
//
// registry.yaml's own comment states the rule this depends on: "STATUS IS NOT A
// PLAN, IT IS A FACT" — it flips to migrated in the same change that lands the
// handler, and internal/hook's parity gate fails the build if the file and the
// registered set disagree. So the file is a trustworthy answer to "what runs
// when this name fires", without importing the package that runs it.
func (f *RegistryFile) OracleFor(row RegistryRow) string {
	if row.Status == StatusMigrated {
		return OracleGo
	}
	return OracleBash
}
