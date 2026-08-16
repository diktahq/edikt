package sidecar

import (
	"regexp"
	"strings"
)

// Text-level directive checks — the second measurement after grounding.
//
// Grounding proves a quote EXISTS in the prose at the recorded lines. It
// says nothing about whether what was extracted is a rule. Nine runbook
// steps rendered as standing law score 100% grounded. The extraction
// problem is NORMATIVITY, not anchoring, and everything below reads the
// directive's own text (and, for force drift, its excerpt) rather than the
// anchor.
//
// Every classifier here is a closed list or an exact comparison. None of
// them judge meaning. Where a check cannot see something, the Ceiling
// string on its report says so rather than leaving the gap to the reader.

// ─── Modal force ──────────────────────────────────────────────────────────
//
// One modal extractor, used by BOTH the MAY-level check and the
// normative-force check. Two copies of "what modal is this?" is how two
// derivations of one fact drift apart — the same reason ClassifyExcerpt
// delegates its containment test to excerptStale's helpers.

// ModalLevel is RFC-2119 force, ordered so it can be compared.
type ModalLevel int

const (
	// ModalNone — no RFC-2119 keyword found. For source prose this is the
	// common case (imperative or descriptive text), and it is UNMEASURED
	// for force purposes, never "weak".
	ModalNone ModalLevel = iota
	ModalMay              // MAY, OPTIONAL — permission
	ModalShould           // SHOULD, SHOULD NOT, RECOMMENDED — recommendation
	ModalMust             // MUST, MUST NOT, SHALL, NEVER, REQUIRED — requirement
)

func (m ModalLevel) String() string {
	switch m {
	case ModalMay:
		return "MAY"
	case ModalShould:
		return "SHOULD"
	case ModalMust:
		return "MUST"
	}
	return "none"
}

// The STRONG forms are matched case-insensitively on BOTH sides.
//
// The first version of this matched the directive side case-sensitively,
// on the assumption that compiled directives are authored in uppercase
// RFC-2119 form. The live corpus says otherwise: "Absence of output is
// never evidence of completion" (INV-011), "The new ADR must reference the
// superseded one" (INV-002), "Held items never vanish" (ADR-050). Four
// directives were reported as force-weakened purely because their modal was
// lowercase and the check could not see it. Pin the shape production
// actually emits, not the shape the schema suggests.
//
// The WEAK forms stay uppercase-only. In ordinary prose "may" and "should"
// are overwhelmingly not modal — "a change may break X", "we should revisit
// this" — so matching them case-insensitively would classify narrative as
// permission. A lowercase "should" in a directive therefore reads as
// ModalNone; the ceiling says so.
var (
	reMustAny     = regexp.MustCompile(`(?i)\b(must|shall|never|required)\b`)
	reShouldUpper = regexp.MustCompile(`\b(SHOULD|RECOMMENDED)\b`)
	reMayUpper    = regexp.MustCompile(`\b(MAY|OPTIONAL)\b`)
)

// DirectiveModal returns the strongest RFC-2119 level asserted by an
// authored directive. Strongest wins: "MUST NOT do X, and SHOULD prefer Y"
// is a MUST-level rule.
func DirectiveModal(text string) ModalLevel {
	switch {
	case reMustAny.MatchString(text):
		return ModalMust
	case reShouldUpper.MatchString(text):
		return ModalShould
	case reMayUpper.MatchString(text):
		return ModalMay
	}
	return ModalNone
}

// SourceModal returns the force detectable in source prose: ModalMust or
// ModalNone. The weak forms are not detected on this side at all — see the
// regex block above.
func SourceModal(text string) ModalLevel {
	if reMustAny.MatchString(text) {
		return ModalMust
	}
	return ModalNone
}

// ─── Sentence attribution ─────────────────────────────────────────────────
//
// A source_excerpt quote is frequently a multi-sentence passage. Asking
// "does this quote contain a MUST?" then compares the directive against
// whichever sentence happened to carry one, which need not be the sentence
// the directive renders.
//
// Three of the first run's seven force findings were exactly that. ADR-040
// paired "Changing the default adversary model requires an amending ADR"
// against a quote whose MUST belongs to a different sentence about
// `model: opus`. The modal was real, the attribution was invented.
//
// So the comparison is made against the ONE sentence of the quote that
// best overlaps the directive, and against nothing else.

// Terminal punctuation, then any run of closing markdown/quote characters,
// then whitespace or end.
//
// The trailing character class is not decoration. ADR-010's excerpt ends a
// sentence with "could not execute.**" — the period is followed by bold
// markers, not a space. Without them the splitter merged that sentence with
// the next one, the merged text carried a MUST from the wrong clause, and
// the check reported a weakening it could not actually attribute. The
// corpus is markdown; the splitter has to read markdown.
var reSentenceSplit = regexp.MustCompile("[.!?][)\\]\"'*_`]*(?:\\s|$)")

// splitSentences cuts a passage on terminal punctuation followed by
// whitespace or end-of-string. Deliberately naive: "v0.6.0" and "e.g" keep
// their periods when not followed by space, and an abbreviation that IS
// followed by a space over-splits into a shorter sentence. Over-splitting
// costs attribution precision; under-splitting invents it.
func splitSentences(s string) []string {
	var out []string
	last := 0
	for _, loc := range reSentenceSplit.FindAllStringIndex(s, -1) {
		if seg := strings.TrimSpace(s[last:loc[1]]); seg != "" {
			out = append(out, seg)
		}
		last = loc[1]
	}
	if seg := strings.TrimSpace(s[last:]); seg != "" {
		out = append(out, seg)
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

// Overlap is scored on CONTENT words only.
//
// Scoring every word made "The compiler MUST stamp the version" overlap
// with "Sandboxes MUST be hermetic" on {the, must} — two unrelated rules
// reading as attributable. Modals are the worst offenders: they are exactly
// the token the comparison is about, so counting them lets any two
// normative sentences vouch for each other.
var stopWords = map[string]bool{
	"the": true, "a": true, "an": true, "of": true, "to": true, "in": true,
	"and": true, "or": true, "is": true, "are": true, "be": true, "been": true,
	"it": true, "its": true, "this": true, "that": true, "these": true,
	"those": true, "for": true, "with": true, "on": true, "by": true,
	"as": true, "at": true, "from": true, "when": true, "if": true,
	"then": true, "than": true, "not": true, "no": true, "any": true,
	"every": true, "all": true, "each": true, "which": true, "so": true,
	// Modals — the subject of the comparison, never evidence for it.
	"must": true, "shall": true, "never": true, "may": true,
	"should": true, "required": true, "optional": true, "recommended": true,
}

func contentWords(s string) map[string]bool {
	m := map[string]bool{}
	for _, w := range strings.Fields(strings.ToLower(reNonWord.ReplaceAllString(s, " "))) {
		if !stopWords[w] {
			m[w] = true
		}
	}
	return m
}

// attributedSentence returns the sentence of quote with the highest word
// overlap against directiveText, and how many words they share. A zero
// overlap means no sentence of the quote corresponds to the directive, and
// the caller must not compare modals across them.
func attributedSentence(directiveText, quote string) (string, int) {
	dw := contentWords(directiveText)
	best, bestScore := "", 0
	for _, s := range splitSentences(quote) {
		score := 0
		for w := range contentWords(s) {
			if dw[w] {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = s, score
		}
	}
	return best, bestScore
}

// ─── 2a. Standalone ───────────────────────────────────────────────────────

// StandaloneVerdict says whether a directive can be obeyed without the
// source document open beside it.
type StandaloneVerdict string

const (
	StandaloneOK StandaloneVerdict = "standalone"

	// StandaloneTrailingColon — the text ends with ':', so it announces
	// content it does not carry. "All integration tests in
	// internal/rag/db_test.go MUST follow this pattern:" is grounded,
	// well-formed, and unusable.
	StandaloneTrailingColon StandaloneVerdict = "trailing_colon"

	// StandaloneDeixis — the text points at something outside itself
	// ("this pattern", "the above") with no antecedent it carries.
	StandaloneDeixis StandaloneVerdict = "unresolved_deixis"
)

// Deictic phrases that cannot resolve inside a single directive. A closed,
// auditable list — not a heuristic for "vagueness".
//
// Two families:
//
//  1. Positional references. "the above", "the following" name a location
//     in a document the directive no longer travels with.
//  2. Demonstrative + generic noun. "this pattern" is unresolvable unless
//     the pattern is named; the noun list is restricted to nouns that carry
//     no content of their own. "this schema" is NOT here — a reader can
//     find the schema; "this way" gives them nothing.
//
// Matched case-insensitively on word boundaries.
var deicticPhrases = []string{
	"as follows", "the former", "the latter", "the preceding",
	"this pattern", "these patterns",
	"this approach", "these approaches",
	"this shape", "these shapes",
	"this way", "these ways",
	"this list", "these lists",
	"this form", "these forms",
	"this case", "these cases",
	"this step", "these steps",
	"the same pattern", "the same approach", "the same way",
}

var reDeicticPhrase = func() *regexp.Regexp {
	quoted := make([]string, len(deicticPhrases))
	for i, p := range deicticPhrases {
		quoted[i] = regexp.QuoteMeta(p)
	}
	return regexp.MustCompile(`(?i)\b(` + strings.Join(quoted, "|") + `)\b`)
}()

// Positional deixis, matched as a construction rather than an enumeration.
//
// The first version listed "the above", "the following", "listed below" and
// so on. It missed ADR-029's "provided the four rules below all hold" —
// the same defect with one more word in the noun phrase. Enumerating the
// surface forms of a productive construction only ever catches the forms
// someone thought of.
//
// But the construction must stay narrow. Matching a bare "above|below|
// following" would catch "following a failed dispatch", where "following"
// is temporal and points at nothing. Each alternative below requires a
// determiner, a positional verb, or "see" — the markers that make the word
// a pointer to a location in a document rather than ordinary English.
var reDeicticPositional = regexp.MustCompile(`(?i)(` +
	// "the above", "the four rules below", "the table above"
	`\bthe\s+(\w+\s+){0,3}(above|below)\b` +
	// "shown above", "listed below", "set out below"
	`|\b(shown|described|listed|defined|named|specified|set\s+out|given|enumerated)\s+(above|below)\b` +
	// "see above", "see below"
	`|\bsee\s+(above|below)\b` +
	// "the following", "the preceding", "the foregoing"
	`|\bthe\s+(following|preceding|foregoing)\b` +
	`)`)

// refTail strips the "(ref: ADR-NNN)" suffix every compiled directive
// carries. It is machinery, not authored text, and must not affect whether
// a directive reads as standalone or count toward duplicate comparison.
var refTail = regexp.MustCompile(`\s*\(ref:\s*[^)]*\)\s*$`)

// StripRefTail removes the compiled "(ref: …)" suffix if present.
func StripRefTail(text string) string {
	return strings.TrimSpace(refTail.ReplaceAllString(strings.TrimSpace(text), ""))
}

// ClassifyStandalone reports whether the directive stands on its own.
//
// The ref tail is stripped first: every compiled directive ends with
// "(ref: ADR-NNN)", so an un-stripped trailing-colon test would never fire
// and an un-stripped deixis test would be reading machinery.
func ClassifyStandalone(text string) StandaloneVerdict {
	t := StripRefTail(text)
	if strings.HasSuffix(t, ":") {
		return StandaloneTrailingColon
	}
	if reDeicticPhrase.MatchString(t) || reDeicticPositional.MatchString(t) {
		return StandaloneDeixis
	}
	return StandaloneOK
}

// ─── 2b. MAY-level ────────────────────────────────────────────────────────

// IsMayLevel reports whether a directive's strongest force is MAY.
//
// A MAY cannot be violated. Nothing an engineer or an agent does can
// contradict "X MAY be rotated manually", so it is not a directive — it is
// a note that reached the directive list. This is a statement about the
// item's category, not its quality.
func IsMayLevel(text string) bool {
	return DirectiveModal(StripRefTail(text)) == ModalMay
}

// ─── 2c. Normative force ──────────────────────────────────────────────────

// ForceVerdict compares a directive's asserted force against the force
// present in the prose it was extracted from.
type ForceVerdict string

const (
	// ForceMatch — both carry MUST-level force.
	ForceMatch ForceVerdict = "force_match"

	// ForceWeakened — the source says MUST (or shall/never) and the
	// directive says SHOULD or MAY. A rule the author made binding, made
	// optional in the artifact agents actually read. This is the defect
	// the check exists for.
	ForceWeakened ForceVerdict = "force_weakened"

	// ForceUnmeasured — the source carries no detectable RFC-2119 force,
	// so there is nothing to compare against. Imperative prose ("Do X")
	// legitimately compiles to "MUST do X", and descriptive prose has no
	// force at all; neither can be judged from the modal alone.
	//
	// Reported as its own verdict, never folded into the pass count
	// (INV-013). An unmeasured pair is not a matching pair.
	ForceUnmeasured ForceVerdict = "force_unmeasured"
)

// ClassifyForce compares directive force against the force of the ONE
// sentence of its excerpt that the directive actually renders.
//
// Only WEAKENING is reported as a defect. Strengthening is not checked:
// SourceModal deliberately cannot see lowercase "may"/"should", so every
// "source is weak, directive is strong" pair would be indistinguishable
// from ordinary imperative prose compiled to MUST. A check that cannot tell
// its signal from its noise must not report either.
func ClassifyForce(directiveText, quote string) ForceVerdict {
	text := StripRefTail(directiveText)
	if strings.TrimSpace(quote) == "" {
		return ForceUnmeasured
	}
	sentence, overlap := attributedSentence(text, quote)
	// No sentence of the quote shares vocabulary with the directive, so
	// there is nothing to attribute a modal to. Comparing anyway is how the
	// first run produced three findings out of unrelated clauses.
	if overlap == 0 {
		return ForceUnmeasured
	}
	if SourceModal(sentence) != ModalMust {
		return ForceUnmeasured
	}
	switch DirectiveModal(text) {
	case ModalMust:
		return ForceMatch
	case ModalShould, ModalMay:
		return ForceWeakened
	}
	// Source sentence is binding, directive asserts no force at all. Same
	// failure as a downgrade, and worse: no modal left to argue about.
	return ForceWeakened
}

// ─── 2d. Duplicates ───────────────────────────────────────────────────────

var reNonWord = regexp.MustCompile(`[^\p{L}\p{N}\s]+`)

// DuplicateKey normalises a directive to a comparison key: ref tail
// stripped, punctuation and backticks removed, case folded, whitespace
// collapsed.
//
// Two directives sharing a key are the same sentence dressed differently.
// It is exact-after-normalisation and nothing cleverer — a fuzzy matcher
// would need a threshold, and a threshold is a judgment call this check
// exists to avoid.
func DuplicateKey(text string) string {
	t := StripRefTail(text)
	t = reNonWord.ReplaceAllString(t, " ")
	return strings.ToLower(normalizeWS(t))
}
