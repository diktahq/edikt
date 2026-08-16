// Package normative measures whether extracted directives are RULES.
//
// Phase 1 (package grounding) asked whether a directive is anchored to its
// prose. 711 of 712 items are. That number is the reason this package
// exists: grounding proves a quote exists at the recorded lines and nothing
// more. Nine runbook steps rendered as standing law score grounded. THE
// EXTRACTION PROBLEM IS NORMATIVITY, NOT ANCHORING.
//
// Four deterministic checks, all on directive text plus its excerpt:
//
//	2a standalone       — unresolved deixis, trailing colons
//	2b MAY-level        — a MAY cannot be violated, so it is not a directive
//	2c normative force  — a source MUST rendered as SHOULD
//	2d duplicates       — one rule restated at different granularity
//
// Read-only, no LLM, no network. It reuses sidecar.Discover — the same
// corpus walker grounding uses — so the two reports can never disagree
// about which artifacts are in the corpus or which are skip-listed.
package normative

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Item is one directive or prohibition, carried with enough identity to
// name it in a finding.
type Item struct {
	ArtifactID string `json:"artifact_id"`
	Topic      string `json:"topic"`
	Kind       string `json:"kind"` // "directive" | "prohibition"
	Index      int    `json:"index"`
	Text       string `json:"text"`
	Quote      string `json:"-"`
}

// Ref renders the item as ARTIFACT[kind:index] for report lines.
func (i Item) Ref() string { return fmt.Sprintf("%s[%s:%d]", i.ArtifactID, i.Kind, i.Index) }

// Finding is one item that failed one check.
type Finding struct {
	Item
	Verdict string `json:"verdict"`
	// Partner names the other item in a pairwise finding (duplicates).
	Partner string `json:"partner,omitempty"`
}

// Corpus is the denominator, reconstructable from its parts exactly as
// grounding's is. The four accounting fields must sum to PairsDiscovered
// or "N scanned" is a number with no stated relationship to the corpus.
type Corpus struct {
	PairsDiscovered int      `json:"pairs_discovered"`
	SkipListed      int      `json:"skip_listed"`
	NoSidecar       int      `json:"no_sidecar"`
	SidecarsScanned int      `json:"sidecars_scanned"`
	Unreadable      []string `json:"unreadable,omitempty"`

	TotalItems int `json:"total_items"`
}

// CheckReport is one check's result. Every check carries its own
// denominator, its own unmeasured count, and its own ceiling — a check
// that does not state what it cannot see invites its silence to be read
// as coverage.
type CheckReport struct {
	Name string `json:"name"`
	// Measured is the number of items this check could actually judge.
	// It is NOT always TotalItems: force drift can only be measured where
	// the source carries detectable force.
	Measured   int            `json:"measured"`
	Unmeasured int            `json:"unmeasured"`
	Clean      int            `json:"clean"`
	ByVerdict  map[string]int `json:"by_verdict"`
	Findings   []Finding      `json:"findings,omitempty"`
	Ceiling    string         `json:"ceiling"`
}

func newCheck(name, ceiling string) *CheckReport {
	return &CheckReport{Name: name, Ceiling: ceiling, ByVerdict: map[string]int{}}
}

// Report is the corpus-wide result of all four checks.
type Report struct {
	Corpus     Corpus       `json:"corpus"`
	Standalone *CheckReport `json:"standalone"`
	MayLevel   *CheckReport `json:"may_level"`
	Force      *CheckReport `json:"force"`
	Duplicates *CheckReport `json:"duplicates"`
}

// Scan walks the artifact directories under projectRoot and runs all four
// checks over every directive and prohibition of every non-skip-listed
// sidecar.
func Scan(projectRoot string, dirs []string) (*Report, error) {
	pairs, err := sidecar.Discover(projectRoot, dirs)
	if err != nil {
		return nil, fmt.Errorf("discover sidecars: %w", err)
	}

	rep := &Report{
		Standalone: newCheck("standalone",
			"detects a trailing colon, a closed list of deictic phrases, and four positional "+
				"constructions. It cannot detect a novel phrasing, and it cannot tell whether a "+
				"demonstrative is resolved by an antecedent inside the same directive."),
		MayLevel: newCheck("may-level",
			"reads the directive's own modal only. A directive that is non-normative "+
				"WITHOUT saying MAY — a completed one-time step, a description of an "+
				"outcome — is invisible to it. That class needs judgment, not a regex."),
		Force: newCheck("normative-force",
			"compares MODALS only, in the one excerpt sentence the directive renders, and "+
				"reports WEAKENING only. It is blind to obligation carried by indicative mood "+
				"— a source reading 'the verdict IS BLOCKED' rendered as 'MAY be BLOCKED' is "+
				"binding in English and UNMEASURED here. Most of the corpus is unmeasured, and "+
				"unmeasured is never a pass."),
		Duplicates: newCheck("duplicates",
			"matches EXACTLY after normalisation, plus whole-key containment, within one "+
				"topic. Two directives stating one rule in genuinely different words are "+
				"invisible to it. It reports redundancy, never which of a pair to keep."),
	}

	var items []Item

	for _, p := range pairs {
		rep.Corpus.PairsDiscovered++
		if p.Skip {
			rep.Corpus.SkipListed++
			continue
		}
		if p.Sidecar == nil {
			if p.LoadErr != nil {
				rep.Corpus.Unreadable = append(rep.Corpus.Unreadable, p.ArtifactID)
			} else {
				rep.Corpus.NoSidecar++
			}
			continue
		}
		rep.Corpus.SidecarsScanned++

		for i, d := range p.Sidecar.Directives {
			items = append(items, Item{
				ArtifactID: p.ArtifactID, Topic: p.Sidecar.Topic,
				Kind: "directive", Index: i, Text: d.Text, Quote: d.SourceExcerpt.Quote,
			})
		}
		for i, pr := range p.Sidecar.Prohibitions {
			items = append(items, Item{
				ArtifactID: p.ArtifactID, Topic: p.Sidecar.Topic,
				Kind: "prohibition", Index: i, Text: pr.Text, Quote: pr.SourceExcerpt.Quote,
			})
		}
	}

	sort.Strings(rep.Corpus.Unreadable)
	rep.Corpus.TotalItems = len(items)

	for _, it := range items {
		rep.checkStandalone(it)
		rep.checkMayLevel(it)
		rep.checkForce(it)
	}
	rep.checkDuplicates(items)

	return rep, nil
}

func (r *Report) checkStandalone(it Item) {
	c := r.Standalone
	c.Measured++
	v := sidecar.ClassifyStandalone(it.Text)
	c.ByVerdict[string(v)]++
	if v == sidecar.StandaloneOK {
		c.Clean++
		return
	}
	c.Findings = append(c.Findings, Finding{Item: it, Verdict: string(v)})
}

func (r *Report) checkMayLevel(it Item) {
	c := r.MayLevel
	c.Measured++
	if !sidecar.IsMayLevel(it.Text) {
		c.Clean++
		c.ByVerdict["normative"]++
		return
	}
	c.ByVerdict["may_level"]++
	c.Findings = append(c.Findings, Finding{Item: it, Verdict: "may_level"})
}

func (r *Report) checkForce(it Item) {
	c := r.Force
	v := sidecar.ClassifyForce(it.Text, it.Quote)
	c.ByVerdict[string(v)]++
	if v == sidecar.ForceUnmeasured {
		c.Unmeasured++
		return
	}
	c.Measured++
	if v == sidecar.ForceMatch {
		c.Clean++
		return
	}
	c.Findings = append(c.Findings, Finding{Item: it, Verdict: string(v)})
}

// checkDuplicates compares within a topic, because a topic file is the unit
// a reader actually loads. The same rule appearing in two different topic
// files is routing, not redundancy.
func (r *Report) checkDuplicates(items []Item) {
	c := r.Duplicates
	c.Measured = len(items)

	byTopic := map[string][]int{}
	keys := make([]string, len(items))
	for i, it := range items {
		keys[i] = sidecar.DuplicateKey(it.Text)
		byTopic[it.Topic] = append(byTopic[it.Topic], i)
	}

	topics := make([]string, 0, len(byTopic))
	for t := range byTopic {
		topics = append(topics, t)
	}
	sort.Strings(topics)

	flagged := map[int]bool{}
	for _, t := range topics {
		idxs := byTopic[t]
		for a := 0; a < len(idxs); a++ {
			for b := a + 1; b < len(idxs); b++ {
				ia, ib := idxs[a], idxs[b]
				ka, kb := keys[ia], keys[ib]
				if ka == "" || kb == "" {
					continue
				}
				var verdict string
				switch {
				case ka == kb:
					verdict = "duplicate_exact"
				case containsKey(ka, kb) || containsKey(kb, ka):
					// One key wholly inside the other: the same rule stated
					// at two granularities, which is the shape the class-3
					// finding described.
					verdict = "duplicate_subsumed"
				default:
					continue
				}
				c.ByVerdict[verdict]++
				c.Findings = append(c.Findings, Finding{
					Item: items[ia], Verdict: verdict, Partner: items[ib].Ref(),
				})
				flagged[ia], flagged[ib] = true, true
			}
		}
	}
	c.Clean = len(items) - len(flagged)
}

// containsKey reports whether outer wholly contains inner on word
// boundaries. Substring containment alone would match "must not" inside
// "must note", so both edges are checked against a space.
func containsKey(outer, inner string) bool {
	if len(inner) == 0 || len(inner) >= len(outer) {
		return false
	}
	return strings.Contains(" "+outer+" ", " "+inner+" ")
}

// ParentLines is exported for callers that want to re-read a parent body;
// the checks here need only the sidecar, but a reporting caller may want
// the prose. Kept in one place so no caller re-implements the CRLF split.
func ParentLines(projectRoot, parentPath string) ([]string, error) {
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

// Summary renders all four checks with their denominators and ceilings.
func (r *Report) Summary() string {
	var b strings.Builder
	fmt.Fprintf(&b, "Normativity: %d item(s) across %d sidecar(s)\n",
		r.Corpus.TotalItems, r.Corpus.SidecarsScanned)
	fmt.Fprintf(&b, "  corpus: %d pair(s) discovered = %d scanned + %d skip-listed + "+
		"%d without a sidecar + %d unreadable\n",
		r.Corpus.PairsDiscovered, r.Corpus.SidecarsScanned, r.Corpus.SkipListed,
		r.Corpus.NoSidecar, len(r.Corpus.Unreadable))
	if n := len(r.Corpus.Unreadable); n > 0 {
		fmt.Fprintf(&b, "  UNMEASURED: %d sidecar(s) would not load: %s\n",
			n, strings.Join(r.Corpus.Unreadable, ", "))
	}
	for _, c := range []*CheckReport{r.Standalone, r.MayLevel, r.Force, r.Duplicates} {
		b.WriteString("\n" + c.render())
	}
	return b.String()
}

func (c *CheckReport) render() string {
	var b strings.Builder
	fmt.Fprintf(&b, "  [%s] %d of %d clean", c.Name, c.Clean, c.Measured)
	if c.Unmeasured > 0 {
		fmt.Fprintf(&b, ", %d unmeasured", c.Unmeasured)
	}
	b.WriteString("\n")
	if c.Unmeasured > 0 {
		b.WriteString("    Unmeasured is NOT a pass — nothing was compared for those.\n")
	}
	kinds := make([]string, 0, len(c.ByVerdict))
	for k := range c.ByVerdict {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	for _, k := range kinds {
		fmt.Fprintf(&b, "    %-28s %d\n", k, c.ByVerdict[k])
	}
	fmt.Fprintf(&b, "    ceiling: %s\n", c.Ceiling)
	return b.String()
}
