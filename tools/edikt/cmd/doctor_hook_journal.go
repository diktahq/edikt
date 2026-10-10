package cmd

// doctor_hook_journal.go — surface the hook journal in `edikt doctor`.
//
// WHY THIS FILE EXISTS
//
// hookmatch.Summary.Report()'s own doc comment said "renders the summary for
// doctor". Doctor never called it. A grep across cmd/doctor*.go found no
// reference at all, which meant the whole suppression-detectability mechanism
// — the journal, the typed outcomes, the counts — reached exactly one surface
// (`edikt hook report`) that nobody runs unprompted. On this maintainer's own
// machine that left 20 recorded index_missing suppressions, each one a write
// that proceeded ungoverned, surfaced to no one.
//
// A control nobody reads is a control that does not exist. Doctor is the
// surface people actually run, so this is where the report belongs.
//
// AN ABSENT JOURNAL IS UNKNOWN, NEVER CLEAN. Report() already refuses to
// render "no journal" as "no suppression"; this check preserves the
// distinction in doctor's WARNING ACCOUNTING too, which Report() cannot do on
// its own — printing an honest UNKNOWN while contributing zero warnings would
// let doctor conclude "result: healthy" over a chain that has never run.

import (
	"fmt"
	"io"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// runHookJournalCheck prints the hook-chain report and returns the number of
// warnings it contributes.
//
// Warning accounting, and the reasoning behind each:
//
//	journal unreadable  1 — UNKNOWN, and unreadable is not zero.
//	journal absent      1 — UNKNOWN. The chain has never run on this machine,
//	                        or cannot write. Not a clean bill of health.
//	journal empty       1 — same class: present, never written to.
//	suppressed > 0      1 — invocations where governance DID NOT RUN.
//	malformed > 0       1 — a corrupt journal must not read as a quiet one.
//	otherwise           0
//
// It ALWAYS runs and always prints: there is no "no subject" case here,
// because the absence of a journal is itself the finding. That is INV-013's
// boundary read correctly — a control with a subject it could not observe must
// say so.
func runHookJournalCheck(out io.Writer) (warnCount int) {
	s, err := hookmatch.ReadSummary()
	return runHookJournalCheckWith(s, err, out)
}

// runHookJournalCheckWith is the check proper, taking the Summary as an
// argument so ONE read of an append-only journal serves every check that
// needs it. The journal grows without bound, and two independent reads
// doubled doctor's cost on exactly the machines where it was largest.
func runHookJournalCheckWith(s hookmatch.Summary, err error, out io.Writer) (warnCount int) {
	if err != nil {
		// Unreadable is its own answer and must not collapse into silence: an
		// unreadable journal is indistinguishable from a clean one at every
		// other surface.
		fmt.Fprintf(out, "  WARN: hook journal unreadable (%v) — suppression counts are UNKNOWN, not zero\n", err)
		return 1
	}

	// Report() owns the wording, including the absent- and empty-journal
	// cases. Reproducing any of it here would be two things that must agree
	// kept separately (GL-002), and the copy is always the one that drifts.
	for _, line := range strings.Split(strings.TrimRight(s.Report(), "\n"), "\n") {
		fmt.Fprintf(out, "  %s\n", line)
	}

	if !s.JournalFound || s.Total == 0 {
		return 1
	}
	warn := 0
	if s.Suppressed > 0 {
		warn++
	}
	if s.Malformed > 0 {
		warn++
	}
	return warn
}
