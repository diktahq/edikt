// doctor_surface_budget.go — the shared surfaces that no per-field cap can guard.
//
// WHY THIS IS A DOCTOR CHECK AND NOT A COMPILE FAILURE.
//
// Three warnings in this project were real and unread: the status:accepted skip
// line, and the pending-paths queue, which prints only on the run that queues a
// proposal and then stays silent forever while the queue stands. A budget
// warning printed once at compile time is missed the same way. These are
// STANDING CONDITIONS -- true between compiles, not events at one -- so doctor
// reports them every run, whether or not the last compile was the one that
// caused them.
//
// WHY A PER-FIELD CAP CANNOT DO THIS JOB. proposed_topic_description has a hard
// cap of 500. Eleven topics each at a perfectly legal 500 render the
// always-loaded core to roughly 3,744 est-tokens, past its ~3,500 target. Every
// individual field is inside its limit and the shared surface they compose is
// not. No combination of per-field caps can express a budget that belongs to
// the assembly, which is why this measures the assembled artifact instead.
//
// ADVISORY, NOT A GATE. Nothing here fails a build. The core growing past its
// target is a fact a maintainer should see and weigh, not a reason to refuse a
// compile that is otherwise correct.
package cmd

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// coreTokenTarget is SPEC-011 AC-007-1's ambient budget and the documented
// default for compile.ambient-token-ceiling.
//
// A NOTE ON WHICH BUDGET THIS IS. edikt-dev overrode
// compile.ambient-token-ceiling to 7000, and test-ambient-budget.sh measures
// the WHOLE ambient set against it (CLAUDE.md, the core, and everything else
// always loaded) -- currently ~5,756 est-tokens. This check measures only
// governance.md, the compiled core, currently ~2,752. The two numbers are
// different subjects, and reporting one against the other's ceiling would be a
// category error. There is no separate core-specific ceiling in config today,
// so 3,500 is used as a documented ADVISORY reference and labelled as such.
const coreTokenTarget = 3500

// estTokens is chars/4 -- the same estimator as the ambient baseline and the
// arrival scan. NOT a tokenizer count, and never to be laundered into one.
func estTokens(n int) int { return n / 4 }

type surfaceNote struct {
	Title       string
	Detail      []string
	Consequence string
}

// runSurfaceBudgetCheck reports the assembled always-loaded core against its
// advisory target, plus sidecar fields above their soft cap.
//
// ran=false when there is no compiled core to measure: an unmeasured surface is
// not a passing one (INV-013).
func runSurfaceBudgetCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	corePath := filepath.Join(projectRoot, ".claude", "rules", "governance.md")
	core, err := os.ReadFile(corePath)
	if err != nil {
		return 0, false
	}

	var notes []surfaceNote

	coreTok := estTokens(len(core))
	if coreTok > coreTokenTarget {
		notes = append(notes, surfaceNote{
			Title: fmt.Sprintf("always-loaded core is %d est-tokens, over the %d advisory target",
				coreTok, coreTokenTarget),
			Detail: []string{
				fmt.Sprintf("%s is %d bytes", filepath.Join(".claude", "rules", "governance.md"), len(core)),
				"est-tokens is chars/4, not a tokenizer count",
			},
			Consequence: "this file is loaded on EVERY edit; growth here is charged to every session",
		})
	}

	if over := overSoftCapFields(projectRoot); len(over) > 0 {
		detail := over
		trimmed := false
		if len(detail) > 8 {
			detail, trimmed = detail[:8], true
		}
		if trimmed {
			detail = append(detail, fmt.Sprintf("... and %d more", len(over)-8))
		}
		notes = append(notes, surfaceNote{
			Title:       fmt.Sprintf("%d sidecar field(s) above their soft cap", len(over)),
			Detail:      detail,
			Consequence: "accepted and compiled — shorten at review; nothing is lost meanwhile",
		})
	}

	fmt.Fprintln(w, "  ── Surface Budget ─────────────────────────────")
	if len(notes) == 0 {
		fmt.Fprintf(w, "    ok   always-loaded core %d est-tokens (target %d); no field over soft cap\n",
			coreTok, coreTokenTarget)
		return 0, true
	}
	// The core size prints whether or not it is over, so the number is visible
	// on every run rather than only when it has already become a problem.
	fmt.Fprintf(w, "    info always-loaded core %d est-tokens (advisory target %d)\n",
		coreTok, coreTokenTarget)
	for _, n := range notes {
		fmt.Fprintf(w, "    warn %s\n", n.Title)
		for _, d := range n.Detail {
			fmt.Fprintf(w, "           %s\n", d)
		}
		fmt.Fprintf(w, "           → %s\n", n.Consequence)
	}
	return len(notes), true
}

// softCapFor returns the soft target for a field name, and whether it has one.
func softCapFor(field string) (int, bool) {
	switch field {
	case "text":
		return sidecar.SoftCapText, true
	case "intent", "falsifying_observation":
		return sidecar.SoftCapIntent, true
	case "evidence":
		return sidecar.SoftCapEvidence, true
	case "proposed_topic_description":
		return sidecar.SoftCapTopicDescription, true
	}
	return 0, false
}

// overSoftCapFields walks the corpus for fields between soft and hard cap.
// These validate, compile, and keep their content; they are surfaced so a
// reviewer can shorten them deliberately rather than an extractor losing them.
func overSoftCapFields(projectRoot string) []string {
	var out []string
	docs := filepath.Join(projectRoot, "docs")
	_ = filepath.Walk(docs, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(p, ".edikt.yaml") {
			return nil
		}
		s, lerr := sidecar.Load(p)
		if lerr != nil || s == nil {
			// A sidecar that will not load is reported by the SCHEMA INVALID
			// check; not this one's business, and not silently a pass either.
			return nil
		}
		rel, _ := filepath.Rel(projectRoot, p)
		check := func(field, val string, idx int, group string) {
			cap, ok := softCapFor(field)
			if !ok || len(val) <= cap {
				return
			}
			out = append(out, fmt.Sprintf("%s: %s[%d].%s is %d chars (soft %d)",
				rel, group, idx, field, len(val), cap))
		}
		for i, d := range s.Directives {
			check("text", d.Text, i, "directives")
			check("intent", d.Intent, i, "directives")
			check("falsifying_observation", d.FalsifyingObservation, i, "directives")
		}
		for i, pr := range s.Prohibitions {
			check("text", pr.Text, i, "prohibitions")
			check("intent", pr.Intent, i, "prohibitions")
			check("falsifying_observation", pr.FalsifyingObservation, i, "prohibitions")
		}
		if c, ok := softCapFor("proposed_topic_description"); ok &&
			len(s.ProposedTopicDescription) > c {
			out = append(out, fmt.Sprintf("%s: proposed_topic_description is %d chars (soft %d)",
				rel, len(s.ProposedTopicDescription), c))
		}
		return nil
	})
	return out
}
