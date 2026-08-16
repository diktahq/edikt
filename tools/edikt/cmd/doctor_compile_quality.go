package cmd

// doctor_compile_quality.go — SPEC-009 Plan H (SR-016).  // edikt-guard:allow
//
// Surfaces the latest compiled-governance quality grade produced by
// `bin/edikt gov grade-compile` and persisted via gradecompile.WriteReport
// at the canonical flat layout
//
//	<projectRoot>/.edikt/state/compile-quality/<fs-safe-ts>.json
//
// (One governance tree per project, so the layout is flat — many
// timestamped reports, newest selected by lexicographic sort.) The check
// prints the overall + per-dimension scores and warns on any dimension
// below the floor. It NEVER blocks doctor's overall exit — the grade is
// an editorial-quality signal, not enforcement (mirrors the advisory
// posture of runCheatRateBenchmarkCheck).

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/diktahq/edikt/tools/edikt/internal/gradecompile"
)

func runCompileQualityCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	io.WriteString(w, "  ── Compiled-governance quality ────────────────\n")

	dir := filepath.Join(projectRoot, ".edikt", "state", "compile-quality")
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) == 0 {
		io.WriteString(w, "  No compile-quality grade yet. Run bin/edikt gov grade-compile after gov compile.\n")
		return 0, true
	}

	// Quarantined reports (*.json.void) are grades a known-broken grader
	// produced — e.g. the pre-ADR-044 in-binary dispatcher, which failed to
	// unwrap `claude --output-format json`'s result envelope and so scored
	// every dimension 0. They are excluded from "latest", but NEVER silently:
	// a blank where a metric used to be reads as "never measured", which is a
	// different and wrong claim. The count and the reason stay visible.
	var names []string
	quarantined := 0
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if filepath.Ext(e.Name()) == ".void" {
			quarantined++
			continue
		}
		if filepath.Ext(e.Name()) != ".json" {
			continue
		}
		names = append(names, e.Name())
	}
	if len(names) == 0 {
		if quarantined > 0 {
			// (ref: ADR-044 — void quarantine rather than a silent-zero dispatcher)
			fmt.Fprintf(w, "  No valid compiled-governance grade — %d report(s) quarantined as void. "+
				"Re-run bin/edikt gov grade-compile once the "+
				"tier-1 dispatch has landed.\n", quarantined)
			return 1, true
		}
		io.WriteString(w, "  No compile-quality grade yet. Run bin/edikt gov grade-compile after gov compile.\n")
		return 0, true
	}
	if quarantined > 0 {
		// (ref: ADR-044 — void quarantine rather than a silent-zero dispatcher)
		fmt.Fprintf(w, "  note: %d report(s) quarantined as void "+
			"and excluded from the grade below.\n", quarantined)
	}
	sort.Strings(names)
	latest := filepath.Join(dir, names[len(names)-1])

	data, err := os.ReadFile(latest)
	if err != nil {
		io.WriteString(w, "  WARN: failed to read "+latest+": "+err.Error()+"\n")
		return 1, true
	}
	var report gradecompile.Report
	if err := json.Unmarshal(data, &report); err != nil {
		io.WriteString(w, "  WARN: failed to parse "+latest+": "+err.Error()+"\n")
		return 1, true
	}

	io.WriteString(w, fmt.Sprintf(
		"  Latest grade: overall %d/10  (coherence %d, conciseness %d, signal-to-noise %d, description %d, tiering %d, no-double-loading %d)\n",
		report.Overall, report.Scores.Coherence, report.Scores.Conciseness,
		report.Scores.SignalToNoise, report.Scores.DescriptionQuality,
		report.Scores.TierAssignment, report.Scores.NoDoubleLoading))

	// scoreFloor is the dimension score below which doctor flags the
	// surface for attention. Advisory only — a low score tells the
	// operator to re-author or re-compile, it never gates doctor's exit.
	const scoreFloor = 6
	type dim struct {
		name  string
		score int
	}
	for _, d := range []dim{
		{"coherence", report.Scores.Coherence},
		{"conciseness", report.Scores.Conciseness},
		{"signal-to-noise", report.Scores.SignalToNoise},
		{"description quality", report.Scores.DescriptionQuality},
		{"tier assignment", report.Scores.TierAssignment},
		{"no double loading", report.Scores.NoDoubleLoading},
	} {
		if d.score < scoreFloor {
			io.WriteString(w, fmt.Sprintf(
				"  ⚠  %s scored %d/10 (below %d) — see findings in %s\n",
				d.name, d.score, scoreFloor, latest))
			warns++
		}
	}
	if report.Overall < scoreFloor {
		io.WriteString(w, fmt.Sprintf(
			"  ⚠  overall %d/10 below %d — tighten compiled governance and re-grade\n",
			report.Overall, scoreFloor))
		warns++
	}
	return warns, true
}
