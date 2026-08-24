package phasea

// plan_config.go — reads the .edikt/config.yaml inputs plan.go needs: the
// three governed-artifact directories (so the SAME sidecar.Discover call
// twophase.go makes can be reproduced here) and the phasea: budget section
// D3/SAC-004 added.
//
// This package cannot import internal/govrun for its existing loadConfig —
// govrun already imports phasea (to construct phasea.Task for the real
// dispatch loop), so the reverse import would be a cycle. Every package
// that needs .edikt/config.yaml reads it independently today (govrun,
// hookcfg each carry their own small loader); this follows the same,
// already-established precedent rather than inventing a shared config
// package as a Phase 2 side quest.

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Defaults for the phasea: config section (SAC-004). Absent keys resolve to
// these; a PRESENT but non-positive value is refused rather than silently
// replaced by the default (SAC-004's own wording).
const (
	DefaultCeiling            = 50
	DefaultForceCeiling       = 200
	DefaultMaxAttemptsPerItem = 2
)

// Config is the resolved plan.go configuration: the governed-artifact
// directories plus the phasea: budget section.
type Config struct {
	DecisionsDir  string
	InvariantsDir string
	GuidelinesDir string

	Ceiling            int
	ForceCeiling       int
	MaxAttemptsPerItem int
}

// Dirs returns the [decisions, invariants, guidelines] slice in the
// positional order sidecar.Discover requires.
func (c *Config) Dirs() []string {
	return []string{c.DecisionsDir, c.InvariantsDir, c.GuidelinesDir}
}

type rawPlanConfig struct {
	Paths struct {
		Decisions  string `yaml:"decisions"`
		Invariants string `yaml:"invariants"`
		Guidelines string `yaml:"guidelines"`
	} `yaml:"paths"`
	Phasea struct {
		Ceiling            *int `yaml:"ceiling"`
		ForceCeiling       *int `yaml:"force_ceiling"`
		MaxAttemptsPerItem *int `yaml:"max_attempts_per_item"`
	} `yaml:"phasea"`
}

// LoadConfig reads .edikt/config.yaml under root. A missing file is not an
// error — every field defaults, mirroring govrun.loadConfig and
// hookcfg.Load's own "absence is not a decision" posture.
//
// A phasea.ceiling / phasea.force_ceiling / phasea.max_attempts_per_item
// key that IS present but not a positive integer is refused with an
// actionable error naming the key and the rejected value (SAC-004) —
// substituting the default for a bad value would hide a typo behind a
// silently-different budget than the one the operator thought they set.
func LoadConfig(root string) (*Config, error) {
	cfg := &Config{
		DecisionsDir:       "docs/architecture/decisions",
		InvariantsDir:      "docs/architecture/invariants",
		GuidelinesDir:      "docs/guidelines",
		Ceiling:            DefaultCeiling,
		ForceCeiling:       DefaultForceCeiling,
		MaxAttemptsPerItem: DefaultMaxAttemptsPerItem,
	}

	data, err := os.ReadFile(filepath.Join(root, ".edikt", "config.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}

	var raw rawPlanConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse .edikt/config.yaml: %w", err)
	}

	if raw.Paths.Decisions != "" {
		cfg.DecisionsDir = raw.Paths.Decisions
	}
	if raw.Paths.Invariants != "" {
		cfg.InvariantsDir = raw.Paths.Invariants
	}
	if raw.Paths.Guidelines != "" {
		cfg.GuidelinesDir = raw.Paths.Guidelines
	}

	if raw.Phasea.Ceiling != nil {
		if *raw.Phasea.Ceiling <= 0 {
			return nil, fmt.Errorf(
				"phasea.ceiling: %d is refused — must be a positive integer; a configured "+
					"non-positive value is a configuration error, not an implicit \"use the default "+
					"of %d\" (SAC-004)",
				*raw.Phasea.Ceiling, DefaultCeiling)
		}
		cfg.Ceiling = *raw.Phasea.Ceiling
	}
	if raw.Phasea.ForceCeiling != nil {
		if *raw.Phasea.ForceCeiling <= 0 {
			return nil, fmt.Errorf(
				"phasea.force_ceiling: %d is refused — must be a positive integer; a configured "+
					"non-positive value is a configuration error, not an implicit \"use the default "+
					"of %d\" (SAC-004, D3)",
				*raw.Phasea.ForceCeiling, DefaultForceCeiling)
		}
		cfg.ForceCeiling = *raw.Phasea.ForceCeiling
	}
	if raw.Phasea.MaxAttemptsPerItem != nil {
		if *raw.Phasea.MaxAttemptsPerItem <= 0 {
			return nil, fmt.Errorf(
				"phasea.max_attempts_per_item: %d is refused — must be a positive integer; a "+
					"configured non-positive value is a configuration error, not an implicit "+
					"\"use the default of %d\" (SAC-004)",
				*raw.Phasea.MaxAttemptsPerItem, DefaultMaxAttemptsPerItem)
		}
		cfg.MaxAttemptsPerItem = *raw.Phasea.MaxAttemptsPerItem
	}

	return cfg, nil
}
