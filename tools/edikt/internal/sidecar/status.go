package sidecar

import (
	"fmt"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/parse"
)

// StatusExcluded reports whether an artifact is disqualified from the
// governed corpus by its STATUS — a `proposed` or `rejected` ADR, a `revoked`
// invariant — and returns a human-readable reason on a hit.
//
// This is THE status filter. ADR-020:d03 is the requirement (accepted ADRs,
// active invariants, all guidelines compile); the decision procedure is
// parse.Document.IsIncluded, and this function is the one kind-mapping
// wrapper around it. It used to live twice — govrun.statusExcluded and its
// byte-identical mirror in phasea (an import cycle kept them apart) — and
// `doctor`'s sidecar-health check had no copy at all, so doctor reported
// ERROR MISSING with "run /edikt:adr:compile" for proposed and rejected ADRs
// that compile --check correctly filtered (D46 (a), bok 2026-09-02). Three
// callers, one filter: a drift between what compile governs and what doctor
// demands is now impossible by construction rather than by discipline.
//
// It deliberately does NOT live in isSkipListed. That predicate also feeds
// `migrate lift`, verify-all and legacy-sentinel detection — none of which
// are asking "does this artifact's governance count"; widening it would
// answer four questions with the answer to one.
func StatusExcluded(kind string, parentPath string) (bool, string) {
	var k string
	switch kind {
	case KindADR:
		k = "adr"
	case KindInvariant:
		k = "inv"
	case KindGuideline:
		k = "guideline"
	default:
		// Parent found in a directory the caller gave no class for. This
		// filter's job is to drop artifacts whose status is known and
		// disqualifying, not artifacts whose class could not be read.
		return false, ""
	}

	doc, err := parse.LoadDocument(parentPath)
	if err != nil {
		// An unreadable parent is not a status verdict; each caller reports
		// load errors on its own terms.
		return false, ""
	}
	if doc.IsIncluded(k) {
		return false, ""
	}

	status := strings.TrimSpace(doc.Frontmatter.Status)
	if status == "" {
		status = strings.TrimSpace(doc.BoldStatus)
	}
	if status == "" {
		status = "no status"
	}
	return true, fmt.Sprintf("status: %s — only accepted ADRs and active invariants compile", status)
}

// IsProposedADR reports whether an artifact is an ADR whose status is
// `proposed` — excluded from the corpus by StatusExcluded, but the one excluded
// status a first extraction may still be planned for: /edikt:adr:new compiles
// the ADR it just wrote BEFORE its acceptance gate flips it to accepted
// (issue #14). Rejected, superseded and deprecated ADRs never qualify.
func IsProposedADR(kind string, parentPath string) bool {
	if kind != KindADR {
		return false
	}
	doc, err := parse.LoadDocument(parentPath)
	if err != nil {
		return false
	}
	status := strings.TrimSpace(doc.Frontmatter.Status)
	if status == "" {
		status = strings.TrimSpace(doc.BoldStatus)
	}
	return strings.EqualFold(status, "proposed")
}
