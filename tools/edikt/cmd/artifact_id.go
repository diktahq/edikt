// artifact_id.go — THE artifact-id extractor. One implementation.
//
// WHY THIS FILE EXISTS.
//
// There were three, and two of them produced a wrong number on 2026-08-30:
//
//   - `^([A-Z]{2,3}-\d{3})(?::|$)` was written for index entry ids
//     (`INV-010:d03`) and then reused on FILENAMES
//     (`INV-010-directive-metadata-lands-in-sidecars.md`). A hyphen follows the
//     id in a filename, not a colon, so it matched NOTHING. The enumeration
//     counted zero artifacts and the check printed "ok — every routed artifact
//     reaches a glob" having measured nothing. On that corpus 66 of 86 were
//     unrouted.
//   - A hand-rolled IndexAny trim in strandedProposals produced "ADR-" instead
//     of "ADR-002".
//
// Both were caught by controls, not by review, which is the reason to have one
// implementation rather than three careful ones.
//
// THE SHAPES IT MUST HANDLE, all of which are real in this corpus:
//
//	INV-010:d03                          index entry id      -> INV-010
//	INV-010-directive-metadata....md     artifact filename   -> INV-010
//	SPEC-018-enforcement-reach           spec DIRECTORY      -> SPEC-018
//	INV-021.md                           filename, no slug   -> INV-021
//	ADR-057                              bare id             -> ADR-057
//
// PREFIX WIDTH IS 2..5, NOT 2..3. GL is two letters and SPEC is four. A {2,3}
// rule silently drops all 18 specs from duplicate-id detection — a check
// reporting nothing where there is something, which is the defect class this
// corpus keeps producing and the one this file was consolidated to stop.
//
// DIGITS ARE {3,}, NOT {3}. Consuming every digit is what stops `INV-0100`
// partial-matching as `INV-010`. A partial match is worse than no match: it
// yields a plausible id for a different artifact.
package cmd

import "regexp"

// artifactIDRe captures the id at the head of a filename, directory name, bare
// id, or index entry id. The trailing class is the set of delimiters that may
// follow an id across those shapes; end-of-string covers the bare form.
var artifactIDRe = regexp.MustCompile(`^([A-Z]{2,5}-\d{3,})(?:[-.:]|$)`)

// artifactIDFrom extracts the artifact id from any of the shapes above.
//
// Returns ok=false rather than a partial or empty id when the input is not an
// artifact reference. Callers MUST branch on ok: a "" id silently grouped with
// other "" ids is how an unparsed name becomes a fake duplicate.
func artifactIDFrom(s string) (string, bool) {
	m := artifactIDRe.FindStringSubmatch(s)
	if m == nil {
		return "", false
	}
	return m[1], true
}
