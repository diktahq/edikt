package cmd

// verify_spec.go — `edikt verify spec <SPEC-ID>` subcommand.
//
// Walks the SPEC sidecar (under paths.specs) and runs every verify: shell
// command declared in requirements[] (SRs) and acceptance_criteria[] (ACs).
// Entries without a verify: are recorded as skipped. Same exit-code
// contract as plan-verify and verify-gov.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var specIDRe = regexp.MustCompile(`^SPEC-\d{3,}$`)

type specSidecar struct {
	ID           string              `yaml:"id"`
	Title        string              `yaml:"title"`
	Requirements []specRequirement   `yaml:"requirements"`
	Criteria     []specAcceptanceCri `yaml:"acceptance_criteria"`
}

type specRequirement struct {
	ID     string `yaml:"id"`
	Text   string `yaml:"text"`
	Verify string `yaml:"verify"`

	// SPEC-009 Plan G — cheat-rate scoring metadata. Optional;  // edikt-guard:allow
	// populated only when the SR carries a behavioral verify the
	// cheat-rate benchmark should score.
	VerifyKind            string `yaml:"verify_kind,omitempty"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	PositiveFixturePath   string `yaml:"positive_fixture_path,omitempty"`
	NegativeFixturePath   string `yaml:"negative_fixture_path,omitempty"`
}

type specAcceptanceCri struct {
	ID     string `yaml:"id"`
	Given  string `yaml:"given"`
	When   string `yaml:"when"`
	Then   string `yaml:"then"`
	Verify string `yaml:"verify"`

	// SPEC-009 Plan G — cheat-rate scoring metadata. Optional.  // edikt-guard:allow
	VerifyKind            string `yaml:"verify_kind,omitempty"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	PositiveFixturePath   string `yaml:"positive_fixture_path,omitempty"`
	NegativeFixturePath   string `yaml:"negative_fixture_path,omitempty"`
}

var verifySPECCmd = &cobra.Command{
	Use:   "spec <SPEC-ID>",
	Short: "Run verify: commands declared in a SPEC sidecar",
	Long: `Run every verify: shell command declared in the matching SPEC sidecar
(under paths.specs). Walks requirements[] (SRs) then acceptance_criteria[]
(ACs); entries without a verify: are recorded as skipped. Same exit-code
contract as plan verify (0 / 1 / 2 / 3).

Used by:
  /edikt:sdlc:drift  (SPEC verify failures contribute to drift signal)`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !specIDRe.MatchString(id) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("invalid SPEC id %q (expected SPEC-NNN)", id)}
		}

		projectRoot, err := findProjectRootForVerify()
		if err != nil {
			return &exitCodeError{code: 3, msg: err.Error()}
		}
		if terr := ensureVerifyTrust(projectRoot); terr != nil {
			return terr
		}

		sidecarPath, err := locateSPECSidecar(projectRoot, id)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		sc, err := loadSPECSidecar(sidecarPath)
		if err != nil {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("load %s: %v", sidecarPath, err)}
		}

		results := runSPECVerifies(sc, projectRoot)
		report := verify.NewReport("spec-"+id, "all", gitSHA(projectRoot), results)

		dir := filepath.Join(projectRoot, ".edikt", "state", "verify")
		jsonPath, werr := verify.WriteReports(dir, report)
		if werr != nil {
			return fmt.Errorf("write report: %w", werr)
		}

		if verifyJSON {
			body, _ := json.MarshalIndent(report, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
		} else {
			for _, r := range results {
				emitItemProgress(cmd.OutOrStdout(), r)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "\nreport: %s\n", jsonPath)
			fmt.Fprintf(cmd.OutOrStdout(), "summary: %d passed, %d failed, %d timeout, %d skipped (total %d)\n",
				report.Summary.Passed, report.Summary.Failed,
				report.Summary.Timeout, report.Summary.Skipped, report.Summary.Total)
		}

		if report.AnyFailures() && !verifyAllowFailures {
			return &exitCodeError{code: 1, msg: ""}
		}
		return nil
	},
}

func loadSPECSidecar(path string) (*specSidecar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc specSidecar
	// SPEC-009 Plan A AC-1.2: loads SPEC sidecar (spec.yaml), not *.edikt.yaml.  // edikt-guard:allow
	// KnownFields off intentional — SPEC sidecars carry user-extension fields under `extensions:`.
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &sc, nil
}

func runSPECVerifies(sc *specSidecar, projectRoot string) []verify.Result {
	var results []verify.Result
	opts := verify.RunOptions{Cwd: projectRoot}
	for _, r := range sc.Requirements {
		results = append(results, verify.RunOne(r.ID, r.Text, r.Verify, opts))
	}
	for _, c := range sc.Criteria {
		statement := fmt.Sprintf("Given %s; When %s; Then %s", c.Given, c.When, c.Then)
		results = append(results, verify.RunOne(c.ID, statement, c.Verify, opts))
	}
	return results
}

func locateSPECSidecar(projectRoot, id string) (string, error) {
	dirs := resolveArtifactDirs(projectRoot)
	if dirs.specs == "" {
		return "", fmt.Errorf("verify spec: paths.specs not configured")
	}
	entries, err := os.ReadDir(dirs.specs)
	if err != nil {
		return "", fmt.Errorf("verify spec: read %s: %w", dirs.specs, err)
	}
	for _, e := range entries {
		name := e.Name()
		// Nested layout (canonical for v2 SPECs): paths.specs/<id>-<slug>/spec.yaml
		// Mirrors the per-spec directory convention already used for spec.md.
		if e.IsDir() && (name == id || startsWithPrefix(name, id+"-")) {
			cand := filepath.Join(dirs.specs, name, "spec.yaml")
			if _, err := os.Stat(cand); err == nil {
				return cand, nil
			}
			continue
		}
		// Flat layout: paths.specs/<id>.yaml or paths.specs/<id>-<slug>.yaml.
		if filepath.Ext(name) != ".yaml" {
			continue
		}
		if name == id+".yaml" || startsWithPrefix(name, id+"-") {
			return filepath.Join(dirs.specs, name), nil
		}
	}
	return "", fmt.Errorf("verify spec: no sidecar for %q under %s", id, dirs.specs)
}

func init() {
	verifyCmd.AddCommand(verifySPECCmd)
}
