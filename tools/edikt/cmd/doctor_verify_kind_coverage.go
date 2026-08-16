package cmd

// doctor_verify_kind_coverage.go — Phase 10 of SPEC-009 Plan A.  // edikt-guard:allow
//
// Adds two soft-signal sections to `edikt doctor`:
//
//   - "Verify Kind Coverage" — walks every gov sidecar in the project and
//     emits a one-line tally of structural / tooling / behavioral counts.
//     Each behavioral entry that lacks a human_approved_at timestamp is
//     surfaced as a WARN. SPEC-009 §Mechanism-Quality axis.  // edikt-guard:allow
//
//   - "Cheat-rate benchmarks" — checks `.edikt/state/benchmark/` for any
//     committed cheat-rate report JSON. Empty / absent → emit a Plan-C
//     stub line so the user sees the baseline gap.
//
// Both sections are informational. They never increment errN. The hard
// gate for behavioral approval lives in Phase B compile and `bin/edikt
// sidecar approve` (ADR-039); the doctor surface is signal, not enforcement.  // edikt-guard:allow

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/benchmark/cheatrate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// runVerifyKindCoverageCheck walks every gov sidecar under the configured
// artifact dirs and emits a per-corpus tally. Returns (warns, ran). ran is
// false when no gov sidecars exist (so doctor stays quiet on non-edikt
// projects). Behavioral entries missing human_approved_at increment warns.
func runVerifyKindCoverageCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	dirs := resolveArtifactDirs(projectRoot)
	govDirs := []string{dirs.decisions, dirs.invariants, dirs.guidelines}

	var (
		structural   int
		tooling      int
		behavioral   int
		missingApprv []string // "ID#directive[N]" of behavioral entries lacking human_approved_at
		sidecarsSeen int
		// unreadable holds sidecars that exist on disk but would not
		// load, and unreadableDirs the artifact dirs that would not open.
		// Both used to be dropped with a bare `continue`, which made the
		// tally describe only the files that still parsed — so coverage
		// improved as data rotted. A control that HAD a subject and could
		// not observe it has to say so (INV-013).
		unreadable     []string
		unreadableDirs []string
	)

	for _, dir := range govDirs {
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			// A dir that does not exist is not a subject — plenty of
			// projects have no guidelines/. A dir that exists and will
			// not open is a subject we failed to read, and hiding it
			// would understate the corpus without saying so.
			if !os.IsNotExist(err) {
				unreadableDirs = append(unreadableDirs, dir)
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
			sc, err := sidecar.Load(path)
			if err != nil {
				unreadable = append(unreadable, strings.TrimSuffix(name, ".edikt.yaml"))
				continue
			}
			sidecarsSeen++
			id := strings.TrimSuffix(name, ".edikt.yaml")

			for i, d := range sc.Directives {
				switch d.VerifyKind {
				case "structural":
					structural++
				case "tooling":
					tooling++
				case "behavioral":
					behavioral++
					if strings.TrimSpace(d.HumanApprovedAt) == "" {
						missingApprv = append(missingApprv, id+"#directive["+itoa(i)+"]")
					}
				}
			}
			for i, p := range sc.Prohibitions {
				switch p.VerifyKind {
				case "structural":
					structural++
				case "tooling":
					tooling++
				case "behavioral":
					behavioral++
					if strings.TrimSpace(p.HumanApprovedAt) == "" {
						missingApprv = append(missingApprv, id+"#prohibition["+itoa(i)+"]")
					}
				}
			}
		}
	}

	// Stay silent only when there was genuinely nothing to look at.
	// sidecarsSeen == 0 used to be that test, but it is also what a
	// wholly-corrupt corpus produces — so doctor went quiet on a project
	// whose governance had rotted through, looking exactly like a project
	// that had never adopted edikt. Having found files and failed to read
	// them is a result, and it gets reported (INV-013).
	total := sidecarsSeen + len(unreadable)
	if total == 0 && len(unreadableDirs) == 0 {
		return 0, false
	}

	io.WriteString(w, "  ── Verify Kind Coverage ───────────────────────\n")
	io.WriteString(w, "  Verify Kind Coverage: structural="+itoa(structural)+
		" tooling="+itoa(tooling)+" behavioral="+itoa(behavioral)+
		" (read "+itoa(sidecarsSeen)+" of "+itoa(total)+" sidecars)\n")

	// Name what was missed, and count it as a warning. A denominator alone
	// would leave the reader to subtract; the fraction has to move with the
	// corpus and the shortfall has to be attributable to a file.
	sort.Strings(unreadable)
	for _, id := range unreadable {
		io.WriteString(w, "  WARN: sidecar "+id+" is unreadable — excluded from the "+
			"coverage tally above; run `bin/edikt gov compile --check` to see why\n")
		warns++
	}
	sort.Strings(unreadableDirs)
	for _, d := range unreadableDirs {
		io.WriteString(w, "  WARN: artifact directory "+d+" could not be read — "+
			"any sidecars inside it are absent from the tally above\n")
		warns++
	}

	for _, ref := range missingApprv {
		io.WriteString(w, "  WARN: behavioral verify "+ref+
			" lacks human_approved_at — run `bin/edikt sidecar approve`\n")
		warns++
	}

	// Always emit the behavioral-approval policy line so adopters see the
	// contract even when no behavioral directives exist. SPEC-009 Plan B  // edikt-guard:allow
	// Phase 5 AC-5.5: doctor surfaces the human_approved_at requirement
	// independent of current corpus state.
	if len(missingApprv) == 0 {
		io.WriteString(w, "  behavioral approval policy: every behavioral verify requires `human_approved_at` "+
			"via `bin/edikt sidecar approve`\n")
	}

	return warns, true
}

// runCheatRateBenchmarkCheck reports cheat-rate benchmark coverage under
// .edikt/state/benchmark/. The reports are produced by `bin/edikt gov
// benchmark cheat-rate` (ADR-040, SPEC-009 Plan C/E) and persisted via  // edikt-guard:allow
// cheatrate.WriteReport at the canonical layout
//
//	<projectRoot>/.edikt/state/benchmark/<sidecar-id>/<fs-safe-ts>.json
//
// (One subdirectory per sidecar; many timestamped reports per
// subdirectory.) The check walks that layout — selecting the most
// recent report per sidecar so doctor scales as benchmark history
// accumulates — and emits two classes of warning:
//
//  1. cheat_rate >= 0.20 (SPEC-009 SAC-008 soft ceiling) — the  // edikt-guard:allow
//     directive's verify is too easily side-stepped by the adversary.
//
//  2. adversary_model != "claude-opus-4-7" — ADR-040 §7 locks the  // edikt-guard:allow
//     adversary at Opus 4.7. A report produced with a weaker model
//     under-counts cheating, so the metric is unsafe to act on
//     (SPEC-009 Plan E AC-5.3).  // edikt-guard:allow
//
// Empty / absent emits a hint pointing to the cheat-rate subcommand so
// the operator knows how to bootstrap the baseline. The section never
// blocks doctor's overall exit — cheat-rate is signal, not enforcement.
func runCheatRateBenchmarkCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	io.WriteString(w, "  ── Cheat-rate benchmarks ──────────────────────\n")

	benchDir := filepath.Join(projectRoot, ".edikt", "state", "benchmark")
	entries, err := os.ReadDir(benchDir)
	if err != nil || len(entries) == 0 {
		io.WriteString(w, "  No cached cheat-rate reports. Run bin/edikt gov benchmark cheat-rate <id> to generate one.\n")
		return 0, true
	}

	// Walk the canonical layout: each subdirectory is one sidecar, each
	// .json file inside is one timestamped report. Pick the most-recent
	// report per sidecar (lexicographic sort works because timestamps
	// are fs-safe RFC3339-shaped — newer sorts later).
	type latestReport struct {
		path    string
		sidecar string
	}
	var latests []latestReport
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		sidecarDir := filepath.Join(benchDir, e.Name())
		jsonEntries, err := os.ReadDir(sidecarDir)
		if err != nil {
			continue
		}
		var jsonNames []string
		for _, je := range jsonEntries {
			if je.IsDir() {
				continue
			}
			if filepath.Ext(je.Name()) != ".json" {
				continue
			}
			jsonNames = append(jsonNames, je.Name())
		}
		if len(jsonNames) == 0 {
			continue
		}
		sort.Strings(jsonNames)
		latests = append(latests, latestReport{
			path:    filepath.Join(sidecarDir, jsonNames[len(jsonNames)-1]),
			sidecar: e.Name(),
		})
	}

	if len(latests) == 0 {
		io.WriteString(w, "  No cached cheat-rate reports. Run bin/edikt gov benchmark cheat-rate <id> to generate one.\n")
		return 0, true
	}

	io.WriteString(w, "  Cheat-rate benchmarks: "+itoa(len(latests))+" sidecar(s) with cached reports under .edikt/state/benchmark/\n")

	// The 0.20 threshold is the SPEC-009 SAC-008 soft ceiling on cheat  // edikt-guard:allow
	// rate (cheated / (total - inconclusive)). Anything at-or-above
	// the threshold means the directive's verify command is too
	// easily side-stepped by the adversary — operator should
	// re-author and re-benchmark.
	const cheatRateThreshold = 0.20
	// ADR-040 §7 locks the adversary at Opus 4.7. Older / weaker  // edikt-guard:allow
	// adversaries under-count cheating; warn loudly so operators
	// realise their report is comparing apples to oranges.
	const adversaryModelLock = "claude-opus-4-7"

	for _, lr := range latests {
		data, err := os.ReadFile(lr.path)
		if err != nil {
			io.WriteString(w, "  WARN: failed to read "+lr.path+": "+err.Error()+"\n")
			warns++
			continue
		}
		var report cheatrate.Report
		if err := json.Unmarshal(data, &report); err != nil {
			io.WriteString(w, "  WARN: failed to parse "+lr.path+": "+err.Error()+"\n")
			warns++
			continue
		}

		id := report.SidecarID
		if id == "" {
			id = lr.sidecar
		}

		if report.Summary.CheatRate >= cheatRateThreshold {
			io.WriteString(w, fmt.Sprintf(
				"  ⚠  %s: cheat_rate=%.2f (threshold %.2f) — run bin/edikt gov benchmark cheat-rate %s --refresh\n",
				id, report.Summary.CheatRate, cheatRateThreshold, id,
			))
			warns++
		}

		if report.AdversaryModel != "" && report.AdversaryModel != adversaryModelLock {
			io.WriteString(w, fmt.Sprintf(
				"  ⚠  %s: adversary_model=%q (benchmark locks at %q) — re-run with --adversary-model=%s for an apples-to-apples cheat rate\n",
				id, report.AdversaryModel, adversaryModelLock, adversaryModelLock,
			))
			warns++
		}
	}

	return warns, true
}
