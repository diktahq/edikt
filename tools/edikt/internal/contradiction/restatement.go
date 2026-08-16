package contradiction

import (
	"fmt"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Restatement is one directive body asserted by two different artifacts:
// same normalized noun-phrase, same modality, different sources.
//
// WHY THIS IS NOT A CONTRADICTION. Detect finds two artifacts that DISAGREE.
// This finds two that AGREE, which is a different defect with a different
// remedy. An unattributed cross-artifact restatement is one of the extractor's
// two residual classes: the rule is real, but the second copy has no
// provenance of its own, so a reader cannot tell which artifact governs it and
// an editor changing one leaves the other stating the old rule.
//
// WHY IT IS NOT TOPIC-SCOPED. Detect scopes to a topic because a topic is the
// co-loading set — two contradicting rules matter when a session loads them
// together. A restatement's damage is the opposite shape: the copies are most
// dangerous when they land in DIFFERENT topics, because then no single surface
// shows both and the divergence is invisible. It also spans the case AC-4.13
// exists for — the same body on a pathless artifact and on a paths-declaring
// one, where dropping the wrong copy silently lowers write-time coverage.
//
// ADVISORY, NEVER AUTO-RESOLVED. Which copy is canonical is a question about
// authorial intent, and the corpus carries no rule that answers it. Picking
// one would be the system resolving a conflict it has no precedence rule for,
// which AC-9.2 forbids.
type Restatement struct {
	NounPhrase string       `json:"noun_phrase"`
	Modality   string       `json:"modality"`
	A          DirectiveRef `json:"a"`
	B          DirectiveRef `json:"b"`
	ATopic     string       `json:"a_topic"`
	BTopic     string       `json:"b_topic"`
	// SameTopic is carried explicitly rather than left to the reader to
	// derive, because the two cases have different remedies: within a topic
	// the duplicate is visible in one compiled file and can be merged; across
	// topics it is not, and the question is which artifact owns the rule.
	SameTopic bool `json:"same_topic"`
}

type restatementEntry struct {
	entry
	topic string
}

// RestatementReport renders findings for a human. It returns the empty string
// for no findings rather than "no restatements found": whether silence means
// "scanned and clean" or "never scanned" is the caller's fact to state, and a
// reassuring line emitted from here would answer it for them wrongly (INV-013).  edikt-guard:allow
func RestatementReport(rs []Restatement) string {
	if len(rs) == 0 {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d cross-artifact restatement(s) — the same rule asserted by two artifacts.\n", len(rs))
	b.WriteString("Advisory: which copy is canonical is an authorial question the corpus carries no rule for.\n")
	for _, r := range rs {
		scope := "different topics"
		if r.SameTopic {
			scope = "same topic"
		}
		fmt.Fprintf(&b, "  %s [%s] and %s [%s] (%s, %s)\n",
			r.A.Source, r.ATopic, r.B.Source, r.BTopic, scope, strings.ToLower(r.Modality))
		fmt.Fprintf(&b, "      %s\n", r.A.Text)
		fmt.Fprintf(&b, "      %s\n", r.B.Text)
	}
	return b.String()
}

// DetectRestatements reports every cross-artifact duplicate in the corpus.
//
// pairs MUST already have retired artifacts excluded, exactly as Detect
// requires — a superseded ADR restating a live one is not a finding, it is
// what supersession means.
func DetectRestatements(pairs []sidecar.Pair) []Restatement {
	var all []restatementEntry
	for _, p := range pairs {
		if p.Sidecar == nil {
			continue
		}
		for _, d := range p.Sidecar.Directives {
			all = append(all, restatementEntry{entry: entryFor(d.Text, p.ArtifactID), topic: p.Sidecar.Topic})
		}
		for _, pr := range p.Sidecar.Prohibitions {
			all = append(all, restatementEntry{entry: entryFor(pr.Text, p.ArtifactID), topic: p.Sidecar.Topic})
		}
	}

	var out []Restatement
	for i := 0; i < len(all); i++ {
		for j := i + 1; j < len(all); j++ {
			a, b := all[i], all[j]

			// SAME-ARTIFACT DUPLICATES ARE NOT THIS FINDING. Two copies inside
			// one sidecar are a within-artifact extraction defect, already the
			// merge formula's business, and reporting them here would bury the
			// cross-artifact case they were never about.
			if a.Source == b.Source {
				continue
			}
			if a.Modality == "" || b.Modality == "" {
				// No modality means no assertable rule to duplicate. Treating
				// modality-less prose as a restatement match would flag every
				// pair of similar sentences in the corpus.
				continue
			}
			if a.Modality != b.Modality {
				// Same subject, different modality is either a contradiction
				// (Detect's business) or a strength difference. Neither is a
				// restatement, and claiming one here would double-report the
				// pairs Detect already surfaces with the correct framing.
				continue
			}
			if a.norm == "" || b.norm == "" {
				continue
			}
			if lossless.LevenshteinRatio(a.norm, b.norm) > nounPhraseMatchThreshold {
				continue
			}
			out = append(out, Restatement{
				NounPhrase: a.norm,
				Modality:   a.Modality,
				A:          a.DirectiveRef,
				B:          b.DirectiveRef,
				ATopic:     a.topic,
				BTopic:     b.topic,
				SameTopic:  a.topic == b.topic,
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].A.Source != out[j].A.Source {
			return out[i].A.Source < out[j].A.Source
		}
		if out[i].B.Source != out[j].B.Source {
			return out[i].B.Source < out[j].B.Source
		}
		return out[i].NounPhrase < out[j].NounPhrase
	})
	return out
}
