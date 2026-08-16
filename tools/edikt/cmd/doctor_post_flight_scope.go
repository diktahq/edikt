package cmd

// doctor_post_flight_scope.go — surfaces the project's post-flight pipeline
// scope in `bin/edikt doctor` output (SPEC-008 SR-015).  // edikt-guard:allow
//
// Reads .edikt/config.yaml's post-flight: block and reports:
//   - enabled state
//   - effective specialist set (auto+required filter, never-subtract)
//   - report directory + entry count
//   - telemetry file presence
//
// Read-only / soft-signal: doctor's exit code does NOT depend on this
// section. The block reports informational state only.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/postflight"
	"gopkg.in/yaml.v3"
)

type doctorPostFlightConfig struct {
	PostFlight struct {
		Enabled     *bool `yaml:"enabled"`
		Specialists struct {
			Required []string `yaml:"required"`
			Auto     []string `yaml:"auto"`
			Never    []string `yaml:"never"`
		} `yaml:"specialists"`
	} `yaml:"post-flight"`
}

// reportPostFlightScope prints the Post-flight Scope section of doctor's
// output. Always exits cleanly — never affects the doctor exit code.
func reportPostFlightScope() {
	fmt.Println()
	fmt.Println("Post-flight Scope:")

	cwd, err := os.Getwd()
	if err != nil {
		fmt.Println("  (could not resolve project root)")
		return
	}

	cfgPath := locateProjectConfig(cwd)
	if cfgPath == "" {
		// Not in an edikt project — defaults apply.
		fmt.Println("  enabled: true (no .edikt/config.yaml found — defaults)")
		fmt.Println("  effective specialists: [] (no diff to analyze)")
		return
	}

	cfg, err := loadDoctorPostFlightConfig(cfgPath)
	if err != nil {
		fmt.Printf("  (could not parse %s: %v)\n", cfgPath, err)
		return
	}

	enabled := true
	if cfg.PostFlight.Enabled != nil {
		enabled = *cfg.PostFlight.Enabled
	}
	fmt.Printf("  enabled: %v\n", enabled)

	// Effective set with empty autoDetected — the static view doctor surfaces
	// shows what the orchestrator would resolve when no diff is in play
	// (required ∪ ∅) − never. The full orchestrator-time set requires the
	// actual diff's auto-detection.
	effective := postflight.EffectiveSet(
		nil,
		cfg.PostFlight.Specialists.Required,
		cfg.PostFlight.Specialists.Never,
	)
	fmt.Printf("  effective specialists: %s\n", formatSpecialistList(effective))

	// Reports directory + entry count.
	projectRoot := filepath.Dir(filepath.Dir(cfgPath))
	reportDir := filepath.Join(projectRoot, ".edikt", "state", "post-flight")
	if entries, err := os.ReadDir(reportDir); err == nil {
		reports := 0
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") && !strings.HasSuffix(e.Name(), ".jsonl") {
				reports++
			}
		}
		fmt.Printf("  reports dir: %s (%d reports)\n", reportDir, reports)
	} else if !os.IsNotExist(err) {
		fmt.Printf("  reports dir: %s (unreadable: %v)\n", reportDir, err)
	} else {
		fmt.Printf("  reports dir: %s (none yet)\n", reportDir)
	}

	// Telemetry log presence.
	metrics := filepath.Join(reportDir, ".metrics.jsonl")
	if info, err := os.Stat(metrics); err == nil {
		fmt.Printf("  telemetry:   %s (%d bytes)\n", metrics, info.Size())
	} else {
		fmt.Printf("  telemetry:   %s (none yet)\n", metrics)
	}
}

// locateProjectConfig walks up from the given cwd looking for
// .edikt/config.yaml. Returns empty string when not in an edikt project.
func locateProjectConfig(cwd string) string {
	dir := cwd
	for {
		candidate := filepath.Join(dir, ".edikt", "config.yaml")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

func loadDoctorPostFlightConfig(path string) (doctorPostFlightConfig, error) {
	var cfg doctorPostFlightConfig
	raw, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	// SPEC-009 Plan A AC-1.2: loads doctor's post-flight config (project-local YAML).  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — config is user-extensible.
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func formatSpecialistList(s []string) string {
	if len(s) == 0 {
		return "[] (none configured as required; auto-detection runs at dispatch time)"
	}
	return "[" + strings.Join(s, ", ") + "]"
}
