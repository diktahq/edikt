package cmd

// verify_gov.go — `edikt verify gov <id>` subcommand.
//
// Walks a governance sidecar (ADR / invariant / guideline) and runs every
// verify: shell command declared in directives[], prohibitions[], and the
// object form of verification[]. Items without a verify: are recorded as
// skipped. Same exit-code contract as plan-verify:
//
// 0 — all executed items passed (or only skipped)
// 1 — at least one item failed or timed out
// 2 — sidecar missing or malformed
// 3 — invalid args
//
// The slash commands /edikt:gov:compile, /edikt:adr:new, etc. shell to this
// subcommand and consume its exit code via the tier-1 / tier-2 exit-code-only
// contract.

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
	"github.com/spf13/cobra"
)

// govIDRe matches the allowed shape of a gov-sidecar id: ADR-NNN, INV-NNN,
// GL-NNN-slug (the actual on-disk convention every guideline in this repo
// uses, e.g. GL-001-capture-gates), or a bare lowercase slug (hyphens,
// ≤80 chars) for a guideline that isn't GL-numbered. Validated before the
// value is interpolated into a filesystem path.
//
// The GL-NNN-slug alternative was added after `verify gov GL-003-...`
// failed against a real, on-disk guideline: the original pattern required
// an all-lowercase leading character, so it rejected every existing
// guideline's own uppercase "GL-NNN-" prefix. `verify all` walks the
// filesystem directly and was unaffected — only this single-id entry
// point had the gap, and nothing had exercised it against a real
// guideline before.
var govIDRe = regexp.MustCompile(`^(ADR|INV)-\d{3,}$|^GL-\d{3,}-[a-z][a-z0-9-]{0,79}$|^[a-z][a-z0-9-]{0,79}$`)

var verifyGovCmd = &cobra.Command{
	Use:   "gov <id>",
	Short: "Run verify: commands declared in a governance sidecar",
	Long: `Run every verify: shell command declared in the matching governance
sidecar (ADR / invariant / guideline). Items without a verify: are recorded
as skipped. Same exit-code contract as plan verify (0 / 1 / 2 / 3).

Used by:
  /edikt:gov:compile (gates the success path on all-pass)
  /edikt:adr:new, /edikt:invariant:new, /edikt:guideline:new
    (warns when the new artifact's verifies fail; never auto-deletes)`,
	Args:         cobra.ExactArgs(1),
	SilenceUsage: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		id := args[0]
		if !govIDRe.MatchString(id) {
			return &exitCodeError{code: 3, msg: fmt.Sprintf("invalid gov id %q (expected ADR-NNN, INV-NNN, or a guideline slug)", id)}
		}

		projectRoot, err := findProjectRootForVerify()
		if err != nil {
			return &exitCodeError{code: 3, msg: err.Error()}
		}
		if terr := ensureVerifyTrust(projectRoot); terr != nil {
			return terr
		}

		sidecarPath, err := locateGovSidecar(projectRoot, id)
		if err != nil {
			return &exitCodeError{code: 2, msg: err.Error()}
		}

		sc, err := sidecar.Load(sidecarPath)
		if err != nil {
			return &exitCodeError{code: 2, msg: fmt.Sprintf("load %s: %v", sidecarPath, err)}
		}

		results := runGovVerifies(sc, projectRoot)
		report := verify.NewReport("gov-"+id, "all", gitSHA(projectRoot), results)

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

// runGovVerifies walks the sidecar's directives, prohibitions, and
// structured verification[] entries — invoking verify.RunOne on each item
// that carries a verify command. Items without verify are recorded as
// skipped:operational so coverage stays measurable.
func runGovVerifies(sc *sidecar.Sidecar, projectRoot string) []verify.Result {
	var results []verify.Result
	opts := verify.RunOptions{Cwd: projectRoot}

	// Directives. Each gets a synthetic ID "directive[N]" so the report
	// disambiguates between entries that share text. The text becomes the
	// statement.
	//
	// suppressed_directives is subtracted here by exact text match, matching
	// the render path (phaseb/merge.go) and compile.EffectiveRules: a
	// suppressed directive is excluded from the compiled corpus, so its
	// verify: (if any) is checking something that no longer compiles and
	// MUST NOT run — running it risks a stale assertion failing (or
	// cheating a pass) for a rule that isn't live. Before this, only the
	// render path honoured suppression; this verb iterated sc.Directives
	// raw and ran every verify: unconditionally.
	suppressed := make(map[string]struct{}, len(sc.SuppressedDirectives))
	for _, sd := range sc.SuppressedDirectives {
		suppressed[sd] = struct{}{}
	}
	for i, d := range sc.Directives {
		id := fmt.Sprintf("directive[%d]", i)
		if _, isSuppressed := suppressed[d.Text]; isSuppressed {
			results = append(results, verify.Result{ID: id, Statement: d.Text, Status: verify.StatusSkippedSuppressed})
			continue
		}
		results = append(results, verify.RunOne(id, d.Text, d.Verify, opts))
	}

	// Prohibitions.
	for i, p := range sc.Prohibitions {
		id := fmt.Sprintf("prohibition[%d]", i)
		results = append(results, verify.RunOne(id, p.Text, p.Verify, opts))
	}

	// Verification[] is a oneOf at the schema level: bare string (legacy,
	// no verify) OR {text, verify?}. The Go struct exposes both through
	// the Verification field as []VerificationEntry; bare-string entries
	// have Verify == "" and are recorded as skipped.
	for i, v := range sc.Verification {
		id := fmt.Sprintf("verification[%d]", i)
		results = append(results, verify.RunOne(id, v.Text, v.Verify, opts))
	}

	return results
}

// emitItemProgress writes one per-item status line. Mirrors the
// plan-verify emitProgress shape but without the phase column since
// gov/prd/spec sidecars don't have phases.
func emitItemProgress(w io.Writer, r verify.Result) {
	mark := "?"
	switch r.Status {
	case verify.StatusPassed:
		mark = "+"
	case verify.StatusFailed:
		mark = "x"
	case verify.StatusTimeout:
		mark = "T"
	case verify.StatusSkippedOperational, verify.StatusSkippedInformational, verify.StatusSkippedSuppressed:
		mark = "~"
	}
	fmt.Fprintf(w, "  %s %s — %s (%dms)\n", mark, r.ID, r.Statement, r.DurationMS)
}

// locateGovSidecar resolves <id>.edikt.yaml under the configured artifact
// dirs (paths.decisions, paths.invariants, paths.guidelines).
func locateGovSidecar(projectRoot, id string) (string, error) {
	dirs := resolveArtifactDirs(projectRoot)
	for _, dir := range []string{dirs.decisions, dirs.invariants, dirs.guidelines} {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if name == id+".edikt.yaml" {
				return filepath.Join(dir, name), nil
			}
			// Slug form: ADR-NNN-some-slug.edikt.yaml
			if startsWithID(name, id) && filepath.Ext(name) == ".yaml" {
				return filepath.Join(dir, name), nil
			}
		}
	}
	return "", fmt.Errorf("verify gov: no sidecar found for %q under paths.{decisions,invariants,guidelines}", id)
}

// startsWithID reports whether name has the form "<id>-...edikt.yaml".
func startsWithID(name, id string) bool {
	prefix := id + "-"
	if len(name) < len(prefix) {
		return false
	}
	return name[:len(prefix)] == prefix &&
		len(name) > len(".edikt.yaml") &&
		name[len(name)-len(".edikt.yaml"):] == ".edikt.yaml"
}

func init() {
	verifyCmd.AddCommand(verifyGovCmd)
}
