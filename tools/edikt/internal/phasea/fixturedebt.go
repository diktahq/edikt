package phasea

// fixturedebt.go — the obligation a regeneration creates but does not announce.
//
// A directive whose verify_kind is "behavioral" requires a fixture PAIR, and
// Phase B refuses to render until both scripts exist. The extractor may
// introduce such directives on any regeneration: a consumer project went from
// 0 to 10 behavioral verifies across three artifacts in a single pass, none of
// them with fixtures, and the next compile refused. The fix they were shipping
// was blocked by its own sidecar.
//
// Nothing in "regenerate the sidecar" predicts that. doctor's Verify Kind
// Coverage tally is post-hoc and reports what exists, not that a fixture will
// be REQUIRED before the next render. This reports the debt at the one moment
// it is both known and not yet blocking: after extraction, before compile.
//
// Reported, never fatal. Verify's exit code answers "did the extractor honour
// the plan", and a missing fixture is a true statement about the sidecar, not
// a failure of the dispatch.

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// fixtureDebt is one artifact's count of behavioral verifies lacking a fixture.
type fixtureDebt struct {
	ArtifactID string
	Missing    int
}

// collectFixtureDebt walks the sidecars a plan just produced and counts, per
// artifact, the behavioral directives and prohibitions whose fixture pair is
// incomplete. An unreadable sidecar is skipped rather than guessed at: verify
// has already reported it through its own per-item verdict.
func collectFixtureDebt(root string, items []PlanItem) []fixtureDebt {
	var out []fixtureDebt
	for _, item := range items {
		p := underRoot(root, item.ExpectedOutput.Path)
		sc, err := sidecar.Load(p)
		if err != nil || sc == nil {
			continue
		}
		missing := 0
		for _, d := range sc.Directives {
			if isBehavioral(d.VerifyKind) && incompletePair(d.PositiveFixturePath, d.NegativeFixturePath) {
				missing++
			}
		}
		for _, pr := range sc.Prohibitions {
			if isBehavioral(pr.VerifyKind) && incompletePair(pr.PositiveFixturePath, pr.NegativeFixturePath) {
				missing++
			}
		}
		if missing == 0 {
			continue
		}
		id := strings.TrimSuffix(filepath.Base(item.ExpectedOutput.Path), ".edikt.yaml")
		if id == "" {
			id = item.ItemID
		}
		out = append(out, fixtureDebt{ArtifactID: id, Missing: missing})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArtifactID < out[j].ArtifactID })
	return out
}

// isBehavioral compares the verify_kind value tolerantly. The extractor prompt
// now pins the bare spelling, but a corpus written before that pin can carry
// `"behavioral"` quoted, and a check that missed those would under-report the
// exact debt it exists to surface.
func isBehavioral(kind string) bool {
	return strings.EqualFold(strings.Trim(strings.TrimSpace(kind), `"'`), "behavioral")
}

// incompletePair reports whether either arm of the fixture pair is absent.
// Both are required — a positive with no negative is a check that has never
// been shown to fail.
func incompletePair(positive, negative string) bool {
	return strings.TrimSpace(positive) == "" || strings.TrimSpace(negative) == ""
}

// fixtureDebtNotice renders the warning appended to a successful verify. Empty
// when there is no debt, so a clean regeneration says nothing.
func fixtureDebtNotice(debts []fixtureDebt) string {
	if len(debts) == 0 {
		return ""
	}
	total := 0
	for _, d := range debts {
		total += d.Missing
	}
	var b strings.Builder
	fmt.Fprintf(&b, "\nwarning: this regeneration leaves %d behavioral verify(s) without a fixture pair:\n", total)
	for _, d := range debts {
		fmt.Fprintf(&b, "  %s — %d\n", d.ArtifactID, d.Missing)
	}
	b.WriteString("Each requires a positive AND a negative script before `gov compile` will render.\n")
	b.WriteString("Write them now, or the next compile refuses and the change you are shipping is\n")
	b.WriteString("blocked by its own sidecar. See https://edikt.dev/governance/behavioral-fixtures\n")
	return b.String()
}
