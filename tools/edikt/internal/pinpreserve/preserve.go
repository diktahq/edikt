package pinpreserve

// preserve.go — carrying human-pinned state across a regeneration.
//
// WHY THIS IS NOT THE EXTRACTOR'S JOB
//
// The extractor is a locked prompt with one input (the artifact's prose) and
// one output (a sidecar). It never sees the previous sidecar, deliberately: an
// extractor that read its own prior output would be free to reproduce a past
// mistake forever, and every measurement of extraction quality would be
// measuring a conversation rather than a contract.
//
// The consequence is that a regeneration WIPES anything a human pinned —
// approved `paths:`, an approved `verify:`, the `human_approved_at` stamp that
// records the ceremony happened. On the first full-corpus run this destroyed
// 48 pinned fields across 20 artifacts, and AC-4.4's survival gate is what
// caught it. The preservation therefore belongs HERE, in the tier-2 code that
// owns the before-and-after: capture, dispatch, restore.
//
// WHAT IS RESTORED, AND WHAT IS REPORTED INSTEAD
//
// Top-level `paths:` and `paths_approval:` restore verbatim. They belong to
// the artifact, not to any one directive, so nothing has to be matched up.
// Newly inferred scope still arrives through `proposed_paths` and still goes
// to the ceremony — restoring the approved set does not approve anything new.
//
// Per-entry pins need to find their entry again, and the v4 contract re-words
// directives by design — one rule split across two sentences becomes one
// directive with two anchors. So matching runs exact-text first, then a
// normalized noun-phrase match. BOTH must be UNAMBIGUOUS: exactly one
// candidate, or the pin is not restored. Entry text is not unique — Validate
// enforces uniqueness on `signals` and on nothing else — so an exact match can
// be as ambiguous as a normalized one, and until SPEC-023 SR-012 only the  // edikt-guard:allow
// normalized branch said so.
//
// AN UNRESTORABLE PIN IS REPORTED, NEVER GUESSED. Attaching an approved verify
// to the wrong directive would be worse than losing it: the sidecar would
// claim a human approved a command for a rule they never saw it against. The
// caller surfaces these for re-approval.

import (
	"fmt"
	"os"
	"reflect"
	"sort"
	"strings"

	"github.com/diktahq/edikt/tools/edikt/internal/lossless"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// PinScope says WHICH of the two preservation mechanisms a reported pin
// belongs to.
//
// It exists because the name is not enough (SPEC-023 SR-011). `paths` names  // edikt-guard:allow
// TWO different things: the artifact-level approved scope, carried verbatim
// with nothing to match, and the per-directive override (ADR-070) that has to  // edikt-guard:allow
// find its entry again. Before this discriminator existed the only thing
// separating them in a report was an empty DirectiveText — which SR-011
// forbids explicitly, and which is not even a discriminator on the load-error
// report, where BOTH arrive with no directive text at all.
type PinScope string

const (
	// ScopeArtifact is a pin belonging to the sidecar itself.
	ScopeArtifact PinScope = "artifact"
	// ScopeEntry is a pin belonging to one directive.
	ScopeEntry PinScope = "entry"
)

// EntryKind names WHICH of the two entry collections an entry-level pin came
// from.
//
// It exists because Scope is not enough once prohibitions enter the mechanism
// (SPEC-023 SR-002). Both kinds run the same table, the same matcher and the  // edikt-guard:allow
// same three reports, so a report carrying only Scope tells an operator the pin
// belonged to *an* entry without saying which — and a sidecar may legitimately
// carry a directive and a prohibition whose text is identical, which is the
// same "nothing but the text tells them apart" hazard SR-011 named one level up.
type EntryKind string

const (
	// KindDirective is a pin that belonged to a directive.
	KindDirective EntryKind = "directive"
	// KindProhibition is a pin that belonged to a prohibition.
	KindProhibition EntryKind = "prohibition"
)

// PinnedField names one human-pinned value that could not be carried forward.
type PinnedField struct {
	ArtifactID string `json:"artifact_id"`
	Field      string `json:"field"`
	// Scope distinguishes the artifact-level mechanism from the entry-level
	// one. Every emission site stamps it, because a report that carries it
	// for some fields and not others is one an operator has to know the
	// mechanism to read (SR-011).
	Scope PinScope `json:"scope"`
	// EntryKind is the collection the entry lived in, `directive` or
	// `prohibition`.
	//
	// It is EMPTY on artifact-level pins, which belong to no entry, and on the
	// load-error at-risk report, where nothing was read and so no entry was
	// ever reached — the same reason DirectiveText is empty there. An empty
	// EntryKind on an entry-scoped pin therefore means "no entry was examined",
	// never "a directive".
	EntryKind EntryKind `json:"entry_kind,omitempty"`
	// DirectiveText is the pre-regeneration directive the pin belonged to.
	DirectiveText string `json:"directive_text"`
	Value         string `json:"value"`
	Reason        string `json:"reason"`
}

func (p PinnedField) String() string {
	scope := string(p.Scope)
	if p.EntryKind != "" {
		scope += "/" + string(p.EntryKind)
	}
	return fmt.Sprintf("%s %s (%s) on %q: %s", p.ArtifactID, p.Field, scope, truncate(p.DirectiveText, 80), p.Reason)
}

// PreserveResult reports what a restore carried and what it could not.
type PreserveResult struct {
	PathsRestored    bool `json:"paths_restored"`
	ApprovalRestored bool `json:"approval_restored"`
	// DirectivePins counts ENTRIES whose pins were written back — directives
	// AND prohibitions, since SPEC-023 SR-002 put both through the same path.  // edikt-guard:allow
	// The json key keeps its name so existing receipts stay readable.
	//
	// It counts entries, not fields (SPEC-023 finding 38): an entry that lost  // edikt-guard:allow
	// two pins reads 1, so this number is evidence for no single field. The
	// per-field evidence is the reinstated list MissingPins returns.
	DirectivePins     int           `json:"directive_pins_restored"`
	Unrestorable      []PinnedField `json:"unrestorable,omitempty"`
	SidecarRewritten  bool          `json:"sidecar_rewritten"`
	MatchedByNormForm int           `json:"matched_by_normalized_form"`
	// LoadFailed is true when the PRIOR sidecar existed on disk but could not
	// be parsed — a categorically different case from "no prior sidecar",
	// which PreservePinned must never silently treat the same way. See
	// pinnedFieldsAtRisk.
	LoadFailed bool `json:"load_failed"`
	// PathsProvenance says what backs the artifact-level `paths:` the sidecar
	// is left carrying, read from the FINAL state: the globs on disk against
	// the receipt's hash. It is empty when there are no paths, and when no
	// final state was read at all (no prior, or a prior that would not parse
	// — nothing was compared, so nothing is claimed).
	//
	// It exists because restore carries unreceipted `paths:` forward
	// (grandfathered, by design) and used to report every carried set as "the
	// approved paths", including sets no human ever approved. A report that
	// says "approved" over a scope with no receipt is the claim the receipt
	// was introduced to make checkable.
	PathsProvenance PathsProvenance `json:"paths_provenance,omitempty"`
}

// PathsProvenance is what backs a sidecar's artifact-level `paths:`.
type PathsProvenance string

const (
	// PathsApproved: an approval receipt whose hash covers the globs.
	PathsApproved PathsProvenance = "approved"
	// PathsReceiptStale: a receipt is present but its hash does not cover
	// the globs — they changed after the approval.
	PathsReceiptStale PathsProvenance = "receipt-stale"
	// PathsUnreceipted: no receipt at all. Carried forward, never approved.
	PathsUnreceipted PathsProvenance = "unreceipted"
)

// pathsProvenanceOf reads the provenance of sc's artifact-level paths.
func pathsProvenanceOf(sc *sidecar.Sidecar) PathsProvenance {
	switch {
	case sc == nil || len(sc.Paths) == 0:
		return ""
	case sc.PathsApproval == nil:
		return PathsUnreceipted
	case sidecar.HashGlobs(sc.Paths) == sc.PathsApproval.GlobsSHA256:
		return PathsApproved
	default:
		return PathsReceiptStale
	}
}

// PathsLabel is the clause a restore report appends for the artifact-level
// paths it carried back, worded from PathsProvenance. It is empty when no
// paths were restored: a label on paths nobody touched would be a claim
// about work this run did not do.
func (r *PreserveResult) PathsLabel() string {
	if r == nil || !r.PathsRestored {
		return ""
	}
	switch r.PathsProvenance {
	case PathsApproved:
		return ", approved paths"
	case PathsReceiptStale:
		return ", paths whose approval receipt no longer covers them (re-approve with `edikt sidecar approve --kind paths`)"
	default:
		return ", unreceipted paths (carried forward as they stood; no approval receipt covers them)"
	}
}

// ---------------------------------------------------------------------------
// THE PINNABLE-FIELD TABLES — SPEC-023 SR-001, SR-006, SR-011.  // edikt-guard:allow
// ---------------------------------------------------------------------------
//
// The pinnable field names used to be typed out in TEN places: the `pin`
// struct, `pin.empty()`, `pinOf`, each of `applyPin`'s `set()` calls, the
// `pinnedFieldNamesAtRisk` slice, the unrestorable map, the `reinstatedFields`
// value map, and three `Field:` literals in `computePins`. Nine of the ten
// could fall out of step with the first with nothing saying so — and did:
// ADR-070/071 added `enforcement` and a per-entry `paths` to the schema,  // edikt-guard:allow
// nobody extended the ten, and neither field has ever survived a
// re-extraction.
//
// Two tables replace them, split by SCOPE rather than merged, because the two
// scopes have genuinely different mechanics (SR-011):
//
//	entry-level     a pin belonging to one directive, which has to FIND that
//	                directive again after the extractor re-worded it
//	artifact-level  a pin belonging to the sidecar itself, carried verbatim
//	                with nothing to match up
//
// Everything downstream — capture, the emptiness test, the write-back, the
// at-risk report, the unrestorable report and the reinstated report — is a
// loop over a table. Adding a pinnable field is one row.
//
// `paths_approval`'s stale-hash rule fits NEITHER table: it is not a restore
// at all but a conditional refusal to re-stamp a receipt, keyed on a hash of
// a DIFFERENT field's restored value. It stays explicit at its site in
// computePins, with the reason written there.

// pinValue is one pinned value, carried in whichever Go shape its field has.
//
// `present` is recorded rather than inferred. Emptiness is a serviceable
// presence test for a string and for a slice, but not for every shape a
// pinnable field can take — `actor_scope` is a bool, where the zero value and
// "unset" are the same bits — so presence is stated by the row's reader
// instead of being re-derived by each consumer (SR-006, SR-010).
type pinValue struct {
	present bool
	text    string
	list    []string
}

// textPin reads a string-shaped pin. An empty string is ABSENT, not a pin: the
// extractor cannot emit these fields, so an empty destination is the ordinary
// post-regeneration state rather than a human clearing a value.
func textPin(s string) pinValue {
	if s == "" {
		return pinValue{}
	}
	return pinValue{present: true, text: s}
}

// listPin reads a list-shaped pin. The slice is copied: a row's reader must
// never hand out an alias into the `before` sidecar, or a later write through
// it would mutate the snapshot the comparison is being made against.
func listPin(v []string) pinValue {
	if len(v) == 0 {
		return pinValue{}
	}
	return pinValue{present: true, list: append([]string(nil), v...)}
}

// boolPin reads a bool-shaped pin. TRUE IS THE PIN; false is ABSENT.
//
// That is D319's ruling on SR-006 and it is the only shape the field can
// support: `actor_scope` carries `omitempty`, so a false pin is permanently
// unrepresentable — false is dropped on write, an absent key decodes back to
// false, and the two states are therefore the same bits before this package
// ever sees the prior sidecar. No presence signal could recover the difference.
// The limit is written down here and in commands/gov/reextract.md rather than
// left implicit: an undocumented two-state field reads as a bug to the next
// maintainer, a documented one is a contract.
//
// The `text` is set so a report naming this field carries something a human
// can read; a name with no value cannot be re-approved.
func boolPin(b bool) pinValue {
	if !b {
		return pinValue{}
	}
	return pinValue{present: true, text: "true"}
}

// copyList is the list-shaped write. It NEVER produces a non-nil empty slice.
//
// That is not tidiness. sidecar.Validate rejects `paths: []` on an entry
// (sidecar.go:729 for directives, :776 for prohibitions) — narrowing to zero
// files is suppressed_directives wearing a different name — and PreservePinned
// validates the WHOLE artifact before writing, so a single entry restored to
// []string{} aborts the entire restore rather than that one field
// (TestRestore_NeverEmptyNonNilSlice).
func copyList(v []string) []string {
	if len(v) == 0 {
		return nil
	}
	return append([]string(nil), v...)
}

func (v pinValue) equal(o pinValue) bool {
	return v.present == o.present && v.text == o.text && sameStrings(v.list, o.list)
}

// String renders the value for a report. A pin named with no value cannot be
// re-approved without going to git history, so every report carries one.
func (v pinValue) String() string {
	if len(v.list) > 0 {
		return strings.Join(v.list, ", ")
	}
	return v.text
}

// entryPin is one row of the entry-level table: a field a human can pin on a
// single directive.
//
// The row carries the field's name, how to READ it from an entry, and how to
// WRITE it back — SPEC-023's three columns. Every derived site takes exactly  // edikt-guard:allow
// those three from here and nothing from anywhere else.
type entryPin struct {
	name  string
	read  func(d *sidecar.Directive) pinValue
	write func(d *sidecar.Directive, v pinValue)
}

// entryPinTable is the authoritative entry-level set.
//
// PROHIBITIONS ARE NOT TRAVERSED YET. The row type is deliberately typed to
// *sidecar.Directive rather than to some entry interface: prohibition
// traversal is its own change with its own red-first evidence (SPEC-023 SR-002),  // edikt-guard:allow
// and quietly widening the type here would land it as a side effect of a
// refactor. TestProhibitionArm_StillUnreached asserts that it has not.
//
// It is a var, not a const table, so a test can add a row and observe that the
// addition reaches every derived site — the falsifier for the whole scheme
// (TestPinTable_AddedRowReachesEverySite).
var entryPinTable = []entryPin{
	{
		name:  "verify",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.Verify) },
		write: func(d *sidecar.Directive, v pinValue) { d.Verify = v.text },
	},
	{
		name:  "verify_kind",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.VerifyKind) },
		write: func(d *sidecar.Directive, v pinValue) { d.VerifyKind = v.text },
	},
	{
		name:  "human_approved_at",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.HumanApprovedAt) },
		write: func(d *sidecar.Directive, v pinValue) { d.HumanApprovedAt = v.text },
	},
	{
		name:  "positive_fixture_path",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.PositiveFixturePath) },
		write: func(d *sidecar.Directive, v pinValue) { d.PositiveFixturePath = v.text },
	},
	{
		name:  "negative_fixture_path",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.NegativeFixturePath) },
		write: func(d *sidecar.Directive, v pinValue) { d.NegativeFixturePath = v.text },
	},
	// ---- SPEC-023 phase 3: the three human-authored fields that were  // edikt-guard:allow
	// declared in the schema (ADR-065/070/071), carried live values in the  // edikt-guard:allow
	// corpus, and had never survived a re-extraction.
	{
		name:  "enforcement",
		read:  func(d *sidecar.Directive) pinValue { return textPin(d.Enforcement) },
		write: func(d *sidecar.Directive, v pinValue) { d.Enforcement = v.text },
	},
	{
		// The PER-DIRECTIVE override (ADR-070), not the artifact-level scope  // edikt-guard:allow
		// of the same name in artifactPinTable. The two are different
		// mechanisms and are kept apart in every report by PinScope, never by
		// an empty DirectiveText (SR-011).
		name:  "paths",
		read:  func(d *sidecar.Directive) pinValue { return listPin(d.Paths) },
		write: func(d *sidecar.Directive, v pinValue) { d.Paths = copyList(v.list) },
	},
	{
		// actor_scope is a BARE BOOL and true IS the pin (D319, SR-006).
		//
		// A false pin is permanently unrepresentable, and that is a STATED
		// limit rather than an omission: the yaml tag carries `omitempty`, so
		// false is dropped on write and an absent key decodes back to false.
		// Restoring only `true` is the whole contract. The negative control
		// (TestActorScope_FalseIsNotAPin) exists because the cheap reading —
		// every bool is a pin — passes the positive test while claiming a
		// phantom restore on every artifact that never set it.
		name:  "actor_scope",
		read:  func(d *sidecar.Directive) pinValue { return boolPin(d.ActorScope) },
		write: func(d *sidecar.Directive, v pinValue) { d.ActorScope = v.present },
	},
}

// artifactPin is one row of the artifact-level table: a field a human can pin
// on the sidecar itself.
//
// Its restore is a whole function rather than a read/write pair, because these
// fields have no matching step and their restore conditions genuinely differ —
// approved scope compares set-wise, the receipt is carried only into an empty
// destination. What the row still owns is the NAME: a restore returns
// PinnedFields with `Field` UNSET and computePins stamps it from the row, so
// the field name is stated once here rather than a second time inside each
// function (SPEC-023 finding 29, sites :237/:249).  // edikt-guard:allow
type artifactPin struct {
	name    string
	restore func(artifactID string, before, after *sidecar.Sidecar, res *PreserveResult) []PinnedField
}

var artifactPinTable = []artifactPin{
	{name: "paths", restore: restoreApprovedPaths},
	{name: "paths_approval", restore: restoreApprovalReceipt},
	// ---- SPEC-023 phase 5: the two AUTHORED LISTS.  // edikt-guard:allow
	//
	// Neither is extracted from anything. A human typed the manual directive,
	// or named a directive to subtract; the extractor's allowed-key list
	// forbids it writing either. So a regeneration produces a sidecar with
	// both absent and the list is gone unless it is carried here.
	{name: "manual_directives", restore: restoreAuthoredList(
		func(s *sidecar.Sidecar) []string { return s.ManualDirectives },
		func(s *sidecar.Sidecar, v []string) { s.ManualDirectives = v },
		"the manually authored directives",
	)},
	{name: "suppressed_directives", restore: restoreAuthoredList(
		func(s *sidecar.Sidecar) []string { return s.SuppressedDirectives },
		func(s *sidecar.Sidecar, v []string) { s.SuppressedDirectives = v },
		"the suppressed-directive list",
	)},
}

// restoreApprovedPaths carries the human-approved glob set forward.
//
// Unconditional on entry matching: the scope belongs to the artifact, so it is
// restored regardless of what happened to the directives. That is why there is
// no per-entry unrestorable arm for this field — there is no match that can
// fail.
//
// The restore condition does not look at the receipt — unreceipted paths are
// grandfathered and carried exactly like approved ones. Only the REASON does:
// it says "approved" when the prior's receipt covers the prior's globs, and
// otherwise says what the paths actually are.
func restoreApprovedPaths(artifactID string, before, after *sidecar.Sidecar, res *PreserveResult) []PinnedField {
	if len(before.Paths) > 0 && !sameStrings(before.Paths, after.Paths) {
		after.Paths = append([]string(nil), before.Paths...)
		sort.Strings(after.Paths)
		res.PathsRestored = true
		reason := "the regenerated sidecar did not carry the approved paths"
		switch pathsProvenanceOf(before) {
		case PathsUnreceipted:
			reason = "the regenerated sidecar did not carry the prior paths (unreceipted: no approval receipt covers them; carried forward as they stood)"
		case PathsReceiptStale:
			reason = "the regenerated sidecar did not carry the prior paths (their approval receipt no longer covers them; carried forward as they stood)"
		}
		return []PinnedField{{
			ArtifactID: artifactID,
			Value:      strings.Join(before.Paths, ", "),
			Reason:     reason,
		}}
	}
	return nil
}

// restoreApprovalReceipt carries the approval receipt forward, and only into
// an EMPTY destination: a receipt the regenerated sidecar already carries is
// the extractor's, and overwriting it would be this code deciding which of two
// approvals happened.
func restoreApprovalReceipt(artifactID string, before, after *sidecar.Sidecar, res *PreserveResult) []PinnedField {
	if before.PathsApproval != nil && after.PathsApproval == nil {
		cp := *before.PathsApproval
		after.PathsApproval = &cp
		res.ApprovalRestored = true
		return []PinnedField{{
			ArtifactID: artifactID,
			Value:      before.PathsApproval.ApprovedAt,
			Reason:     "the regenerated sidecar did not carry the approval receipt",
		}}
	}
	return nil
}

// restoreAuthoredList carries a WHOLE-LIST artifact-level field forward.
//
// `manual_directives` and `suppressed_directives` are the two fields of this
// shape (SPEC-023 SR-013). They belong to the sidecar, not to any one entry,  // edikt-guard:allow
// and there is NO MATCHING STEP: nothing has to find a directive again, so
// nothing can fail to. That is the same structural fact artifact-level `paths`
// has, and it is why neither field has a per-entry unrestorable arm — a report
// that can never fire is not a report. The one REACHABLE report for these is
// the load-error at-risk one (AC-5.2): when the prior sidecar will not parse,
// the list might have held anything and this package cannot see it.
//
// THE COMPARISON IS ORDER-SENSITIVE, unlike the approved glob set's. A manual
// directive is prose a human wrote and its position in the list is that
// human's; a reordering is a change, and restoring `before` verbatim is the
// conservative answer. sameStrings sorts before comparing, which is right for a
// glob set — where order carries nothing — and would silently accept a
// reordered authored list here.
//
// It sets no flag on PreserveResult, deliberately. PathsRestored and
// ApprovalRestored predate the tables and are two of the hand-maintained sites
// SR-001 exists to stop multiplying; the PinnedField this returns is what marks
// the artifact changed, names the field and carries its value, which is
// everything a fifth and sixth bool would have said.
func restoreAuthoredList(
	read func(*sidecar.Sidecar) []string,
	write func(*sidecar.Sidecar, []string),
	what string,
) func(string, *sidecar.Sidecar, *sidecar.Sidecar, *PreserveResult) []PinnedField {
	return func(artifactID string, before, after *sidecar.Sidecar, _ *PreserveResult) []PinnedField {
		want := read(before)
		if len(want) == 0 || sameOrderedStrings(want, read(after)) {
			return nil
		}
		write(after, append([]string(nil), want...))
		return []PinnedField{{
			ArtifactID: artifactID,
			// Joined on a pipe rather than a comma: these are sentences, and a
			// comma-joined list of prose is unreadable as a report of which
			// items were carried.
			Value:  strings.Join(want, " | "),
			Reason: "the regenerated sidecar did not carry " + what,
		}}
	}
}

// pinnedFieldsAtRisk is every field a human can pin that this package
// restores, DERIVED from the two tables rather than restated, and carrying the
// SCOPE each one came from.
//
// It used to be a hand-typed slice of names, and the comment above it claimed
// that adding a field to `pin`/`pinOf` without adding it here would be caught.
// It was not: the only test that read the list built its expectations by
// iterating the same list, so a name missing from both was never asked about
// (INV-014). The list is now a projection of the tables, and  // edikt-guard:allow
// TestAtRiskSetDerivedFromStruct checks the tables against the authoritative
// struct instead of against themselves.
//
// It returns PinnedField prototypes rather than names because `paths` is in
// BOTH tables from SPEC-023 phase 3 onward: the load-error report names it  // edikt-guard:allow
// twice, once per mechanism, and the two are told apart by Scope. Neither
// carries a DirectiveText — nothing was matched, because nothing was read —
// so the empty-DirectiveText discriminator SR-011 forbids would not have
// worked on this report at all.
//
// (The frozen characterisation file's comment at :134 still names the old
// identifier. AC-2.5 requires that file byte-identical, so the reference is
// left stale deliberately rather than edited.)
//
// The order — artifact-level first, then entry-level — is the reported order
// on a load failure, and is stable so the report does not churn between runs.
func pinnedFieldsAtRisk() []PinnedField {
	out := make([]PinnedField, 0, len(artifactPinTable)+len(entryPinTable))
	for _, f := range artifactPinTable {
		out = append(out, PinnedField{Field: f.name, Scope: ScopeArtifact})
	}
	for _, f := range entryPinTable {
		out = append(out, PinnedField{Field: f.name, Scope: ScopeEntry})
	}
	return out
}

// pin is the pinned state of one entry, keyed by field name. Absent fields are
// absent from the map, so `empty` is a length test rather than a conjunction
// that has to be extended every time a row is added.
type pin struct {
	values map[string]pinValue
}

func (p pin) empty() bool {
	return len(p.values) == 0
}

func pinOf(d sidecar.Directive) pin {
	p := pin{values: make(map[string]pinValue, len(entryPinTable))}
	for _, f := range entryPinTable {
		if v := f.read(&d); v.present {
			p.values[f.name] = v
		}
	}
	return p
}

// applyPin writes the pinned values and reports whether anything CHANGED.
//
// The distinction matters for the count, not just for the write: a restore
// that reported 11 artifacts every time it ran — including runs where all 11
// were already correct — is reporting work it did not do, and a number that
// cannot go to zero is one nobody can use to tell whether the repair is
// finished.
//
// It never CLEARS: a field absent from the pin is skipped, not blanked. That
// is correct here because the extractor cannot emit any of these fields, so a
// value in the destination came from somewhere else and is not this function's
// to discard. It is also NOT an empty-destination test — a differing
// destination IS overwritten by the pin (SPEC-023 finding 30).  // edikt-guard:allow
//
// SR-006 requires the never-clear half to be ASSERTED rather than assumed, and
// it now is: TestPinWrite_NeverClearsOnAbsentPin drives every row in the table
// with a value in the destination and no pin for it, because a write that
// blanks on an absent pin passes AC-2.1, AC-2.2, AC-3.1 and AC-3.3 while
// destroying exactly what this package exists to carry. Until phase 3 the
// property lived in this paragraph and nowhere else.
func applyPin(d *sidecar.Directive, p pin) []string {
	var changed []string
	for _, f := range entryPinTable {
		v, ok := p.values[f.name]
		if !ok {
			continue
		}
		if f.read(d).equal(v) {
			continue
		}
		f.write(d, v)
		changed = append(changed, f.name)
	}
	return changed
}

// ---------------------------------------------------------------------------
// BOTH ENTRY KINDS, ONE PATH — SPEC-023 SR-002.  // edikt-guard:allow
// ---------------------------------------------------------------------------
//
// computePins used to loop `before.Directives` and nothing else, and there was
// no prohibition loop anywhere in this package or in any of its three callers.
// So 13 prohibitions carrying 27 pinned values across 8 governance artifacts —
// and 15 more across 10 prohibitions in templates/sidecars/baseline/ — sat
// outside the mechanism entirely. Not merely unrestored: a prohibition dropped
// whole reported Unrestorable = 0, where a directive in the same position
// reported every pin it carried (SPEC-023 finding 39). The survival gate built  // edikt-guard:allow
// to catch exactly this class reached the corpus through this function and
// printed PASS on a run that wiped all 27.
//
// Prohibitions do NOT get a parallel loop. `Directive` and `Prohibition` are
// two independently declared structs (SR-011), but every pinnable field is
// declared on both, so a prohibition is PROJECTED onto the Directive shape
// entryPinTable's columns are typed to, run through the same index, the same
// applyPin and the same three reports, and written back. A row added to the
// table reaches prohibitions with no second edit — which is the property SR-001
// exists for, and which a hand-written second table would have destroyed on the
// very next field.
//
// The projection is REFLECTIVE over yaml keys rather than a hand-written field
// list, for that same reason: a hand-written copy would be an eleventh
// hand-maintained site, and a new row would compile, pass on directives, and
// silently do nothing on prohibitions.
// TestProhibitionProjection_CarriesEveryTableRow drives every row through it in
// both directions, and asserts there is no shared yaml key whose Go types
// disagree — the one case copyByYAMLKey cannot carry.

// yamlFieldIndex maps a struct's yaml key to its field index.
func yamlFieldIndex(rt reflect.Type) map[string]int {
	out := make(map[string]int, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		if name := strings.Split(tag, ",")[0]; name != "" {
			out[name] = i
		}
	}
	return out
}

var (
	directiveYAMLFields   = yamlFieldIndex(reflect.TypeOf(sidecar.Directive{}))
	prohibitionYAMLFields = yamlFieldIndex(reflect.TypeOf(sidecar.Prohibition{}))
)

// sharedYAMLTypeMismatches names every yaml key declared on BOTH entry structs
// with disagreeing Go types — the one shape copyByYAMLKey cannot carry across.
//
// It is a function rather than a silent skip inside the copy because a field
// that quietly fails to project is exactly the invisible loss this package
// exists to stop. TestProhibitionProjection_CarriesEveryTableRow requires this
// to be empty.
func sharedYAMLTypeMismatches() []string {
	dt := reflect.TypeOf(sidecar.Directive{})
	pt := reflect.TypeOf(sidecar.Prohibition{})
	var out []string
	for key, di := range directiveYAMLFields {
		pi, ok := prohibitionYAMLFields[key]
		if !ok {
			continue
		}
		if dt.Field(di).Type != pt.Field(pi).Type {
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// copyByYAMLKey copies every field the two structs share by YAML KEY.
//
// A key on the source that the destination does not declare is left behind;
// a key the destination declares and the source does not is UNTOUCHED, which is
// what keeps a prohibition's `derived_from` across a round trip through the
// Directive shape.
func copyByYAMLKey(dst reflect.Value, dstFields map[string]int, src reflect.Value, srcFields map[string]int) {
	for key, si := range srcFields {
		di, ok := dstFields[key]
		if !ok {
			continue
		}
		sf, df := src.Field(si), dst.Field(di)
		if sf.Type() != df.Type() {
			continue
		}
		df.Set(sf)
	}
}

// directiveShapeOf projects a prohibition onto the Directive shape the
// entry-level table reads and writes.
func directiveShapeOf(p sidecar.Prohibition) sidecar.Directive {
	var d sidecar.Directive
	copyByYAMLKey(reflect.ValueOf(&d).Elem(), directiveYAMLFields, reflect.ValueOf(p), prohibitionYAMLFields)
	return d
}

// writeDirectiveShapeInto is the projection's inverse: it carries a restored
// Directive shape back onto the prohibition it came from.
func writeDirectiveShapeInto(d sidecar.Directive, p *sidecar.Prohibition) {
	copyByYAMLKey(reflect.ValueOf(p).Elem(), prohibitionYAMLFields, reflect.ValueOf(d), directiveYAMLFields)
}

// entryArm is ONE entry collection, presented to the shared restore body in the
// Directive shape entryPinTable is typed to.
//
// `before` is the pre-regeneration collection, already projected. `get` and
// `put` read and write the REGENERATED collection at an index, so the body
// never has to know which of the two kinds it is working on.
type entryArm struct {
	kind   EntryKind
	before []sidecar.Directive
	count  int
	get    func(i int) sidecar.Directive
	put    func(i int, d sidecar.Directive)
}

func entryArms(before, after *sidecar.Sidecar) []entryArm {
	beforeProhibitions := make([]sidecar.Directive, len(before.Prohibitions))
	for i := range before.Prohibitions {
		beforeProhibitions[i] = directiveShapeOf(before.Prohibitions[i])
	}
	return []entryArm{
		{
			kind:   KindDirective,
			before: before.Directives,
			count:  len(after.Directives),
			get:    func(i int) sidecar.Directive { return after.Directives[i] },
			put:    func(i int, d sidecar.Directive) { after.Directives[i] = d },
		},
		{
			kind:   KindProhibition,
			before: beforeProhibitions,
			count:  len(after.Prohibitions),
			get:    func(i int) sidecar.Directive { return directiveShapeOf(after.Prohibitions[i]) },
			put:    func(i int, d sidecar.Directive) { writeDirectiveShapeInto(d, &after.Prohibitions[i]) },
		},
	}
}

// preserveArm runs the per-entry restore for ONE collection and reports whether
// it changed anything.
//
// It is a function rather than an inline loop because both arms run the SAME
// body — SR-002's "one path", not a directive loop with a prohibition loop
// beside it. Everything the two kinds do differently is in entryArm's four
// fields.
func preserveArm(artifactID string, arm entryArm, res *PreserveResult, reinstated *[]PinnedField) bool {
	// Named armChanged, not `changed`: computePins has its own, and the
	// mutation harness's selftest records `return changed` as a needle that
	// must match exactly one site in this file. Two identically-named locals
	// make that record ambiguous and the harness refuses it — which is the
	// refusal working, but the fix belongs here rather than in phase 1's
	// instrument.
	armChanged := false

	// Indexed WITHIN the arm. A pin captured from a prohibition is never
	// reattached to a directive that happens to share its text: they are
	// different entries, and welding an approval across kinds would be the same
	// invention this package refuses to make between two directives.
	//
	// BOTH INDEXES HOLD EVERY CANDIDATE (SPEC-023 SR-012). byText used to be a  // edikt-guard:allow
	// `map[string]int` assigned by `byText[d.Text] = i`, so two entries carrying
	// identical text left only the LAST index behind and the exact-text branch
	// welded the pin onto whichever one the extractor happened to write second —
	// silently, with nothing in Unrestorable. Directive text is not unique:
	// sidecar.Validate enforces uniqueness on `signals` and on nothing else, so
	// duplicates are representable and valid. The normalized branch has guarded
	// this since the package was written; the exact branch is where the guard
	// was missing, which is the one place it could do the most damage — an
	// EXACT text match is the branch a reviewer trusts.
	byText := map[string][]int{}
	byNorm := map[string][]int{}
	for i := 0; i < arm.count; i++ {
		d := arm.get(i)
		byText[d.Text] = append(byText[d.Text], i)
		n := lossless.NormalizeNounPhrase(d.Text)
		if n != "" {
			byNorm[n] = append(byNorm[n], i)
		}
	}

	for _, bd := range arm.before {
		p := pinOf(bd)
		if p.empty() {
			continue
		}
		exact := byText[bd.Text]
		if len(exact) == 1 {
			d := arm.get(exact[0])
			if fields := applyPin(&d, p); len(fields) > 0 {
				arm.put(exact[0], d)
				res.DirectivePins++
				armChanged = true
				*reinstated = append(*reinstated, reinstatedFields(artifactID, arm.kind, bd.Text, p, fields)...)
			}
			continue
		}
		n := lossless.NormalizeNounPhrase(bd.Text)
		norm := byNorm[n]
		// The normalized fallback is reached only when the exact text found
		// NOTHING. An ambiguous exact match must not fall through to it: the
		// duplicates normalize to the same form, so the fallback would either
		// re-derive the same ambiguity under a reason describing the wrong
		// mechanism, or — when the text has no normal form at all — report a
		// deletion that did not happen.
		if len(exact) == 0 && len(norm) == 1 {
			d := arm.get(norm[0])
			if fields := applyPin(&d, p); len(fields) > 0 {
				arm.put(norm[0], d)
				res.DirectivePins++
				res.MatchedByNormForm++
				armChanged = true
				*reinstated = append(*reinstated, reinstatedFields(artifactID, arm.kind, bd.Text, p, fields)...)
			}
			continue
		}
		// AMBIGUOUS OR ABSENT. Reported for re-approval, never attached to a
		// best guess: a sidecar claiming a human approved a command against a
		// rule they never saw it on is worse than one missing the command.
		//
		// THE THREE REASONS ARE KEPT APART because an operator acts on them
		// differently: a deletion is accepted or the rule is re-authored, while
		// an ambiguity is resolved by re-approving against one of several
		// entries that are still sitting in the file. Telling them a directive
		// has "no counterpart" when two carry its exact text sends them hunting
		// for something that was never removed.
		reason := fmt.Sprintf("the %s it was pinned to has no counterpart after regeneration", arm.kind)
		switch {
		case len(exact) > 1:
			reason = fmt.Sprintf("%d %ss now carry its exact text — attaching it to one would invent an approval", len(exact), arm.kind)
		case len(norm) > 1:
			reason = fmt.Sprintf("%d %ss now match its subject — attaching it to one would invent an approval", len(norm), arm.kind)
		}
		for _, f := range entryPinTable {
			v, ok := p.values[f.name]
			if !ok {
				continue
			}
			res.Unrestorable = append(res.Unrestorable, PinnedField{
				ArtifactID:    artifactID,
				Field:         f.name,
				Scope:         ScopeEntry,
				EntryKind:     arm.kind,
				DirectiveText: bd.Text,
				Value:         v.String(),
				Reason:        reason,
			})
		}
	}
	return armChanged
}

// PreservePinned restores human-pinned state from `before` into the sidecar at
// afterPath, writing it back when anything changed.
//
// before is the sidecar as it stood BEFORE the regeneration. A nil before
// means there was nothing to preserve, which is the ordinary bootstrap case —
// UNLESS beforeLoadErr is non-nil, in which case before is nil for a
// completely different reason: a prior sidecar existed on disk but failed to
// parse. Those two are not the same claim. "No prior sidecar" means there was
// genuinely nothing pinned. "Prior sidecar unreadable" means there might have
// been anything — approved paths, a human-approved verify, all of it — and
// this code cannot see it. Silently treating the second as the first is
// exactly the swallow this function exists to prevent for every OTHER shape
// of loss; it must not reintroduce the same shape here.
//
// beforeLoadErr takes priority over before being nil — the caller
// (discover.go via reextract.go) sets Sidecar and LoadErr as mutually
// exclusive, but this function does not trust that invariant blindly: a
// non-nil beforeLoadErr always means "refuse to proceed as if empty",
// regardless of what before itself contains.
func PreservePinned(artifactID string, before *sidecar.Sidecar, beforeLoadErr error, afterPath string) (*PreserveResult, error) {
	if res, done := earlyResult(artifactID, before, beforeLoadErr); done {
		return res, nil
	}
	after, err := sidecar.Load(afterPath)
	if err != nil {
		return &PreserveResult{}, fmt.Errorf("load regenerated sidecar %s: %w", afterPath, err)
	}
	res, _, changed := computePins(artifactID, before, after)
	// Provenance comes from the final state, which is `after` as computePins
	// left it — the same bytes the write below puts on disk, or the bytes
	// already there when nothing changed.
	res.PathsProvenance = pathsProvenanceOf(after)
	if !changed {
		return res, nil
	}
	if err := after.Validate(); err != nil {
		return res, fmt.Errorf("%s: restored sidecar fails validation: %w", artifactID, err)
	}
	body, err := sidecar.Marshal(after)
	if err != nil {
		return res, fmt.Errorf("%s: marshal restored sidecar: %w", artifactID, err)
	}
	if err := os.WriteFile(afterPath, body, 0o644); err != nil {
		return res, fmt.Errorf("%s: write restored sidecar: %w", artifactID, err)
	}
	res.SidecarRewritten = true
	return res, nil
}

// earlyResult handles the two cases that never touch the regenerated
// sidecar at all: a prior sidecar that existed but would not parse (every
// pinnable field is at risk and none can be checked), and a genuinely
// absent prior sidecar (nothing was pinned, so nothing can be lost). done
// is false only when there is a real `before` to compare against.
func earlyResult(artifactID string, before *sidecar.Sidecar, beforeLoadErr error) (*PreserveResult, bool) {
	res := &PreserveResult{}
	if beforeLoadErr != nil {
		res.LoadFailed = true
		for _, at := range pinnedFieldsAtRisk() {
			at.ArtifactID = artifactID
			at.Reason = fmt.Sprintf(
				"the prior sidecar existed but failed to load (%v) — could not check whether this %s-level field was pinned before regeneration; review the prior sidecar manually (git history or .edikt/state/reextract-snapshots/) before treating this artifact's re-extraction as final",
				beforeLoadErr, at.Scope,
			)
			res.Unrestorable = append(res.Unrestorable, at)
		}
		return res, true
	}
	if before == nil {
		return res, true
	}
	return res, false
}

// computePins is the one comparison both the mutating and the read-only
// entry points run. It mutates `after` IN MEMORY only — writing it back is
// PreservePinned's job and MissingPins deliberately never does it, which is
// what lets `phasea verify` reuse this logic while performing no writes
// (SPEC-013 D6: the mutation and the gate are separate verbs).
//
// It returns three things: the PreserveResult (carrying Unrestorable — pins
// that could NOT be reattached to any directive), the list of pins that WERE
// reinstated (i.e. fields the live sidecar was missing), and whether
// anything changed at all. A caller checking survival treats both lists as
// "not present on disk": Unrestorable can never be restored, and a non-empty
// reinstated list means the live file did not carry the pin when it was read.
func computePins(artifactID string, before, after *sidecar.Sidecar) (*PreserveResult, []PinnedField, bool) {
	res := &PreserveResult{}
	var reinstated []PinnedField
	changed := false

	// ---- artifact-level pins -----------------------------------------------
	// The row owns the name; the restore owns the condition and the value.
	for _, f := range artifactPinTable {
		for _, pf := range f.restore(artifactID, before, after, res) {
			pf.Field = f.name
			pf.Scope = ScopeArtifact
			reinstated = append(reinstated, pf)
			changed = true
		}
	}

	// A restored receipt must still describe the restored globs. If the
	// approval's hash no longer covers them, the receipt is reported as
	// unrestorable rather than re-stamped: re-stamping would be this code
	// approving a scope on a human's behalf.
	//
	// THIS RULE IS DELIBERATELY NOT A TABLE ROW. It is not a restore — it
	// carries nothing forward — but a conditional refusal, keyed on a hash of
	// a DIFFERENT field's restored value, and it fires on receipts the
	// artifact-level restore never touched. Folding it into the scalar
	// artifact case is precisely the flattening AC-1.3 was written to catch.
	if after.PathsApproval != nil && sidecar.HashGlobs(after.Paths) != after.PathsApproval.GlobsSHA256 {
		res.Unrestorable = append(res.Unrestorable, PinnedField{
			ArtifactID: artifactID,
			Field:      "paths_approval",
			Scope:      ScopeArtifact,
			Value:      after.PathsApproval.ApprovedAt,
			Reason:     "the approval receipt's hash does not cover the restored globs — re-approve with `edikt sidecar approve --kind paths`",
		})
	}

	// ---- per-entry pins ----------------------------------------------------
	// BOTH KINDS, ONE PATH (SR-002). The body lives in preserveArm and runs
	// once per collection; nothing here knows whether it is looking at a
	// directive or a prohibition.
	for _, arm := range entryArms(before, after) {
		if preserveArm(artifactID, arm, res, &reinstated) {
			changed = true
		}
	}
	sortPinnedFields(res.Unrestorable)

	sortPinnedFields(reinstated)
	return res, reinstated, changed
}

// reinstatedFields renders the field names applyPin actually wrote back into
// PinnedField records, so a read-only caller can NAME what the live sidecar
// was missing rather than merely count it (AC-3.5: verify "names the field").
func reinstatedFields(artifactID string, kind EntryKind, directiveText string, p pin, fields []string) []PinnedField {
	out := make([]PinnedField, 0, len(fields))
	for _, f := range fields {
		out = append(out, PinnedField{
			ArtifactID:    artifactID,
			Field:         f,
			Scope:         ScopeEntry,
			EntryKind:     kind,
			DirectiveText: directiveText,
			Value:         p.values[f].String(),
			Reason:        "the regenerated sidecar did not carry this pinned value",
		})
	}
	return out
}

// sortPinnedFields gives every report a stable order. EntryKind is part of the
// key because a directive and a prohibition may carry identical text (SR-002):
// sorting on text and field alone would leave two genuinely different entries
// interleaving run to run, and a report that churns between identical runs is
// one nobody can diff.
func sortPinnedFields(fs []PinnedField) {
	sort.Slice(fs, func(i, j int) bool {
		if fs[i].DirectiveText != fs[j].DirectiveText {
			return fs[i].DirectiveText < fs[j].DirectiveText
		}
		if fs[i].EntryKind != fs[j].EntryKind {
			return fs[i].EntryKind < fs[j].EntryKind
		}
		return fs[i].Field < fs[j].Field
	})
}

// MissingPins is PreservePinned's READ-ONLY sibling: it reports every pinned
// field from `before` that the sidecar at afterPath does not currently carry,
// and writes nothing anywhere.
//
// It exists because SPEC-013's D6 seam splits the restore from the gate.
// `bin/edikt phasea restore` performs the mutation; `bin/edikt phasea verify`
// only observes, and an observer that could not tell a surviving pin from a
// lost one would have to either re-run the mutation (collapsing the seam) or
// trust the mutation's own report of itself (the exact "did the write happen"
// fusion the seam exists to undo). Both returned classes mean the same thing
// to a gate — the pin is not on disk — but they are kept distinguishable by
// Reason: an Unrestorable pin can never be reattached, while a reinstated one
// simply had not been restored yet when verify looked.
func MissingPins(artifactID string, before *sidecar.Sidecar, beforeLoadErr error, afterPath string) ([]PinnedField, error) {
	if res, done := earlyResult(artifactID, before, beforeLoadErr); done {
		return res.Unrestorable, nil
	}
	after, err := sidecar.Load(afterPath)
	if err != nil {
		return nil, fmt.Errorf("load regenerated sidecar %s: %w", afterPath, err)
	}
	res, reinstated, _ := computePins(artifactID, before, after)
	out := append(append([]PinnedField(nil), res.Unrestorable...), reinstated...)
	sortPinnedFields(out)
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// sameOrderedStrings compares two lists AS WRITTEN — same items, same order.
//
// sameStrings sorts first, which is correct for a glob set: two orderings of
// the same globs scope the same files, so a reordering is not a change. An
// authored list is the opposite case — the order is the human's — so the two
// comparisons are kept separate rather than one being made to serve both.
func sameOrderedStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	return strings.Join(x, "\x00") == strings.Join(y, "\x00")
}
