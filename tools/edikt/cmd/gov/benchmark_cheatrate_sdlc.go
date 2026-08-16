package gov

// benchmark_cheatrate_sdlc.go — Plan G: SDLC corpus discovery for the
// cheat-rate benchmark.
//
// Plan E's `--all` originally scanned only governance dirs
// (paths.{decisions,invariants,guidelines}). Plan F added the baseline
// pack at templates/sidecars/baseline/. Plan G closes SR-008's third
// corpus class: SDLC sidecars under paths.{specs,prds,plans}.
//
// Adapter design (rather than refactor):
//
// SDLC sidecars are structurally different from gov sidecars. SPEC
// uses requirements[] / acceptance_criteria[]; PRD has the same shape
// with status fields; PLAN-criteria uses phases[].criteria[]. None of
// them have directives[] or prohibitions[] arrays.
//
// Rather than refactor the cheat-rate machinery to be type-aware, we
// adapt SDLC sidecars TO the gov shape at discovery time: walk
// requirements/criteria/phases.criteria, filter to entries that
// declare `verify_kind: behavioral`, synthesize one sidecar.Directive
// per entry, and emit a sidecar.Pair carrying a fake gov-shaped
// Sidecar. RunCheatRateForVerify then iterates these like any other.
//
// Per-entry VerifyID uses the entry's natural identifier ("SR-001",
// "AC-003-2", phase "p1" criterion "AC-1.1") so the report is
// disambiguated correctly.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"gopkg.in/yaml.v3"
)

// sdlcPathsConfig is the minimal subset of .edikt/config.yaml that
// Plan G's SDLC discovery needs. Loaded separately from
// govrun.GovernanceDirs because govrun only exposes gov paths.
type sdlcPathsConfig struct {
	Paths struct {
		PRDs  string `yaml:"prds"`
		Specs string `yaml:"specs"`
		Plans string `yaml:"plans"`
	} `yaml:"paths"`
}

// loadSDLCPathsConfig reads .edikt/config.yaml from projectRoot and
// returns the three SDLC paths. Missing config returns zero-values
// (which downstream treats as "skip that corpus"). Missing individual
// paths in config are tolerated.
func loadSDLCPathsConfig(projectRoot string) sdlcPathsConfig {
	var cfg sdlcPathsConfig
	raw, err := os.ReadFile(filepath.Join(projectRoot, ".edikt", "config.yaml"))
	if err != nil {
		return cfg
	}
	_ = yaml.Unmarshal(raw, &cfg)
	return cfg
}

// discoverSDLCSidecars walks paths.{specs,prds,plans} under
// projectRoot and returns one sidecar.Pair per discovered SDLC
// sidecar whose entries contain at least one `verify_kind: behavioral`
// item. Entries are adapted into gov-shaped Directive structs so the
// cheat-rate machinery iterates them like normal.
//
// Empty / absent dirs are not errors — the corresponding corpus is
// just skipped.
func discoverSDLCSidecars(projectRoot string) ([]sidecar.Pair, error) {
	cfg := loadSDLCPathsConfig(projectRoot)
	var pairs []sidecar.Pair

	if cfg.Paths.Specs != "" {
		specPairs, err := discoverSpecSidecars(filepath.Join(projectRoot, cfg.Paths.Specs))
		if err != nil {
			return nil, fmt.Errorf("discover specs: %w", err)
		}
		pairs = append(pairs, specPairs...)
	}
	if cfg.Paths.PRDs != "" {
		prdPairs, err := discoverPRDSidecars(filepath.Join(projectRoot, cfg.Paths.PRDs))
		if err != nil {
			return nil, fmt.Errorf("discover prds: %w", err)
		}
		pairs = append(pairs, prdPairs...)
	}
	if cfg.Paths.Plans != "" {
		planPairs, err := discoverPlanCriteriaSidecars(filepath.Join(projectRoot, cfg.Paths.Plans))
		if err != nil {
			return nil, fmt.Errorf("discover plan criteria: %w", err)
		}
		pairs = append(pairs, planPairs...)
	}
	return pairs, nil
}

// specYAMLShape is the SPEC sidecar shape we need to extract
// behavioral verifies. Mirrors verify_spec.go's specSidecar but in
// the gov package to avoid an import cycle.
type specYAMLShape struct {
	ID           string              `yaml:"id"`
	Requirements []sdlcEntryYAML     `yaml:"requirements"`
	Criteria     []sdlcCriterionYAML `yaml:"acceptance_criteria"`
}

// prdYAMLShape — same for PRD sidecars.
type prdYAMLShape struct {
	ID           string              `yaml:"id"`
	Requirements []sdlcEntryYAML     `yaml:"requirements"`
	Criteria     []sdlcCriterionYAML `yaml:"acceptance_criteria"`
}

// sdlcEntryYAML matches both SPEC requirements and PRD requirements.
// The Plan G fields are the discriminators; everything else passes
// through. Decoded leniently (no KnownFields) so unrelated fields
// don't error.
type sdlcEntryYAML struct {
	ID                    string `yaml:"id"`
	Text                  string `yaml:"text"`
	Verify                string `yaml:"verify"`
	VerifyKind            string `yaml:"verify_kind"`
	Intent                string `yaml:"intent"`
	FalsifyingObservation string `yaml:"falsifying_observation"`
	PositiveFixturePath   string `yaml:"positive_fixture_path"`
	NegativeFixturePath   string `yaml:"negative_fixture_path"`
}

// sdlcCriterionYAML matches acceptance_criteria items on both SPEC
// and PRD. Adds Given/When/Then over sdlcEntryYAML.
type sdlcCriterionYAML struct {
	ID                    string `yaml:"id"`
	Given                 string `yaml:"given"`
	When                  string `yaml:"when"`
	Then                  string `yaml:"then"`
	Verify                string `yaml:"verify"`
	VerifyKind            string `yaml:"verify_kind"`
	Intent                string `yaml:"intent"`
	FalsifyingObservation string `yaml:"falsifying_observation"`
	PositiveFixturePath   string `yaml:"positive_fixture_path"`
	NegativeFixturePath   string `yaml:"negative_fixture_path"`
}

// discoverSpecSidecars finds spec.yaml files under specDir (one per
// SPEC-NNN-* directory) and adapts behavioral entries to gov shape.
func discoverSpecSidecars(specDir string) ([]sidecar.Pair, error) {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pairs []sidecar.Pair
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sidecarPath := filepath.Join(specDir, e.Name(), "spec.yaml")
		raw, err := os.ReadFile(sidecarPath)
		if err != nil {
			continue
		}
		var sp specYAMLShape
		if err := yaml.Unmarshal(raw, &sp); err != nil {
			continue
		}
		directives := adaptSDLCEntries(sp.Requirements, sp.Criteria)
		if len(directives) == 0 {
			continue
		}
		artifactID := sp.ID
		if artifactID == "" {
			artifactID = e.Name()
		}
		pairs = append(pairs, sidecar.Pair{
			SidecarPath: sidecarPath,
			ArtifactID:  artifactID,
			Sidecar: &sidecar.Sidecar{
				SchemaVersion: 1,
				Directives:    directives,
			},
		})
	}
	return pairs, nil
}

// discoverPRDSidecars finds PRD-*.yaml files under prdDir and adapts
// behavioral entries. PRD sidecars sit at the top level of the dir
// (one .yaml per PRD), not in subdirectories.
func discoverPRDSidecars(prdDir string) ([]sidecar.Pair, error) {
	entries, err := os.ReadDir(prdDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pairs []sidecar.Pair
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".yaml") || !strings.HasPrefix(name, "PRD-") {
			continue
		}
		sidecarPath := filepath.Join(prdDir, name)
		raw, err := os.ReadFile(sidecarPath)
		if err != nil {
			continue
		}
		var pp prdYAMLShape
		if err := yaml.Unmarshal(raw, &pp); err != nil {
			continue
		}
		directives := adaptSDLCEntries(pp.Requirements, pp.Criteria)
		if len(directives) == 0 {
			continue
		}
		artifactID := pp.ID
		if artifactID == "" {
			artifactID = strings.TrimSuffix(name, ".yaml")
		}
		pairs = append(pairs, sidecar.Pair{
			SidecarPath: sidecarPath,
			ArtifactID:  artifactID,
			Sidecar: &sidecar.Sidecar{
				SchemaVersion: 1,
				Directives:    directives,
			},
		})
	}
	return pairs, nil
}

// discoverPlanCriteriaSidecars finds PLAN-*-criteria.yaml files under
// plansDir and adapts behavioral criteria. The criteria sidecar's
// shape (phases[].criteria[]) differs from SPEC/PRD; we walk it
// type-aware via verify.LoadCriteria so the v1 validator runs first.
func discoverPlanCriteriaSidecars(plansDir string) ([]sidecar.Pair, error) {
	entries, err := os.ReadDir(plansDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var pairs []sidecar.Pair
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, "-criteria.yaml") {
			continue
		}
		sidecarPath := filepath.Join(plansDir, name)
		cf, err := verify.LoadCriteria(sidecarPath)
		if err != nil {
			// Strict v1 validator rejected — skip; doctor reports
			// invalid criteria files separately.
			continue
		}
		var directives []sidecar.Directive
		for _, ph := range cf.Phases {
			for _, cr := range ph.Criteria {
				if cr.VerifyKind != "behavioral" || cr.Verify == "" {
					continue
				}
				directives = append(directives, sidecar.Directive{
					Text:                  cr.Statement,
					Verify:                cr.Verify,
					VerifyKind:            cr.VerifyKind,
					Intent:                cr.Intent,
					FalsifyingObservation: cr.FalsifyingObservation,
					PositiveFixturePath:   cr.PositiveFixturePath,
					NegativeFixturePath:   cr.NegativeFixturePath,
				})
			}
		}
		if len(directives) == 0 {
			continue
		}
		artifactID := cf.Plan
		if artifactID == "" {
			artifactID = strings.TrimSuffix(name, "-criteria.yaml")
		}
		pairs = append(pairs, sidecar.Pair{
			SidecarPath: sidecarPath,
			ArtifactID:  artifactID,
			Sidecar: &sidecar.Sidecar{
				SchemaVersion: 1,
				Directives:    directives,
			},
		})
	}
	return pairs, nil
}

// adaptSDLCEntries walks SDLC requirements + acceptance_criteria and
// returns only the behavioral entries adapted to gov-shaped
// Directive structs. Non-behavioral entries are silently dropped.
//
// The adapter preserves the natural entry id (SR-NNN, FR-NNN,
// AC-NNN-N) in the Text field so RunCheatRateForVerify's report
// shows the original ids when convenient. Plan E's
// RunOpts.VerifyID override lets the caller surface those ids.
func adaptSDLCEntries(reqs []sdlcEntryYAML, crits []sdlcCriterionYAML) []sidecar.Directive {
	var directives []sidecar.Directive
	for _, r := range reqs {
		if r.VerifyKind != "behavioral" || r.Verify == "" {
			continue
		}
		directives = append(directives, sidecar.Directive{
			Text:                  fmt.Sprintf("[%s] %s", r.ID, r.Text),
			Verify:                r.Verify,
			VerifyKind:            r.VerifyKind,
			Intent:                r.Intent,
			FalsifyingObservation: r.FalsifyingObservation,
			PositiveFixturePath:   r.PositiveFixturePath,
			NegativeFixturePath:   r.NegativeFixturePath,
		})
	}
	for _, c := range crits {
		if c.VerifyKind != "behavioral" || c.Verify == "" {
			continue
		}
		directives = append(directives, sidecar.Directive{
			Text:                  fmt.Sprintf("[%s] %s / %s / %s", c.ID, c.Given, c.When, c.Then),
			Verify:                c.Verify,
			VerifyKind:            c.VerifyKind,
			Intent:                c.Intent,
			FalsifyingObservation: c.FalsifyingObservation,
			PositiveFixturePath:   c.PositiveFixturePath,
			NegativeFixturePath:   c.NegativeFixturePath,
		})
	}
	return directives
}
