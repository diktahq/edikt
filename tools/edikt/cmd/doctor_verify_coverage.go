package cmd

// doctor_verify_coverage.go — Phase 4 of PLAN-v060-completion-evidence.
//
// Adds a "Sidecar Verify Coverage" group to `edikt doctor`. Walks every
// gov / prd / spec sidecar in the project and reports two soft signals:
//
//   - coverage: how many items declare a verify: command vs how many don't
//   - health:   how many sidecars currently have at least one failing verify
//
// Both are warnings, never errors — the doctor stays informative. The
// completion-evidence gate that actually blocks completion claims lives
// in `bin/edikt gov compile`'s post-merge step (see cmd/gov/compile.go),
// not here.

import (
	"io"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/trust"
	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// runVerifyCoverageCheck walks every sidecar and surfaces verify coverage
// + health as warnings. Returns (warnings, ran) — ran is false when the
// project has no sidecars at all (silent in that case so the doctor's
// output stays clean for non-edikt projects).
func runVerifyCoverageCheck(projectRoot string, w io.Writer) (warns int, ran bool) {
	// The coverage check executes every sidecar's verify: shell command via
	// runVerifyAll. In an unapproved project that would run repo-controlled
	// shell during a read-only diagnostic — refuse (ADR-041). Skip silently so  // edikt-guard:allow
	// doctor's output stays clean; the actionable --trust hint lives on the
	// gov compile / verify paths the user invokes deliberately.
	if !trust.IsTrusted(projectRoot) {
		return 0, false
	}
	report, err := runVerifyAll(projectRoot, false)
	if err != nil {
		// Surface as a single warning rather than an error — verify all
		// has its own structured-error reporting under the JSON shape.
		io.WriteString(w, "  WARN: sidecar verify coverage skipped — "+err.Error()+"\n")
		return 1, true
	}
	if report.Summary.SidecarsTotal == 0 {
		return 0, false
	}

	io.WriteString(w, "  ── Sidecar Verify Coverage ────────────────────\n")

	// Per-sidecar lines for failures, then a coverage summary.
	for _, s := range report.Sidecars {
		failing, withVerify, total := 0, 0, len(s.Results)
		for _, r := range s.Results {
			switch r.Status {
			case verify.StatusFailed, verify.StatusTimeout:
				failing++
				withVerify++
			case verify.StatusPassed:
				withVerify++
			}
		}
		if failing > 0 {
			io.WriteString(w, "  WARN: "+s.Kind+"/"+s.ID+" — "+
				itoa(failing)+" of "+itoa(total)+
				" verify(s) failing. Run: edikt verify "+s.Kind+" "+s.ID+"\n")
			warns++
		}
		// Coverage = sidecars with at least one item carrying verify:
		_ = withVerify
	}

	// sidecars-with-any-verify / total + items-with-verify / total-claim-items
	// — both axes so the user can see whether coverage is sparse-but-wide
	// or dense-on-few-artifacts.
	sidecarsCovered := 0
	itemsWithVerify := 0
	totalItems := report.Summary.ItemsTotal
	// Skip-reason breakdown: "N skipped" alone reads as "N unchecked," which
	// a high-skip corpus can misreport as weak governance when most of it is
	// StatusSkippedInformational/Operational — GL-002's own design, not a
	// gap: verify: is correctly omitted whenever a rule has no mechanical
	// proxy. Reporting the reason breakdown lets a reader tell "this corpus
	// is mostly judgment calls, by design" from "this corpus has verify:
	// commands nobody wrote yet" — the flat count cannot distinguish them.
	skipOperational, skipInformational, skipSuppressed := 0, 0, 0
	for _, s := range report.Sidecars {
		anyHere := false
		for _, r := range s.Results {
			switch r.Status {
			case verify.StatusPassed, verify.StatusFailed, verify.StatusTimeout:
				itemsWithVerify++
				anyHere = true
			case verify.StatusSkippedOperational:
				skipOperational++
			case verify.StatusSkippedInformational:
				skipInformational++
			case verify.StatusSkippedSuppressed:
				skipSuppressed++
			}
		}
		if anyHere {
			sidecarsCovered++
		}
	}
	sidecarPct := 0
	if report.Summary.SidecarsTotal > 0 {
		sidecarPct = (sidecarsCovered * 100) / report.Summary.SidecarsTotal
	}
	itemPct := 0
	if totalItems > 0 {
		itemPct = (itemsWithVerify * 100) / totalItems
	}

	// Soft warning thresholds. Sidecar coverage below 25% almost always means
	// the project has shipped the schema but not authored verifies yet.
	mark := "[ok]"
	if sidecarPct < 25 && report.Summary.SidecarsTotal > 0 {
		mark = "[!!]"
		io.WriteString(w, "  WARN: sidecar verify coverage low ("+itoa(sidecarPct)+
			"% of sidecars carry at least one verify:) — consider adding mechanical "+
			"checks to high-traffic directives, FRs, or SRs.\n")
		warns++
	}

	io.WriteString(w, "  "+mark+" Sidecar verify coverage — "+
		itoa(sidecarsCovered)+"/"+itoa(report.Summary.SidecarsTotal)+" sidecars ("+itoa(sidecarPct)+
		"%); items "+itoa(itemsWithVerify)+"/"+itoa(totalItems)+" ("+itoa(itemPct)+"%); "+
		itoa(report.Summary.Passed)+" passed, "+
		itoa(report.Summary.Failed+report.Summary.Timeout)+" failing, "+
		itoa(report.Summary.Skipped)+" skipped (operational "+itoa(skipOperational)+
		", informational "+itoa(skipInformational)+", suppressed "+itoa(skipSuppressed)+").\n")

	// A skip rate dominated by suppressed items is a different situation from
	// one dominated by operational/informational — suppressed means a
	// directive was excluded from the compiled corpus entirely (dropped, not
	// judged-unverifiable-by-design), which is worth a distinct callout.
	if report.Summary.Skipped > 0 && skipSuppressed > 0 {
		io.WriteString(w, "  ── "+itoa(skipSuppressed)+" skip(s) are suppressed (directive excluded from the compiled "+
			"corpus, not a judgment-call omission) — review .edikt/state/pending-* and suppressed_directives entries.\n")
	}
	return warns, true
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

// stringsHasPrefix wraps strings.HasPrefix without taking the import
// hit when no other caller in this file needs strings. Currently unused
// but reserved for future per-class filtering — kept for symmetry with
// the verify_all.go helpers.
var _ = strings.HasPrefix
