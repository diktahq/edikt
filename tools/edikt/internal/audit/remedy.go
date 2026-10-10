package audit

// A TABLE, NOT A GREP OVER MESSAGE STRINGS.
//
// D118 (3): every remediation this project prints was additive — add a
// directive, add a record, enrich the artifact, fix the citation — and never
// subtractive: delete the assertion, move it elsewhere, suppress the
// directive, decide the topic shouldn't exist. The contract's own most
// common correct answer is "nothing", while the tooling's vocabulary only
// ever suggested adding, which taught the opposite.
//
// The fix is NOT to grep every message string for a verb and bolt "or delete
// it" onto whatever matches — that reproduces the message-emitting logic
// inside a guard, which INV-014 forbids (edikt-guard:allow), and it produces "or delete it"
// cargo-culted onto messages where deleting was never a real option. The fix
// is this table: one entry per finding class, each carrying both moves (or,
// where none exists, an explicit reason none exists — see NoSubtractive).
// A caller that emits a finding of a listed class renders both entries; a
// finding class absent from this table is unreviewed, not "no subtractive
// option" by default.
type Remedy struct {
	// FindingClass names the kind of finding this remedy pair covers, e.g.
	// "unresolved-citation".
	FindingClass string
	// Additive is the "add/fix forward" move.
	Additive string
	// Subtractive is the "remove/decline" move. Empty only when
	// NoSubtractiveBecause is set — see the invariant this file enforces via
	// TestRemedyTable_EveryEntryHasBothMovesOrARecordedReason.
	Subtractive string
	// NoSubtractiveBecause states why no subtractive option exists for this
	// finding class, when Subtractive is empty. A class with neither field
	// set is a defect in the table, not a legitimate "nothing to say".
	NoSubtractiveBecause string
}

// RemedyTable is the five finding classes D118 named, from bok's own
// findings — the cases the contract's remediation vocabulary actually walked
// into, not a hypothetical superset. Extend it only by adding a real finding
// class a real message emits, with both moves stated (or the reason one is
// absent) — never by widening what a message matches.
var RemedyTable = []Remedy{
	{
		FindingClass: "unresolved-citation",
		Additive:     "fix the citation",
		Subtractive:  "remove the assertion the citation was supporting — an unresolved citation is as often a sign the claim doesn't belong as it is a typo",
	},
	{
		FindingClass: "pending-topic-description",
		Additive:     "approve a description for the topic",
		Subtractive:  "decide the topic should not exist and withdraw it",
	},
	{
		FindingClass: "missing-sidecar-non-accepted",
		Additive:     "compile the sidecar",
		Subtractive:  "leave it — an artifact that isn't accepted or active isn't governed yet, so an absent sidecar may be correct, not a gap",
	},
	{
		FindingClass: "directive-contradicted-by-later-record",
		Additive:     "add-manual-directive to reconcile the two",
		Subtractive:  "suppress-directive on the superseded one",
	},
	{
		FindingClass: "anchor-drift",
		Additive:     "regenerate the sidecar so its anchors match the current prose",
		Subtractive:  "supersede the record — the prose moved on, and re-anchoring to it may be re-affirming a decision that no longer holds",
	},
}

// RemedyFor looks up one finding class's remedy pair. The bool reports
// whether the class is in the table at all — a caller MUST NOT treat a
// missing class as "no subtractive option"; it means the class hasn't been
// reviewed for one yet.
func RemedyFor(findingClass string) (Remedy, bool) {
	for _, r := range RemedyTable {
		if r.FindingClass == findingClass {
			return r, true
		}
	}
	return Remedy{}, false
}
