package cmd

// verify_all.go — `edikt verify all` subcommand.
//
// Walks every sidecar in the project (gov + prd + spec) and runs every
// verify: command in each. Returns the same exit-code contract as the
// per-class subcommands. Used by:
//
//   - bin/edikt gov compile (post-Phase-B success-path gate)
//   - bin/edikt doctor (verify-coverage soft check)
//   - e2e tests that need a single-shot "is the project clean?" call

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"github.com/spf13/cobra"
)

var verifyAllCmd = &cobra.Command{
	Use:   "all",
	Short: "Run verify: across every gov / prd / spec sidecar in the project",
	Long: `Walks every governance, PRD, and SPEC sidecar configured by
paths.{decisions,invariants,guidelines,prds,specs} and runs every
verify: shell command declared in each. Same exit-code contract as the
per-class subcommands (0 / 1 / 2 / 3). Each sidecar's report is written
to .edikt/state/verify/ under its own filename so the per-artifact
history is preserved.

Used by gov compile (post-merge success-path gate), doctor (coverage
soft signal), and the v0.6.0 completion-evidence e2e tests.`,
	Args:         cobra.NoArgs,
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		projectRoot, err := findProjectRootForVerify()
		if err != nil {
			return &exitCodeError{code: 3, msg: err.Error()}
		}
		if terr := ensureVerifyTrust(projectRoot); terr != nil {
			return terr
		}

		report, err := runVerifyAll(projectRoot, verifyGovOnly)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		if verifyJSON {
			body, _ := json.MarshalIndent(report, "", "  ")
			fmt.Fprintln(cmd.OutOrStdout(), string(body))
		} else {
			emitAllReport(cmd.OutOrStdout(), report)
		}

		if report.AnyFailures() && !verifyAllowFailures {
			return &exitCodeError{code: 1, msg: ""}
		}
		return nil
	},
}

// AllReport is the structured report shape returned by runVerifyAll.
// One entry per sidecar; the top-level Summary aggregates across all.
type AllReport struct {
	Sidecars []SidecarReport `json:"sidecars"`
	Summary  AllSummary      `json:"summary"`
}

type SidecarReport struct {
	Kind    string          `json:"kind"` // "gov" | "prd" | "spec"
	ID      string          `json:"id"`
	Path    string          `json:"path"`
	Results []verify.Result `json:"results"`
}

type AllSummary struct {
	SidecarsTotal   int `json:"sidecars_total"`
	SidecarsFailing int `json:"sidecars_failing"`
	ItemsTotal      int `json:"items_total"`
	Passed          int `json:"passed"`
	Failed          int `json:"failed"`
	Timeout         int `json:"timeout"`
	Skipped         int `json:"skipped"`
}

// AnyFailures returns true when at least one item failed or timed out.
func (r *AllReport) AnyFailures() bool {
	return r.Summary.Failed > 0 || r.Summary.Timeout > 0
}

// runVerifyAll walks every gov / prd / spec sidecar under the configured
// artifact dirs and runs every verify command. Per-sidecar results are
// also persisted via the regular WriteReports path so the per-artifact
// history under .edikt/state/verify/ stays consistent.
//
// govOnly restricts the walk to ADR / INV / guideline sidecars. The
// post-compile gate uses it: prd / spec / plan verifies routinely reference
// deliberately-unbuilt future work, and gating governance compilation on
// them turned every compile in a WIP project into an error exit. Those
// classes keep their own runners (`verify prd|spec|<plan-id>`).
func runVerifyAll(projectRoot string, govOnly bool) (*AllReport, error) {
	dirs := resolveArtifactDirs(projectRoot)
	opts := verify.RunOptions{Cwd: projectRoot}
	stateDir := filepath.Join(projectRoot, ".edikt", "state", "verify")
	gitsha := gitSHA(projectRoot)

	out := &AllReport{Sidecars: []SidecarReport{}}

	// ── gov ──────────────────────────────────────────────────────────
	for _, dir := range []string{dirs.decisions, dirs.invariants, dirs.guidelines} {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			path := filepath.Join(dir, name)
			// A retired artifact's leftover sidecar (superseded / deprecated /
			// migration:skip parent) is inert — compile ignores it, so its
			// verifies must not gate anything either.
			parentMD := strings.TrimSuffix(path, ".edikt.yaml") + ".md"
			if skip, _ := sidecar.IsSkipListed(parentMD); skip {
				continue
			}
			sc, err := sidecar.Load(path)
			if err != nil {
				// Surface as a parse failure on this sidecar — one bad
				// sidecar should not abort the whole walk.
				out.Sidecars = append(out.Sidecars, SidecarReport{
					Kind: "gov", ID: stripGovSuffix(name), Path: path,
					Results: []verify.Result{{
						ID:            "load",
						Statement:     "load sidecar",
						Status:        verify.StatusFailed,
						StderrExcerpt: err.Error(),
						ExitCode:      -2,
					}},
				})
				continue
			}
			id := govIDFromSidecar(sc, name)
			results := runGovVerifies(sc, projectRoot)
			persistReport(stateDir, "gov-"+id, gitsha, results)
			out.Sidecars = append(out.Sidecars, SidecarReport{
				Kind: "gov", ID: id, Path: path, Results: results,
			})
			_ = opts // keep linter happy if RunOne is inlined later
		}
	}

	// ── prd ──────────────────────────────────────────────────────────
	if dirs.prds != "" && !govOnly {
		entries, err := os.ReadDir(dirs.prds)
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if filepath.Ext(name) != ".yaml" {
					continue
				}
				path := filepath.Join(dirs.prds, name)
				sc, err := loadPRDSidecar(path)
				if err != nil {
					out.Sidecars = append(out.Sidecars, SidecarReport{
						Kind: "prd", ID: stripExt(name), Path: path,
						Results: []verify.Result{{
							ID:            "load",
							Statement:     "load sidecar",
							Status:        verify.StatusFailed,
							StderrExcerpt: err.Error(),
							ExitCode:      -2,
						}},
					})
					continue
				}
				id := sc.ID
				if id == "" {
					id = stripExt(name)
				}
				results := runPRDVerifies(sc, projectRoot)
				persistReport(stateDir, "prd-"+id, gitsha, results)
				out.Sidecars = append(out.Sidecars, SidecarReport{
					Kind: "prd", ID: id, Path: path, Results: results,
				})
			}
		}
	}

	// ── spec ─────────────────────────────────────────────────────────
	// Walks both layouts: flat (paths.specs/<id>.yaml) and nested
	// (paths.specs/<id>-<slug>/spec.yaml). Nested is the canonical v2
	// per-spec directory convention established for spec.md.
	if dirs.specs != "" && !govOnly {
		entries, err := os.ReadDir(dirs.specs)
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				var path string
				if e.IsDir() {
					cand := filepath.Join(dirs.specs, name, "spec.yaml")
					if _, statErr := os.Stat(cand); statErr != nil {
						continue
					}
					path = cand
				} else {
					if filepath.Ext(name) != ".yaml" {
						continue
					}
					path = filepath.Join(dirs.specs, name)
				}
				sc, err := loadSPECSidecar(path)
				if err != nil {
					out.Sidecars = append(out.Sidecars, SidecarReport{
						Kind: "spec", ID: stripExt(name), Path: path,
						Results: []verify.Result{{
							ID:            "load",
							Statement:     "load sidecar",
							Status:        verify.StatusFailed,
							StderrExcerpt: err.Error(),
							ExitCode:      -2,
						}},
					})
					continue
				}
				id := sc.ID
				if id == "" {
					id = stripExt(name)
				}
				results := runSPECVerifies(sc, projectRoot)
				persistReport(stateDir, "spec-"+id, gitsha, results)
				out.Sidecars = append(out.Sidecars, SidecarReport{
					Kind: "spec", ID: id, Path: path, Results: results,
				})
			}
		}
	}

	// Aggregate the summary.
	out.Summary.SidecarsTotal = len(out.Sidecars)
	for _, s := range out.Sidecars {
		anyFail := false
		for _, r := range s.Results {
			out.Summary.ItemsTotal++
			switch r.Status {
			case verify.StatusPassed:
				out.Summary.Passed++
			case verify.StatusFailed:
				out.Summary.Failed++
				anyFail = true
			case verify.StatusTimeout:
				out.Summary.Timeout++
				anyFail = true
			case verify.StatusSkippedOperational, verify.StatusSkippedInformational:
				out.Summary.Skipped++
			}
		}
		if anyFail {
			out.Summary.SidecarsFailing++
		}
	}

	return out, nil
}

// persistReport writes the per-sidecar verify report to .edikt/state/verify/
// so the doctor and history tools can find the same data they would have
// from a per-class invocation. Errors are silent — persistence is
// secondary; the in-memory aggregation is the authoritative result.
func persistReport(dir, id, sha string, results []verify.Result) {
	report := verify.NewReport(id, "all", sha, results)
	_, _ = verify.WriteReports(dir, report)
}

// emitAllReport writes a one-line-per-sidecar text summary, with a
// bottom-line aggregate. Format mirrors the per-class output so the
// shapes feel familiar.
func emitAllReport(w io.Writer, r *AllReport) {
	for _, s := range r.Sidecars {
		fail := 0
		skipped := 0
		passed := 0
		for _, res := range s.Results {
			switch res.Status {
			case verify.StatusPassed:
				passed++
			case verify.StatusFailed, verify.StatusTimeout:
				fail++
			case verify.StatusSkippedOperational, verify.StatusSkippedInformational:
				skipped++
			}
		}
		mark := "+"
		if fail > 0 {
			mark = "x"
		} else if passed == 0 && skipped > 0 {
			mark = "~"
		}
		fmt.Fprintf(w, "  %s %s/%s — %d passed, %d failed, %d skipped\n",
			mark, s.Kind, s.ID, passed, fail, skipped)
		// Per-item lines for any failures, so the user knows what to fix.
		for _, res := range s.Results {
			if res.Status == verify.StatusFailed || res.Status == verify.StatusTimeout {
				fmt.Fprintf(w, "      ↳ %s — exit=%d %s\n",
					res.ID, res.ExitCode, oneLine(res.StderrExcerpt))
			}
		}
	}
	fmt.Fprintf(w, "\nsummary: %d sidecars (%d failing); %d items: %d passed, %d failed, %d timeout, %d skipped\n",
		r.Summary.SidecarsTotal, r.Summary.SidecarsFailing,
		r.Summary.ItemsTotal, r.Summary.Passed, r.Summary.Failed,
		r.Summary.Timeout, r.Summary.Skipped)
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	if len(s) > 120 {
		s = s[:117] + "…"
	}
	return s
}

func stripGovSuffix(name string) string {
	return strings.TrimSuffix(name, ".edikt.yaml")
}

func stripExt(name string) string {
	return strings.TrimSuffix(name, filepath.Ext(name))
}

// govIDFromSidecar prefers the canonical id derived from the file stem
// (the sidecar package doesn't yet expose an `id` field for gov-class
// sidecars — its identity is encoded in the filename ADR-NNN-...).
func govIDFromSidecar(sc *sidecar.Sidecar, name string) string {
	_ = sc // reserved for future use when id moves into the sidecar
	stem := strings.TrimSuffix(name, ".edikt.yaml")
	// ADR-NNN-slug → ADR-NNN; bare ADR-NNN → ADR-NNN.
	for _, prefix := range []string{"ADR-", "INV-"} {
		if strings.HasPrefix(stem, prefix) {
			rest := stem[len(prefix):]
			end := 0
			for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
				end++
			}
			if end > 0 {
				return prefix + rest[:end]
			}
		}
	}
	return stem
}

var verifyGovOnly bool

func init() {
	verifyAllCmd.Flags().BoolVar(&verifyGovOnly, "gov-only", false,
		"walk only ADR / INV / guideline sidecars (used by the gov compile gate; prd/spec keep their own runners)")
	verifyCmd.AddCommand(verifyAllCmd)
}
