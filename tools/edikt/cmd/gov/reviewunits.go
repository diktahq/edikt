package gov

// reviewunits.go — resolve a file set into REVIEW UNITS for semantic review.
//
// THE BUG THIS REPLACES
//
// `gov verify-diff` read its directive list out of the compiled TOPIC FILES.
// After scoped directives were re-tiered into directive-index.yaml, those files
// render an empty Directives region by design — the text moved to the index and
// is delivered per-path at write time. Measured on this corpus:
//
//	compiled topic files    5 directives  (architecture only; five topics at 0)
//	directive-index.yaml    2,847 entries across 150 globs, 805 distinct
//
// So diff-time semantic review ran against 0.18% of the corpus, and did not
// skip: an empty region produced `verdicts: []`, which downstream consumers
// read as "verified, no violations". Absence rendering as pass.
//
// The re-tiering brainstorm predicted this before it shipped, and specified the
// remedy: re-point the gate at the data plane, gated by a coverage-equivalence
// check. This is that re-pointing.
//
// WHY THE INDEX AND NOT THE SIDECARS
//
// Reading sidecars directly would re-open the double-parser window the sidecar
// migration closed — two readers of the same source drifting apart. The index
// is the compiled data plane, and resolving through the SAME matcher the
// write-time hook uses is what keeps write-time and review-time coverage from
// diverging again. If a directive stops reaching the hook, it stops reaching
// review, and one coverage number describes both.
//
// WHY BATCH BY DIRECTIVE SET, NOT BY FILE
//
// Reviewing per file re-sends the same directives once per file. Files whose
// paths resolve to the SAME directive set are one review unit, because the
// question asked of the model is identical for all of them. The collapse ratio
// (files -> units) is the real cost driver and is reported, not hidden.

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// Directive is one compiled entry as the index carries it. Grade is PINNED at
// render time and never re-derived here — a consumer that re-derived it could
// silently downgrade an invariant with nothing reporting the difference.
type Directive struct {
	ID    string `json:"id"`
	Grade string `json:"grade"`
	Class string `json:"class,omitempty"`
	// Surface names which compiled surface delivers this directive
	// (write-time | topic-file | skill | unrouted). Since ADR-079 a directive
	// can govern a path without being injected at write time, so "governs" and
	// "is injected" are separate facts and the report states both.
	Surface string `json:"surface,omitempty"`
	// Text and FalsifyingObservation are NOT populated from the routing table,
	// which carries no directive bodies by design. They remain on the struct
	// for callers that resolve text from the sidecars themselves.
	Text                  string `json:"text,omitempty"`
	FalsifyingObservation string `json:"falsifying_observation,omitempty"`
}

// ReviewUnit is one batch: a set of files that resolve to an identical
// directive set, reviewed together.
type ReviewUnit struct {
	SetHash    string      `json:"set_hash"`
	Files      []string    `json:"files"`
	Directives []Directive `json:"directives"`
}

// ScopeReport is the whole resolution, including the numbers a caller must show
// the user BEFORE spending anything.
type ScopeReport struct {
	FilesScanned  int          `json:"files_scanned"`
	FilesMatched  int          `json:"files_matched"`
	FilesUnmapped int          `json:"files_unmatched"`
	ReviewUnits   int          `json:"review_units"`
	DirectivesTot int          `json:"directives_in_scope"`
	Units         []ReviewUnit `json:"units"`

	// Unmeasured is set when the resolution could not be performed at all —
	// no index, or an index that yielded nothing for every file. It is NOT the
	// same as a clean scan, and a caller must never render it as verdicts: [].
	Unmeasured bool   `json:"unmeasured"`
	Reason     string `json:"reason,omitempty"`
}

// directiveSetKey produces a stable identity for a directive set. Sorted by id
// so two files matching the same directives in a different order collapse into
// one unit rather than two.
func directiveSetKey(ds []Directive) string {
	ids := make([]string, 0, len(ds))
	for _, d := range ds {
		ids = append(ids, d.ID)
	}
	sort.Strings(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	return hex.EncodeToString(sum[:])[:16]
}

// BuildReviewUnits groups files by their resolved directive set.
//
// resolve is injected so the caller supplies the matcher — in production the
// same one the write-time hook uses. A file resolving to zero directives is
// counted as unmatched and carried in the report rather than dropped: "this
// file is governed by nothing" is a finding, not an absence.
func BuildReviewUnits(files []string, resolve func(path string) ([]Directive, error)) ScopeReport {
	rep := ScopeReport{FilesScanned: len(files)}

	if len(files) == 0 {
		rep.Unmeasured = true
		rep.Reason = "no files in scope — nothing was examined, which is not the same as nothing being wrong"
		return rep
	}

	byHash := map[string]*ReviewUnit{}
	distinct := map[string]bool{}
	resolveFailures := 0

	for _, f := range files {
		ds, err := resolve(f)
		if err != nil {
			resolveFailures++
			continue
		}
		if len(ds) == 0 {
			rep.FilesUnmapped++
			continue
		}
		rep.FilesMatched++
		for _, d := range ds {
			distinct[d.ID] = true
		}
		h := directiveSetKey(ds)
		if u, ok := byHash[h]; ok {
			u.Files = append(u.Files, f)
			continue
		}
		sorted := append([]Directive(nil), ds...)
		sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
		byHash[h] = &ReviewUnit{SetHash: h, Files: []string{f}, Directives: sorted}
	}

	// Every file failed to resolve: the matcher is broken or the index is
	// missing. Reporting zero units as a clean scan is the exact defect this
	// file exists to remove.
	if resolveFailures == len(files) {
		rep.Unmeasured = true
		rep.Reason = "every file failed to resolve against the directive index — the index is missing, empty or unreadable"
		return rep
	}

	hashes := make([]string, 0, len(byHash))
	for h := range byHash {
		hashes = append(hashes, h)
	}
	sort.Strings(hashes)
	for _, h := range hashes {
		u := byHash[h]
		sort.Strings(u.Files)
		rep.Units = append(rep.Units, *u)
	}

	rep.ReviewUnits = len(rep.Units)
	rep.DirectivesTot = len(distinct)

	// Files were examined and NONE is governed. True and worth saying, but it
	// is not a verified-clean result.
	if rep.ReviewUnits == 0 {
		rep.Unmeasured = true
		rep.Reason = "no file in scope is governed by any compiled directive — nothing was reviewed"
	}
	return rep
}
