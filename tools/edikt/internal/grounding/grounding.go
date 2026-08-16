// Package grounding measures whether compiled governance is anchored to
// the prose it claims to come from.
//
// For every directive and prohibition in every sidecar, it asks one
// question: does source_excerpt.quote actually appear in the parent .md at
// line_start..line_end? A directive whose quote is absent, empty, or
// pointing at the wrong lines is a rule the corpus cannot trace to a
// decision — enforceable-looking text with nothing behind it.
//
// Deterministic and read-only. No LLM, no network. It reuses
// sidecar.ClassifyExcerpt so it can never disagree with the compile
// staleness gate about whether a quote matches.
package grounding

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ItemFinding is one directive or prohibition that did not come back
// grounded. Grounded items produce no finding — the report carries counts
// for those and names only what failed.
type ItemFinding struct {
	ArtifactID string                 `json:"artifact_id"`
	Kind       string                 `json:"kind"`  // "directive" | "prohibition"
	Index      int                    `json:"index"` // position within its array
	Verdict    sidecar.ExcerptVerdict `json:"verdict"`
	Text       string                 `json:"text"`
}

// Report is the corpus-wide result.
//
// Unreadable is not folded into any other count. A sidecar that would not
// load, or a parent .md that would not open, is a subject the check HAD
// and could not observe — reporting it as grounded or as ungrounded would
// both be claims the scan cannot support (INV-013).
type Report struct {
	// PairsDiscovered is every (parent, sidecar) pair Discover returned.
	// The four counts below must account for all of it — a reader has to be
	// able to reconstruct the denominator, or "57 scanned" is a number with
	// no stated relationship to the corpus.
	PairsDiscovered int `json:"pairs_discovered"`
	// SkipListed: superseded, deprecated, or migration:skip. Excluded by
	// design — their directives never reach compiled governance.
	SkipListed int `json:"skip_listed"`
	// NoSidecar: the artifact has no .edikt.yaml at all. Not a scan
	// failure; it has simply never been extracted.
	NoSidecar int `json:"no_sidecar"`

	SidecarsScanned    int      `json:"sidecars_scanned"`
	SidecarsUnreadable []string `json:"sidecars_unreadable,omitempty"`
	ParentsUnreadable  []string `json:"parents_unreadable,omitempty"`

	TotalItems int `json:"total_items"`
	Grounded   int `json:"grounded"`

	// ByVerdict counts every item by its classification, so the
	// denominator is always reconstructable from the parts.
	ByVerdict map[sidecar.ExcerptVerdict]int `json:"by_verdict"`

	Findings []ItemFinding `json:"findings,omitempty"`
}

// Scan walks the given artifact directories under projectRoot and grounds
// every directive and prohibition it finds.
//
// Skip-listed artifacts (superseded, deprecated, migration:skip) are
// excluded: their directives are withheld from compiled governance, so
// grounding them would measure text no reader ever sees.
func Scan(projectRoot string, dirs []string) (*Report, error) {
	pairs, err := sidecar.Discover(projectRoot, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}

	rep := &Report{ByVerdict: map[sidecar.ExcerptVerdict]int{}}

	rep.PairsDiscovered = len(pairs)

	for _, p := range pairs {
		if p.Skip {
			rep.SkipListed++
			continue
		}
		if p.Sidecar == nil {
			// A pair with no sidecar on disk is not a scan failure — the
			// artifact simply has not been extracted. A pair whose sidecar
			// exists and would not parse is.
			if p.LoadErr != nil {
				rep.SidecarsUnreadable = append(rep.SidecarsUnreadable, p.ArtifactID)
			} else {
				rep.NoSidecar++
			}
			continue
		}

		lines, lerr := parentLines(projectRoot, p.ParentPath)
		if lerr != nil {
			rep.ParentsUnreadable = append(rep.ParentsUnreadable, p.ArtifactID)
			continue
		}
		rep.SidecarsScanned++

		// v2: every anchor is classified and recorded on its own. Collapsing
		// a multi-anchor directive to one verdict would hide an ungrounded
		// second anchor behind a grounded first — and the second anchor is
		// usually the definition the rule actually depends on.
		for i, d := range p.Sidecar.Directives {
			for _, v := range classifyAll(d.Anchors(), d.Text, lines) {
				rep.record(p.ArtifactID, "directive", i, d.Text, v)
			}
		}
		for i, pr := range p.Sidecar.Prohibitions {
			for _, v := range classifyAll(pr.Anchors(), pr.Text, lines) {
				rep.record(p.ArtifactID, "prohibition", i, pr.Text, v)
			}
		}
	}

	sort.Strings(rep.SidecarsUnreadable)
	sort.Strings(rep.ParentsUnreadable)
	return rep, nil
}

// classifyAll returns one verdict per anchor. An item with NO anchors yields a
// single ExcerptNoQuote rather than an empty slice: dropping it would remove the
// item from the denominator entirely, turning an ungrounded directive into a
// directive that was never counted (INV-013).
func classifyAll(anchors []sidecar.SourceExcerpt, text string, lines []string) []sidecar.ExcerptVerdict {
	if len(anchors) == 0 {
		return []sidecar.ExcerptVerdict{sidecar.ExcerptNoQuote}
	}
	out := make([]sidecar.ExcerptVerdict, 0, len(anchors))
	for _, se := range anchors {
		out = append(out, sidecar.ClassifyExcerpt(se, text, lines))
	}
	return out
}

func (r *Report) record(id, kind string, idx int, text string, v sidecar.ExcerptVerdict) {
	r.TotalItems++
	r.ByVerdict[v]++
	if v.Grounded() {
		r.Grounded++
		return
	}
	r.Findings = append(r.Findings, ItemFinding{
		ArtifactID: id, Kind: kind, Index: idx, Verdict: v, Text: text,
	})
}

func parentLines(projectRoot, parentPath string) ([]string, error) {
	p := parentPath
	if !filepath.IsAbs(p) {
		p = filepath.Join(projectRoot, p)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	return strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n"), nil
}

// Summary renders the one-line result, followed by the per-verdict
// breakdown. It always states the denominator and never collapses the
// unreadable counts into the pass/fail split.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Grounding: %d of %d items grounded across %d sidecar(s)\n",
		r.Grounded, r.TotalItems, r.SidecarsScanned)
	fmt.Fprintf(&b, "  corpus: %d pair(s) discovered = %d scanned + %d skip-listed + "+
		"%d without a sidecar + %d unreadable\n",
		r.PairsDiscovered, r.SidecarsScanned, r.SkipListed, r.NoSidecar,
		len(r.SidecarsUnreadable)+len(r.ParentsUnreadable))

	kinds := make([]string, 0, len(r.ByVerdict))
	for v := range r.ByVerdict {
		kinds = append(kinds, string(v))
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "  %-32s %d\n", k, r.ByVerdict[sidecar.ExcerptVerdict(k)])
	}

	if n := len(r.SidecarsUnreadable); n > 0 {
		fmt.Fprintf(&b, "  UNMEASURED: %d sidecar(s) would not load: %s\n",
			n, strings.Join(r.SidecarsUnreadable, ", "))
	}
	if n := len(r.ParentsUnreadable); n > 0 {
		fmt.Fprintf(&b, "  UNMEASURED: %d parent .md file(s) would not open: %s\n",
			n, strings.Join(r.ParentsUnreadable, ", "))
	}

	// What this check cannot see, stated in its own output.
	b.WriteString("  ceiling: grounding proves a quote EXISTS in the prose at the " +
		"recorded lines. It cannot judge whether the directive text follows from " +
		"that quote, whether the quote is the right passage, or whether the " +
		"directive is normative.\n")
	return b.String()
}
