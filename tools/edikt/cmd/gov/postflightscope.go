package gov

// postflightscope.go — `bin/edikt gov post-flight-scope` subcommand.
//
// Resolves the effective L3 specialist set for the project's post-flight
// review composition (SPEC-008). Reads `post-flight:` from  // edikt-guard:allow
// .edikt/config.yaml (defaults to enabled=true when absent), composes
// (auto-detected ∪ required) − never via internal/postflight, and emits
// the result as JSON.
//
// Callers:
//   - commands/sdlc/post-flight.md (orchestrator queries this before L3
//     dispatch)
//   - bin/edikt doctor (surfaces the effective scope in its output —
//     SPEC-008 SR-015)  // edikt-guard:allow
//
// Per ADR-029 Rule 3, the subcommand falls under the `gov <subcommand>`  // edikt-guard:allow
// group permit — not a new top-level verb. Per ADR-033, no LLM heredocs;  // edikt-guard:allow
// pure Go.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/postflight"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	postFlightScopeJSON         bool
	postFlightScopeAutoDetected string
)

var postFlightScopeCmd = &cobra.Command{
	Use:   "post-flight-scope",
	Short: "Print the effective L3 specialist set for the post-flight pipeline",
	Long: `Reads post-flight: from .edikt/config.yaml and resolves the
effective specialist set as (auto-detected ∪ required) − never.

The auto-detected list normally comes from the orchestrator's analysis
of the diff (file-pattern routing in _shared-agent-routing.md). For
manual invocation or doctor reporting, pass --auto-detected to supply it
directly; the default empty list reflects "no diff to analyze" which is
how the doctor surfaces the static scope.

Exit codes:
  0 — always (this is a query, not a gate)
`,
	Args: cobra.NoArgs,
	RunE: runPostFlightScope,
}

func init() {
	postFlightScopeCmd.Flags().BoolVar(&postFlightScopeJSON, "json", true,
		"emit JSON to stdout (currently the only output mode)")
	postFlightScopeCmd.Flags().StringVar(&postFlightScopeAutoDetected, "auto-detected", "",
		"comma-separated list of auto-detected specialists (default: empty — for static doctor view)")
	Cmd.AddCommand(postFlightScopeCmd)
}

// postFlightConfig mirrors the post-flight: block in .edikt/config.yaml.
// Absence of the block / field means "default" — Enabled=true and empty
// specialist lists. The cobra subcommand never reads outside this shape.
type postFlightConfig struct {
	PostFlight struct {
		Enabled     *bool `yaml:"enabled"`
		Specialists struct {
			Required []string `yaml:"required"`
			Auto     []string `yaml:"auto"`
			Never    []string `yaml:"never"`
		} `yaml:"specialists"`
	} `yaml:"post-flight"`
}

type postFlightScopeReport struct {
	Enabled     bool     `json:"enabled"`
	Specialists []string `json:"specialists"`
	// Echoes back the inputs that produced `specialists` — useful for
	// debugging and for doctor's "computed from..." line.
	Inputs struct {
		AutoDetected []string `json:"auto_detected"`
		Required     []string `json:"required"`
		Never        []string `json:"never"`
		AutoFilter   []string `json:"auto_filter"`
	} `json:"inputs"`
}

func runPostFlightScope(cmd *cobra.Command, args []string) error {
	cfgPath, err := locatePostFlightConfig()
	if err != nil {
		return err
	}

	cfg, err := loadPostFlightConfig(cfgPath)
	if err != nil {
		return err
	}

	enabled := true
	if cfg.PostFlight.Enabled != nil {
		enabled = *cfg.PostFlight.Enabled
	}

	// Parse --auto-detected. Empty string → empty slice (static doctor view).
	autoDetected := splitCSV(postFlightScopeAutoDetected)

	// Apply the auto: filter. ['*'] (or absent / empty) means "use
	// autoDetected as-is". Any other value means "intersect autoDetected
	// with the auto: list".
	autoFilter := cfg.PostFlight.Specialists.Auto
	filteredAuto := applyAutoFilter(autoDetected, autoFilter)

	effective := postflight.EffectiveSet(
		filteredAuto,
		cfg.PostFlight.Specialists.Required,
		cfg.PostFlight.Specialists.Never,
	)

	report := postFlightScopeReport{
		Enabled:     enabled,
		Specialists: effective,
	}
	report.Inputs.AutoDetected = autoDetected
	report.Inputs.Required = cfg.PostFlight.Specialists.Required
	report.Inputs.Never = cfg.PostFlight.Specialists.Never
	report.Inputs.AutoFilter = autoFilter

	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(report)
}

// locatePostFlightConfig walks up from cwd looking for .edikt/config.yaml,
// mirroring the project-root resolution other gov subcommands use. Returns
// the empty string if no config is found — caller treats that as "use
// defaults".
func locatePostFlightConfig() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".edikt", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", nil
}

func loadPostFlightConfig(path string) (postFlightConfig, error) {
	var cfg postFlightConfig
	if path == "" {
		return cfg, nil // defaults: Enabled=true, empty lists
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, fmt.Errorf("read %s: %w", path, err)
	}
	// SPEC-009 Plan A AC-1.2: loads project post-flight scope config (.edikt/config.yaml subset).  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — config is user-extensible.
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, fmt.Errorf("parse %s: %w", path, err)
	}
	return cfg, nil
}

// splitCSV returns [] for an empty string (not [""]).
func splitCSV(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return []string{}
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// applyAutoFilter returns autoDetected as-is when filter is ['*'] / empty /
// absent. Otherwise returns the intersection of autoDetected with filter.
func applyAutoFilter(autoDetected, filter []string) []string {
	if len(filter) == 0 || slices.Contains(filter, "*") {
		return autoDetected
	}
	allow := make(map[string]struct{}, len(filter))
	for _, f := range filter {
		allow[f] = struct{}{}
	}
	out := make([]string, 0, len(autoDetected))
	for _, a := range autoDetected {
		if _, ok := allow[a]; ok {
			out = append(out, a)
		}
	}
	return out
}
