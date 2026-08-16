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
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// IndexEntry is one graded directive as the hook consumes it.
type IndexEntry struct {
	ID                    string
	Grade                 string
	Class                 string
	Text                  string
	Intent                string
	FalsifyingObservation string
	Reminders             []string
}

// gradeFor pins the enforcement grade at RENDER time.
//
// `must` iff the source artifact is an invariant (structural — the
// strongest class regardless of wording), or the text's strongest RFC-2119
// modal is MUST-level (MUST, SHALL, NEVER, REQUIRED — either polarity).
// Pinned here — and recorded per entry — precisely so the hook never
// re-derives it: a consumer re-deriving grade could silently downgrade an
// invariant to advisory and nothing would report the difference.
//
// ADR-064: this used to check only for the negative-polarity markers  edikt-guard:allow
// "MUST NOT"/"NEVER"/"NO EXCEPTIONS", so a bare positive MUST — the
// overwhelming majority of how directives are actually authored — fell
// through to advisory for every non-invariant artifact. Reuses
// sidecar.DirectiveModal (already trusted for force-weakening detection
// during extraction) rather than a second, independent keyword list — two
// implementations of "what modal is this?" is the drift GL-002 names, and
// the weaker one is exactly what the old three-word list turned out to be.
func gradeFor(artifactID, text string) string {
	if strings.HasPrefix(artifactID, "INV-") {
		return "must"
	}
	if sidecar.DirectiveModal(text) == sidecar.ModalMust {
		return "must"
	}
	return "advisory"
}

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

// BuildDirectiveIndex maps each declared glob to the entries scoped to it.
//
// Only artifacts that DECLARE paths contribute: an index keyed by glob has no
// key for an artifact that named none. Those artifacts reach the reader
// through the ambient core (pathless invariants) or the skill package
// (everything else) — the tier ladder, not this surface.
func BuildDirectiveIndex(pairs []sidecar.Pair) map[string][]IndexEntry {
	out := map[string][]IndexEntry{}

	sorted := append([]sidecar.Pair(nil), pairs...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ArtifactID < sorted[j].ArtifactID })

	for _, p := range sorted {
		sc := p.Sidecar
		if sc == nil || len(sc.Paths) == 0 {
			continue
		}

		dirs, prohs := IndexEntriesFor(p)
		entries := make([]IndexEntry, 0, len(dirs)+len(prohs))
		entries = append(entries, dirs...)
		entries = append(entries, prohs...)
		if len(entries) == 0 {
			continue
		}

		globs := append([]string(nil), sc.Paths...)
		sort.Strings(globs)
		for _, g := range globs {
			out[g] = append(out[g], entries...)
		}
	}
	return out
}

// IndexEntriesFor returns the directive and prohibition entries a single
// SCOPED sidecar contributes to the directive index, split by kind. A sidecar
// declaring no `paths:` contributes nothing and yields two nil slices.
//
// It is exported, and split by kind, because the topic-file and skill-package
// renderers have to STATE how much of a topic was routed here instead of into
// their own bodies (ADR-066). A second, independently written counter is  edikt-guard:allow
// exactly the drift GL-002 names: the honest note would quietly stop matching
// the index it describes, and nothing would report the difference. One
// function decides what lands in the index; everything that reports on the
// index calls it.
func IndexEntriesFor(p sidecar.Pair) (directives, prohibitions []IndexEntry) {
	sc := p.Sidecar
	if sc == nil || len(sc.Paths) == 0 {
		return nil, nil
	}

	suppressed := make(map[string]struct{}, len(sc.SuppressedDirectives))
	for _, sd := range sc.SuppressedDirectives {
		suppressed[sd] = struct{}{}
	}

	for i, d := range sc.Directives {
		if _, skip := suppressed[d.Text]; skip {
			continue
		}
		// ADR-065: actor-scoped directives constrain an edikt-internal
		// automated operation, not file content a human edit could
		// violate. This index is hook match's exclusive write-time
		// data source (see file header) — excluding them here, and
		// only here, removes the PreToolUse noise while leaving every
		// other rendered surface (ambient core, topic files) untouched,
		// since those render from sc.Directives/sc.Prohibitions
		// directly, not through this function.
		if d.ActorScope {
			continue
		}
		directives = append(directives, IndexEntry{
			ID:                    entryID(p.ArtifactID, 'd', i+1),
			Grade:                 gradeFor(p.ArtifactID, d.Text),
			Class:                 classFor(p.ArtifactID),
			Text:                  d.Text,
			Intent:                d.Intent,
			FalsifyingObservation: d.FalsifyingObservation,
			Reminders:             sc.Reminders,
		})
	}
	for i, ph := range sc.Prohibitions {
		if ph.ActorScope {
			continue
		}
		prohibitions = append(prohibitions, IndexEntry{
			ID:                    entryID(p.ArtifactID, 'p', i+1),
			Grade:                 gradeFor(p.ArtifactID, ph.Text),
			Class:                 classFor(p.ArtifactID),
			Text:                  ph.Text,
			Intent:                ph.Intent,
			FalsifyingObservation: ph.FalsifyingObservation,
			Reminders:             sc.Reminders,
		})
	}
	return directives, prohibitions
}

// RenderDirectiveIndex emits the index as deterministic YAML.
//
// Hand-written rather than yaml.Marshal'd: marshalling a Go map randomises key
// order, and a surface whose bytes move on identical input cannot be hashed
// into a manifest.
func RenderDirectiveIndex(idx map[string][]IndexEntry) string {
	var b strings.Builder
	b.WriteString("# edikt directive index — generated by gov-compile, do not edit manually.\n")
	b.WriteString("# Read by `bin/edikt hook match`. Grade is PINNED here; consumers never re-derive it.\n")
	b.WriteString("schema_version: 1\n")

	globs := make([]string, 0, len(idx))
	for g := range idx {
		globs = append(globs, g)
	}
	sort.Strings(globs)

	if len(globs) == 0 {
		// A measured zero, written explicitly. `globs: {}` says compile ran and
		// found no path-scoped directives; a MISSING file says nothing ran.
		b.WriteString("globs: {}\n")
		return b.String()
	}

	b.WriteString("globs:\n")
	for _, g := range globs {
		fmt.Fprintf(&b, "  %s:\n", yamlQuote(g))
		for _, e := range idx[g] {
			fmt.Fprintf(&b, "    - id: %s\n", yamlQuote(e.ID))
			fmt.Fprintf(&b, "      grade: %s\n", e.Grade)
			if e.Class != "" {
				fmt.Fprintf(&b, "      class: %s\n", e.Class)
			}
			fmt.Fprintf(&b, "      text: %s\n", yamlQuote(e.Text))
			if e.Intent != "" {
				fmt.Fprintf(&b, "      intent: %s\n", yamlQuote(e.Intent))
			}
			if e.FalsifyingObservation != "" {
				fmt.Fprintf(&b, "      falsifying_observation: %s\n", yamlQuote(e.FalsifyingObservation))
			}
			if len(e.Reminders) == 0 {
				b.WriteString("      reminders: []\n")
			} else {
				b.WriteString("      reminders:\n")
				for _, r := range e.Reminders {
					fmt.Fprintf(&b, "        - %s\n", yamlQuote(r))
				}
			}
		}
	}
	return b.String()
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
