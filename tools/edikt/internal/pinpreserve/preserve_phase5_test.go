package pinpreserve

// preserve_phase5_test.go — PLAN-SPEC-023 phase 5 (AC-5.1, AC-5.2, AC-5.4).
//
// Phase 5 puts `manual_directives` and `suppressed_directives` into the
// mechanism. They are the only two pinnable fields whose content a human TYPED
// rather than approved: a manual directive is a rule the extractor could not
// derive from the artifact's prose, and a suppressed one is a rule the team
// decided to subtract. The extractor may write neither, so a regeneration
// produces a sidecar with both absent and the lists are gone unless something
// carries them.
//
//	AC-5.1  both lists are CARRIED FORWARD, asserted on the file on disk and
//	        in the reinstated report, and asserted not to be rewritten when
//	        nothing was lost
//	AC-5.2  the REPORTING half, on the reachable report. These are whole-list
//	        artifact-level fields with no text-matching step, so there is no
//	        per-entry unrestorable arm for them at all — the report a caller
//	        can actually reach is the LOAD-ERROR at-risk one, and that is what
//	        is asserted. The unreachable arm is asserted ABSENT in the same
//	        test, two-sided, so this cannot pass by the entry loop having
//	        quietly started naming them.
//	AC-5.4  the SCHEME's falsifier, for the table this phase actually used.
//	        AC-2.1's TestPinTable_AddedRowReachesEverySite adds a row to
//	        `entryPinTable` and drives all six of ITS derived sites; it never
//	        touches `artifactPinTable`. Phase 5's two rows are artifact-level,
//	        so the property "a new row reaches every derived site" was
//	        demonstrated for a different table than the one the phase extended
//	        — and AC-5.1/AC-5.2 assert the two named fields, which a hand-coded
//	        pair of special cases would satisfy just as well as a table.

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

const p5Text = "The compiler MUST refuse an unknown topic. (ref: ADR-901)"

// p5Manual and p5Suppressed are deliberately NOT in sorted order.
//
// restoreApprovedPaths sorts what it restores, which is correct for a glob set
// and would be a silent corruption here: the order of an authored list is the
// author's. A restore that sorted would leave both fixtures on disk with their
// elements swapped, and the equality below is what catches it.
var (
	p5Manual = []string{
		"Zero-length topic names are rejected before the registry is consulted.",
		"Always verify the hook script is executable before running the suite.",
	}
	p5Suppressed = []string{
		"Do not cache hook results across sessions.",
		"Avoid shelling out from a hook body.",
	}
)

// p5AuthoredList is one of the two fields under test, with the accessors that
// read and write it. The two cells are identical apart from these, and writing
// them as a pair is what keeps a fix applied to one from being forgotten on the
// other.
type p5AuthoredList struct {
	field string
	want  []string
	pin   func(*sidecar.Sidecar)
	read  func(*sidecar.Sidecar) []string
}

func p5ManualList() p5AuthoredList {
	return p5AuthoredList{
		field: "manual_directives",
		want:  p5Manual,
		pin:   func(s *sidecar.Sidecar) { s.ManualDirectives = append([]string(nil), p5Manual...) },
		read:  func(s *sidecar.Sidecar) []string { return s.ManualDirectives },
	}
}

func p5SuppressedList() p5AuthoredList {
	return p5AuthoredList{
		field: "suppressed_directives",
		want:  p5Suppressed,
		pin:   func(s *sidecar.Sidecar) { s.SuppressedDirectives = append([]string(nil), p5Suppressed...) },
		read:  func(s *sidecar.Sidecar) []string { return s.SuppressedDirectives },
	}
}

// p5Before builds the pre-regeneration sidecar: one anchored directive carrying
// NO entry pins at all, plus the authored list.
//
// The absence of an entry pin is load-bearing. With a `verify` co-pinned the
// rewrite would happen anyway and this fixture could not tell an artifact-level
// row that works from one that is never consulted; with nothing else pinned,
// the rewrite can only have come from the row under test.
func p5Before(c p5AuthoredList) *sidecar.Sidecar {
	sc := &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p5Text)}}
	c.pin(sc)
	return sc
}

// p5RegeneratedPath writes what the extractor would produce: the same directive,
// no authored lists, since the extractor may write neither field.
func p5RegeneratedPath(t *testing.T, mutate func(*sidecar.Sidecar)) string {
	t.Helper()
	sc := &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p5Text)}}
	if mutate != nil {
		mutate(sc)
	}
	return tblAfter(t, sc)
}

// p5CarriedRoundTrip is AC-5.1 for one field, driven both ways round: NAMED as
// missing by MissingPins before the restore (the only entry point that returns
// the reinstated list — preserve.go's finding 37), then ON DISK after it.
func p5CarriedRoundTrip(t *testing.T, c p5AuthoredList) {
	t.Helper()
	before := p5Before(c)
	after := p5RegeneratedPath(t, nil)

	// HALF ONE — the reinstated report names the field, at ARTIFACT scope,
	// with a value a human could re-author from.
	missing, err := MissingPins("ADR-901", before, nil, after)
	if err != nil {
		t.Fatalf("MissingPins on the regenerated sidecar: %v", err)
	}
	hits := p3Named(missing, c.field)
	if len(hits) != 1 {
		t.Fatalf("MissingPins named %q %d time(s); want exactly 1. The regenerated sidecar carries no %s, "+
			"so a caller checking survival is told nothing was lost", c.field, len(hits), c.field)
	}
	if hits[0].Scope != ScopeArtifact {
		t.Errorf("%q is reported at scope %q; want %q — it belongs to the sidecar, not to an entry",
			c.field, hits[0].Scope, ScopeArtifact)
	}
	if hits[0].EntryKind != "" {
		t.Errorf("%q carries EntryKind %q; an artifact-level pin belonged to no entry, and a non-empty "+
			"kind here would tell an operator to go looking for a directive that does not exist",
			c.field, hits[0].EntryKind)
	}
	for _, item := range c.want {
		if !strings.Contains(hits[0].Value, item) {
			t.Errorf("%q is named with a value that omits %q. A name with no value cannot be re-authored",
				c.field, item)
		}
	}

	// HALF TWO — the values, through the mutating path and a fresh Load.
	res, err := PreservePinned("ADR-901", before, nil, after)
	if err != nil {
		t.Fatalf("PreservePinned: %v", err)
	}
	if !res.SidecarRewritten {
		t.Fatalf("the sidecar was not rewritten. Nothing else is pinned on this fixture, so the "+
			"artifact-level %s row is the only thing that could have marked the artifact changed — "+
			"and it did not", c.field)
	}
	if res.DirectivePins != 0 {
		t.Errorf("DirectivePins = %d; want 0. This fixture pins no entry field, so a non-zero count means "+
			"the authored list was counted as an entry pin", res.DirectivePins)
	}
	got := c.read(tblLoad(t, after))
	if !sameOrderedStrings(got, c.want) {
		t.Fatalf("%s on disk = %q; want %q (same items, SAME ORDER — the order of an authored list is the "+
			"author's, and a restore that sorted would corrupt it silently)", c.field, got, c.want)
	}
}

// TestPreserve_ManualDirectivesCarried — AC-5.1.
//
// A manual directive is a rule the extractor could not derive from the
// artifact's prose; losing it removes the rule from the effective set with
// nothing recording that it was ever there.
func TestPreserve_ManualDirectivesCarried(t *testing.T) {
	p5CarriedRoundTrip(t, p5ManualList())

	// The other side: nothing to carry, nothing carried. An artifact-level
	// restore that fired on every run would rewrite every sidecar in the
	// corpus on every re-extraction, and the count of restored artifacts could
	// never go to zero — the same defect applyPin's changed-tracking exists to
	// avoid, one level up.
	t.Run("no rewrite when the regenerated sidecar already carries it", func(t *testing.T) {
		c := p5ManualList()
		after := p5RegeneratedPath(t, func(s *sidecar.Sidecar) { c.pin(s) })
		res, err := PreservePinned("ADR-901", p5Before(c), nil, after)
		if err != nil {
			t.Fatal(err)
		}
		if res.SidecarRewritten {
			t.Error("the sidecar was rewritten although manual_directives already matched. A restore that " +
				"reports work it did not do is a number nobody can use to tell whether the repair is finished")
		}
	})
}

// TestPreserve_SuppressedDirectivesCarried — AC-5.1.
//
// A suppressed directive is subtracted from the effective rule set at compile
// time. Losing the list REINSTATES rules a team deliberately turned off, which
// is the louder half of this pair: the failure shows up as governance that came
// back, with nothing saying who decided it should.
func TestPreserve_SuppressedDirectivesCarried(t *testing.T) {
	p5CarriedRoundTrip(t, p5SuppressedList())

	t.Run("no rewrite when the regenerated sidecar already carries it", func(t *testing.T) {
		c := p5SuppressedList()
		after := p5RegeneratedPath(t, func(s *sidecar.Sidecar) { c.pin(s) })
		res, err := PreservePinned("ADR-901", p5Before(c), nil, after)
		if err != nil {
			t.Fatal(err)
		}
		if res.SidecarRewritten {
			t.Error("the sidecar was rewritten although suppressed_directives already matched")
		}
	})
}

// TestAtRisk_NamesManualAndSuppressedOnLoadError is AC-5.2 — the REPORTING
// half, named on the report that can actually be reached.
//
// AC-5.1 alone is satisfiable with no reporting at all, and INV-015's reporting
// half would then never be built (SR-013). But the report to assert is not the
// obvious one: these are whole-list artifact-level fields with NO text-matching
// step, so there is no per-entry unrestorable arm they could ever appear in. A
// criterion demanding one would demand a report that cannot fire, and the
// cheapest way to make such a test green is to wire the fields into the entry
// loop where they do not belong.
//
// So this asserts BOTH directions: named on the load-error at-risk report,
// which is reachable, and ABSENT from the dropped-entry unrestorable report,
// which is not. The second half carries its own control — a `verify` pin on the
// dropped entry — so an absence produced by the report never running is not
// mistaken for the absence under test (INV-013).
func TestAtRisk_NamesManualAndSuppressedOnLoadError(t *testing.T) {
	fields := []string{"manual_directives", "suppressed_directives"}

	t.Run("named at-risk when the prior sidecar will not parse", func(t *testing.T) {
		after := p5RegeneratedPath(t, nil)
		res, err := PreservePinned("ADR-901", nil, errUnparseable(), after)
		if err != nil {
			t.Fatalf("a load-error report must not itself be an error: %v", err)
		}
		if !res.LoadFailed {
			t.Fatal("LoadFailed is false — the load error was answered as a clean bootstrap, so the report " +
				"below is not the one under test (INV-013)")
		}
		// The control. `verify` has been in the at-risk report since before
		// this phase, so an empty or truncated report is distinguishable from
		// one that simply omits the two fields under test.
		if len(p3Named(res.Unrestorable, "verify")) == 0 {
			t.Fatalf("the at-risk report does not name `verify`, which has been in it since phase 1 — the "+
				"report is not the at-risk one, so nothing below is measured. It named %v",
				tblSorted(reportFieldSet(res.Unrestorable)))
		}
		for _, field := range fields {
			hits := p3Named(res.Unrestorable, field)
			if len(hits) == 0 {
				t.Errorf("%q is not named at-risk on a load failure. The prior sidecar might have carried "+
					"any authored content at all and this package cannot see it; a report that omits the "+
					"field under-states the loss (INV-015). It named %v",
					field, tblSorted(reportFieldSet(res.Unrestorable)))
				continue
			}
			if hits[0].Scope != ScopeArtifact {
				t.Errorf("%q is reported at scope %q; want %q", field, hits[0].Scope, ScopeArtifact)
			}
			if strings.TrimSpace(hits[0].Reason) == "" {
				t.Errorf("%q is named at-risk with an empty reason; an operator is told a field is at risk "+
					"and not what to do about it", field)
			}
		}
	})

	// The unreachable arm, asserted as an absence and kept two-sided by the
	// control below. If a later change ever gives these fields a matching step,
	// this is the cell that has to be updated deliberately rather than the one
	// that quietly starts passing for a new reason.
	t.Run("no per-entry unrestorable arm on a dropped entry", func(t *testing.T) {
		pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-901)")
		pinned.Verify = gapControlVerify
		before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
		p5ManualList().pin(before)
		p5SuppressedList().pin(before)

		after := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-901)")},
		})
		res, err := PreservePinned("ADR-901", before, nil, after)
		if err != nil {
			t.Fatal(err)
		}
		// THE CONTROL: the dropped entry's `verify` must be named, or the
		// unrestorable report never ran and the absences below are vacuous.
		if len(p3Named(res.Unrestorable, "verify")) == 0 {
			t.Fatalf("the dropped entry's `verify` is not named unrestorable — the per-entry report did not "+
				"run, so the absences asserted here observe nothing (INV-013). It named %v",
				tblSorted(reportFieldSet(res.Unrestorable)))
		}
		for _, field := range fields {
			for _, hit := range p3Named(res.Unrestorable, field) {
				if hit.Scope == ScopeEntry {
					t.Errorf("%q is named unrestorable at ENTRY scope on a dropped directive. It is a "+
						"whole-list artifact-level field with no matching step, so no entry can have "+
						"lost it; reporting one sends an operator looking for a directive that never "+
						"held it. Reported: %s", field, hit.String())
				}
			}
		}
		// And the restore still happened: the authored lists reached disk even
		// though the entry they shared a sidecar with was dropped.
		restored := tblLoad(t, after)
		if !sameOrderedStrings(restored.ManualDirectives, p5Manual) ||
			!sameOrderedStrings(restored.SuppressedDirectives, p5Suppressed) {
			t.Errorf("the authored lists did not survive an artifact whose only directive was dropped: "+
				"manual_directives=%q suppressed_directives=%q. The absence asserted above would then be "+
				"the machinery not running rather than the arm being unreachable",
				restored.ManualDirectives, restored.SuppressedDirectives)
		}
	})
}

// ---------------------------------------------------------------------------
// AC-5.4 — the ARTIFACT-level table, and every site derived from it
// ---------------------------------------------------------------------------

// p5ArtifactText is the fixture's one directive, and it carries NO ENTRY PIN.
//
// That is load-bearing in the same way p5Before's is. With a `verify` co-pinned
// the artifact would be rewritten, the reinstated report populated and the
// unrestorable report exercised whatever the artifact table did — and every
// observation below would be satisfied by machinery that never consulted the
// added rows.
const p5ArtifactText = "The registry MUST reject a topic it does not declare. (ref: ADR-901)"

// TestArtifactPinTable_AddedRowReachesEverySite is AC-2.1's falsifier for the
// OTHER table, and it exists because AC-2.1's was written for the entry one.
//
// `TestPinTable_AddedRowReachesEverySite` appends to `entryPinTable` and asks
// six derived sites whether they know about the addition. `artifactPinTable`
// has its own four, reached by completely different code — a whole-function
// restore rather than a read/write pair, an at-risk projection over a second
// loop, and a `Field`/`Scope` stamp that lives at computePins' call site rather
// than inside the row. None of that is exercised by adding an entry row.
//
// So the same execution, on the table phase 5 extended: add a row, then ask
// every site. A site that kept a hand-maintained copy of the artifact-level
// names cannot know about the addition, and the corresponding half goes red.
//
// THE ROWS ARE TEST-ONLY, and they write `reminders` and
// `proposed_topic_description` — two `Sidecar` fields production does not
// preserve, so nothing here changes what the package restores. One is
// LIST-shaped and one is STRING-shaped because SR-006 requires the mechanism to
// carry both, and because the artifact table's restores are whole functions,
// where a shape that only ever appears once is a shape nobody has checked.
//
// The list row reuses production's `restoreAuthoredList`. That is deliberate:
// its restore is the generic phase 5 landed, so a row built on it is the same
// KIND of row as `manual_directives` — the test adds a row, not a mechanism.
// The string row's restore is written out here because production has no
// string-shaped artifact restore to borrow, and inventing one in production for
// a test to use would be the test writing the code it checks.
//
// NEITHER restore sets a `Field`. computePins stamps it from the row (SPEC-023
// finding 29), which is itself one of the sites under test: a report naming ""
// twice is what a row that had to repeat its own name would eventually produce.
func TestArtifactPinTable_AddedRowReachesEverySite(t *testing.T) {
	original := artifactPinTable
	t.Cleanup(func() { artifactPinTable = original })

	const wantText = "the compiler's refusal to load an undeclared topic"
	wantList := []string{
		"Check the hook script is executable before running the suite.",
		"Read the prior sidecar before treating a re-extraction as final.",
	}

	artifactPinTable = append(append([]artifactPin(nil), original...),
		artifactPin{
			name: "test_only_artifact_text_pin",
			restore: func(artifactID string, before, after *sidecar.Sidecar, _ *PreserveResult) []PinnedField {
				want := before.ProposedTopicDescription
				if want == "" || want == after.ProposedTopicDescription {
					return nil
				}
				after.ProposedTopicDescription = want
				return []PinnedField{{
					ArtifactID: artifactID,
					Value:      want,
					Reason:     "the regenerated sidecar did not carry the test-only string pin",
				}}
			},
		},
		artifactPin{
			name: "test_only_artifact_list_pin",
			restore: restoreAuthoredList(
				func(s *sidecar.Sidecar) []string { return s.Reminders },
				func(s *sidecar.Sidecar, v []string) { s.Reminders = v },
				"the test-only artifact list",
			),
		},
	)
	added := []string{"test_only_artifact_text_pin", "test_only_artifact_list_pin"}

	pinArtifact := func(s *sidecar.Sidecar) {
		s.ProposedTopicDescription = wantText
		s.Reminders = append([]string(nil), wantList...)
	}
	beforeSidecar := func() *sidecar.Sidecar {
		sc := &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p5ArtifactText)}}
		pinArtifact(sc)
		return sc
	}
	regenerated := func(t *testing.T) string {
		t.Helper()
		return tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p5ArtifactText)}})
	}

	// SITE 1 — the at-risk report, via pinnedFieldsAtRisk's artifact arm.
	//
	// A load failure reaches this without reading a single entry, so it is the
	// one site that can only be derived from artifactPinTable: nothing else in
	// the package knows an artifact-level name at that point.
	t.Run("at-risk report names the added rows at artifact scope", func(t *testing.T) {
		res, err := PreservePinned("ADR-901", nil, errUnparseable(), regenerated(t))
		if err != nil {
			t.Fatalf("a load-error report must not itself be an error: %v", err)
		}
		if !res.LoadFailed {
			t.Fatal("LoadFailed is false — the load error was answered as a clean bootstrap, so the " +
				"report below is not the one under test (INV-013)")
		}
		named := tblNamed(res.Unrestorable)
		for _, name := range added {
			hit, ok := named[name]
			if !ok {
				t.Errorf("%q is not named at-risk on a load failure — the at-risk report's artifact arm is "+
					"not derived from artifactPinTable; it named %v",
					name, tblSorted(reportFieldSet(res.Unrestorable)))
				continue
			}
			if hit.Scope != ScopeArtifact {
				t.Errorf("%q is reported at scope %q; want %q — computePins' artifact arm is not stamping "+
					"the scope, so an operator cannot tell the two mechanisms apart (SR-011)",
					name, hit.Scope, ScopeArtifact)
			}
			if hit.EntryKind != "" {
				t.Errorf("%q carries EntryKind %q; nothing was read on a load failure, so no entry kind was "+
					"ever observed", name, hit.EntryKind)
			}
		}
	})

	// SITE 2 — the reinstated report, which ONLY MissingPins returns:
	// PreservePinned discards it at computePins' call site (SPEC-023 finding
	// 37). It is also where the row's NAME is stamped onto a PinnedField the
	// restore returned without one.
	t.Run("reinstated report names the added rows with their values", func(t *testing.T) {
		missing, err := MissingPins("ADR-901", beforeSidecar(), nil, regenerated(t))
		if err != nil {
			t.Fatalf("MissingPins on the regenerated sidecar: %v", err)
		}
		named := tblNamed(missing)
		for _, name := range added {
			hit, ok := named[name]
			if !ok {
				t.Errorf("%q is not named as missing from the live sidecar — the reinstated report is not "+
					"derived from artifactPinTable, so a caller checking survival is told nothing was lost; "+
					"it named %v", name, tblSorted(reportFieldSet(missing)))
				continue
			}
			if hit.Value == "" {
				t.Errorf("%q is reported reinstated with an empty value; a name with no value cannot be "+
					"re-authored without going to git history", name)
			}
			if hit.Scope != ScopeArtifact {
				t.Errorf("%q is reinstated at scope %q; want %q", name, hit.Scope, ScopeArtifact)
			}
		}
	})

	// SITES 3 and 4 — the write-back to DISK, and the changed flag that
	// triggers it. Nothing else on this fixture is pinned, so `changed` can
	// only have come from the added rows: a `changed` that ignored the artifact
	// arm would leave the file untouched and both halves go red together.
	t.Run("restored to disk, and the artifact marked changed", func(t *testing.T) {
		path := regenerated(t)
		res, err := PreservePinned("ADR-901", beforeSidecar(), nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if !res.SidecarRewritten {
			t.Fatalf("the sidecar was not rewritten. This fixture pins NOTHING at entry level, so the "+
				"added artifact-level rows are the only thing that could have marked the artifact "+
				"changed — and they did not. Result: %+v", res)
		}
		if res.DirectivePins != 0 {
			t.Errorf("DirectivePins = %d; want 0 — an artifact-level pin is not an entry pin, and counting "+
				"one here would inflate a number that already counts entries rather than fields "+
				"(SPEC-023 finding 38)", res.DirectivePins)
		}
		got := tblLoad(t, path)
		if got.ProposedTopicDescription != wantText {
			t.Errorf("the string row's destination on disk = %q; want %q — the added STRING-shaped "+
				"artifact row is not written back", got.ProposedTopicDescription, wantText)
		}
		if !sameOrderedStrings(got.Reminders, wantList) {
			t.Errorf("the list row's destination on disk = %q; want %q (same items, SAME ORDER) — the "+
				"added LIST-shaped artifact row is not written back (SR-006)", got.Reminders, wantList)
		}
	})

	// THE OTHER SIDE of site 4. An artifact-level restore that fired whether or
	// not anything was missing would rewrite every sidecar in the corpus on
	// every re-extraction, and the count of repaired artifacts could never
	// reach zero — the same defect applyPin's changed-tracking exists to avoid,
	// one level up. Without this, a row whose restore returned a PinnedField
	// unconditionally would pass all four sites above.
	t.Run("no rewrite when the regenerated sidecar already carries the added rows", func(t *testing.T) {
		sc := &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p5ArtifactText)}}
		pinArtifact(sc)
		res, err := PreservePinned("ADR-901", beforeSidecar(), nil, tblAfter(t, sc))
		if err != nil {
			t.Fatal(err)
		}
		if res.SidecarRewritten {
			t.Error("the sidecar was rewritten although both added rows already matched. A restore that " +
				"reports work it did not do is a number nobody can use to tell whether the repair is finished")
		}
	})
}
