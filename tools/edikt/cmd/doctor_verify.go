package cmd

// doctor_verify.go — Phase 12 of PLAN-sidecar-architecture.
//
// Adds a "Plan Verification" check group to `edikt doctor`. For each
// plan that has a sibling -criteria.yaml, walk its progress table and
// emit a WARN for every row marked `done` that lacks a recent passing
// verification report. "Recent" = newer than the most recent commit
// that touched the criteria sidecar (best-effort via git log).
//
// This is a soft check: it never increments errN and never causes
// doctor to exit non-zero. The point is informational pressure on
// stale phase-completions, not a hard gate.

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// runVerifyChecks emits warnings for ✅/done progress rows that lack a
// recent passing verification report. Returns warnings count and a
// `ran` flag (false when the project has no plans+criteria pairs).
// runVerifyChecks reports plan-phase staleness — ONE roll-up line per ACTIVE
// plan (FR-035).
//
// It used to print one line per unverified phase across every plan the
// project had ever had: 159 of 184 warning lines on the reference repo were
// this single finding, repeated, and the one signal worth reading was
// underneath them. A completed historical plan is not a finding at all —
// nobody is going to re-verify a plan that shipped — so it stays silent, and
// the active plan gets a count with its remedy.
func runVerifyChecks(projectRoot string, w io.Writer) (warns int, ran bool) {
	plans := discoverPlansWithCriteria(projectRoot)
	if len(plans) == 0 {
		return 0, false
	}
	io.WriteString(w, "  ── Plan Verification ──────────────────────────\n")

	stateDir := filepath.Join(projectRoot, ".edikt", "state", "verify")
	examined, reported := 0, 0
	for _, p := range plans {
		rows := parseProgressTable(p.planPath)
		if !planNeedsReport(rows) {
			// A plan whose every phase is done, with no override, is
			// history. Reporting it every run is what buried the finding
			// that mattered. An OVERRIDE is the exception: a phase flipped
			// without verification is an unverified claim whether or not the
			// plan has since finished, and completeness must not silence it.
			continue
		}
		examined++
		criteriaMTime, _ := lastCriteriaCommitTime(projectRoot, p.criteriaPath)
		criteriaMTime = criteriaMTime.Truncate(time.Second)

		var stalePhases, overriddenPhases []string
		for _, row := range rows {
			if !rowMarkedDone(row.status) {
				continue
			}
			if hasOverrideMarker(row.updated) {
				overriddenPhases = append(overriddenPhases, row.phase)
				continue
			}
			if ok, _ := hasRecentPassingReport(stateDir, p.planID, row.phase, criteriaMTime); !ok {
				stalePhases = append(stalePhases, row.phase)
			}
		}
		if len(stalePhases)+len(overriddenPhases) == 0 {
			continue
		}
		// ONE line, and it still names the phases: the roll-up exists to stop
		// the repetition, not to take away what the reader needs to act.
		detail := ""
		if len(overriddenPhases) > 0 {
			detail = ", " + itoa(len(overriddenPhases)) + " flipped with an override (phase " +
				strings.Join(overriddenPhases, ", ") + ")"
		}
		phases := "none"
		if len(stalePhases) > 0 {
			phases = "phase " + strings.Join(stalePhases, ", ")
		}
		io.WriteString(w, "  WARN: "+p.planID+": "+itoa(len(stalePhases))+
			" phase(s) without a recent passing report ("+phases+")"+detail+
			" — run: edikt verify "+p.planID+" --phase <n>\n")
		warns++
		reported++
	}
	// A check that looked and found nothing SAYS so. Silence here reads the
	// same as a check that did not run (INV-013).  // edikt-guard:allow
	if examined > 0 && reported == 0 {
		io.WriteString(w, "  [ok] All marked-done phases of the active plan(s) have a recent passing report.\n")
	}
	return warns, true
}

// planNeedsReport reports whether a plan is worth a line: it still has work,
// or one of its done rows was flipped with an override.
func planNeedsReport(rows []progressRow) bool {
	for _, r := range rows {
		if !rowMarkedDone(r.status) {
			return true
		}
		if hasOverrideMarker(r.updated) {
			return true
		}
	}
	return false
}

type planEntry struct {
	planID       string
	planPath     string
	criteriaPath string
}

// discoverPlansWithCriteria walks the conventional plan directories and
// returns each plan that has a criteria sidecar beside it.
func discoverPlansWithCriteria(projectRoot string) []planEntry {
	var out []planEntry
	for _, sub := range []string{
		"docs/internal/plans",
		"docs/plans",
		"docs/product/plans",
	} {
		dir := filepath.Join(projectRoot, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if !strings.HasPrefix(name, "PLAN-") || !strings.HasSuffix(name, ".md") {
				continue
			}
			planID := strings.TrimSuffix(strings.TrimPrefix(name, "PLAN-"), ".md")
			criteria := filepath.Join(dir, "PLAN-"+planID+"-criteria.yaml")
			if _, err := os.Stat(criteria); err != nil {
				continue
			}
			out = append(out, planEntry{
				planID:       planID,
				planPath:     filepath.Join(dir, name),
				criteriaPath: criteria,
			})
		}
	}
	return out
}

// progressRow captures the columns the doctor cares about: phase id,
// status, and the contents of the Updated cell (used to detect the
// `done (overrides: K)` marker).
type progressRow struct {
	phase   string
	status  string
	updated string
}

// progressRowRe matches the canonical progress-table rows of the form
// `| 1 | done | 2/5 | 2026-05-02 |`. The phase column is the first cell.
var progressRowRe = regexp.MustCompile(`^\|\s*([0-9]+[a-zA-Z]?)\s*\|\s*([^|]+?)\s*\|\s*[^|]*\s*\|\s*([^|]*?)\s*\|`)

// parseProgressTable scans a plan markdown file for progress-table rows.
// Returns an empty slice if no recognizable table is present.
func parseProgressTable(planPath string) []progressRow {
	f, err := os.Open(planPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	var rows []progressRow
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		// Skip header + separator lines.
		if strings.Contains(line, "Status") || strings.Contains(line, "----") {
			continue
		}
		m := progressRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		rows = append(rows, progressRow{
			phase:   m[1],
			status:  strings.ToLower(strings.TrimSpace(m[2])),
			updated: strings.TrimSpace(m[3]),
		})
	}
	return rows
}

// rowMarkedDone returns true for the status values that the doctor
// considers "phase complete" — `done` is the canonical edikt status
// and ✅ covers older plan files / external conventions.
func rowMarkedDone(status string) bool {
	s := strings.ToLower(strings.TrimSpace(status))
	return s == "done" || strings.Contains(s, "✅") || s == "pass" || s == "passed"
}

// hasOverrideMarker detects the `(overrides: N)` annotation that the
// /edikt:sdlc:plan command writes when the user accepts an override.
func hasOverrideMarker(updated string) bool {
	return strings.Contains(strings.ToLower(updated), "override")
}

// reportFilenameRe matches `<plan>-phase-<phase>-<timestamp>.json` so we
// can tell which JSON files in the state dir belong to a given phase.
var reportFilenameRe = regexp.MustCompile(`^(.*)-phase-([0-9]+[a-zA-Z]?)-([0-9TZ]+)\.json$`)

// hasRecentPassingReport returns true if the state dir contains a JSON
// report for (planID, phase) with summary.failed == 0 and timestamp
// newer than threshold (zero time means "any passing report counts").
func hasRecentPassingReport(stateDir, planID, phase string, threshold time.Time) (bool, error) {
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		m := reportFilenameRe.FindStringSubmatch(e.Name())
		if m == nil || m[1] != planID || m[2] != phase {
			continue
		}
		ts, err := time.Parse("20060102T150405Z", m[3])
		if err != nil {
			continue
		}
		if !threshold.IsZero() && ts.Before(threshold) {
			continue
		}
		body, err := os.ReadFile(filepath.Join(stateDir, e.Name()))
		if err != nil {
			continue
		}
		var r struct {
			Summary struct {
				Failed  int `json:"failed"`
				Timeout int `json:"timeout"`
			} `json:"summary"`
		}
		if err := json.Unmarshal(body, &r); err != nil {
			continue
		}
		if r.Summary.Failed == 0 && r.Summary.Timeout == 0 {
			return true, nil
		}
	}
	return false, nil
}

// lastCriteriaCommitTime returns the commit time of the most recent
// commit touching path. Falls back to the file mtime when not in a git
// tree, and to zero time on hard error so the warning fires.
func lastCriteriaCommitTime(projectRoot, path string) (time.Time, error) {
	cmd := exec.Command("git", "-C", projectRoot, "log", "-1", "--format=%cI", "--", path)
	out, err := cmd.Output()
	if err == nil {
		s := strings.TrimSpace(string(out))
		if s != "" {
			if t, perr := time.Parse(time.RFC3339, s); perr == nil {
				return t, nil
			}
		}
	}
	if info, ferr := os.Stat(path); ferr == nil {
		return info.ModTime(), nil
	}
	return time.Time{}, err
}
