package pinpreserve

// preserve_phase3_test.go — PLAN-SPEC-023 phase 3 (AC-3.1 … AC-3.8).
//
// Phase 3 puts `enforcement`, per-directive `paths` and `actor_scope` into the
// mechanism. AC-3.1's exit — the nine gap cells flip with one line each — is
// asserted next door in preserve_table_test.go. What lives HERE is everything
// that a green AC-3.1 would not have caught:
//
//	AC-3.1  three POSITIVE round-trips, one per joining field, each asserted
//	        both ways round: NAMED as missing before the restore (MissingPins,
//	        the only call that returns the reinstated list — finding 37) and
//	        ON DISK after it
//	AC-3.3  a list-valued restore never writes a non-nil empty slice, which
//	        would abort the WHOLE artifact at Validate, not just that entry
//	AC-3.4  the BOOL NEGATIVE CONTROL. AC-3.1's actor_scope test is positive
//	        only, so an every-bool-is-a-pin implementation passes it while
//	        claiming a phantom restore on every artifact that never set it
//	AC-3.5  the two mechanisms named `paths` are distinguishable in
//	        PinnedField, not by an empty DirectiveText (SR-011)
//	AC-3.6  the write NEVER CLEARS on an absent pin — a property that lived
//	        only in a comment on applyPin until this phase
//	AC-3.8  the deferral bucket rejects a reason that names no spec and no
//	        phase (SSP-009's third-state clause)

import (
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ---------------------------------------------------------------------------
// Shared fixtures for this file
// ---------------------------------------------------------------------------

const p3Text = "The loader MUST reject unknown keys. (ref: ADR-901)"

// p3Named indexes a report by field name, keeping EVERY hit rather than the
// first: `paths` legitimately appears twice from phase 3 onward, and a helper
// that kept only one would hide the exact collision AC-3.5 is about.
func p3Named(fs []PinnedField, field string) []PinnedField {
	var out []PinnedField
	for _, f := range fs {
		if f.Field == field {
			out = append(out, f)
		}
	}
	return out
}

// p3JoiningRoundTrip drives one joining field both ways round on ONE fixture:
// MissingPins FIRST (read-only — it is the only entry point that returns the
// reinstated list, preserve.go's finding 37), then PreservePinned, then a
// reload from disk. `verify` is co-pinned as the control so a silent failure
// of the whole machinery cannot read as a field-specific loss (INV-013).
func p3JoiningRoundTrip(t *testing.T, field string, pin func(*sidecar.Directive), onDisk func(sidecar.Directive) bool) {
	t.Helper()
	const control = "bash test/fixtures/behavioral/ADR-901/positive.sh"

	pinned := anchored(p3Text)
	pinned.Verify = control
	pin(&pinned)

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})

	// Half one — NAMED as missing, before anything is written.
	missing, err := MissingPins("ADR-901", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(p3Named(missing, "verify")) == 0 {
		t.Fatalf("the control pin `verify` is not named as missing either — the report did not run, so "+
			"nothing below measures %s (INV-013); it named %v", field, tblSorted(reportFieldSet(missing)))
	}
	hits := p3Named(missing, field)
	if len(hits) == 0 {
		t.Fatalf("%q is not named as missing from the regenerated sidecar; the report named %v",
			field, tblSorted(reportFieldSet(missing)))
	}
	if hits[0].Value == "" {
		t.Errorf("%q is named with an empty value — a name with no value cannot be re-approved", field)
	}
	if hits[0].Scope != ScopeEntry {
		t.Errorf("%q is reported with scope %q; an entry-level pin must say so", field, hits[0].Scope)
	}
	if hits[0].DirectiveText != p3Text {
		t.Errorf("%q's report does not say WHICH directive it belongs to: %q", field, hits[0].DirectiveText)
	}

	// Half two — ON DISK, after the restore.
	res, err := PreservePinned("ADR-901", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.SidecarRewritten {
		t.Fatalf("the sidecar was not rewritten, so %q cannot have reached disk", field)
	}
	d := tblLoad(t, path).Directives[0]
	if d.Verify != control {
		t.Fatalf("the control pin did not survive either (%q) — the restore did not run", d.Verify)
	}
	if !onDisk(d) {
		t.Fatalf("%q did not reach disk: %+v", field, d)
	}
}

// ---------------------------------------------------------------------------
// AC-3.1 — the three joining fields, positively
// ---------------------------------------------------------------------------

// TestPreserve_EnforcementRestored — ADR-070/071's explicit delivery-strength
// override. Two live `deny` directives in the corpus had never survived a
// re-extraction.
func TestPreserve_EnforcementRestored(t *testing.T) {
	p3JoiningRoundTrip(t, "enforcement",
		func(d *sidecar.Directive) { d.Enforcement = "deny" },
		func(d sidecar.Directive) bool { return d.Enforcement == "deny" })
}

// TestPreserve_PerDirectivePathsRestored — ADR-070's PER-ENTRY glob override,
// which is a different mechanism from the artifact-level field of the same
// name. The artifact level is asserted UNTOUCHED here: the two share a name and
// nothing else, and a restore that wrote one from the other would still pass a
// test that only looked at the entry.
func TestPreserve_PerDirectivePathsRestored(t *testing.T) {
	want := []string{"commands/**/*.md", "tools/edikt/**/*.go"}
	p3JoiningRoundTrip(t, "paths",
		func(d *sidecar.Directive) { d.Paths = append([]string(nil), want...) },
		func(d sidecar.Directive) bool { return sameStrings(d.Paths, want) })

	// The artifact level was never pinned in that fixture, so it must still be
	// absent — a per-entry restore that leaked upward would widen the approved
	// scope of the whole sidecar without any approval.
	pinned := anchored(p3Text)
	pinned.Paths = append([]string(nil), want...)
	path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})
	if _, err := PreservePinned("ADR-901", &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}, nil, path); err != nil {
		t.Fatal(err)
	}
	after := tblLoad(t, path)
	if len(after.Paths) != 0 {
		t.Errorf("artifact-level paths = %v after restoring a PER-ENTRY override; the entry-level "+
			"mechanism widened the artifact's approved scope", after.Paths)
	}
	if !sameStrings(after.Directives[0].Paths, want) {
		t.Errorf("the per-entry override did not reach disk: %v", after.Directives[0].Paths)
	}
}

// TestPreserve_ActorScopeRestored — ADR-065's per-directive opt-in, 18 live
// values across ADR-027 and INV-010. POSITIVE ONLY, deliberately: the negative
// half is AC-3.4's, because a positive bool test is exactly what an
// every-bool-is-a-pin implementation passes.
func TestPreserve_ActorScopeRestored(t *testing.T) {
	p3JoiningRoundTrip(t, "actor_scope",
		func(d *sidecar.Directive) { d.ActorScope = true },
		func(d sidecar.Directive) bool { return d.ActorScope })
}

// ---------------------------------------------------------------------------
// AC-3.4 — the bool NEGATIVE control (SR-003, SR-010)
// ---------------------------------------------------------------------------

// TestActorScope_FalseIsNotAPin is the control AC-3.1's positive test cannot
// supply.
//
// SR-003 names the failure exactly: a presence signal implemented as
// every-bool-is-a-pin passes TestPreserve_ActorScopeRestored while reporting a
// phantom restore on every artifact but the handful that set the field. On a
// hundred-artifact corpus that is a restore count nobody can use, and — because
// DirectivePins counts ENTRIES, not fields (finding 38) — nothing else in the
// result distinguishes it.
//
// The assertion is FALSE before and ABSENT after yields no restore, no record,
// and a file that is not rewritten. It is TWO-SIDED: the same fixture with
// `true` must restore, or "nothing happened" would be satisfied by a package
// that does nothing at all (INV-013).
func TestActorScope_FalseIsNotAPin(t *testing.T) {
	// D319's premise, executed rather than asserted: `false` and `absent`
	// round-trip to the same bytes, which is WHY true has to be the pin.
	roundTrip := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{func() sidecar.Directive {
		d := anchored(p3Text)
		d.ActorScope = false
		return d
	}()}})
	if body, err := os.ReadFile(roundTrip); err != nil {
		t.Fatal(err)
	} else if strings.Contains(string(body), "actor_scope") {
		t.Fatalf("a false actor_scope was written to disk as a key; D319's ruling rests on `omitempty` "+
			"dropping it, and if it no longer does, the bare-bool decision needs revisiting:\n%s", body)
	}

	newFixture := func(t *testing.T, actorScope bool) (*sidecar.Sidecar, string, []byte) {
		t.Helper()
		pinned := anchored(p3Text)
		pinned.ActorScope = actorScope
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}, path, body
	}

	t.Run("false before, absent after — nothing restored, nothing written", func(t *testing.T) {
		before, path, body := newFixture(t, false)
		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if res.DirectivePins != 0 {
			t.Errorf("DirectivePins = %d for an entry whose only bool is FALSE; a phantom restore is "+
				"reported on every artifact that never set the field", res.DirectivePins)
		}
		if res.SidecarRewritten {
			t.Error("the sidecar was REWRITTEN for a field nobody pinned — every artifact in the corpus " +
				"would be rewritten on every re-extraction")
		}
		if len(res.Unrestorable) != 0 {
			t.Errorf("a false bool produced %d unrestorable record(s): %+v", len(res.Unrestorable), res.Unrestorable)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(after) != string(body) {
			t.Errorf("the file on disk changed for a false bool:\n--- before\n%s\n--- after\n%s", body, after)
		}
	})

	// THE OTHER SIDE. Without it, "nothing happened" is satisfied by a package
	// that lost the ability to restore anything at all.
	t.Run("true before, absent after — restored", func(t *testing.T) {
		before, path, _ := newFixture(t, true)
		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if res.DirectivePins != 1 || !res.SidecarRewritten {
			t.Fatalf("a TRUE actor_scope was not restored (%+v) — the negative control above measured "+
				"a package that restores nothing", res)
		}
		if !tblLoad(t, path).Directives[0].ActorScope {
			t.Error("actor_scope did not reach disk")
		}
	})
}

// TestActorScope_AbsentWritesNoRecord is the REPORTING half of the same
// control: a false bool must not be NAMED as a lost pin either, in either
// report that can name one.
//
// Each half carries a co-pinned `verify` whose record MUST be present, so the
// absence of an actor_scope record is the field's silence rather than a report
// that never ran (INV-013).
func TestActorScope_AbsentWritesNoRecord(t *testing.T) {
	const control = "bash test/fixtures/behavioral/ADR-901/positive.sh"

	t.Run("the reinstated report", func(t *testing.T) {
		pinned := anchored(p3Text)
		pinned.Verify = control
		pinned.ActorScope = false

		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})
		missing, err := MissingPins("ADR-901", &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if len(p3Named(missing, "verify")) == 0 {
			t.Fatalf("the control is not named — the report did not run; it named %v",
				tblSorted(reportFieldSet(missing)))
		}
		if hits := p3Named(missing, "actor_scope"); len(hits) != 0 {
			t.Errorf("a FALSE actor_scope is reported as a missing pin: %+v. Every artifact that never "+
				"set the field would carry this record", hits)
		}
	})

	t.Run("the unrestorable report on a dropped entry", func(t *testing.T) {
		pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-901)")
		pinned.Verify = control
		pinned.ActorScope = false

		path := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-901)")},
		})
		res, err := PreservePinned("ADR-901", &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if len(p3Named(res.Unrestorable, "verify")) == 0 {
			t.Fatalf("the control is not named unrestorable — the report did not run; it named %v",
				tblSorted(reportFieldSet(res.Unrestorable)))
		}
		if hits := p3Named(res.Unrestorable, "actor_scope"); len(hits) != 0 {
			t.Errorf("a FALSE actor_scope is reported as an unrestorable pin: %+v", hits)
		}
	})
}

// ---------------------------------------------------------------------------
// AC-3.5 — the two `paths` mechanisms are distinguishable (SR-011 clause ii)
// ---------------------------------------------------------------------------

// TestPinnedField_DistinguishesPathsMechanisms.
//
// From phase 3 onward the name `paths` is emitted by TWO mechanisms — the
// artifact-level approved scope and ADR-070's per-directive override — and
// SR-011 forbids telling them apart by an empty DirectiveText. On the
// load-error report that discriminator does not even exist: nothing was
// matched, so BOTH arrive with no directive text at all.
//
// Both halves assert on Scope alone and never read DirectiveText to decide
// which is which.
func TestPinnedField_DistinguishesPathsMechanisms(t *testing.T) {
	assertPair := func(t *testing.T, hits []PinnedField, where string) {
		t.Helper()
		if len(hits) != 2 {
			t.Fatalf("%s named `paths` %d time(s); want 2 — one per mechanism: %+v", where, len(hits), hits)
		}
		scopes := map[PinScope]bool{}
		for _, h := range hits {
			if h.Scope == "" {
				t.Errorf("%s: a `paths` record carries NO scope (%+v). A discriminator stamped at some "+
					"emission sites and not others is not a discriminator", where, h)
			}
			scopes[h.Scope] = true
		}
		if !scopes[ScopeArtifact] || !scopes[ScopeEntry] {
			t.Errorf("%s: the two `paths` records carry scopes %v; want both %q and %q, or an operator "+
				"cannot tell which mechanism lost the value", where, scopes, ScopeArtifact, ScopeEntry)
		}
	}

	t.Run("the load-error report, where DirectiveText cannot discriminate", func(t *testing.T) {
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})
		res, err := PreservePinned("ADR-901", nil, errUnparseable(), path)
		if err != nil {
			t.Fatal(err)
		}
		if !res.LoadFailed {
			t.Fatal("LoadFailed is false — the load error was answered as a clean bootstrap")
		}
		hits := p3Named(res.Unrestorable, "paths")
		assertPair(t, hits, "the at-risk report")
		for _, h := range hits {
			if h.DirectiveText != "" {
				t.Errorf("a load-error record carries a DirectiveText (%q); nothing was matched, so the "+
					"field is meaningless here and cannot be the thing that separates the two", h.DirectiveText)
			}
		}
	})

	t.Run("one live report carrying both losses", func(t *testing.T) {
		entryGlobs := []string{"tools/edikt/**/*.go"}
		artifactGlobs := []string{"commands/**/*.md", "docs/**/*.md"}

		pinned := anchored(p3Text)
		pinned.Paths = append([]string(nil), entryGlobs...)
		before := &sidecar.Sidecar{
			Directives: []sidecar.Directive{pinned},
			Paths:      artifactGlobs,
		}
		// The regenerated sidecar carries neither — what the extractor writes.
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})

		missing, err := MissingPins("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		hits := p3Named(missing, "paths")
		assertPair(t, hits, "the reinstated report")

		byScope := map[PinScope]PinnedField{}
		for _, h := range hits {
			byScope[h.Scope] = h
		}
		if got := byScope[ScopeEntry].Value; got != strings.Join(entryGlobs, ", ") {
			t.Errorf("the ENTRY-scoped record carries value %q; want the per-directive override %v", got, entryGlobs)
		}
		if got := byScope[ScopeArtifact].Value; got != strings.Join(artifactGlobs, ", ") {
			t.Errorf("the ARTIFACT-scoped record carries value %q; want the approved scope %v", got, artifactGlobs)
		}
	})

	// Every record in every report must carry a scope, not just the `paths`
	// ones — a discriminator that appears only where it was needed for a test
	// is one the next field to collide will not have.
	t.Run("every emitted record carries a scope", func(t *testing.T) {
		pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-901)")
		pinned.Verify = "bash check-cosign.sh"
		pinned.HumanApprovedAt = "2026-08-04T00:00:00Z"
		before := &sidecar.Sidecar{
			Directives:    []sidecar.Directive{pinned},
			Paths:         []string{"tools/**/*.go"},
			PathsApproval: &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs([]string{"other/**"})},
		}
		path := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-901)")},
		})
		missing, err := MissingPins("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if len(missing) == 0 {
			t.Fatal("the combined report is empty — nothing was measured (INV-013)")
		}
		for _, m := range missing {
			if m.Scope != ScopeArtifact && m.Scope != ScopeEntry {
				t.Errorf("record %+v carries scope %q, which is neither mechanism", m, m.Scope)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// AC-3.6 — the write NEVER CLEARS on an absent pin (SR-006)
// ---------------------------------------------------------------------------

// p3DestinationValues gives every entryPinTable row a legal value to place in
// the DESTINATION sidecar. It is keyed by row name and asserted COMPLETE below,
// so a new row cannot slip past this test by having no value to test with.
//
// Every value is Validate-legal (`enforcement` is a closed enum, an entry
// `paths` may not be empty), because the property is asserted end-to-end
// through PreservePinned rather than against applyPin in isolation: a write
// that clears is only interesting if the cleared file is the one that gets
// written.
var p3DestinationValues = map[string]func(*sidecar.Directive){
	"verify":                func(d *sidecar.Directive) { d.Verify = "bash do-not-blank-me.sh" },
	"verify_kind":           func(d *sidecar.Directive) { d.VerifyKind = "behavioral" },
	"human_approved_at":     func(d *sidecar.Directive) { d.HumanApprovedAt = "2026-08-09T00:00:00Z" },
	"positive_fixture_path": func(d *sidecar.Directive) { d.PositiveFixturePath = "test/fixtures/behavioral/ADR-901/positive.sh" },
	"negative_fixture_path": func(d *sidecar.Directive) { d.NegativeFixturePath = "test/fixtures/behavioral/ADR-901/negative.sh" },
	"enforcement":           func(d *sidecar.Directive) { d.Enforcement = "advisory" },
	"paths":                 func(d *sidecar.Directive) { d.Paths = []string{"docs/**/*.md"} },
	"actor_scope":           func(d *sidecar.Directive) { d.ActorScope = true },
}

// TestPinWrite_NeverClearsOnAbsentPin asserts the property that lived only in
// applyPin's comment: a field ABSENT from the pin is skipped, not blanked.
//
// The shape it excludes is readable-and-unguarded, one level down from the
// defect this whole spec exists to close. A write function that blanks its
// destination on an absent pin passes AC-2.1 (the added row still reaches every
// site), AC-2.2 (the derivation is unchanged), AC-3.1 (a PINNED field still
// restores) and AC-3.3 (nothing empty is written — it writes a zero value) —
// while destroying every value the regenerated sidecar legitimately carried.
//
// It is driven from the TABLE, one subtest per row, so a row added later is
// covered without editing this test.
func TestPinWrite_NeverClearsOnAbsentPin(t *testing.T) {
	if len(entryPinTable) == 0 {
		t.Fatal("entryPinTable is empty — this test would sweep nothing and report clean (INV-013)")
	}
	for _, f := range entryPinTable {
		if _, ok := p3DestinationValues[f.name]; !ok {
			t.Errorf("entryPinTable has a row for %q with no destination value in p3DestinationValues; "+
				"the never-clear property is UNTESTED for it — add one", f.name)
		}
	}
	// The reverse direction is checked against the STRUCT, not against the
	// table. A key here that is not a yaml key on Directive is a typo the
	// forward check cannot see. It deliberately does NOT require the key to be
	// a current row: a value left behind for a row that was removed is dead
	// weight, not a defect, and asserting otherwise would make this test go red
	// on the row deletion that is the recorded mutation kill for
	// TestAtRiskSetDerivedFromStruct — destroying that kill's isolation while
	// measuring nothing this test is about.
	directiveKeys := yamlKeys(reflect.TypeOf(sidecar.Directive{}))
	for name := range p3DestinationValues {
		if !directiveKeys[name] {
			t.Errorf("p3DestinationValues names %q, which is not a yaml key on Directive — the map has "+
				"rotted away from the struct it is meant to describe", name)
		}
	}

	for _, f := range entryPinTable {
		f := f
		set, ok := p3DestinationValues[f.name]
		if !ok {
			continue
		}
		t.Run(f.name, func(t *testing.T) {
			// The CONTROL is a different row, pinned in `before` and absent
			// from the destination, so the restore certainly runs and writes.
			control := entryPinTable[0]
			if control.name == f.name {
				control = entryPinTable[1]
			}
			pinned := anchored(p3Text)
			p3DestinationValues[control.name](&pinned)
			before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}

			// The DESTINATION carries a value for the field under test and
			// NOTHING for the control.
			dest := anchored(p3Text)
			set(&dest)
			want := f.read(&dest)
			if !want.present {
				t.Fatalf("the destination value for %q reads back as ABSENT, so this subtest could not "+
					"tell a skip from a blank", f.name)
			}
			path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{dest}})

			res, err := PreservePinned("ADR-901", before, nil, path)
			if err != nil {
				t.Fatalf("the restore failed outright: %v", err)
			}
			if !res.SidecarRewritten {
				t.Fatalf("the sidecar was not rewritten, so nothing was written over %q and this "+
					"subtest measured nothing (INV-013): %+v", f.name, res)
			}
			d := tblLoad(t, path).Directives[0]
			if got := control.read(&d); !got.present {
				t.Fatalf("the control pin %q did not land — the write path did not run", control.name)
			}
			if got := f.read(&d); !got.equal(want) {
				t.Errorf("%q was BLANKED by a restore that carried no pin for it: on disk %q, want %q. "+
					"A field absent from the pin is skipped, not cleared (SR-006) — the value in the "+
					"destination came from somewhere this package does not own",
					f.name, got.String(), want.String())
			}
		})
	}
}

// ---------------------------------------------------------------------------
// AC-3.3 — a list restore never writes a non-nil empty slice
// ---------------------------------------------------------------------------

// TestRestore_NeverEmptyNonNilSlice.
//
// The blast radius is what makes this worth its own criterion. `Validate`
// rejects an entry whose `paths` is present-but-empty (sidecar.go:729 for
// directives, :776 for prohibitions), and PreservePinned validates the WHOLE
// artifact in memory before writing — so ONE entry restored to []string{}
// aborts the entire artifact's restore, not just that field.
func TestRestore_NeverEmptyNonNilSlice(t *testing.T) {
	// The consequence, asserted rather than assumed: this is why the property
	// matters at all.
	t.Run("an empty entry paths list fails validation", func(t *testing.T) {
		d := anchored(p3Text)
		d.Paths = []string{}
		sc := &sidecar.Sidecar{
			SchemaVersion: 2,
			Topic:         "testing",
			Path:          "docs/architecture/decisions/ADR-901.md",
			Directives:    []sidecar.Directive{d},
		}
		if err := sc.Validate(); err == nil {
			t.Fatal("a directive with `paths: []` validates — the abort this criterion is about cannot " +
				"happen, so the rest of this test measures a hazard that no longer exists")
		}
	})

	// The write column of every row, driven with an ABSENT-shaped value.
	t.Run("no write column produces a non-nil empty slice", func(t *testing.T) {
		for _, f := range entryPinTable {
			for _, v := range []pinValue{{present: true}, {present: true, list: []string{}}} {
				d := anchored(p3Text)
				f.write(&d, v)
				if field, ok := p3EmptyNonNilSlice(d); ok {
					t.Errorf("%q's write column produced a non-nil EMPTY slice on %s from %+v — one such "+
						"entry aborts the whole artifact at Validate", f.name, field, v)
				}
			}
		}
	})

	// End to end: an empty list in `before` is not a pin at all, so it is never
	// captured and never written.
	t.Run("an empty list in before is not captured", func(t *testing.T) {
		pinned := anchored(p3Text)
		pinned.Verify = "bash control.sh"
		pinned.Paths = []string{}

		before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(p3Text)}})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatalf("the restore aborted — an empty list reached the destination and failed validation: %v", err)
		}
		if !res.SidecarRewritten {
			t.Fatal("the control pin did not cause a write, so nothing was observed about the empty list")
		}
		d := tblLoad(t, path).Directives[0]
		if d.Verify != "bash control.sh" {
			t.Fatalf("the control pin did not land: %+v", d)
		}
		if d.Paths != nil {
			t.Errorf("paths on disk = %#v; an empty list is ABSENT, not a pin, and must not be written", d.Paths)
		}
		if hits := p3Named(res.Unrestorable, "paths"); len(hits) != 0 {
			t.Errorf("an empty list was reported as a lost pin: %+v", hits)
		}
	})
}

// p3EmptyNonNilSlice reports the first field of d that is a non-nil, zero-length
// slice. Reflection rather than an enumerated field list: a new list-shaped
// field added to Directive is covered without editing this helper.
func p3EmptyNonNilSlice(d sidecar.Directive) (string, bool) {
	rv := reflect.ValueOf(d)
	rt := rv.Type()
	for i := 0; i < rt.NumField(); i++ {
		fv := rv.Field(i)
		if fv.Kind() == reflect.Slice && !fv.IsNil() && fv.Len() == 0 {
			return rt.Field(i).Name, true
		}
	}
	return "", false
}

// ---------------------------------------------------------------------------
// AC-3.8 — SSP-009's third state must name a spec and a phase
// ---------------------------------------------------------------------------

// TestNotYetPinnable_ReasonNamesSpecAndPhase.
//
// SSP-009 requires the derivation's third state — a field that is neither
// pinned nor extractor-owned — to name the SR and the phase that closes it.
// TestAtRiskSetDerivedFromStruct only asks whether a key is PRESENT in
// notYetPinnableEntryFields, so an entry mapped to "" classified a field with
// no reason, no SR and no phase and passed. A deferral bucket that accepts an
// empty reason is a denylist that explains nothing.
//
// The bucket is EMPTY as of phase 3, so iterating it would assert nothing
// (INV-013). The rule is therefore driven both ways: known-bad reasons must be
// REJECTED, a well-formed one must be ACCEPTED, and only then is the production
// map — however large it is at the time — held to the same rule.
func TestNotYetPinnable_ReasonNamesSpecAndPhase(t *testing.T) {
	t.Run("a reason that does not name a spec and a phase is rejected", func(t *testing.T) {
		for _, bad := range []struct{ reason, why string }{
			{"", "the empty reason SSP-009's clause is about"},
			{"   ", "whitespace is not a reason"},
			{"human-authored; nobody has got to it yet", "names neither a spec nor a phase"},
			{"deferred by SPEC-023", "names a spec but no phase"},
			{"closed in phase 3", "names a phase but no spec"},
		} {
			if msg := deferralReasonDefect("some_field", bad.reason); msg == "" {
				t.Errorf("reason %q was ACCEPTED — %s", bad.reason, bad.why)
			}
		}
	})

	t.Run("a well-formed reason is accepted", func(t *testing.T) {
		good := "the PER-ENTRY override (ADR-070), distinct from the artifact-level field of the same name — SPEC-023 phase 3"
		if msg := deferralReasonDefect("paths", good); msg != "" {
			t.Fatalf("a reason naming both a spec and a phase was rejected: %s. The rule above would "+
				"then reject everything and prove nothing", msg)
		}
	})

	t.Run("every deferred field in the production bucket obeys the rule", func(t *testing.T) {
		for field, reason := range notYetPinnableEntryFields {
			if msg := deferralReasonDefect(field, reason); msg != "" {
				t.Error(msg)
			}
		}
		// Recorded, not asserted: phase 3 emptied the bucket, so the loop above
		// swept nothing. The two subtests before it are what carry this test.
		if len(notYetPinnableEntryFields) == 0 {
			t.Log("the deferral bucket is empty (SPEC-023 phase 3 closed all three entries); the rule " +
				"is held by the positive and negative controls above, not by this sweep")
		}
	})
}

// errUnparseable is the prior-sidecar load failure used by the at-risk arm.
func errUnparseable() error {
	return &p3LoadErr{}
}

type p3LoadErr struct{}

func (*p3LoadErr) Error() string { return "yaml: line 4: mapping values are not allowed in this context" }
