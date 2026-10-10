package cmd

// doctor_verify_coverage.go — Phase 4 of PLAN-v060-completion-evidence,
// reshaped under D38 defect 2 (2026-09-02).
//
// Adds a "Sidecar Verify Coverage" group to `edikt doctor`. It READS the
// per-sidecar reports `edikt verify all` persists under
// .edikt/state/verify/gov-<id>-all-<stamp>.json and reports:
//
//   - health:   sidecars whose LAST persisted run had a failing verify
//   - coverage: how many sidecars have ever been measured, and how old the
//               newest measurement is
//
// It never executes a verify. It used to — runVerifyAll ran every sidecar's
// verify: shell command inside a read-only diagnostic — and on this corpus
// that was 137 s of the 168 s doctor took, with nothing printed while it
// ran. A diagnostic that takes longer than the tool timeout it is run under
// is indistinguishable from a hang, and a doctor that hangs is a can't-fail
// control in the PRD-009 sense: nobody waits for it, so nobody reads it.
// Measurement is `edikt verify all`'s job, invoked deliberately; doctor's is
// to say what was measured, when, and — never silently — what was not
// (INV-013).  // edikt-guard:allow
//
// Both signals are warnings, never errors — the doctor stays informative.
// The completion-evidence gate that blocks completion claims lives in
// `bin/edikt gov compile`'s post-merge step, not here.

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// verifyReportStaleAfter is how old the newest persisted measurement may be
// before doctor calls it stale. A week: long enough that a weekly cadence
// never trips it, short enough that "we measured once in June" does.
const verifyReportStaleAfter = 7 * 24 * time.Hour

// indexVerifyState reads the persisted-report directory ONCE and groups the
// filenames by artifact id. It returns the index and the number of directory
// reads it performed, so a test can assert the count rather than infer it
// from a stopwatch.
//
// The per-id Glob it replaces read the whole directory once PER SIDECAR: 88
// sidecars against 129,170 report files, growing 5–11k a day on the
// reference repo. Doctor's cost is supposed to be independent of how long
// the project has been measured.
func indexVerifyState(stateDir string) (map[string][]string, int) {
	idx := map[string][]string{}
	entries, err := os.ReadDir(stateDir)
	if err != nil {
		return idx, 1
	}
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, "gov-") || !strings.HasSuffix(n, ".json") {
			continue
		}
		rest := strings.TrimPrefix(n, "gov-")
		i := strings.Index(rest, "-all-")
		if i <= 0 {
			continue
		}
		id := rest[:i]
		idx[id] = append(idx[id], filepath.Join(stateDir, n))
	}
	for id := range idx {
		sort.Strings(idx[id])
	}
	return idx, 1
}

// latestVerifyReportFrom returns the newest persisted report for an id, using
// the index above. The stamp in the filename is UTC "20060102T150405Z", so
// lexical order is time order.
func latestVerifyReportFrom(idx map[string][]string, id string) *verify.Report {
	matches := idx[id]
	if len(matches) == 0 {
		return nil
	}
	for i := len(matches) - 1; i >= 0; i-- {
		b, err := os.ReadFile(matches[i])
		if err != nil {
			continue
		}
		var r verify.Report
		if err := json.Unmarshal(b, &r); err != nil {
			continue
		}
		return &r
	}
	return nil
}

// runVerifyCoverageCheck reports verify health and coverage from persisted
// reports. Returns (warnings, ran) — ran is false when the project has no
// gov sidecars at all (silent, so doctor stays clean for non-edikt projects).
func runVerifyCoverageChecks(projectRoot string, w io.Writer) (errs, warns int, ran bool) {
	dirs := migrate.ResolveArtifactDirs(projectRoot)
	var ids []string
	for _, dir := range []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			name := e.Name()
			if e.IsDir() || !strings.HasSuffix(name, ".edikt.yaml") {
				continue
			}
			if id, ok := artifactIDFrom(name); ok {
				ids = append(ids, id)
			}
		}
	}
	if len(ids) == 0 {
		return 0, 0, false
	}
	sort.Strings(ids)

	io.WriteString(w, "  ── Sidecar Verify Coverage ────────────────────\n")

	stateDir := filepath.Join(projectRoot, ".edikt", "state", "verify")
	measured := 0
	var newest time.Time
	stateIndex, _ := indexVerifyState(stateDir)
	passed, failing, skipped := 0, 0, 0
	for _, id := range ids {
		r := latestVerifyReportFrom(stateIndex, id)
		if r == nil {
			continue
		}
		measured++
		if t, err := time.Parse(time.RFC3339, r.RanAt); err == nil && t.After(newest) {
			newest = t
		}
		fail := 0
		for _, c := range r.Criteria {
			switch c.Status {
			case verify.StatusPassed:
				passed++
			case verify.StatusFailed, verify.StatusTimeout:
				fail++
				failing++
			default:
				skipped++
			}
		}
		if fail > 0 {
			// RED, not a warning (FR-035): a verify that RAN and FAILED is
			// the one signal in this output that describes a broken rule,
			// and it was printed in the same shade as "never measured".
			io.WriteString(w, "  🔴 ERROR: gov/"+id+" — "+itoa(fail)+" of "+itoa(len(r.Criteria))+
				" verify(s) failing in its last persisted run ("+r.RanAt+"). Run: edikt verify gov "+id+"\n")
			errs++
		}
	}

	if measured == 0 {
		io.WriteString(w, "  WARN: sidecar verify coverage NOT MEASURED — no persisted verify report under "+
			".edikt/state/verify/ for any of the "+itoa(len(ids))+" sidecar(s). Run: edikt verify all\n")
		warns++
		return errs, warns, true
	}

	mark := "[ok]"
	unmeasured := len(ids) - measured
	if unmeasured > 0 {
		mark = "[!!]"
	}
	age := "unknown age"
	if !newest.IsZero() {
		age = "newest " + newest.UTC().Format("2006-01-02T15:04Z")
		if time.Since(newest) > verifyReportStaleAfter {
			io.WriteString(w, "  WARN: the newest persisted verify report is older than 7 days ("+
				newest.UTC().Format("2006-01-02")+") — the numbers below describe that run, not this tree. Run: edikt verify all\n")
			warns++
			mark = "[!!]"
		}
	}
	io.WriteString(w, "  "+mark+" Sidecar verify coverage — measured "+itoa(measured)+"/"+itoa(len(ids))+
		" sidecars from persisted reports ("+age+"); "+itoa(passed)+" passed, "+itoa(failing)+" failing, "+
		itoa(skipped)+" skipped/unmeasured items")
	if unmeasured > 0 {
		io.WriteString(w, "; "+itoa(unmeasured)+" sidecar(s) never measured — run: edikt verify all")
		warns++
	}
	io.WriteString(w, ".\n")
	return errs, warns, true
}

// itoa avoids pulling in strconv just for one integer-to-string call.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
