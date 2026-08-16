package gov

// benchmark_cheatrate.go — `bin/edikt gov benchmark cheat-rate` subcommand.
//
// SPEC-009 Plan C Phase 2 / ADR-040. Dispatches an adversary agent per  // edikt-guard:allow
// verify and reports cheat-rate; this skeleton wires the flags, exit
// codes, and stub-mode fixture loader. Phase 3 will add the full
// sandbox + dispatch implementation.
//
// Exit-code contract (ADR-040):  // edikt-guard:allow
//   0 — run completed (or stub returned canned verdicts)
//   1 — sandbox / dispatch error
//   2 — sidecar id specified but no matching .edikt.yaml found
//   3 — invalid or missing arguments / flags
//
// Stub mode: when EDIKT_CHEAT_RATE_STUB=1 is set, the command skips
// adversary dispatch entirely and prints canned fixture JSON from
// test/fixtures/benchmark-stubs/<id>.json. This keeps the exit-code
// contract tests deterministic and lets downstream tooling exercise
// the integration without burning Opus tokens.
//
// ADR-030 / no-llm-in-tier-2: the production code path here does NOT  // edikt-guard:allow
// import an LLM client; the full LLM dispatch lands in Phase 3 via a
// strictly isolated host-agent shim. Until then the non-stub body is
// a documented stub that exits 0 with an explanation.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/benchmark/cheatrate"
	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/spf13/cobra"
)

// Test-injection seams. Production leaves these nil and the
// implementation falls back to cheatrate.DispatchAdversary /
// cheatrate.RunVerifyInSandbox / time.Now. Tests override to control
// per-run verdicts deterministically without spawning real claude.
//
// Package-level vars (rather than function parameters) keep the cobra
// command signature clean; tests reset them via t.Cleanup so parallel
// tests don't see leftover state.
var (
	dispatcherForTests   cheatrate.Dispatcher
	verifyRunnerForTests cheatrate.VerifyRunner
	nowForTests          func() time.Time
)

// inconclusiveRateThreshold is the ADR-040 §6.6 ceiling. If  // edikt-guard:allow
// Summary.InconclusiveRate > this value across a --all run, the
// command exits 1 to signal "adversary starved" rather than
// "verifies robust". Pulled out so tests can verify the threshold
// without rebuilding the binary.
const inconclusiveRateThreshold = 0.10

var (
	cheatRateAll            bool
	cheatRateRefresh        bool
	cheatRateAdversaryModel string
)

// cheatRateIDRe matches the allowed shape of a sidecar id passed on
// the command line — same shape verify gov accepts (ADR-NNN, INV-NNN,
// or a slug). Validated before any filesystem interpolation per INV-006.  // edikt-guard:allow
var cheatRateIDRe = regexp.MustCompile(`^(ADR|INV)-\d{3,}$|^[a-z][a-z0-9-]{0,79}$`)

var cheatRateCmd = &cobra.Command{
	Use:   "cheat-rate [id]",
	Short: "Run the adversarial cheat-rate benchmark against a sidecar's verifies",
	Long: `Dispatch an adversary agent per verify: command in the addressed sidecar
and report how often the verify accepts behavior it should reject
(the cheat-rate). Target: <20% on the held-out corpus.

Run modes:
  cheat-rate <id>           Run against a single sidecar.
  cheat-rate --all          Run against every sidecar under the configured
                            governance paths.

Caching: results are keyed by (sidecar-content-hash, verify-text-hash)
so repeated runs over an unchanged sidecar replay from cache. Pass
--refresh to force re-dispatch.

Stub mode (testing): set EDIKT_CHEAT_RATE_STUB=1 to skip adversary
dispatch entirely and load canned verdicts from
test/fixtures/benchmark-stubs/<id>.json. The fixture must conform to
templates/schemas/cheat-rate-report.v1.schema.json.

Exit codes:
  0 — run completed
  1 — sandbox / dispatch error
  2 — sidecar id specified but no matching .edikt.yaml found
  3 — invalid or missing arguments / flags`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		err := runCheatRate(cmd, args)
		exitFromExitErr(err)
		return err
	},
}

func init() {
	cheatRateCmd.Flags().BoolVar(&cheatRateAll, "all", false,
		"run against every sidecar under the configured governance paths")
	cheatRateCmd.Flags().BoolVar(&cheatRateRefresh, "refresh", false,
		"invalidate the result cache and re-dispatch the adversary")
	cheatRateCmd.Flags().StringVar(&cheatRateAdversaryModel, "adversary-model", "claude-opus-4-7",
		"override the adversary model (default: claude-opus-4-7)")
	benchmarkCmd.AddCommand(cheatRateCmd)
}

func runCheatRate(cmd *cobra.Command, args []string) error {
	// Argument validation. Either a positional id OR --all is required;
	// passing neither (or both) is exit 3 per ADR-040.  // edikt-guard:allow
	if len(args) == 0 && !cheatRateAll {
		return &exitErr{code: 3, msg: "cheat-rate: provide a sidecar id or pass --all"}
	}
	if len(args) > 0 && cheatRateAll {
		return &exitErr{code: 3, msg: "cheat-rate: pass either a sidecar id or --all, not both"}
	}
	if len(args) == 1 {
		if !cheatRateIDRe.MatchString(args[0]) {
			return &exitErr{code: 3, msg: fmt.Sprintf("cheat-rate: invalid sidecar id %q (expected ADR-NNN, INV-NNN, or a guideline slug)", args[0])}
		}
	}
	if cheatRateAdversaryModel == "" {
		return &exitErr{code: 3, msg: "cheat-rate: --adversary-model must not be empty"}
	}

	// Stub mode: EDIKT_CHEAT_RATE_STUB=1 short-circuits dispatch and
	// loads canned fixtures from test/fixtures/benchmark-stubs/. Used
	// for the exit-code contract tests and downstream tooling that
	// shouldn't burn Opus tokens.
	if os.Getenv("EDIKT_CHEAT_RATE_STUB") == "1" {
		return runCheatRateStub(cmd, args)
	}

	// ADR-044: the binary no longer dispatches the adversary. Refuse here,
	// at the entry point, rather than letting the run reach the dispatcher
	// seam and fail per-verify — a partially-scored report is worse than no
	// report, because it looks like a measurement.
	//
	// The deterministic half (sandbox creation, verify execution, verdict
	// aggregation, report persistence) stays in tier-2 and is reachable
	// through the tier-1 flow in commands/gov/benchmark.md.
	// (ref: INV-012 — tier-2 Go binaries must not dispatch an LLM)
	return &exitErr{code: 3, msg: "cheat-rate: this binary does not dispatch an LLM.\n" +
		"The adversary is dispatched from tier-1: run /edikt:gov:benchmark, which dispatches\n" +
		"cheat-rate-adversary per verify and feeds the results back for scoring.\n" +
		"For a token-free pipeline check: EDIKT_CHEAT_RATE_STUB=1 bin/edikt gov benchmark cheat-rate --all"}
}

// runCheatRateProduction is the Phase 4 production wiring. Flow:
//
//  1. Resolve project root, state dir, sandboxes dir, cache dir.
//  2. Discover sidecars via sidecar.Discover; filter to either a
//     single id (positional arg) or all (--all).
//  3. For each sidecar, iterate Directives[] and Prohibitions[] —
//     score every behavioral verify; emit a Report.
//  4. Write the report to .edikt/state/benchmark/ via WriteReport.
//  5. On --all, gate exit 1 when aggregate inconclusive_rate > 0.10.
//
// Returns:
//
//	exit 2 — sidecar id specified but no matching .edikt.yaml found
//	exit 1 — sandbox/dispatch/state-dir failure, or inconclusive gate
//	exit 0 — clean run (report(s) written)
func runCheatRateProduction(cmd *cobra.Command, args []string) error {
	projectRoot, err := os.Getwd()
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate: getwd: %v", err)}
	}
	stateDir := filepath.Join(projectRoot, ".edikt", "state")
	sandboxesDir := filepath.Join(stateDir, "benchmark", "sandboxes")
	cacheDir := filepath.Join(stateDir, "benchmark", "cache")

	templatePath, err := resolveAdversaryTemplate(projectRoot)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate: %v", err)}
	}

	pairs, err := selectSidecarPairs(projectRoot, args, cheatRateAll)
	if err != nil {
		return err
	}

	now := time.Now
	if nowForTests != nil {
		now = nowForTests
	}

	// Aggregate across all sidecars in the run; gate on --all only.
	var aggregateTotal, aggregateInconclusive int
	ctx := cmd.Context()
	if ctx == nil {
		ctx = context.Background()
	}

	for _, pair := range pairs {
		if pair.Sidecar == nil {
			continue
		}
		report := scoreSidecar(ctx, pair, scoreSidecarOpts{
			SandboxesDir:   sandboxesDir,
			CacheDir:       cacheDir,
			SourceDir:      projectRoot,
			TemplatePath:   templatePath,
			AdversaryModel: cheatRateAdversaryModel,
			Refresh:        cheatRateRefresh,
			Now:            now,
		})
		aggregateTotal += report.Summary.Total
		aggregateInconclusive += report.Summary.Inconclusive

		// Persist the report. Failure to write is fatal — the operator
		// needs the artifact for inspection.
		outPath, werr := cheatrate.WriteReport(stateDir, &report)
		if werr != nil {
			return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate: write report for %s: %v", pair.ArtifactID, werr)}
		}

		// Print the report JSON + the output path. Matches the
		// stub-mode contract.
		raw, _ := json.MarshalIndent(report, "", "  ")
		fmt.Fprintln(cmd.OutOrStdout(), string(raw))
		fmt.Fprintln(cmd.OutOrStdout(), "Report written to: "+outPath)
	}

	// AC-4.6: --all + inconclusive_rate > 0.10 → exit 1 with the
	// ADR-040 §6.6 "adversary starved" message. Single-id runs are  // edikt-guard:allow
	// not subject to the gate (too small a sample).
	if cheatRateAll && aggregateTotal > 0 {
		rate := float64(aggregateInconclusive) / float64(aggregateTotal)
		if rate > inconclusiveRateThreshold {
			return &exitErr{code: 1, msg: fmt.Sprintf(
				"cheat-rate: aggregate inconclusive_rate %.2f exceeds the %.2f threshold (%d of %d verifies inconclusive) — adversary likely starved",
				rate, inconclusiveRateThreshold, aggregateInconclusive, aggregateTotal)}
		}
	}

	return nil
}

// baselinePackDir is the canonical location of the cheat-rate
// baseline pack — orphan .edikt.yaml sidecars (no .md parent) that
// ship with edikt as a starter corpus for adopters. They are
// gov-schema-shaped, carry verify_kind: behavioral on real
// directives, and are part of the SR-008 corpus union.
const baselinePackDir = "templates/sidecars/baseline"

// selectSidecarPairs resolves the positional id or --all flag to a
// slice of sidecar.Pair entries to score. Exit-2 on single-id miss
// preserves the Plan C contract.
//
// Corpus scope (SR-008, full coverage after Plans E + F + G):
//
//	(a) governance sidecars under paths.{decisions,invariants,guidelines}
//	(b) SDLC sidecars under paths.{specs,prds,plans}                    [Plan G]
//	(c) baseline pack at templates/sidecars/baseline/                   [Plan F]
//
// SDLC entries (SPEC requirements, PRD requirements, plan-criteria
// entries) are adapted into gov-shaped Directive structs at discovery
// time so the cheat-rate machinery iterates them uniformly. Only
// entries with `verify_kind: behavioral` are surfaced — structural /
// tooling / unset entries are silently dropped.
func selectSidecarPairs(projectRoot string, args []string, all bool) ([]sidecar.Pair, error) {
	govDirs := govrun.GovernanceDirs(projectRoot)
	govPairs, err := sidecar.Discover(projectRoot, govDirs)
	if err != nil {
		return nil, &exitErr{code: 2, msg: fmt.Sprintf("cheat-rate: discover sidecars: %v", err)}
	}
	baselinePairs, err := discoverBaselinePackSidecars(filepath.Join(projectRoot, baselinePackDir))
	if err != nil {
		return nil, &exitErr{code: 2, msg: fmt.Sprintf("cheat-rate: discover baseline pack: %v", err)}
	}
	sdlcPairs, err := discoverSDLCSidecars(projectRoot)
	if err != nil {
		return nil, &exitErr{code: 2, msg: fmt.Sprintf("cheat-rate: discover SDLC sidecars: %v", err)}
	}

	allPairs := append([]sidecar.Pair{}, govPairs...)
	allPairs = append(allPairs, baselinePairs...)
	allPairs = append(allPairs, sdlcPairs...)

	if all {
		var withSidecar []sidecar.Pair
		for _, p := range allPairs {
			if p.Sidecar != nil && !p.Skip {
				withSidecar = append(withSidecar, p)
			}
		}
		return withSidecar, nil
	}
	// Single-id flow — args[0] already passed the regex validator.
	// Match across all three corpora; gov sidecars use ADR-NNN/INV-NNN
	// ids, baseline pack uses slugs, SDLC sidecars use SPEC-NNN /
	// PRD-NNN / plan-slug.
	targetID := args[0]
	for _, p := range allPairs {
		if p.ArtifactID == targetID && p.Sidecar != nil {
			return []sidecar.Pair{p}, nil
		}
	}
	return nil, &exitErr{code: 2, msg: fmt.Sprintf(
		"cheat-rate: sidecar %q not found in governance dirs %v, baseline pack %s, or SDLC dirs",
		targetID, govDirs, baselinePackDir)}
}

// discoverBaselinePackSidecars walks templates/sidecars/baseline/
// looking for orphan .edikt.yaml files (no .md parent — the baseline
// pack ships as a sidecar-only corpus). Returns one sidecar.Pair per
// loadable file with ArtifactID derived from the filename.
//
// An absent baseline directory is not an error — adopter projects
// that haven't installed the pack just see governance-only `--all`.
// Sidecars that fail to load are recorded with LoadErr set so the
// caller can decide whether to skip or surface; this implementation
// silently drops them (consistent with sidecar.Discover's behavior
// for unparseable gov sidecars).
func discoverBaselinePackSidecars(dir string) ([]sidecar.Pair, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read baseline dir %q: %w", dir, err)
	}
	var pairs []sidecar.Pair
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, ".edikt.yaml") {
			continue
		}
		sidecarPath := filepath.Join(dir, name)
		sc, err := sidecar.Load(sidecarPath)
		if err != nil {
			// Malformed sidecar — skip silently so one bad file
			// doesn't poison the whole --all run.
			continue
		}
		artifactID := strings.TrimSuffix(name, ".edikt.yaml")
		pairs = append(pairs, sidecar.Pair{
			ParentPath:  "", // baseline pack sidecars have no .md parent
			SidecarPath: sidecarPath,
			ArtifactID:  artifactID,
			Sidecar:     sc,
		})
	}
	return pairs, nil
}

// scoreSidecarOpts groups arguments to scoreSidecar — keeps the
// signature stable as more knobs accrete.
type scoreSidecarOpts struct {
	SandboxesDir   string
	CacheDir       string
	SourceDir      string
	TemplatePath   string
	AdversaryModel string
	Refresh        bool
	Now            func() time.Time
}

// scoreSidecar iterates Directives[] and Prohibitions[] under one
// sidecar, scores every behavioral verify via the per-verify
// orchestrator (Phase 3), and assembles the cheatrate.Report.
//
// Structural and tooling verifies are skipped per ADR-040 §6.3 —  // edikt-guard:allow
// they are not subject to the cheat-rate measurement.
//
// SR-008 compliance: both arrays are walked. VerifyID is set to
// "directive[N]" or "prohibition[N]" to disambiguate in the report.
func scoreSidecar(ctx context.Context, pair sidecar.Pair, opts scoreSidecarOpts) cheatrate.Report {
	verifies := make([]cheatrate.Verify, 0)

	for i, d := range pair.Sidecar.Directives {
		if d.VerifyKind != "behavioral" {
			continue
		}
		v := scoreOneVerify(ctx, scoreOneVerifyOpts{
			SidecarPath:         pair.SidecarPath,
			SidecarID:           pair.ArtifactID,
			VerifyID:            fmt.Sprintf("directive[%d]", i),
			VerifyIdx:           i,
			Intent:              d.Intent,
			FalsifyingObserv:    d.FalsifyingObservation,
			VerifyCommand:       d.Verify,
			NegativeFixturePath: d.NegativeFixturePath,
			SandboxesDir:        opts.SandboxesDir,
			CacheDir:            opts.CacheDir,
			SourceDir:           opts.SourceDir,
			TemplatePath:        opts.TemplatePath,
			AdversaryModel:      opts.AdversaryModel,
			Refresh:             opts.Refresh,
		})
		verifies = append(verifies, v)
	}
	for i, p := range pair.Sidecar.Prohibitions {
		if p.VerifyKind != "behavioral" {
			continue
		}
		v := scoreOneVerify(ctx, scoreOneVerifyOpts{
			SidecarPath:         pair.SidecarPath,
			SidecarID:           pair.ArtifactID,
			VerifyID:            fmt.Sprintf("prohibition[%d]", i),
			VerifyIdx:           i,
			Intent:              p.Intent,
			FalsifyingObserv:    p.FalsifyingObservation,
			VerifyCommand:       p.Verify,
			NegativeFixturePath: p.NegativeFixturePath,
			SandboxesDir:        opts.SandboxesDir,
			CacheDir:            opts.CacheDir,
			SourceDir:           opts.SourceDir,
			TemplatePath:        opts.TemplatePath,
			AdversaryModel:      opts.AdversaryModel,
			Refresh:             opts.Refresh,
		})
		verifies = append(verifies, v)
	}

	summary := computeSummary(verifies)

	return cheatrate.Report{
		SchemaVersion:  1,
		SidecarID:      pair.ArtifactID,
		RanAt:          opts.Now().UTC().Format(time.RFC3339),
		AdversaryModel: opts.AdversaryModel,
		Verifies:       verifies,
		Summary:        summary,
	}
}

// scoreOneVerifyOpts groups the inputs scoreOneVerify needs to look
// up cache / dispatch the per-verify orchestrator / write back to
// cache. The struct exists only inside this file; it is not exported.
type scoreOneVerifyOpts struct {
	SidecarPath         string
	SidecarID           string
	VerifyID            string
	VerifyIdx           int
	Intent              string
	FalsifyingObserv    string
	VerifyCommand       string
	NegativeFixturePath string
	SandboxesDir        string
	CacheDir            string
	SourceDir           string
	TemplatePath        string
	AdversaryModel      string
	Refresh             bool
}

// scoreOneVerify wraps the cache-lookup / dispatch / cache-store loop
// around cheatrate.RunCheatRateForVerify. The cache key is derived
// from (sidecar-content-hash, verify-text-hash) per AC-4.3; on a
// hit, the cached verdict is reused without dispatching the
// adversary. --refresh bypasses the lookup but still writes back.
func scoreOneVerify(ctx context.Context, opts scoreOneVerifyOpts) cheatrate.Verify {
	sidecarHash := hashFile(opts.SidecarPath)
	verifyHash := hashString(opts.VerifyCommand)
	cacheKey := cheatrate.CacheKey(sidecarHash, verifyHash)

	if !opts.Refresh && opts.CacheDir != "" {
		if cached, err := cheatrate.CacheGet(opts.CacheDir, cacheKey); err == nil && cached != nil {
			for _, v := range cached.Verifies {
				if v.VerifyID == opts.VerifyID {
					return v
				}
			}
		}
	}

	runOpts := cheatrate.RunOpts{
		SidecarID:             opts.SidecarID,
		VerifyIdx:             opts.VerifyIdx,
		VerifyID:              opts.VerifyID,
		Intent:                opts.Intent,
		FalsifyingObservation: opts.FalsifyingObserv,
		VerifyCommand:         opts.VerifyCommand,
		NegativeFixturePath:   opts.NegativeFixturePath,
		AdversaryModel:        opts.AdversaryModel,
		TemplatePath:          opts.TemplatePath,
		SandboxesDir:          opts.SandboxesDir,
		SourceDir:             opts.SourceDir,
		Dispatcher:            dispatcherForTests,
		VerifyRunner:          verifyRunnerForTests,
	}
	verify, err := cheatrate.RunCheatRateForVerify(ctx, runOpts)
	if err != nil {
		// Per-verify orchestration returned a fatal error (bad
		// opts, missing source). Record as inconclusive so the
		// overall report stays well-formed; the operator inspects
		// the per-verify trace to diagnose.
		return cheatrate.Verify{
			VerifyID:   opts.VerifyID,
			Intent:     opts.Intent,
			VerifyKind: "behavioral",
			Verdict:    cheatrate.VerdictInconclusive,
			SandboxPath: filepath.Join(opts.SandboxesDir, opts.SidecarID,
				fmt.Sprintf("%d", opts.VerifyIdx)),
		}
	}

	// Cache the verify for subsequent runs unless --refresh was set
	// (we still want to refresh the cache contents on --refresh).
	if opts.CacheDir != "" {
		cacheReport := &cheatrate.Report{
			SchemaVersion:  1,
			SidecarID:      opts.SidecarID,
			AdversaryModel: opts.AdversaryModel,
			Verifies:       []cheatrate.Verify{verify},
			Summary:        computeSummary([]cheatrate.Verify{verify}),
		}
		_ = cheatrate.CachePut(opts.CacheDir, cacheKey, cacheReport)
	}
	return verify
}

// computeSummary aggregates per-verify verdicts into the Summary
// envelope. cheat_rate uses (total - inconclusive) as denominator per
// ADR-040 §6.6 — inconclusive verifies do NOT count against the rate.  // edikt-guard:allow
// inconclusive_rate uses total as denominator (how much of the run
// failed to converge).
func computeSummary(verifies []cheatrate.Verify) cheatrate.Summary {
	s := cheatrate.Summary{Total: len(verifies)}
	for _, v := range verifies {
		switch v.Verdict {
		case cheatrate.VerdictCheated:
			s.Cheated++
		case cheatrate.VerdictInconclusive:
			s.Inconclusive++
		}
	}
	denominator := s.Total - s.Inconclusive
	if denominator > 0 {
		s.CheatRate = float64(s.Cheated) / float64(denominator)
	}
	if s.Total > 0 {
		s.InconclusiveRate = float64(s.Inconclusive) / float64(s.Total)
	}
	return s
}

// resolveAdversaryTemplate locates the cheat-rate-adversary prompt
// template, honoring the project-template override pattern (ADR-005):  // edikt-guard:allow
//
//  1. `<projectRoot>/.edikt/templates/agents/cheat-rate-adversary.md`
//  2. `<projectRoot>/templates/agents/cheat-rate-adversary.md`
//
// Returns an error if neither resolves to a readable file. Plan C
// Phase 1 ships #2 under the edikt repo; #1 is the adopter override
// slot.
func resolveAdversaryTemplate(projectRoot string) (string, error) {
	candidates := []string{
		filepath.Join(projectRoot, ".edikt", "templates", "agents", "cheat-rate-adversary.md"),
		filepath.Join(projectRoot, "templates", "agents", "cheat-rate-adversary.md"),
	}
	for _, p := range candidates {
		if info, err := os.Stat(p); err == nil && !info.IsDir() {
			return p, nil
		}
	}
	return "", fmt.Errorf("cheat-rate-adversary template not found (looked under .edikt/templates/agents/ and templates/agents/)")
}

// hashFile returns the hex SHA-256 of the file at path. Returns the
// empty string when the file is unreadable — falling back to no
// cache reuse rather than failing the whole run.
func hashFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return hashString(string(raw))
}

// hashString returns the hex SHA-256 of s.
func hashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// runCheatRateStub loads stub fixtures for one or all sidecars and
// prints them to stdout. Exit codes still match the contract: missing
// fixture for a single id → exit 2; corrupt fixture → exit 1.
func runCheatRateStub(cmd *cobra.Command, args []string) error {
	stubDir := stubFixturesDir()

	if cheatRateAll {
		entries, err := os.ReadDir(stubDir)
		if err != nil {
			return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate stub: read fixtures dir %s: %v", stubDir, err)}
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if filepath.Ext(e.Name()) != ".json" {
				continue
			}
			names = append(names, e.Name())
		}
		sort.Strings(names)
		for _, name := range names {
			path := filepath.Join(stubDir, name)
			if err := emitStubReport(cmd, path); err != nil {
				return err
			}
		}
		return nil
	}

	// Single-id path. args[0] is already validated against the regex.
	id := args[0]
	path := filepath.Join(stubDir, id+".json")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return &exitErr{code: 2, msg: fmt.Sprintf("cheat-rate stub: no fixture for sidecar id %q at %s", id, path)}
	}
	return emitStubReport(cmd, path)
}

// emitStubReport reads a stub fixture, parses it as a cheat-rate
// Report, writes it to the canonical `.edikt/state/benchmark/<id>/`
// path via cheatrate.WriteReport, and surfaces both the fixture
// contents and the written path on stdout.
//
// This keeps stub-mode end-to-end: the integration test under
// test/integration/benchmark-cheat-rate.sh can rely on a real report
// file existing on disk without needing an LLM dispatch (ADR-040  // edikt-guard:allow
// stub-mode contract).
//
// Returns exit 1 on read / parse / write failure.
func emitStubReport(cmd *cobra.Command, path string) error {
	raw, err := os.ReadFile(path)
	if err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate stub: read %s: %v", path, err)}
	}
	var report cheatrate.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate stub: parse %s: %v", path, err)}
	}
	// Print the fixture contents — preserves the Phase 2 contract that
	// the report JSON is visible on stdout.
	fmt.Fprintln(cmd.OutOrStdout(), string(raw))

	// Write a real report file under `.edikt/state/benchmark/`. The
	// caller's working directory provides the project root; we resolve
	// stateDir relative to that. INV-007: WriteReport never touches  // edikt-guard:allow
	// the host's `~/.claude/` directory — output is rooted at cwd.
	stateDir := stubStateDir()
	out, werr := cheatrate.WriteReport(stateDir, &report)
	if werr != nil {
		return &exitErr{code: 1, msg: fmt.Sprintf("cheat-rate stub: write report: %v", werr)}
	}
	fmt.Fprintln(cmd.OutOrStdout(), "Report written to: "+out)
	return nil
}

// stubStateDir returns the directory under which WriteReport persists
// reports. In production this is the project's `.edikt/state` dir;
// stub-mode uses the same path so the integration test can verify a
// report was written.
func stubStateDir() string {
	if cwd, err := os.Getwd(); err == nil {
		return filepath.Join(cwd, ".edikt", "state")
	}
	return filepath.Join(".edikt", "state")
}

// stubFixturesDir returns the absolute path to
// test/fixtures/benchmark-stubs/ resolved relative to the working
// directory's git/module root. Falls back to a path next to this
// source file when invoked from a test (`go test` cwd is the package
// dir).
func stubFixturesDir() string {
	// First try cwd/test/fixtures/benchmark-stubs — production path.
	if cwd, err := os.Getwd(); err == nil {
		// Walk up looking for a `test/fixtures/benchmark-stubs` dir;
		// this lets the binary work from any subdir of the repo.
		dir := cwd
		for i := 0; i < 8; i++ {
			candidate := filepath.Join(dir, "test", "fixtures", "benchmark-stubs")
			if info, err := os.Stat(candidate); err == nil && info.IsDir() {
				return candidate
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	// Fallback: locate via this source file (tools/edikt/cmd/gov/...) and
	// walk up to the repo root.
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
		return filepath.Join(repoRoot, "test", "fixtures", "benchmark-stubs")
	}
	// Last resort — return a relative path; the caller will exit 1
	// with a clear ENOENT message.
	return filepath.Join("test", "fixtures", "benchmark-stubs")
}
