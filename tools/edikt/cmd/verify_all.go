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
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"io"
	"os"
	"path/filepath"
	"sort"
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
history is preserved; the newest 10 runs per artifact id are kept and
older ones are removed, and the run says how many it removed. Reports
written by a plan phase verify are never touched.

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
	// Unmeasured: a verify existed and its subject could not be read
	// (SPEC-018 SR-004). NOT folded into Skipped — skipped means no verify was
	// declared, unmeasured means one could not run. It does not set anyFail:
	// an unreadable subject is not a violated rule, it is an absent measurement.
	Unmeasured int `json:"unmeasured"`
	Skipped    int `json:"skipped"`
	// ReportsPruned: files removed by the retention policy on this run. A
	// deletion nobody is told about is indistinguishable from data loss, so
	// the count is reported wherever the run is.
	ReportsPruned int `json:"reports_pruned"`
	// ReportsOrphaned: files removed because their artifact is no longer in the
	// compiled corpus — deleted, renamed, superseded, deprecated, or not yet
	// accepted. NOT necessarily "gone from disk": the subject is the set this
	// walk covers, and the walk skips a skip-listed artifact whose file is
	// still there. Counted SEPARATELY from ReportsPruned: they are different
	// events.
	// Retention removes old runs of a live artifact; the orphan pass removes
	// every run of an artifact that is gone. Folding them into one number would
	// let "we deleted an artifact's entire history" hide inside "we trimmed
	// some old files", which is the shape of every silent-loss defect in this
	// file's history.
	ReportsOrphaned int `json:"reports_orphaned"`
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
	dirs := migrate.ResolveArtifactDirs(projectRoot)
	opts := verify.RunOptions{Cwd: projectRoot}
	stateDir := filepath.Join(projectRoot, ".edikt", "state", "verify")
	gitsha := gitSHA(projectRoot)

	out := &AllReport{Sidecars: []SidecarReport{}}

	// THE ORPHAN PASS'S SUBJECT, and it is only trustworthy when the walk was
	// complete. `enumComplete` goes false the moment a class of artifact could
	// not be enumerated for a reason other than "the directory is not there",
	// and it starts false under govOnly, which never looks at prd or spec at
	// all — running the orphan pass from a gov-only walk would classify every
	// prd and spec report as orphaned and delete the lot.
	//
	// The absence rule in its deletion form: an empty or failed scan means DO
	// NOTHING, never "nothing is live, remove everything". The ownership rule
	// says the same from the other side — remove only what is provably retired.
	// (ref: INV-013, INV-019)  edikt-guard:allow
	liveIDs := map[string]bool{}
	enumComplete := !govOnly

	// ── gov ──────────────────────────────────────────────────────────
	for _, dir := range []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines} {
		if dir == "" {
			// A class with no configured directory is a class this walk cannot
			// speak for. Not proof that its artifacts are gone.
			enumComplete = false
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A directory that does not exist IS proof there are no artifacts
			// of that class — that is a measurement. A permission error, an I/O
			// error, or anything else is a failure to measure, and a failure to
			// measure must never authorise a deletion.
			if !os.IsNotExist(err) {
				enumComplete = false
			}
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
				//
				// LIVE, not orphaned. The artifact is on disk; only its parse
				// failed. Letting an unparseable sidecar orphan its own history
				// would delete the reports precisely when someone needs them.
				liveIDs["gov-"+govIDFromName(name)] = true
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
			liveIDs["gov-"+id] = true
			results := runGovVerifies(sc, projectRoot)
			persistReport(stateDir, "gov-"+id, gitsha, results)
			out.Sidecars = append(out.Sidecars, SidecarReport{
				Kind: "gov", ID: id, Path: path, Results: results,
			})
			_ = opts // keep linter happy if RunOne is inlined later
		}
	}

	// ── prd ──────────────────────────────────────────────────────────
	if dirs.Prds == "" && !govOnly {
		enumComplete = false
	}
	if dirs.Prds != "" && !govOnly {
		entries, err := os.ReadDir(dirs.Prds)
		if err != nil && !os.IsNotExist(err) {
			enumComplete = false
		}
		if err == nil {
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				name := e.Name()
				if filepath.Ext(name) != ".yaml" {
					continue
				}
				path := filepath.Join(dirs.Prds, name)
				sc, err := loadPRDSidecar(path)
				if err != nil {
					liveIDs["prd-"+stripExt(name)] = true
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
				liveIDs["prd-"+id] = true
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
	if dirs.Specs == "" && !govOnly {
		enumComplete = false
	}
	if dirs.Specs != "" && !govOnly {
		entries, err := os.ReadDir(dirs.Specs)
		if err != nil && !os.IsNotExist(err) {
			enumComplete = false
		}
		if err == nil {
			for _, e := range entries {
				name := e.Name()
				var path string
				if e.IsDir() {
					cand := filepath.Join(dirs.Specs, name, "spec.yaml")
					if _, statErr := os.Stat(cand); statErr != nil {
						continue
					}
					path = cand
				} else {
					if filepath.Ext(name) != ".yaml" {
						continue
					}
					path = filepath.Join(dirs.Specs, name)
				}
				sc, err := loadSPECSidecar(path)
				if err != nil {
					liveIDs["spec-"+stripExt(name)] = true
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
				liveIDs["spec-"+id] = true
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
			case verify.StatusUnmeasured:
				out.Summary.Unmeasured++
			case verify.StatusSkippedOperational, verify.StatusSkippedInformational,
				verify.StatusSkippedSuppressed, verify.StatusSkippedOwner:
				// Suppressed and Owner were absent from this switch while being
				// present in reporter.go's, so they counted in ItemsTotal and in
				// nothing else. Found because SR-004's new bucket made the
				// arithmetic reconcile to 1291 of 1292 — the same silent
				// under-statement, one status over.
				out.Summary.Skipped++
			}
		}
		if anyFail {
			out.Summary.SidecarsFailing++
		}
	}

	// Retention runs LAST and ONCE, after every persist, so the newest report
	// for each id is already on disk and one ReadDir covers the whole
	// directory. Pruning inside persistReport would read the state dir once
	// per sidecar — 88 reads of a directory whose size is the problem — which
	// is the cost shape doctor was just taken off.
	// `live` is nil unless the walk could account for every class. nil turns
	// the orphan pass OFF and leaves retention doing exactly what it did
	// before — the safe direction, and the only one an incomplete measurement
	// entitles this code to.
	var live map[string]bool
	if enumComplete {
		live = liveIDs
	}
	out.Summary.ReportsPruned, out.Summary.ReportsOrphaned =
		pruneVerifyState(stateDir, verifyAllRetainPerID, live)

	return out, nil
}

// verifyAllRetainPerID is how many `verify all` reports are kept per artifact
// id. Every run writes one per sidecar and nothing ever removed them: this
// repo's .edikt/state/verify/ held 135,852 files, 135,250 of them `-all-`
// reports, growing 5–11k a day. Ten is enough to see a recent regression
// appear and still bound the directory at roughly (sidecars x 10 x 2) files.
//
// WHAT ACTUALLY READS MORE THAN THE NEWEST, measured rather than assumed,
// because a value retained for a consumer that does not exist is the same
// defect class as everything else in this file: the only code reader is
// `latestVerifyReportFrom` (cmd/doctor_verify_coverage.go), and it walks the
// sorted matches BACKWARDS returning the first that PARSES. So it normally
// reads exactly one — but it falls through to older reports when the newest is
// unreadable or truncated, which WriteReports can leave behind since it writes
// the .json and .txt separately and an interrupted run truncates the first.
// Retention of 1 would therefore hand doctor nothing whenever a run was
// interrupted. That justifies "more than one"; it does not justify ten.
//
// Ten is not derived from that reader and is left as an owner's call rather
// than retuned here. Anything above the fallback depth is retained for a human
// reading the directory, which is a real but unmeasured consumer.
//
// A var, not a const, so a test can drive the boundary without writing a
// thousand fixtures.
var verifyAllRetainPerID = 10

// pruneVerifyState keeps the newest verifyAllRetainPerID reports per id under
// stateDir and removes the rest, returning how many FILES it removed.
//
// Scope is deliberately narrow in two ways:
//
//   - Only `-all-` reports, which are the ones `verify all` writes. A plan's
//     `<plan>-phase-<n>-<stamp>` reports are the evidence a plan's progress
//     table cites; `verify all` did not create them and does not get to
//     decide they are expendable.
//   - Both halves of a report — `.json` and `.txt` — are removed together.
//     Removing one and leaving the other would leave a report that reads as
//     present to one consumer and absent to the other.
//
// The stamp is UTC "20060102T150405Z", so lexical order over the filename is
// time order — the same property doctor's indexVerifyState relies on. Errors
// are silent for the same reason persistReport's are: retention is
// housekeeping, and a read-only state dir must not fail a verify run.
// live, when non-nil, is the set of report ids — "<kind>-<artifact id>", the
// same key persistReport writes — whose artifact is still in the corpus. Ids
// outside it are ORPHANED and every report for them is removed, however few.
//
// WHY AN ORPHAN PASS AT ALL. Retention alone trims per id and never asks
// whether the id should still exist, so a retired artifact keeps its full
// allowance forever. Measured on the dogfood corpus: six superseded decision
// records each held their full retained history long after `gov compile`
// stopped writing to them. Dead disk is the mild half.
//
// The real hazard is ID REUSE. Reports are keyed by BARE artifact id, and this
// project has already carried two different artifacts under one id on two
// branches. A stale report under a reused id attaches to the NEW artifact, and
// doctor reports its last run as though it belonged there — one artifact's
// measurement presented as another's.
//
// live == nil means the caller could not establish what is live, and the pass
// is skipped entirely. That direction is not a convenience: an empty or failed
// scan reading as "nothing is live" would delete the whole directory. That is
// the absence-is-never-a-finding rule aimed at data, and the inverse of the
// rule that a removal requires provable ownership.
//
// (ref: INV-013, INV-019)  edikt-guard:allow
func pruneVerifyState(stateDir string, keep int, live map[string]bool) (pruned, orphaned int) {
	if keep <= 0 && live == nil {
		return 0, 0
	}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return 0, 0
	}
	byID := map[string][]string{}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasSuffix(n, ".json") {
			continue
		}
		stem := strings.TrimSuffix(n, ".json")
		i := strings.LastIndex(stem, "-all-")
		if i <= 0 {
			continue
		}
		byID[stem[:i]] = append(byID[stem[:i]], stem)
	}
	for id, stems := range byID {
		// ORPHAN: the artifact is gone, so every run of it goes, not just the
		// ones over the retention bound. Checked BEFORE the keep test, since an
		// orphan under the bound is still an orphan.
		if live != nil && !live[id] {
			for _, stem := range stems {
				orphaned += removeReportPair(stateDir, stem)
			}
			continue
		}
		if keep <= 0 || len(stems) <= keep {
			continue
		}
		sort.Strings(stems)
		for _, stem := range stems[:len(stems)-keep] {
			pruned += removeReportPair(stateDir, stem)
		}
	}
	return pruned, orphaned
}

// removeReportPair removes both halves of one report and returns how many
// FILES went. Both halves together, always: a surviving .txt whose .json is
// gone reads as present to one consumer and absent to the other.
func removeReportPair(stateDir, stem string) int {
	n := 0
	for _, ext := range []string{".json", ".txt"} {
		if err := os.Remove(filepath.Join(stateDir, stem+ext)); err == nil {
			n++
		}
	}
	return n
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
	fmt.Fprintf(w, "\nsummary: %d sidecars (%d failing); %d items: %d passed, %d failed, %d unmeasured, %d timeout, %d skipped\n",
		r.Summary.SidecarsTotal, r.Summary.SidecarsFailing,
		r.Summary.ItemsTotal, r.Summary.Passed, r.Summary.Failed,
		r.Summary.Unmeasured, r.Summary.Timeout, r.Summary.Skipped)
	if r.Summary.ReportsPruned > 0 {
		fmt.Fprintf(w, "retention: removed %d old report file(s), keeping the newest %d run(s) per id\n",
			r.Summary.ReportsPruned, verifyAllRetainPerID)
	}
	// Named separately from retention, because it is a different event: this is
	// an artifact's ENTIRE history going, not its oldest runs. A reader who sees
	// only "removed N files" cannot tell those apart.
	//
	// "no longer in the corpus", NOT "no longer exists". The live set is what
	// this walk covers, and the walk skips a superseded, deprecated OR PROPOSED
	// artifact whose file is still sitting on disk — a proposed decision record
	// is in this set. Saying "no longer exists" of a file the reader can open
	// would be exactly the kind of statement this whole change exists to stop
	// the tooling making.
	if r.Summary.ReportsOrphaned > 0 {
		fmt.Fprintf(w, "orphans: removed %d report file(s) for artifact(s) no longer in the compiled corpus "+
			"(deleted, renamed, superseded, deprecated, or not yet accepted)\n",
			r.Summary.ReportsOrphaned)
	}
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
	return govIDFromName(name)
}

// govIDFromName is govIDFromSidecar's filename half, split out so the
// sidecar-load FAILURE path can still name the artifact. That path has no
// *Sidecar to pass, and the id is what marks the artifact live so the orphan
// pass does not delete an unparseable artifact's report history.
func govIDFromName(name string) string {
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
