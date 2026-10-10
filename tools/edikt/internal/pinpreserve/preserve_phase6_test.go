package pinpreserve

// preserve_phase6_test.go — PLAN-SPEC-023 phase 6 (AC-6.1).
//
// THE DEFECT. `preserveArm` indexes the regenerated collection with
// `byText[d.Text] = i`, a plain assignment, so two entries carrying identical
// text leave only the LAST index in the map. The pinned entry then matches on
// the exact-text branch, the pin is welded onto whichever duplicate happened to
// be written last, and nothing appears in `Unrestorable`. The normalized-form
// branch beside it has guarded ambiguity since the package was written
// (`len(cands) == 1`); the exact branch never did (SPEC-023 SR-012, finding 31).
//
// That is the outcome the package header forbids in as many words — "a sidecar
// claiming a human approved a command against a rule they never saw it on is
// worse than one missing the command" — reachable through the one branch with
// no guard.
//
// WHY TWO TESTS AND NOT ONE. Asserting only that `Unrestorable` is non-empty
// passes an implementation that attaches the pin AND reports it, which is
// strictly worse than either half alone: the sidecar carries an invented
// approval and the report says the approval was lost, so the operator
// re-approves something that is already claimed. The two halves are therefore
// separate functions, and each is independently red on the pre-phase-6 tree.
//
//	TestExactBranch_DuplicateTextWithheld   nothing on disk carries the pin
//	TestExactBranch_DuplicateTextReported   the loss is named, with a value and
//	                                        the AMBIGUITY as its reason
//	TestDuplicateTextFixture_ValidatesToday the fixture is a state the schema
//	                                        actually permits
//
// THE THIRD TEST IS THE EXPIRY DATE ON THE OTHER TWO. Duplicate directive text
// is representable today: `Validate` enforces uniqueness on `signals` only, and
// says nothing about directive or prohibition text. If a later change adds a
// text-uniqueness rule, the two tests above would quietly become tests of an
// unreachable state — green forever, measuring nothing. So the fixture is
// stamped `schema_version: 2` (below which Validate says nothing about
// uniqueness at all) and run through `Validate()` INSIDE the test, requiring
// nil. The day uniqueness is enforced, this goes red and someone has to decide
// what the other two now mean.

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// p6DupText is the text the regenerated sidecar carries TWICE.
const p6DupText = "The gate MUST refuse an unsigned payload. (ref: ADR-901)"

// p6ControlText is the entry that is NOT ambiguous. It is pinned on every
// fixture that asserts a report, because an empty `Unrestorable` produced by a
// restore that never ran is indistinguishable from one produced by a guard that
// works (INV-013).
const p6ControlText = "The registry MUST reject a topic it does not declare. (ref: ADR-901)"

const (
	p6Verify     = "bash test/fixtures/behavioral/ADR-901/positive.sh"
	p6VerifyKind = "behavioral"
	p6ApprovedAt = "2026-08-03T00:00:00Z"
)

// p6Fields are the three pins carried on every fixture entry here.
//
// Three rather than one because the report is emitted per FIELD: a guard that
// withheld the entry but reported only the first of its pins would satisfy a
// one-field assertion while losing the other two silently.
var p6Fields = []string{"verify", "verify_kind", "human_approved_at"}

// p6Pinned is a pre-regeneration entry carrying all three pins.
func p6Pinned(text string) sidecar.Directive {
	d := anchored(text)
	d.Verify = p6Verify
	d.VerifyKind = p6VerifyKind
	d.HumanApprovedAt = p6ApprovedAt
	return d
}

func p6PinnedProhibition(text string) sidecar.Prohibition {
	p := tblProhibition(text)
	p.Verify = p6Verify
	p.VerifyKind = p6VerifyKind
	p.HumanApprovedAt = p6ApprovedAt
	return p
}

// p6carriesAPin reports whether an entry — in the Directive shape both arms are
// read through — carries any of the three pins.
func p6carriesAPin(d sidecar.Directive) bool {
	return d.Verify != "" || d.VerifyKind != "" || d.HumanApprovedAt != ""
}

// p6DuplicateDirectives is the regenerated sidecar for the directive arm: the
// SAME text twice, which is what the extractor produces when one rule is
// re-worded into two directives that happen to land on identical prose.
func p6DuplicateDirectives(extra ...sidecar.Directive) *sidecar.Sidecar {
	return &sidecar.Sidecar{
		Directives: append([]sidecar.Directive{anchored(p6DupText), anchored(p6DupText)}, extra...),
	}
}

// p6DuplicateProhibitions is the same shape in the OTHER collection. Both arms
// run the same body (SR-002), so a guard added to one and not the other is a
// half-fix that the directive arm alone cannot see.
func p6DuplicateProhibitions() *sidecar.Sidecar {
	return &sidecar.Sidecar{
		Prohibitions: []sidecar.Prohibition{tblProhibition(p6DupText), tblProhibition(p6DupText)},
	}
}

// TestExactBranch_DuplicateTextWithheld — AC-6.1, the WITHHELD half.
//
// Nothing on disk may carry the pin. This is asserted against the reloaded file
// rather than against PreserveResult, for the same reason phase 1's
// characterisation cells are: a result that claims a restore is not evidence
// that one happened, and a result that claims none is not evidence that none
// did.
//
// `SidecarRewritten` is asserted false as the second, independent signal. These
// fixtures pin NOTHING else, so a rewrite could only have come from the pin
// being attached — an implementation that attached it and then blanked one
// field would pass the on-disk check and fail here.
func TestExactBranch_DuplicateTextWithheld(t *testing.T) {
	t.Run("directives", func(t *testing.T) {
		before := &sidecar.Sidecar{Directives: []sidecar.Directive{p6Pinned(p6DupText)}}
		path := tblAfter(t, p6DuplicateDirectives())

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatalf("PreservePinned: %v", err)
		}
		got := tblLoad(t, path)
		if len(got.Directives) != 2 {
			t.Fatalf("the regenerated sidecar carries %d directive(s); the fixture is two identically-worded "+
				"ones, so nothing below is measuring the ambiguity", len(got.Directives))
		}
		for i, d := range got.Directives {
			if p6carriesAPin(d) {
				t.Errorf("directives[%d] carries a pin (verify=%q verify_kind=%q human_approved_at=%q). Two "+
					"directives share this text, so nothing distinguishes the one a human approved the "+
					"command against; attaching it to either invents an approval",
					i, d.Verify, d.VerifyKind, d.HumanApprovedAt)
			}
		}
		if res.DirectivePins != 0 {
			t.Errorf("DirectivePins = %d; want 0 — the restore counted work it must not have done", res.DirectivePins)
		}
		if res.SidecarRewritten {
			t.Error("the sidecar was rewritten. Nothing else is pinned on this fixture, so the only thing " +
				"that could have marked it changed is the pin being attached to one of the duplicates")
		}
	})

	t.Run("prohibitions", func(t *testing.T) {
		before := &sidecar.Sidecar{Prohibitions: []sidecar.Prohibition{p6PinnedProhibition(p6DupText)}}
		path := tblAfter(t, p6DuplicateProhibitions())

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatalf("PreservePinned: %v", err)
		}
		got := tblLoad(t, path)
		if len(got.Prohibitions) != 2 {
			t.Fatalf("the regenerated sidecar carries %d prohibition(s); want the fixture's two", len(got.Prohibitions))
		}
		for i, p := range got.Prohibitions {
			if p6carriesAPin(directiveShapeOf(p)) {
				t.Errorf("prohibitions[%d] carries a pin (verify=%q verify_kind=%q human_approved_at=%q). Both "+
					"entry collections run the same body (SR-002), so a guard on the directive arm alone "+
					"leaves this one welding pins exactly as before",
					i, p.Verify, p.VerifyKind, p.HumanApprovedAt)
			}
		}
		if res.SidecarRewritten {
			t.Error("the prohibition sidecar was rewritten although the only pin on the fixture is ambiguous")
		}
	})
}

// TestExactBranch_DuplicateTextReported — AC-6.1, the REPORTED half.
//
// Withholding alone is not enough: a pin dropped with nothing said is the
// silent loss this whole package exists to prevent. Every field the ambiguous
// entry carried must be NAMED, with a VALUE a human can re-approve from, and
// with the ambiguity as the reason.
//
// THE REASON IS ASSERTED, not just its presence. The unrestorable arm has two
// reasons — "no counterpart after regeneration" and the ambiguity one — and the
// wrong one here is a lie an operator acts on: it sends them looking for a
// directive that was deleted, when in fact two of them are sitting in the file.
//
// The control is `p6ControlText`, pinned and unambiguous. It must be restored,
// or the report asserted below was produced by machinery that never reached the
// ambiguous entry at all (INV-013).
func TestExactBranch_DuplicateTextReported(t *testing.T) {
	before := &sidecar.Sidecar{Directives: []sidecar.Directive{
		p6Pinned(p6DupText),
		p6Pinned(p6ControlText),
	}}
	path := tblAfter(t, p6DuplicateDirectives(anchored(p6ControlText)))

	res, err := PreservePinned("ADR-901", before, nil, path)
	if err != nil {
		t.Fatalf("PreservePinned: %v", err)
	}

	// THE CONTROL, first: the unambiguous pin reached disk.
	got := tblLoad(t, path)
	control := got.Directives[len(got.Directives)-1]
	if control.Text != p6ControlText {
		t.Fatalf("the control directive is not where the fixture put it: %q", control.Text)
	}
	if control.Verify != p6Verify || control.HumanApprovedAt != p6ApprovedAt {
		t.Fatalf("the UNAMBIGUOUS control pin did not survive (verify=%q human_approved_at=%q). The restore "+
			"did not run over this fixture, so the report asserted below observes nothing (INV-013)",
			control.Verify, control.HumanApprovedAt)
	}

	// And the ambiguous entry is still untouched — the withheld half holds when
	// a restore DID happen elsewhere in the same artifact.
	for i, d := range got.Directives[:2] {
		if p6carriesAPin(d) {
			t.Errorf("directives[%d] carries a pin although its text is duplicated; the guard is bypassed "+
				"whenever some other entry in the same sidecar matches cleanly", i)
		}
	}

	// THE REPORT. One entry per field, carrying the text it was pinned to.
	for _, field := range p6Fields {
		hits := p3Named(res.Unrestorable, field)
		var hit *PinnedField
		for i := range hits {
			if hits[i].DirectiveText == p6DupText {
				hit = &hits[i]
				break
			}
		}
		if hit == nil {
			t.Errorf("%q is not named unrestorable for the ambiguous entry. The pin was withheld and NOTHING "+
				"said so — a silent drop, which is the failure this package exists to prevent. The report "+
				"named %v", field, tblSorted(reportFieldSet(res.Unrestorable)))
			continue
		}
		if hit.Value == "" {
			t.Errorf("%q is named with an empty value; a name with no value cannot be re-approved without "+
				"going to git history", field)
		}
		if hit.Scope != ScopeEntry {
			t.Errorf("%q is reported at scope %q; want %q — it belonged to one entry, not to the artifact",
				field, hit.Scope, ScopeEntry)
		}
		if hit.EntryKind != KindDirective {
			t.Errorf("%q is reported with EntryKind %q; want %q — an operator told only that it belonged to "+
				"'an entry' cannot find it when a directive and a prohibition share text (SR-002)",
				field, hit.EntryKind, KindDirective)
		}
		if strings.Contains(hit.Reason, "no counterpart") {
			t.Errorf("%q is reported as having no counterpart after regeneration: %q. TWO counterparts are "+
				"sitting in the file; that reason sends an operator hunting for a directive that was never "+
				"deleted", field, hit.Reason)
		}
		if !strings.Contains(hit.Reason, "2 ") {
			t.Errorf("%q's reason does not say how many entries now carry the text: %q. The count is what "+
				"tells an operator this is an ambiguity to resolve rather than a deletion to accept",
				field, hit.Reason)
		}
	}

	// The control's pins must NOT be reported: it was restored, and a report
	// naming it would send an operator to re-approve something already on disk.
	for _, u := range res.Unrestorable {
		if u.DirectiveText == p6ControlText {
			t.Errorf("the restored control entry is also reported unrestorable: %s", u.String())
		}
	}
}

// TestDuplicateTextFixture_ValidatesToday — AC-6.1's expiry date.
//
// The two tests above assert behaviour on a sidecar carrying duplicate entry
// text. That state is representable and VALID today: `Sidecar.Validate`
// enforces uniqueness on `signals` and on nothing else. If a later change adds a
// text-uniqueness rule, those two tests do not fail — they keep passing while
// exercising a state the schema no longer permits, which is a test of an
// unreachable branch wearing the clothes of a guard.
//
// So the fixtures are validated HERE, at `schema_version: 2` — the version whose
// rules Validate actually applies, and below which it says nothing about
// uniqueness at all — and nil is REQUIRED. The version is asserted rather than
// assumed, because a fixture that silently stamped v1 would make this test pass
// against rules nobody is claiming.
func TestDuplicateTextFixture_ValidatesToday(t *testing.T) {
	cases := []struct {
		name    string
		sidecar func() *sidecar.Sidecar
		texts   func(*sidecar.Sidecar) []string
	}{
		{
			name:    "duplicate directive text",
			sidecar: func() *sidecar.Sidecar { return p6DuplicateDirectives() },
			texts: func(s *sidecar.Sidecar) []string {
				out := make([]string, 0, len(s.Directives))
				for _, d := range s.Directives {
					out = append(out, d.Text)
				}
				return out
			},
		},
		{
			name:    "duplicate prohibition text",
			sidecar: p6DuplicateProhibitions,
			texts: func(s *sidecar.Sidecar) []string {
				out := make([]string, 0, len(s.Prohibitions))
				for _, p := range s.Prohibitions {
					out = append(out, p.Text)
				}
				return out
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Round-tripped through disk deliberately: this is the SAME
			// construction the two behavioural tests run against, so a fixture
			// that stopped being valid cannot go unnoticed in one place while
			// staying validated in another.
			got := tblLoad(t, tblAfter(t, tc.sidecar()))

			if got.SchemaVersion != sidecar.SchemaVersionV2 {
				t.Fatalf("schema_version = %d; want %d. Validate applies the v2 rules only at v2, so a "+
					"fixture stamped anything else is being checked against rules this test does not name",
					got.SchemaVersion, sidecar.SchemaVersionV2)
			}
			texts := tc.texts(got)
			if len(texts) != 2 || texts[0] != texts[1] {
				t.Fatalf("the fixture does not carry duplicate text: %q", texts)
			}
			if err := got.Validate(); err != nil {
				t.Fatalf("the duplicate-text fixture no longer validates: %v.\n\n"+
					"Entry text uniqueness is now enforced somewhere, which means the state "+
					"TestExactBranch_DuplicateTextWithheld and TestExactBranch_DuplicateTextReported "+
					"exercise can no longer occur. Those two are not failing — they are passing against an "+
					"unreachable branch, which is worse. Decide deliberately whether the exact-text "+
					"ambiguity guard is still load-bearing (SPEC-023 SR-012) before touching this test",
					err)
			}
		})
	}
}
