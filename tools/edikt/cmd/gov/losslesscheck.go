package gov

// losslesscheck.go — `edikt gov lossless-check` subcommand.
//
// Phase 11 of PLAN-v060-governance-accuracy. For each ADR/INV in
// paths.decisions / paths.invariants, locates the matching v0.4.3
// baseline snapshot under test/fixtures/sidecar-baseline-v043/ and
// runs the lossless-set check (every legacy directive tuple covered
// by the v0.6.0 sidecar's directives + prohibitions + manual_directives).
//
// Pure Go, no LLM (ADR-030).

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var losslessReportPath string

var losslessCheckCmd = &cobra.Command{
	Use:   "lossless-check",
	Short: "Verify v0.6.0 sidecars are at least as faithful as their v0.4.3 baselines",
	Long: `Walks paths.decisions + paths.invariants, loads each .edikt.yaml,
finds the matching .md snapshot under test/fixtures/sidecar-baseline-v043/,
and asserts that every (modality, ref_id, normalised noun-phrase) tuple
from the legacy sentinel block is covered by the sidecar's directives,
prohibitions, or manual_directives.

Exit codes:
  0 — clean (all artifacts pass)
  1 — at least one loss detected (artifact tuple missing in v0.6.0)
  2 — baseline directory missing or unreadable
  3 — flag/argument error
`,
	Args: cobra.NoArgs,
	RunE: runLosslessCheck,
}

func init() {
	losslessCheckCmd.Flags().StringVar(&losslessReportPath, "report-json",
		".edikt/state/lossless-report.json",
		"path to write the JSON loss report (validated against the project root)")
	Cmd.AddCommand(losslessCheckCmd)
}

type losslessConfig struct {
	Paths struct {
		Decisions  string `yaml:"decisions"`
		Invariants string `yaml:"invariants"`
	} `yaml:"paths"`
}

type artifactReport struct {
	ID           string           `json:"id"`
	SidecarPath  string           `json:"sidecar_path"`
	BaselinePath string           `json:"baseline_path,omitempty"`
	Status       string           `json:"status"` // "pass" | "fail" | "skip"
	SkipReason   string           `json:"skip_reason,omitempty"`
	Losses       []lossless.Loss  `json:"losses,omitempty"`
}

type losslessReport struct {
	Summary struct {
		TotalArtifacts int `json:"total_artifacts"`
		Passed         int `json:"passed"`
		Failed         int `json:"failed"`
		Skipped        int `json:"skipped"`
		TotalLosses    int `json:"total_losses"`
	} `json:"summary"`
	Artifacts []artifactReport `json:"artifacts"`
}

func runLosslessCheck(cmd *cobra.Command, args []string) error {
	root, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("getwd: %w", err)
	}

	// INV-006: validate --report-json path. Resolve absolute, refuse
	// traversal, ensure it stays under the project root.
	absReport, err := filepath.Abs(losslessReportPath)
	if err != nil {
		return fmt.Errorf("invalid --report-json: %w", err)
	}
	if !strings.HasPrefix(absReport, root+string(os.PathSeparator)) && absReport != root {
		return fmt.Errorf("--report-json must be under project root: %s", absReport)
	}
	if strings.Contains(losslessReportPath, "..") {
		return fmt.Errorf("--report-json must not contain '..'")
	}

	cfgBytes, err := os.ReadFile(filepath.Join(root, ".edikt/config.yaml"))
	if err != nil {
		return fmt.Errorf("read .edikt/config.yaml: %w", err)
	}
	var cfg losslessConfig
	if err := yaml.Unmarshal(cfgBytes, &cfg); err != nil {
		return fmt.Errorf("parse .edikt/config.yaml: %w", err)
	}

	baselineDir := filepath.Join(root, "test/fixtures/sidecar-baseline-v043")
	if _, err := os.Stat(baselineDir); err != nil {
		return &exitErr{code: 2, msg: fmt.Sprintf("baseline directory not found: %s — run Phase 11 setup", baselineDir)}
	}

	dirs := []string{cfg.Paths.Decisions, cfg.Paths.Invariants}
	pairs, err := sidecar.Discover(root, dirs)
	if err != nil {
		return fmt.Errorf("discover sidecars: %w", err)
	}

	rpt := losslessReport{}
	for _, p := range pairs {
		if p.Skip {
			rpt.Artifacts = append(rpt.Artifacts, artifactReport{
				ID:          p.ArtifactID,
				SidecarPath: relPath(root, p.SidecarPath),
				Status:      "skip",
				SkipReason:  p.SkipReason,
			})
			rpt.Summary.Skipped++
			continue
		}
		if p.Sidecar == nil {
			rpt.Artifacts = append(rpt.Artifacts, artifactReport{
				ID:          p.ArtifactID,
				SidecarPath: relPath(root, p.SidecarPath),
				Status:      "skip",
				SkipReason:  "sidecar missing — out of scope for lossless check",
			})
			rpt.Summary.Skipped++
			continue
		}

		baselinePath := filepath.Join(baselineDir, filepath.Base(p.ParentPath))
		baselineBytes, berr := os.ReadFile(baselinePath)
		if berr != nil {
			rpt.Artifacts = append(rpt.Artifacts, artifactReport{
				ID:          p.ArtifactID,
				SidecarPath: relPath(root, p.SidecarPath),
				Status:      "skip",
				SkipReason:  "no v0.4.3 baseline — artifact created post-migration",
			})
			rpt.Summary.Skipped++
			continue
		}

		losses := lossless.CheckLossless(baselineBytes, p.Sidecar)
		ar := artifactReport{
			ID:           p.ArtifactID,
			SidecarPath:  relPath(root, p.SidecarPath),
			BaselinePath: relPath(root, baselinePath),
			Losses:       losses,
		}
		if len(losses) == 0 {
			ar.Status = "pass"
			rpt.Summary.Passed++
		} else {
			ar.Status = "fail"
			rpt.Summary.Failed++
			rpt.Summary.TotalLosses += len(losses)
		}
		rpt.Artifacts = append(rpt.Artifacts, ar)
	}
	rpt.Summary.TotalArtifacts = len(rpt.Artifacts)
	sort.Slice(rpt.Artifacts, func(i, j int) bool { return rpt.Artifacts[i].ID < rpt.Artifacts[j].ID })

	if err := writeReport(absReport, &rpt); err != nil {
		return fmt.Errorf("write report: %w", err)
	}

	// Print human summary.
	fmt.Printf("Lossless check: %d artifacts (passed=%d, failed=%d, skipped=%d, total_losses=%d)\n",
		rpt.Summary.TotalArtifacts, rpt.Summary.Passed, rpt.Summary.Failed,
		rpt.Summary.Skipped, rpt.Summary.TotalLosses)
	for _, ar := range rpt.Artifacts {
		if ar.Status != "fail" {
			continue
		}
		fmt.Printf("  FAIL %s — %d loss(es):\n", ar.ID, len(ar.Losses))
		for _, l := range ar.Losses {
			fmt.Printf("    [%s] %s\n", l.Type, truncate(l.LegacyText, 80))
		}
	}
	fmt.Printf("Report: %s\n", losslessReportPath)

	if rpt.Summary.Failed > 0 {
		return &exitErr{code: 1, msg: ""}
	}
	return nil
}

func writeReport(absPath string, rpt *losslessReport) error {
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		return err
	}
	out, err := json.MarshalIndent(rpt, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(absPath, out, 0o644)
}

func relPath(root, p string) string {
	if rel, err := filepath.Rel(root, p); err == nil {
		return rel
	}
	return p
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

type exitErr struct {
	code int
	msg  string
}

func (e *exitErr) Error() string {
	if e.msg != "" {
		return e.msg
	}
	return fmt.Sprintf("exit %d", e.code)
}
func (e *exitErr) ExitCode() int { return e.code }

// exitFromExitErr translates an *exitErr returned by a sub-RunE into an
// os.Exit with the carried code, matching the cobra → os.Exit handling
// pattern in compile.go's RunE. The root cmd's Execute() only knows
// about *exitCodeError (cmd package); cross-package exit codes need
// their own bridge.
func exitFromExitErr(err error) {
	if err == nil {
		return
	}
	var ee *exitErr
	if errAs(err, &ee) {
		if ee.msg != "" {
			fmt.Fprintln(os.Stderr, ee.msg)
		}
		os.Exit(ee.code)
	}
}

// errAs is a tiny errors.As shim so importers don't have to depend on
// the errors package just for this single use site.
func errAs(err error, target **exitErr) bool {
	if err == nil {
		return false
	}
	if e, ok := err.(*exitErr); ok {
		*target = e
		return true
	}
	return false
}
