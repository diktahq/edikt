package audit

import (
	"fmt"
	"sort"
	"strings"
)

// RESTATEMENT DETECTION, BEFORE ACCEPTANCE.
//
// A draft was an existing invariant's sentence with one noun substituted, and
// nothing in the pipeline looked. The comparison is mechanical — normalise the
// draft's operative sentences, compare against every directive in the corpus —
// so it runs before a model is asked anything, and its answer is a verdict a
// human can check: `amendment to <record>`, not a new record.
//
// The same comparison, run over the corpus against itself, is the pair-overlap
// detector: two records saying one thing.

// CorpusRule is one directive already in the corpus.
type CorpusRule struct {
	// Artifact is the record that carries it, e.g. "INV-900".
	Artifact string
	// Kind is adr | invariant | guideline.
	Kind string
	// Text is the directive as compiled.
	Text string
}

// Restatement is a draft sentence that says what a corpus rule already says.
type Restatement struct {
	// DraftSentence is the operative sentence from the draft.
	DraftSentence string `json:"draft_sentence"`
	// Artifact is the record that already says it.
	Artifact string `json:"artifact"`
	// ExistingText is that record's directive, verbatim.
	ExistingText string `json:"existing_text"`
	// Verdict is what the auditor must return instead of a new record.
	Verdict string `json:"verdict"`
	// Similarity is the token overlap, 0..1, reported so a reader can judge
	// the match rather than trust it.
	Similarity float64 `json:"similarity"`
	// Exact says the normalised forms were identical — the same sentence,
	// cosmetics aside.
	Exact bool `json:"exact"`
}

// NearMatchThreshold is the token overlap at which two rules are reported as
// the same rule.
//
// 0.85 is deliberately high. The cost of a false positive is a human being
// asked to look at two sentences that differ; the cost of a false negative is
// a corpus with two records requiring the same thing, which is the state this
// detector exists to prevent — but reporting every loosely-related rule as a
// restatement would train a reader to dismiss the report, which is worse than
// not having it. One noun substituted lands well above this line.
const NearMatchThreshold = 0.85

// FindRestatements compares each of the draft's operative sentences against
// every corpus rule and returns the matches, strongest first.
//
// An empty corpus returns no matches AND is worth the caller distinguishing
// from "no matches found": nothing was compared. The caller reports which,
// because a detector that says "clean" over an empty corpus is the shape that
// makes absence read as safety.
func FindRestatements(draftSentences []string, corpus []CorpusRule) []Restatement {
	var out []Restatement
	for _, s := range draftSentences {
		ns := Normalise(s)
		if ns == "" {
			continue
		}
		for _, rule := range corpus {
			nr := Normalise(rule.Text)
			if nr == "" {
				continue
			}
			exact := ns == nr
			sim := tokenOverlap(ns, nr)
			if !exact && sim < NearMatchThreshold {
				continue
			}
			out = append(out, Restatement{
				DraftSentence: strings.TrimSpace(s),
				Artifact:      rule.Artifact,
				ExistingText:  rule.Text,
				Verdict:       fmt.Sprintf("amendment to %s", rule.Artifact),
				Similarity:    sim,
				Exact:         exact,
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Similarity != out[j].Similarity {
			return out[i].Similarity > out[j].Similarity
		}
		return out[i].Artifact < out[j].Artifact
	})
	return out
}

// Overlap is two corpus rules that say the same thing.
type Overlap struct {
	A          CorpusRule `json:"a"`
	B          CorpusRule `json:"b"`
	Similarity float64    `json:"similarity"`
	Exact      bool       `json:"exact"`
}

// FindOverlaps runs the same comparison over the corpus against itself: two
// records requiring one thing. It comes free with the restatement detector,
// and it is the same defect seen from the other side.
func FindOverlaps(corpus []CorpusRule) []Overlap {
	var out []Overlap
	for i := 0; i < len(corpus); i++ {
		for j := i + 1; j < len(corpus); j++ {
			a, b := Normalise(corpus[i].Text), Normalise(corpus[j].Text)
			if a == "" || b == "" {
				continue
			}
			// A rule paired with itself in the SAME record is not an overlap.
			if corpus[i].Artifact == corpus[j].Artifact && a == b {
				continue
			}
			exact := a == b
			sim := tokenOverlap(a, b)
			if !exact && sim < NearMatchThreshold {
				continue
			}
			out = append(out, Overlap{A: corpus[i], B: corpus[j], Similarity: sim, Exact: exact})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Similarity > out[j].Similarity })
	return out
}

// tokenOverlap is the Jaccard index over normalised tokens: shared tokens over
// distinct tokens across both.
//
// Chosen over an edit distance because it is invariant to word ORDER, and the
// rewrite that hides a restatement is usually a reordering plus a synonym. It
// is reported alongside every match so a reader judges the number rather than
// trusting the threshold.
func tokenOverlap(a, b string) float64 {
	at, bt := tokenSet(a), tokenSet(b)
	if len(at) == 0 || len(bt) == 0 {
		return 0
	}
	shared := 0
	for tok := range at {
		if bt[tok] {
			shared++
		}
	}
	union := len(at) + len(bt) - shared
	if union == 0 {
		return 0
	}
	return float64(shared) / float64(union)
}

func tokenSet(s string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range strings.Fields(s) {
		out[tok] = true
	}
	return out
}
