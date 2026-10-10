package phaseb

// directiveindex.go — surface (c) of the four-surface render.
//
// The directive index is the hook tier's input: a map of glob -> graded
// directive entries, written only here and read by `bin/edikt hook match`.
//
// WHY IT CARRIES REMINDERS
//
// The ambient core used to end with a Reminders section listing every
// reminder in the corpus, loaded on every edit regardless of subject. Stage 1
// removes that section — and a reminder with nowhere to go is a silent loss,
// which is the failure class this release exists to close. So reminder
// aggregation RE-TARGETS here: each entry carries its source artifact's
// reminders, and they are delivered at write-touch time, when the reader is
// actually editing a file the directive covers.
//
// DETERMINISM
//
// Glob keys and entries are emitted in sorted order through a text writer,
// never through yaml.Marshal of a map: Go randomises map iteration, so
// marshalling one would produce a different byte sequence per run and make
// the render manifest's hash churn on identical input. No timestamps.

import (
	"fmt"
	"strings"
)

// gradeFor pins the enforcement grade at RENDER time.
//
// ADR-071 (amends ADR-064): grade is class-weighted, not derived from modal  edikt-guard:allow
// verbs in directive text. Measured cause of the prior approach's failure:
// gradeFor read its own upstream normalization back through a modal regex —
// ADR prose is overwhelmingly written as plain declarative decisions, and a
// present-tense declarative describing an architectural decision is,
// correctly, promoted to MUST-phrased text at extraction time, so the modal
// check downstream had almost nothing left to discriminate on (98.2% must).
//
// Class table (ADR-071):
//
//	INV-*, directive or prohibition  -> must
//	ADR-*, prohibition               -> must
//	ADR-*, directive                 -> advisory
//	GL-*,  directive or prohibition  -> advisory
//
// `enforcement` (ADR-070's schema v2.1 field) overrides the class default
// when set, in EITHER direction — "deny" forces must even on an otherwise-
// advisory class, "advisory" forces advisory even on an invariant or ADR
// prohibition a reviewer judges doesn't warrant blocking. Absent, the class
// table applies unconditionally — this is what keeps invariants must-graded
// by default, unchanged from before this ADR.
//
// Pinned here — and recorded per entry — precisely so the hook never
// re-derives it: a consumer re-deriving grade could silently downgrade an
// invariant to advisory and nothing would report the difference.
func gradeFor(class string, isProhibition bool, enforcement string) string {
	switch enforcement {
	case "deny":
		return gradeMust
	case "advisory":
		return gradeAdvisory
	}
	if class == "invariant" {
		return gradeMust
	}
	// SR-003 (SPEC-018): an ADR prohibition is NO LONGER promoted to must.
	//
	// The rule it promoted read `class == "adr" && isProhibition -> must`, and it
	// put 112 of 238 write-time MUSTs into the deny channel — 34.8% of the
	// delivered payload — as REJECTED OPTIONS. A prohibition on an ADR records
	// the alternative the decision turned down. Interrupting a write to say what
	// somebody decided against is not the same speech act as telling the author
	// what this write must do, and the deny channel is the one that interrupts.
	//
	// Measured, not assumed (Stage 0, 20553b39): dropping them is the only
	// payload change that carries a verdict on its own — 1,133 -> 738
	// est-tokens per deny, -34.9%, independent of the depth question that killed
	// designs C and D.
	//
	// They are not dropped from governance. Advisory routes them to the topic
	// file, which loads on a paths: match BEFORE the write rather than
	// interrupting it — earlier delivery for a rule about what not to choose.
	//
	// An ADR prohibition that genuinely must interrupt keeps `enforcement: deny`,
	// which is checked above and still returns must. The class table stops being
	// the mechanism; the per-directive override becomes it.
	return gradeAdvisory
}

// The two grades, named because ADR-079 makes grade the ROUTING KEY between  edikt-guard:allow
// two surfaces rendered by two different files. A literal "advisory" compared
// in merge.go against a literal "advisory" returned here is a coupling no
// compiler checks; a typo on either side would route a directive to neither
// surface and no gate would report the silence.
const (
	gradeMust     = "must"
	gradeAdvisory = "advisory"
)

// classFor names the source artifact's structural kind, from its ID prefix
// alone — the same signal gradeFor already consults for invariants, just
// retained on the entry instead of discarded after the grade decision.
//
// DESIGN-QUESTIONS-2026-08-16.md Q2, option 1: ADR-064 correctly made grade  edikt-guard:allow
// derive from actual obligation strength, and the practical effect is that
// `must` is now the default state for most of a real corpus (743/759 on this
// repo's own, ~98% on a measured foreign one) — grade alone no longer
// separates "this blocks a write" from "how costly is being wrong about this
// one." Class is a zero-authoring-cost, compile-time-derived first cut at a
// second axis: every sidecar already carries this information in its own ID
// prefix, so deriving it here costs nothing and depends on no re-extraction.
// It is presentation-only until a delivery-tier consumer decides to act on
// it — see hook_match.go's class-priority sort of denied entries, which is
// the one consumer today.
func classFor(artifactID string) string {
	switch {
	case strings.HasPrefix(artifactID, "INV-"):
		return "invariant"
	case strings.HasPrefix(artifactID, "ADR-"):
		return "adr"
	case strings.HasPrefix(artifactID, "GL-"):
		return "guideline"
	default:
		return "unknown"
	}
}

// entryID builds the stable "<ARTIFACT>:<d|p><NN>" id.
//
// Session dedup keys on this value, so byte-equal sidecar input must yield
// byte-equal ids. The ordinal is 1-based and zero-padded to two digits, which
// the schema pattern requires and which keeps ids sorting lexically in the
// same order they appear in the sidecar.
func entryID(artifactID string, kind byte, ordinal int) string {
	return fmt.Sprintf("%s:%c%02d", artifactID, kind, ordinal)
}

// resolvePaths implements ADR-070's per-entry override: an entry's own Paths
// when set (schema/Validate already reject an explicit empty override, so
// non-empty is the only non-nil case), else the sidecar's Paths unchanged.
func resolvePaths(entryPaths, sidecarPaths []string) []string {
	if len(entryPaths) > 0 {
		return entryPaths
	}
	return sidecarPaths
}

// yamlQuote double-quotes a scalar and escapes what a double-quoted YAML
// scalar requires. Directive text routinely contains ": ", backticks, "#" and
// quotes — emitting it bare is the exact class of parse failure the extractor
// prompt already warns about, and it would land here as a corrupt index.
func yamlQuote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}
