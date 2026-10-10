// Package govrun holds the two-phase compile (twophase.go) and the config
// loader every dispatcher shares. The legacy in-body compile (Run) and its
// dead fast path were deleted under ADR-082 (c): steady-state components
// are migration-blind, and a compile that read sentinel blocks was not.
package govrun

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// ediktConfig is the minimal shape of .edikt/config.yaml that govrun needs.
// Base is the project-wide doc base (default "docs"). Specs/Prds/Plans paths
// derive from Base when not explicitly set. Decisions/Invariants/Guidelines
// keep their historical hardcoded "docs/..." defaults for compat — fixing
// those to derive from Base is a separate behavioral change.
type ediktConfig struct {
	Base  string `yaml:"base"`
	Paths struct {
		Decisions   string `yaml:"decisions"`
		Invariants  string `yaml:"invariants"`
		Guidelines  string `yaml:"guidelines"`
		Specs       string `yaml:"specs"`
		Prds        string `yaml:"prds"`
		Plans       string `yaml:"plans"`
		Discovery   string `yaml:"discovery"`
		Brainstorms string `yaml:"brainstorms"`
	} `yaml:"paths"`
}

// CompilerVersion is the version string embedded in compiled output.
// It mirrors the binary version and is set by the cmd layer before the
// two-phase compile runs.
var CompilerVersion = "0.1.0"

// Config is the exported view of ediktConfig for callers outside govrun
// (cmd/nextid.go etc.). Mirrors the YAML shape exactly.
type Config = ediktConfig

// LoadConfig is the exported entry point for reading .edikt/config.yaml.
// Same behavior as the package-internal loadConfig — returns sensible
// defaults when the file is absent.
func LoadConfig(root string) (Config, error) {
	return loadConfig(root)
}

// GovernanceDirs returns the configured paths for ADRs, invariants, and
// guidelines from .edikt/config.yaml under root. Falls back to defaults
// (docs/architecture/{decisions,invariants}, docs/guidelines) when the
// config is absent or unreadable.
//
// Use this from any dispatcher that needs to know "where does this project
// keep its governance" — never hardcode the defaults at a call site,
// because that silently breaks projects with customized paths.*.
func GovernanceDirs(root string) []string {
	cfg, _ := loadConfig(root)
	return []string{cfg.Paths.Decisions, cfg.Paths.Invariants, cfg.Paths.Guidelines}
}

func loadConfig(root string) (ediktConfig, error) {
	var cfg ediktConfig
	cfg.Base = "docs"
	cfg.Paths.Decisions = "docs/architecture/decisions"
	cfg.Paths.Invariants = "docs/architecture/invariants"
	cfg.Paths.Guidelines = "docs/guidelines"

	data, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			applyBaseDefaults(&cfg)
			return cfg, nil
		}
		return cfg, err
	}
	var raw struct {
		Base  string `yaml:"base"`
		Paths struct {
			Decisions   string `yaml:"decisions"`
			Invariants  string `yaml:"invariants"`
			Guidelines  string `yaml:"guidelines"`
			Specs       string `yaml:"specs"`
			Prds        string `yaml:"prds"`
			Plans       string `yaml:"plans"`
			Discovery   string `yaml:"discovery"`
			Brainstorms string `yaml:"brainstorms"`
		} `yaml:"paths"`
	}
	// SPEC-009 Plan A AC-1.2: loads .edikt/config.yaml paths subset (base + per-section path map).  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — config is user-extensible.
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return cfg, err
	}
	if raw.Base != "" {
		cfg.Base = raw.Base
	}
	if raw.Paths.Decisions != "" {
		cfg.Paths.Decisions = raw.Paths.Decisions
	}
	if raw.Paths.Invariants != "" {
		cfg.Paths.Invariants = raw.Paths.Invariants
	}
	if raw.Paths.Guidelines != "" {
		cfg.Paths.Guidelines = raw.Paths.Guidelines
	}
	if raw.Paths.Specs != "" {
		cfg.Paths.Specs = raw.Paths.Specs
	}
	if raw.Paths.Prds != "" {
		cfg.Paths.Prds = raw.Paths.Prds
	}
	if raw.Paths.Plans != "" {
		cfg.Paths.Plans = raw.Paths.Plans
	}
	if raw.Paths.Discovery != "" {
		cfg.Paths.Discovery = raw.Paths.Discovery
	}
	if raw.Paths.Brainstorms != "" {
		cfg.Paths.Brainstorms = raw.Paths.Brainstorms
	}
	applyBaseDefaults(&cfg)
	return cfg, nil
}

// applyBaseDefaults fills in Specs/Prds/Plans/Discovery/Brainstorms from Base
// when not explicitly set. Decisions/Invariants/Guidelines are NOT touched
// here — they keep their historical hardcoded "docs/..." defaults set above.
func applyBaseDefaults(cfg *ediktConfig) {
	if cfg.Paths.Specs == "" {
		cfg.Paths.Specs = cfg.Base + "/product/specs"
	}
	if cfg.Paths.Prds == "" {
		cfg.Paths.Prds = cfg.Base + "/product/prds"
	}
	if cfg.Paths.Plans == "" {
		cfg.Paths.Plans = cfg.Base + "/plans"
	}
	if cfg.Paths.Discovery == "" {
		cfg.Paths.Discovery = cfg.Base + "/product/discovery"
	}
	if cfg.Paths.Brainstorms == "" {
		cfg.Paths.Brainstorms = cfg.Base + "/brainstorms"
	}
}
