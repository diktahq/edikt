package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TWO NUMBERS THAT WERE INVISIBLE, AND INVISIBLE READS AS HEALTHY.
//
//  1. Enforcement coverage — of every directive and prohibition in the
//     corpus, how many carry any `verify:` at all. A directive with no check
//     is not a finding: it is a COUNT. Governance that states a rule and
//     checks nothing is the normal state of most of a corpus, and the number
//     is what tells a team whether that is a deliberate ratio or a drift.
//  2. Never-run verifies — claimed checks with no recorded result, counted
//     apart from failing ones. A consumer's compile ran 30 of 976 verify
//     items and skipped 946 with nothing reporting it; "never ran" and "ran
//     and failed" are different problems with different remedies.
//
// Neither is a warning. Both are measurements, and doctor prints them the way
// it prints coverage: a line with its denominator.
func runEnforcementCoverageCheck(projectRoot string, out io.Writer) {
	dirs := migrate.ResolveArtifactDirs(projectRoot)
	type counts struct{ total, withVerify int }
	var all counts
	byKind := map[string]int{}
	withVerifyIDs := map[string]bool{}

	for _, dir := range []string{dirs.Decisions, dirs.Invariants, dirs.Guidelines} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".edikt.yaml") {
				continue
			}
			sc, err := sidecar.Load(filepath.Join(dir, e.Name()))
			if err != nil || sc == nil {
				continue
			}
			id := strings.TrimSuffix(e.Name(), ".edikt.yaml")
			if i := strings.Index(id, "-"); i > 0 {
				if j := strings.Index(id[i+1:], "-"); j >= 0 {
					id = id[:i+1+j]
				}
			}
			// D160: a suppressed directive is subtracted from every
			// delivered surface at gov:compile time (INV-015's recorded // edikt-guard:allow
			// removal), but this counting loop iterated sc.Directives raw
			// and had no idea suppression existed — a retired rule still
			// inflated the denominator and, if it happened to lack a
			// verify, inflated the "unmeasured" side of the ratio too.
			suppressed := make(map[string]bool, len(sc.SuppressedDirectives))
			for _, s := range sc.SuppressedDirectives {
				suppressed[s] = true
			}
			for _, d := range sc.Directives {
				if suppressed[d.Text] {
					continue
				}
				all.total++
				if strings.TrimSpace(d.Verify) != "" {
					all.withVerify++
					byKind[kindOrUnstated(d.VerifyKind)]++
					withVerifyIDs[id] = true
				}
			}
			for _, p := range sc.Prohibitions {
				all.total++
				if strings.TrimSpace(p.Verify) != "" {
					all.withVerify++
					byKind[kindOrUnstated(p.VerifyKind)]++
					withVerifyIDs[id] = true
				}
			}
		}
	}
	if all.total == 0 {
		return // no corpus: nothing to measure, and 0 of 0 is not a state
	}

	var kinds []string
	for k, n := range byKind {
		kinds = append(kinds, fmt.Sprintf("%s %d", k, n))
	}
	sort.Strings(kinds)
	breakdown := "none"
	if len(kinds) > 0 {
		breakdown = strings.Join(kinds, ", ")
	}
	pct := all.withVerify * 100 / all.total
	fmt.Fprintf(out, "  [--] Enforcement coverage — %d of %d directive(s)/prohibition(s) carry a verify (%d%%; %s); "+
		"%d unenforced. Unenforced is a count, not a defect: it says how much of the corpus states a rule and checks nothing.\n",
		all.withVerify, all.total, pct, breakdown, all.total-all.withVerify)

	// Never-run: a claimed check with no persisted result at all. Counted
	// apart from failing, because the remedies differ.
	stateDir := filepath.Join(projectRoot, ".edikt", "state", "verify")
	idx, _ := indexVerifyState(stateDir)
	neverRun, failing := 0, 0
	var neverRunIDs []string
	for id := range withVerifyIDs {
		r := latestVerifyReportFrom(idx, id)
		if r == nil {
			neverRun++
			neverRunIDs = append(neverRunIDs, id)
			continue
		}
		for _, c := range r.Criteria {
			if c.Status == "failed" || c.Status == "timeout" {
				failing++
			}
		}
	}
	sort.Strings(neverRunIDs)
	shown := neverRunIDs
	if len(shown) > 5 {
		shown = append(append([]string{}, shown[:5]...), "…")
	}
	fmt.Fprintf(out, "  [--] Verify results — %d artifact(s) with a verify have never run (%s); %d failing item(s) in the newest persisted reports. "+
		"Never run is not passing, and it is not failing either.\n",
		neverRun, strings.Join(shown, ", "), failing)
}

func kindOrUnstated(k string) string {
	if strings.TrimSpace(k) == "" {
		return "kind-unstated"
	}
	return k
}
