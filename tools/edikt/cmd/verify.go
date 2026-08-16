package cmd

// verify.go — Phase 12 of PLAN-sidecar-architecture.
//
// `edikt verify <plan-id> [--phase N]` walks the plan's criteria sidecar,
// runs every `verify:` shell command, and writes a JSON+text report under
// .edikt/state/verify/. Exit codes:
//
// 0 — all executed criteria passed (or only skipped/informational)
// 1 — at least one criterion failed or timed out
// 2 — sidecar missing or malformed YAML
// 3 — invalid args (unknown plan-id, etc.)
//
// `--allow-failures` suppresses exit-1 (used by /edikt:sdlc:plan to surface
// failures without blocking; the plan-command makes the gating decision).

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

var (
	verifyPhase         string
	verifyJSON          bool
	verifyAllowFailures bool
)

// planIDRe is the allowlist for plan-id arguments. Plan slugs in this repo
// look like `sidecar-architecture` or `v0.6.0-rc1`. Validated:
// the value is interpolated into a filesystem path.
var planIDRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,79}$`)

// phaseIDRe is the allowlist for --phase values.
//
// Phase ids are a whole number with an OPTIONAL LETTER SUFFIX: "1", "12",
// "4b", "04a". The letter suffix is the insertion notation — when a phase has
// to be added between 8 and 9 it is 8a, then 8b, never 8.5. Renumbering is
// not required and should not be attempted; it breaks every reference to
// later phases.
//
// Decimals are deliberately NOT accepted, and the reason is ordering: phase
// ids are carried as opaque strings end-to-end — nothing in the verify path
// parses them numerically — so they compare LEXICALLY. A consumer lost
// machine evaluation on two phases by writing 8.5/8.6; the capability was
// there, the notation was undocumented, and the rejection named neither.
//
// DO NOT relax this to also accept decimals. It reads as a friendly
// widening and is not: "8.10" sorts BEFORE "8.5" under lexical comparison,
// so a plan silently reorders the moment a tenth insert appears. The bug
// surfaces only in long plans, long after the change, and looks like a
// planning error rather than a parsing one. Accepting both notations would
// also give one concept two spellings, and the letter form is the one real
// plans already use (04a, 4b, 4c, 6b, 7a, 7b, 11b).
//
// If a future change genuinely needs decimals, it must first make phase ids
// ordered data rather than strings — parse and compare them numerically
// everywhere they are sorted. Widening the regex alone is not that change.
var phaseIDRe = regexp.MustCompile(`^[0-9]+[a-z]?$`)

var verifyCmd = &cobra.Command{
	Use:   "verify <plan-id>",
	Short: "Run a plan's criteria sidecar verifications",
	Long: `Run the verify: shell commands declared in PLAN-<plan-id>-criteria.yaml,
capture pass/fail/timeout/skipped per criterion, and write a JSON + text
report to .edikt/state/verify/.

Exit codes:
  0 — all executed criteria passed (or only skipped/informational)
  1 — at least one criterion failed or timed out
  2 — sidecar missing or malformed YAML
  3 — invalid args (unknown plan-id, malformed --phase, etc.)

Use --allow-failures to suppress exit-1 (the run still records failures
in the report). The /edikt:sdlc:plan command gates row-flips on exit 0.`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		planID := args[0]
		if !planIDRe.MatchString(planID) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("invalid plan-id %q", planID)}
		}
		if verifyPhase != "" && !phaseIDRe.MatchString(verifyPhase) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf(
				"invalid --phase %q.\n"+
					"Phase ids are a number with an optional letter suffix: 1, 12, 4b, 04a.\n"+
					"To insert a phase between 8 and 9, use the insertion form 8a (then 8b) — "+
					"not 8.5, and never renumber later phases.\n"+
					"See commands/sdlc/plan.md.", verifyPhase)}
		}

		projectRoot, err := findProjectRootForVerify()
		if err != nil {
			return &exitCodeError{code: 3, msg: err.Error()}
		}
		if terr := ensureVerifyTrust(projectRoot); terr != nil {
			return terr
		}

		criteriaPath, err := locateCriteriaSidecar(projectRoot, planID)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		cf, err := verify.LoadCriteria(criteriaPath)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		// Filter phases.
		var phases []verify.Phase
		phaseLabel := "all"
		if verifyPhase == "" {
			phases = cf.Phases
		} else {
			p := cf.FindPhase(verifyPhase)
			if p == nil {
				return &exitCodeError{code: 3, msg: fmt.Sprintf(
					"phase %q not found in %s", verifyPhase, criteriaPath)}
			}
			phases = []verify.Phase{*p}
			phaseLabel = verifyPhase
		}

		// Run criteria.
		var results []verify.Result
		for _, p := range phases {
			for _, c := range p.Criteria {
				res := verify.RunCriterion(p, c, verify.RunOptions{Cwd: projectRoot})
				if !verifyJSON {
					emitProgress(cmd.OutOrStdout(), p, res)
				}
				results = append(results, res)
			}
		}

		report := verify.NewReport(planID, phaseLabel, gitSHA(projectRoot), results)

		// Persist report.
		dir := filepath.Join(projectRoot, ".edikt", "state", "verify")
		jsonPath, werr := verify.WriteReports(dir, report)
		if werr != nil {
			return fmt.Errorf("write report: %w", werr)
		}

		if verifyJSON {
			body, _ := json.MarshalIndent(report, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
		} else {
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

// emitProgress writes a single status line per criterion to w.
func emitProgress(w io.Writer, p verify.Phase, r verify.Result) {
	mark := "?"
	switch r.Status {
	case verify.StatusPassed:
		mark = "+"
	case verify.StatusFailed:
		mark = "x"
	case verify.StatusTimeout:
		mark = "T"
	case verify.StatusSkippedOperational, verify.StatusSkippedInformational:
		mark = "~"
	}
	fmt.Fprintf(w, "  %s [phase %s] %s — %s (%dms)\n",
		mark, p.ID, r.ID, r.Statement, r.DurationMS)
}

// findProjectRootForVerify walks from CWD up looking for .edikt/config.yaml.
// Falls back to CWD if no config is found, since the verify runner only
// needs a directory tree containing docs/internal/plans/.
func findProjectRootForVerify() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("getwd: %w", err)
	}
	home := os.Getenv("HOME")
	for dir := cwd; ; dir = filepath.Dir(dir) {
		if _, err := os.Stat(filepath.Join(dir, ".edikt", "config.yaml")); err == nil {
			if home != "" && filepath.Clean(dir) == filepath.Clean(home) {
				break
			}
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	// No config — fall back to CWD. Verify only needs the plan paths.
	return cwd, nil
}

// locateCriteriaSidecar resolves PLAN-<id>-criteria.yaml under the
// configured plans dir, falling back to a small set of conventional paths.
func locateCriteriaSidecar(projectRoot, planID string) (string, error) {
	plansDirs := []string{plansPathFromConfig(projectRoot)}
	for _, d := range []string{
		"docs/internal/plans",
		"docs/plans",
		"docs/product/plans",
	} {
		plansDirs = append(plansDirs, filepath.Join(projectRoot, d))
	}

	// Accept both bare ("SPEC-032-x") and PLAN-prefixed ("PLAN-SPEC-032-x")
	// ids — unconditional prefixing produced PLAN-PLAN-<id>-criteria.yaml
	// lookups when users pasted the sidecar's own stem.
	stem := "PLAN-" + strings.TrimPrefix(planID, "PLAN-")
	for _, dir := range plansDirs {
		if dir == "" {
			continue
		}
		cand := filepath.Join(dir, stem+"-criteria.yaml")
		if _, err := os.Stat(cand); err == nil {
			return cand, nil
		}
	}
	expected := filepath.Join(plansDirs[0], stem+"-criteria.yaml")
	return "", fmt.Errorf("verify: no criteria sidecar at %s", expected)
}

// plansPathFromConfig reads paths.plans from .edikt/config.yaml; returns
// an absolute path or "" when no config is readable.
func plansPathFromConfig(projectRoot string) string {
	data, err := os.ReadFile(filepath.Join(projectRoot, ".edikt", "config.yaml"))
	if err != nil {
		return ""
	}
	var raw struct {
		Paths struct {
			Plans string `yaml:"plans"`
		} `yaml:"paths"`
	}
	// SPEC-009 Plan A AC-1.2: loads .edikt/config.yaml subset (paths.plans only).  // edikt-guard:allow
	// Not *.edikt.yaml. KnownFields off intentional — config is user-extensible.
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ""
	}
	if raw.Paths.Plans == "" {
		return ""
	}
	if filepath.IsAbs(raw.Paths.Plans) {
		return raw.Paths.Plans
	}
	return filepath.Join(projectRoot, raw.Paths.Plans)
}

// gitSHA returns an abbreviated HEAD sha (or "dirty"/"unknown" sentinels).
func gitSHA(projectRoot string) string {
	cmd := exec.Command("git", "-C", projectRoot, "rev-parse", "--short", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	sha := strings.TrimSpace(string(out))
	// Detect dirty tree.
	st := exec.Command("git", "-C", projectRoot, "status", "--porcelain")
	if so, err := st.Output(); err == nil && len(strings.TrimSpace(string(so))) > 0 {
		return sha + "-dirty"
	}
	return sha
}

func init() {
	verifyCmd.Flags().StringVar(&verifyPhase, "phase", "", "verify a single phase by id (e.g. 1, 4b, 12)")
	// --json and --allow-failures are persistent so the gov / prd / spec
	// subcommands inherit them with the same semantics.
	verifyCmd.PersistentFlags().BoolVar(&verifyJSON, "json", false, "emit the JSON report to stdout (in addition to writing it)")
	verifyCmd.PersistentFlags().BoolVar(&verifyAllowFailures, "allow-failures", false, "exit 0 even when criteria fail (failures still recorded in the report)")
	// --trust approves this project to run its repo-defined verify: commands
	// (ADR-041). Persistent so verify all / gov / prd / spec inherit it.  // edikt-guard:allow
	verifyCmd.PersistentFlags().BoolVar(&verifyTrust, "trust", false, "approve this project to run its repo-defined verify: commands (records it in ~/.edikt/state)")
	rootCmd.AddCommand(verifyCmd)
}
