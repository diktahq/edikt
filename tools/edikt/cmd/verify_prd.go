package cmd

// verify_prd.go — `edikt verify prd <PRD-ID>` subcommand.
//
// Walks the PRD sidecar (under paths.prds) and runs every verify: shell
// command declared in requirements[] (FRs) and acceptance_criteria[] (ACs).
// Entries without a verify: are recorded as skipped. Same exit-code
// contract as plan-verify and verify-gov:
//
// 0 — all executed items passed (or only skipped)
// 1 — at least one item failed or timed out
// 2 — sidecar missing or malformed
// 3 — invalid args
//
// PRD lifecycle commands (/edikt:sdlc:prd ship / supersede) shell to this
// subcommand and consume its exit code.

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

// prdIDRe matches PRD-NNN. Validated before any filesystem interpolation.
var prdIDRe = regexp.MustCompile(`^PRD-\d{3,}$`)

// prdSidecar models the subset of prd-sidecar.v1 that the verify runner
// needs. Other fields are tolerated via KnownFields(false) below — this
// loader is read-only and intentionally lenient; it does NOT validate
// the full schema (that's the doctor's job).
type prdSidecar struct {
	ID           string             `yaml:"id"`
	Title        string             `yaml:"title"`
	Requirements []prdRequirement   `yaml:"requirements"`
	Criteria     []prdAcceptanceCri `yaml:"acceptance_criteria"`
}

type prdRequirement struct {
	ID     string `yaml:"id"`
	Text   string `yaml:"text"`
	Verify string `yaml:"verify"`

	// SPEC-009 Plan G — cheat-rate scoring metadata. Optional.  // edikt-guard:allow
	VerifyKind            string `yaml:"verify_kind,omitempty"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	PositiveFixturePath   string `yaml:"positive_fixture_path,omitempty"`
	NegativeFixturePath   string `yaml:"negative_fixture_path,omitempty"`
}

type prdAcceptanceCri struct {
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

var verifyPRDCmd = &cobra.Command{
	Use:   "prd <PRD-ID>",
	Short: "Run verify: commands declared in a PRD sidecar",
	Long: `Run every verify: shell command declared in the matching PRD sidecar
(under paths.prds). Walks requirements[] (FRs) then acceptance_criteria[]
(ACs); entries without a verify: are recorded as skipped. Same exit-code
contract as plan verify (0 / 1 / 2 / 3).

Used by:
  /edikt:sdlc:prd ship / supersede  (refuses the transition when any verify fails)
  /edikt:sdlc:drift                 (PRD verify failures contribute to drift signal)`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !prdIDRe.MatchString(id) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("invalid PRD id %q (expected PRD-NNN)", id)}
		}

		projectRoot, err := findProjectRootForVerify()
		if err != nil {
			return &exitCodeError{code: 3, msg: err.Error()}
		}
		if terr := ensureVerifyTrust(projectRoot); terr != nil {
			return terr
		}

		sidecarPath, err := locatePRDSidecar(projectRoot, id)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		sc, err := loadPRDSidecar(sidecarPath)
		if err != nil {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("load %s: %v", sidecarPath, err)}
		}

		results := runPRDVerifies(sc, projectRoot)
		report := verify.NewReport("prd-"+id, "all", gitSHA(projectRoot), results)

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

func loadPRDSidecar(path string) (*prdSidecar, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var sc prdSidecar
	// SPEC-009 Plan A AC-1.2: loads PRD sidecar (PRD-NNN.yaml), not *.edikt.yaml.  // edikt-guard:allow
	// KnownFields off intentional — PRD sidecars carry user-extension fields under `extensions:`.
	if err := yaml.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	return &sc, nil
}

func runPRDVerifies(sc *prdSidecar, projectRoot string) []verify.Result {
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

// locatePRDSidecar resolves <id>*.yaml under paths.prds. PRD sidecars are
// named <id>-<slug>.yaml (no .edikt suffix — the .md and .yaml share the
// same stem for the PRD/SPEC class, distinct from the gov-sidecar
// .edikt.yaml convention).
func locatePRDSidecar(projectRoot, id string) (string, error) {
	dirs := resolveArtifactDirs(projectRoot)
	if dirs.prds == "" {
		return "", fmt.Errorf("verify prd: paths.prds not configured")
	}
	entries, err := os.ReadDir(dirs.prds)
	if err != nil {
		return "", fmt.Errorf("verify prd: read %s: %w", dirs.prds, err)
	}
	for _, e := range entries {
		name := e.Name()
		if filepath.Ext(name) != ".yaml" {
			continue
		}
		if name == id+".yaml" || startsWithPrefix(name, id+"-") {
			return filepath.Join(dirs.prds, name), nil
		}
	}
	return "", fmt.Errorf("verify prd: no sidecar for %q under %s", id, dirs.prds)
}

// startsWithPrefix is a small helper kept out of the regex path for safety.
func startsWithPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

func init() {
	verifyCmd.AddCommand(verifyPRDCmd)
}
