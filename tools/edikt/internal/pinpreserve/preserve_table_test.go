package pinpreserve

// preserve_table_test.go — PLAN-SPEC-023 phase 2 (AC-2.1 … AC-2.4).
//
// Four things are asserted here, and they are deliberately different KINDS of
// assertion:
//
//	AC-2.1  the tables are the single source — adding a row reaches every
//	        derived site, checked by adding one and looking at all six
//	AC-2.2  the guard's expectations come from the AUTHORITATIVE struct
//	        (internal/sidecar's Directive/Prohibition yaml tags), never from
//	        the list it is checking (INV-014)
//	AC-2.3  the nine known gaps are TWO-SIDED: each executes, observes the
//	        loss, and an unexpected PASS fails the suite
//	AC-2.4  the prohibition arm was asserted STILL UNREACHED — a NEGATIVE
//	        control, because prohibition traversal was phase 4's subject and
//	        needed its own red-first evidence (SSP-010). RETIRED by AC-4.6
//	        when phase 4 landed that traversal; see the test at the bottom of
//	        this file for why it is a skip rather than a deletion.

import (
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

func tblAfter(t *testing.T, sc *sidecar.Sidecar) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "ADR-901.edikt.yaml")
	writeSidecar(t, path, sc)
	return path
}

func tblLoad(t *testing.T, path string) *sidecar.Sidecar {
	t.Helper()
	sc, err := sidecar.Load(path)
	if err != nil {
		t.Fatalf("reloading the restored sidecar: %v", err)
	}
	return sc
}

func tblProhibition(text string) sidecar.Prohibition {
	return sidecar.Prohibition{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 9, LineEnd: 9, Quote: "A prohibition lives here."}},
	}
}

// tblNamed indexes a report by field name, carrying the value so a test can
// tell "named" from "named with nothing a human could re-approve from".
func tblNamed(fs []PinnedField) map[string]PinnedField {
	out := make(map[string]PinnedField, len(fs))
	for _, f := range fs {
		if _, seen := out[f.Field]; !seen {
			out[f.Field] = f
		}
	}
	return out
}

func tblSorted(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// AC-2.1 — one table, and every site derives from it
// ---------------------------------------------------------------------------

// TestPinTable_AddedRowReachesEverySite is the operative falsifier for the
// whole scheme, and it is written as an EXECUTION rather than an inspection:
// "the ten sites collapsed to one" is not checkable by reading, because a site
// that quietly kept its own copy reads exactly like one that did not.
//
// So the test adds a row — two, one string-shaped and one list-shaped, because
// SR-006 requires the table to carry both — and then asks every derived site
// whether it knows about them. A site still holding a hand-maintained list
// cannot: the added field is absent from it by construction, and the
// corresponding half of this test goes red.
//
// The rows are TEST-ONLY. They write `intent` and `falsifying_observation`,
// two EXTRACTOR-OWNED fields production does not preserve, so nothing here
// changes what the package restores: production's set is `entryPinTable` as
// declared, and this test restores it on cleanup.
//
// The list-shaped row used to borrow per-directive `paths`, which was the other
// unpreserved field at phase 2. SPEC-023 phase 3 made `paths` a production row,
// and two rows writing the same destination cannot both be observed: whichever
// runs first satisfies the second's read, so applyPin reports no change for it
// and site 6 goes red on a table that is working correctly. The row therefore
// moved to a field production still does not own, and keeps its LIST shape —
// which is the half SR-006 requires — by encoding the list into that field.
func TestPinTable_AddedRowReachesEverySite(t *testing.T) {
	original := entryPinTable
	t.Cleanup(func() { entryPinTable = original })

	entryPinTable = append(append([]entryPin(nil), original...),
		entryPin{
			name:  "test_only_text_pin",
			read:  func(d *sidecar.Directive) pinValue { return textPin(d.Intent) },
			write: func(d *sidecar.Directive, v pinValue) { d.Intent = v.text },
		},
		entryPin{
			name: "test_only_list_pin",
			read: func(d *sidecar.Directive) pinValue {
				if d.FalsifyingObservation == "" {
					return pinValue{}
				}
				return listPin(strings.Split(d.FalsifyingObservation, "|"))
			},
			write: func(d *sidecar.Directive, v pinValue) { d.FalsifyingObservation = strings.Join(v.list, "|") },
		},
	)
	added := []string{"test_only_text_pin", "test_only_list_pin"}

	const (
		text       = "The loader MUST reject unknown keys. (ref: ADR-901)"
		wantIntent = "the loader refuses keys it does not declare"
	)
	wantList := []string{"a key the loader accepted", "a key it silently dropped"}

	pinEntry := func(d *sidecar.Directive) {
		d.Intent = wantIntent
		d.FalsifyingObservation = strings.Join(wantList, "|")
	}

	// SITE 1 — the at-risk report (pinnedFieldsAtRisk, via earlyResult).
	t.Run("at-risk report names the added rows", func(t *testing.T) {
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(text)}})
		res, err := PreservePinned("ADR-901", nil, errors.New("yaml: line 4: mapping values are not allowed"), path)
		if err != nil {
			t.Fatalf("a load-error report must not itself be an error: %v", err)
		}
		named := tblNamed(res.Unrestorable)
		for _, name := range added {
			if _, ok := named[name]; !ok {
				t.Errorf("%q is not named at-risk on a load failure — the at-risk report is not derived "+
					"from the table; it named %v", name, tblSorted(reportFieldSet(res.Unrestorable)))
			}
		}
	})

	// SITES 2, 3 and 4 — capture (pinOf), the emptiness test (pin.empty) and
	// the write-back (applyPin). Pinned with NOTHING ELSE on the entry, which
	// is what forces all three: if pinOf skipped the row the entry would be
	// empty(), hit `continue`, and never reach applyPin at all.
	t.Run("captured, not skipped as empty, and written back", func(t *testing.T) {
		pinned := anchored(text)
		pinEntry(&pinned)
		before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(text)}})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if res.DirectivePins != 1 {
			t.Fatalf("DirectivePins = %d; want 1 — an entry whose ONLY pins are the added rows was "+
				"treated as unpinned, so capture or the emptiness test is not derived from the table", res.DirectivePins)
		}
		d := tblLoad(t, path).Directives[0]
		if d.Intent != wantIntent {
			t.Errorf("intent on disk = %q; want %q — the added string row is not written back", d.Intent, wantIntent)
		}
		if got := strings.Split(d.FalsifyingObservation, "|"); !sameStrings(got, wantList) {
			t.Errorf("the list row's destination on disk = %v; want %v — the added LIST-shaped row is not written back (SR-006)", got, wantList)
		}
	})

	// SITE 5 — the unrestorable report, on an entry that was dropped.
	t.Run("unrestorable report names the added rows with their values", func(t *testing.T) {
		pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-901)")
		pinEntry(&pinned)
		before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
		path := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-901)")},
		})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		named := tblNamed(res.Unrestorable)
		for _, name := range added {
			hit, ok := named[name]
			if !ok {
				t.Errorf("%q was dropped without being named — the unrestorable report is not derived from the table", name)
				continue
			}
			if hit.Value == "" {
				t.Errorf("%q is named with an empty value; a name with no value cannot be re-approved", name)
			}
		}
	})

	// SITE 6 — the reinstated report, which only MissingPins returns
	// (PreservePinned discards it at computePins' call site — SPEC-023
	// finding 37).
	t.Run("reinstated report names the added rows with their values", func(t *testing.T) {
		pinned := anchored(text)
		pinEntry(&pinned)
		before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(text)}})

		missing, err := MissingPins("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		named := tblNamed(missing)
		for _, name := range added {
			hit, ok := named[name]
			if !ok {
				t.Errorf("%q is not named as missing from the live sidecar — the reinstated report is not "+
					"derived from the table", name)
				continue
			}
			if hit.Value == "" {
				t.Errorf("%q is reported reinstated with an empty value", name)
			}
		}
	})
}

// ---------------------------------------------------------------------------
// AC-2.2 — the guard derives from the struct, never from the list under test
// ---------------------------------------------------------------------------

// extractorOwnedEntryFields are the yaml keys on an entry that the EXTRACTOR
// produces. They are regenerated from the artifact's prose every run, so there
// is nothing human-authored to carry forward and their absence from the
// pinnable set is a decision, not an omission.
var extractorOwnedEntryFields = map[string]string{
	"text":                   "the rule itself — re-worded by the v4 contract every run, by design",
	"source_excerpt":         "v1 single-anchor provenance, produced by the extractor",
	"source_excerpts":        "v2 multi-anchor provenance, produced by the extractor",
	"needs_review":           "a flag the extractor raises for what it could not adjudicate from one artifact",
	"intent":                 "the extractor's one-line semantic claim, regenerated with the text it restates",
	"falsifying_observation": "extractor-produced, and meaningless once detached from the text it describes",
	"derived_from":           "prohibition-only provenance the extractor writes",
}

// notYetPinnableEntryFields is the DEFERRAL BUCKET: human-authored entry
// fields measured to be outside the mechanism, each with live values in the
// corpus, each with a reason naming the spec and the phase that closes it.
// They are listed rather than silently folded into either other category
// because the whole point of deriving from the struct is that a field lands in
// exactly one bucket and no branch is silent.
//
// IT IS EMPTY AS OF SPEC-023 PHASE 3. `enforcement`, per-entry `paths` and
// `actor_scope` — the three it held — are rows in `entryPinTable` now, so the
// bucket has nothing left to defer.
//
// An entry added here must carry a reason that NAMES A SPEC AND A PHASE.
// SSP-009 requires the derivation's third state to say which SR and which
// phase closes it, and the classification check below only asks whether a key
// is PRESENT in this map — so an entry mapped to "" would classify a field
// with no reason, no SR and no phase and pass. A deferral bucket that accepts
// an empty reason is a denylist that explains nothing. The rule is enforced by
// deferralReasonDefect, driven both ways by
// TestNotYetPinnable_ReasonNamesSpecAndPhase (AC-3.8).
var notYetPinnableEntryFields = map[string]string{}

// deferralReasonSpecRe and deferralReasonPhaseRe are the two things SSP-009
// requires a deferral reason to name.
var (
	deferralReasonSpecRe  = regexp.MustCompile(`\b(SPEC|SR|SSP)-\d+\b`)
	deferralReasonPhaseRe = regexp.MustCompile(`(?i)\bphase\s+\d+\b`)
)

// deferralReasonDefect returns "" when a deferral reason is adequate, and the
// failure text otherwise. It is a function rather than an inline check so that
// TestNotYetPinnable_ReasonNamesSpecAndPhase can drive KNOWN-BAD reasons
// through the same rule the production map is judged by: with the map empty,
// iterating it alone would assert nothing at all (INV-013).
func deferralReasonDefect(field, reason string) string {
	switch {
	case strings.TrimSpace(reason) == "":
		return fmt.Sprintf("%q is deferred with an EMPTY reason. A deferral bucket that accepts an empty "+
			"reason is a denylist that explains nothing — SSP-009 requires the third state to name the "+
			"SR and the phase that closes it", field)
	case !deferralReasonSpecRe.MatchString(reason):
		return fmt.Sprintf("%q is deferred with a reason naming no spec requirement (%q). Name the SR or "+
			"SPEC that owns the deferral, or nobody can find out whether it is still true", field, reason)
	case !deferralReasonPhaseRe.MatchString(reason):
		return fmt.Sprintf("%q is deferred with a reason naming no phase (%q). A deferral with no phase "+
			"behind it is indistinguishable from a field nobody decided about", field, reason)
	}
	return ""
}

// artifactPinnableFields is the artifact-level half of the at-risk set, and it
// is stated LITERALLY rather than derived.
//
// SR-011: these are `Sidecar` fields, not entry fields, so a derivation over
// the entry struct would drop them from the report — silently regressing
// coverage that was bought deliberately. Stating them here is not the defect
// INV-014 forbids: the expectation is independent of the tables under test, so
// deleting a row from artifactPinTable turns this red. Each is asserted to be
// a real `Sidecar` yaml key below, so a typo cannot pass as a field.
//
// `manual_directives` and `suppressed_directives` joined at SPEC-023 phase 5.
// They are listed here for the same reason the other two are — this expectation
// is independent of artifactPinTable, so deleting either row turns the
// set-equality sub-test red — and NOT for the reason a derivation would give:
// there is no rule over `Sidecar`'s yaml keys that selects exactly these four.
// Most of that struct is extractor-owned or compile-derived.
var artifactPinnableFields = []string{"paths", "paths_approval", "manual_directives", "suppressed_directives"}

// yamlKeys returns the yaml key of every field on a struct type.
func yamlKeys(rt reflect.Type) map[string]bool {
	out := map[string]bool{}
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("yaml")
		if tag == "" || tag == "-" {
			continue
		}
		name := strings.Split(tag, ",")[0]
		if name != "" {
			out[name] = true
		}
	}
	return out
}

func setDiff(a, b map[string]bool) []string {
	var out []string
	for k := range a {
		if !b[k] {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

// TestAtRiskSetDerivedFromStruct replaces the tautology this package shipped
// since ADR-070.
//
// The old guard iterated `pinnedFieldNamesAtRisk` to build the assertion set it
// then checked against a report GENERATED by iterating the same slice. A field
// missing from the slice was missing from both sides and was never asked
// about; the assertion set shrank in lockstep with the defect (INV-014).
// SPEC-023 finding 40 established this by execution, not by reading: deleting
// `human_approved_at` from the list left the whole package suite green.
//
// The expectation here comes from `internal/sidecar`'s struct tags — the
// authoritative declaration of what an entry can carry — and is asserted
// against the RESOLVED report rather than against any list inside this
// package. Deleting a row from `entryPinTable` therefore turns this red.
func TestAtRiskSetDerivedFromStruct(t *testing.T) {
	directiveKeys := yamlKeys(reflect.TypeOf(sidecar.Directive{}))
	prohibitionKeys := yamlKeys(reflect.TypeOf(sidecar.Prohibition{}))
	if len(directiveKeys) == 0 || len(prohibitionKeys) == 0 {
		t.Fatal("reflected ZERO yaml keys off Directive/Prohibition — the derivation observed nothing, " +
			"so everything below would pass vacuously (INV-013)")
	}

	// SR-011: there is no "entry type". Directive and Prohibition are two
	// structs with independently declared fields, and a pinnable field added
	// to one and not the other would silently reopen the asymmetry SR-002
	// exists to close. Directive is authoritative; the other is asserted to
	// agree, with exactly one sanctioned difference.
	t.Run("Prohibition agrees with the authoritative Directive", func(t *testing.T) {
		if extra := setDiff(directiveKeys, prohibitionKeys); len(extra) > 0 {
			t.Errorf("Directive declares %v, which Prohibition does not — a pinnable field on one entry "+
				"kind only is the asymmetry SR-002 closes", extra)
		}
		got := setDiff(prohibitionKeys, directiveKeys)
		want := []string{"derived_from"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Prohibition declares %v beyond Directive; want exactly %v. A new prohibition-only "+
				"field must be classified before it can be assumed unpinnable", got, want)
		}
	})

	// Every declared key must land in exactly one bucket: pinned today,
	// extractor-owned, or measured-and-deferred. An unclassified key is a new
	// field nobody decided about — which is the failure SR-001's derivation
	// exists to make impossible to miss.
	union := map[string]bool{}
	for k := range directiveKeys {
		union[k] = true
	}
	for k := range prohibitionKeys {
		union[k] = true
	}

	pinnedNow := map[string]bool{}
	for _, f := range entryPinTable {
		pinnedNow[f.name] = true
	}

	t.Run("every declared entry key is classified exactly once", func(t *testing.T) {
		for _, key := range tblSorted(union) {
			n := 0
			if pinnedNow[key] {
				n++
			}
			if _, ok := extractorOwnedEntryFields[key]; ok {
				n++
			}
			if _, ok := notYetPinnableEntryFields[key]; ok {
				n++
			}
			switch {
			case n == 0:
				t.Errorf("yaml key %q is declared on an entry struct and classified nowhere — decide: "+
					"pinnable (add a row to entryPinTable), extractor-owned, or deferred with a reason", key)
			case n > 1:
				t.Errorf("yaml key %q is classified %d times; a field is pinned, extractor-owned, or "+
					"deferred — exactly one", key, n)
			}
		}
		for _, f := range entryPinTable {
			if !union[f.name] {
				t.Errorf("entryPinTable has a row for %q, which is not a yaml key on Directive or "+
					"Prohibition — the table is pinning a field that does not exist", f.name)
			}
		}
	})

	// The artifact-level half is a Sidecar field, not an entry field.
	sidecarKeys := yamlKeys(reflect.TypeOf(sidecar.Sidecar{}))
	t.Run("artifact-level names are real Sidecar keys", func(t *testing.T) {
		for _, name := range artifactPinnableFields {
			if !sidecarKeys[name] {
				t.Errorf("%q is expected in the at-risk report as an artifact-level pin, but it is not a "+
					"yaml key on Sidecar", name)
			}
		}
	})

	// THE RESOLVED OUTCOME. Not `pinnedFieldNamesAtRisk()` read directly —
	// INV-014: a guard asserts the outcome it protects, it does not reproduce
	// the logic that produces it. The report is what a caller sees.
	t.Run("the load-error report names exactly the derived set", func(t *testing.T) {
		want := map[string]bool{}
		for k := range directiveKeys {
			if pinnedNow[k] {
				want[k] = true
			}
		}
		for _, name := range artifactPinnableFields {
			want[name] = true
		}

		path := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-901)")},
		})
		res, err := PreservePinned("ADR-901", nil, errors.New("yaml: unknown field \"bogus\""), path)
		if err != nil {
			t.Fatalf("a load-error report must not itself be an error: %v", err)
		}
		if !res.LoadFailed {
			t.Fatal("LoadFailed is false — the load error was answered as a clean bootstrap, so the report " +
				"below is not the one under test")
		}
		got := map[string]bool{}
		for _, u := range res.Unrestorable {
			got[u.Field] = true
		}
		if missing := setDiff(want, got); len(missing) > 0 {
			t.Errorf("the at-risk report does not name %v. Every one of those is a pinnable field a load "+
				"failure puts at risk; a report that omits one under-states the loss", missing)
		}
		if extra := setDiff(got, want); len(extra) > 0 {
			t.Errorf("the at-risk report names %v, which the struct-derived set does not contain. Either "+
				"the field is pinnable and its classification is wrong, or the report is naming something "+
				"it cannot lose", extra)
		}
	})
}

// ---------------------------------------------------------------------------
// AC-2.3 — the nine known gaps, two-sided
// ---------------------------------------------------------------------------

// gapControlVerify is co-pinned on every gap fixture. It is a field that IS
// preserved, so its movement proves the machinery ran: without it, a gap cell
// observing "the field did not survive" cannot tell a real loss from a fixture
// that never reached the restore at all (INV-013).
const gapControlVerify = "bash test/fixtures/behavioral/ADR-901/positive.sh"

// gapObservation is what a cell SAW. It carries no verdict: the verdict is
// gapCellVerdict's, so TestExpectRed_UnexpectedPassFails can drive the same
// observation through the same rule and check the rule itself.
type gapObservation struct {
	// achieved is true when the outcome the cell is named for happened —
	// the value reached disk, or the field was named in the report.
	achieved bool
	// measured is true when the control pin moved, i.e. the machinery ran.
	measured bool
	detail   string
}

type gapCell struct {
	field   string
	outcome string
	// expectRed records that this (field x outcome) is a KNOWN GAP today.
	// Phase 3 flips these to false, one line each.
	expectRed bool
	observe   func(t *testing.T) gapObservation
}

// gapCellVerdict is the two-sided rule. It returns "" when the observation
// matches the record, and the failure text otherwise.
//
// The UNEXPECTED PASS branch is the half that makes this a control rather than
// a comment: a gap cell that stops executing looks exactly like a gap cell that
// was fixed unless the harness fails on the pass.
func gapCellVerdict(c gapCell, obs gapObservation) string {
	if !obs.measured {
		return fmt.Sprintf("UNMEASURED: %s x %s — the control pin did not move either, so nothing was "+
			"observed about %s. A loss reported by a fixture that never ran is not evidence (INV-013). %s",
			c.field, c.outcome, c.field, obs.detail)
	}
	if c.expectRed && obs.achieved {
		return fmt.Sprintf("UNEXPECTED PASS: %s x %s HOLDS on this tree, but the cell records it as a "+
			"known gap. Either the gap was closed — flip expectRed to false in the same change that "+
			"closed it — or the cell is measuring something other than what it names. %s",
			c.field, c.outcome, obs.detail)
	}
	if !c.expectRed && !obs.achieved {
		return fmt.Sprintf("REGRESSION: %s x %s no longer holds, and the cell records it as closed. %s",
			c.field, c.outcome, obs.detail)
	}
	return ""
}

// gapRestoreCell drives an exact-text match with the gap field and the control
// `verify` pinned on the same entry, and reports whether the gap field reached
// disk.
func gapRestoreCell(t *testing.T, pin func(*sidecar.Directive), onDisk func(sidecar.Directive) bool) gapObservation {
	t.Helper()
	const text = "The loader MUST reject unknown keys. (ref: ADR-901)"
	pinned := anchored(text)
	pinned.Verify = gapControlVerify
	pin(&pinned)

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(text)}})
	if _, err := PreservePinned("ADR-901", before, nil, path); err != nil {
		t.Fatal(err)
	}
	d := tblLoad(t, path).Directives[0]
	return gapObservation{
		achieved: onDisk(d),
		measured: d.Verify == gapControlVerify,
		detail:   fmt.Sprintf("on disk: verify=%q enforcement=%q paths=%v actor_scope=%v", d.Verify, d.Enforcement, d.Paths, d.ActorScope),
	}
}

// gapUnrestorableCell drops the entry the pin belonged to and reports whether
// the gap field is NAMED in Unrestorable.
func gapUnrestorableCell(t *testing.T, field string, pin func(*sidecar.Directive)) gapObservation {
	t.Helper()
	pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-901)")
	pinned.Verify = gapControlVerify
	pin(&pinned)

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := tblAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-901)")},
	})
	res, err := PreservePinned("ADR-901", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	named := tblNamed(res.Unrestorable)
	_, gap := named[field]
	_, control := named["verify"]
	return gapObservation{
		achieved: gap,
		measured: control,
		detail:   fmt.Sprintf("the unrestorable report named %v", tblSorted(reportFieldSet(res.Unrestorable))),
	}
}

// gapAtRiskCell drives the prior-sidecar load failure and reports whether the
// gap field is NAMED in the at-risk report.
func gapAtRiskCell(t *testing.T, field string) gapObservation {
	t.Helper()
	path := tblAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-901)")},
	})
	res, err := PreservePinned("ADR-901", nil, errors.New("yaml: line 4: mapping values are not allowed"), path)
	if err != nil {
		t.Fatalf("a load-error report must not itself be an error: %v", err)
	}
	named := tblNamed(res.Unrestorable)
	_, gap := named[field]
	_, control := named["verify"]
	return gapObservation{
		achieved: gap,
		measured: control && res.LoadFailed,
		detail:   fmt.Sprintf("the at-risk report named %v", tblSorted(reportFieldSet(res.Unrestorable))),
	}
}

func reportFieldSet(fs []PinnedField) map[string]bool {
	out := map[string]bool{}
	for _, f := range fs {
		out[f.Field] = true
	}
	return out
}

// gapCells — the nine cells: three human-authored entry fields crossed with
// the three observable outcomes.
//
// EVERY MARKER IS NOW `false`, and the cells are kept rather than deleted.
// Each one was a MEASURED LOSS with live values in the corpus behind it — two
// `deny` directives for `enforcement`, one override for per-entry `paths`,
// eighteen values for `actor_scope`. Phase 3 (AC-3.1) closed the eight that
// were still red, one line each; the ninth was already closed at phase 2's
// exit and is noted at its site. The cells stay because the rule stays
// two-sided in the other direction: a `false` cell that stops holding is a
// REGRESSION and fails, which is what keeps the three fields in the mechanism
// rather than merely having put them there once.
func gapCells() []gapCell {
	pinEnforcement := func(d *sidecar.Directive) { d.Enforcement = "deny" }
	pinPaths := func(d *sidecar.Directive) { d.Paths = []string{"tools/edikt/**/*.go"} }
	pinActorScope := func(d *sidecar.Directive) { d.ActorScope = true }

	return []gapCell{
		{field: "enforcement", outcome: "restored", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapRestoreCell(t, pinEnforcement, func(d sidecar.Directive) bool { return d.Enforcement == "deny" })
		}},
		{field: "enforcement", outcome: "unrestorable", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapUnrestorableCell(t, "enforcement", pinEnforcement)
		}},
		{field: "enforcement", outcome: "at-risk", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapAtRiskCell(t, "enforcement")
		}},

		{field: "paths (per-entry)", outcome: "restored", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapRestoreCell(t, pinPaths, func(d sidecar.Directive) bool { return len(d.Paths) > 0 })
		}},
		{field: "paths (per-entry)", outcome: "unrestorable", expectRed: false, observe: func(t *testing.T) gapObservation {
			// The DIRECTIVE-level pin. `paths` also names the artifact-level
			// field, which has no unrestorable arm at all — this fixture pins
			// no artifact-level scope, so a hit here could only be the entry.
			return gapUnrestorableCell(t, "paths", pinPaths)
		}},
		{field: "paths (per-entry)", outcome: "at-risk", expectRed: false, observe: func(t *testing.T) gapObservation {
			// The one cell that was NOT a gap at phase 2's exit: `paths` was
			// already named at-risk on a load failure, by the ARTIFACT-LEVEL
			// row alone. SR-011 predicted the collision ("`paths` names two
			// different mechanisms and nothing but an empty DirectiveText
			// tells them apart"), and phase 3 closed it — the report now
			// names `paths` TWICE, once per mechanism, told apart by
			// PinScope. This cell asks only whether the name appears, so
			// TestPinnedField_DistinguishesPathsMechanisms (AC-3.5) is what
			// asserts the two are separable; without it a green cell here
			// would be satisfied by either mechanism alone.
			return gapAtRiskCell(t, "paths")
		}},

		{field: "actor_scope", outcome: "restored", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapRestoreCell(t, pinActorScope, func(d sidecar.Directive) bool { return d.ActorScope })
		}},
		{field: "actor_scope", outcome: "unrestorable", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapUnrestorableCell(t, "actor_scope", pinActorScope)
		}},
		{field: "actor_scope", outcome: "at-risk", expectRed: false, observe: func(t *testing.T) gapObservation {
			return gapAtRiskCell(t, "actor_scope")
		}},
	}
}

// TestExpectRed_TwoSided executes all nine cells. Each one runs, observes, and
// is judged by gapCellVerdict — which fails on an unexpected PASS as readily as
// on a regression.
func TestExpectRed_TwoSided(t *testing.T) {
	cells := gapCells()
	if len(cells) != 9 {
		t.Fatalf("gapCells() returned %d cell(s); SPEC-023 measured NINE (three fields x three outcomes). "+
			"A cell that stopped executing is indistinguishable from one that was fixed", len(cells))
	}
	for _, c := range cells {
		c := c
		t.Run(fmt.Sprintf("%s_%s", strings.NewReplacer(" ", "_", "(", "", ")", "", "-", "_").Replace(c.field), c.outcome), func(t *testing.T) {
			if msg := gapCellVerdict(c, c.observe(t)); msg != "" {
				t.Error(msg)
			}
		})
	}
}

// TestExpectRed_UnexpectedPassFails proves the harness itself, which is the
// half AC-2.3 exists for: a two-sided rule nobody has seen reject anything is
// a one-sided rule with a comment.
//
// It drives a REAL observation of a field that IS preserved (`verify`) through
// the same rule, once recorded as a known gap and once recorded as closed, and
// requires the first to be rejected. The falsifier AC-2.3 names — "flip one
// expectRed cell's subject to a field that IS preserved and the suite goes
// red" — is exactly this, executed rather than asserted.
func TestExpectRed_UnexpectedPassFails(t *testing.T) {
	observe := func(t *testing.T) gapObservation {
		return gapRestoreCell(t,
			func(d *sidecar.Directive) { d.VerifyKind = "behavioral" },
			func(d sidecar.Directive) bool { return d.VerifyKind == "behavioral" })
	}
	obs := observe(t)
	if !obs.measured || !obs.achieved {
		t.Fatalf("the control observation did not hold: verify_kind IS preserved today, so this test's "+
			"subject must be an achieved outcome, or it proves nothing about the unexpected-pass branch. %+v", obs)
	}

	misrecorded := gapCell{field: "verify_kind", outcome: "restored", expectRed: true, observe: observe}
	msg := gapCellVerdict(misrecorded, obs)
	if msg == "" {
		t.Fatal("gapCellVerdict returned no failure for a PRESERVED field recorded as a known gap — the " +
			"expectRed harness reports green on an unexpected pass, so every gap cell in " +
			"TestExpectRed_TwoSided could have silently stopped measuring")
	}
	if !strings.Contains(msg, "UNEXPECTED PASS") {
		t.Errorf("the failure does not name the unexpected pass: %s", msg)
	}

	// The other side of the same rule: correctly recorded, the SAME observation
	// must pass. Without this the rule could fail everything and still satisfy
	// the check above.
	recorded := gapCell{field: "verify_kind", outcome: "restored", expectRed: false, observe: observe}
	if msg := gapCellVerdict(recorded, obs); msg != "" {
		t.Errorf("a correctly recorded preserved field was rejected: %s", msg)
	}
}

// ---------------------------------------------------------------------------
// AC-2.4 — the prohibition arm is STILL UNREACHED (negative control)
// ---------------------------------------------------------------------------

// TestProhibitionArm_StillUnreached is RETIRED as of SPEC-023 phase 4, and it
// is retired here rather than deleted because phase 2 genuinely passed it.
//
// WHAT IT ASSERTED. That a defect was STILL PRESENT: a prohibition carrying
// pins, dropped from `after`, reported Unrestorable = 0 and no rewrite. That
// reads perversely and was deliberate. Prohibition traversal was phase 4's
// subject and needed its own red-first evidence; a table refactor that quietly
// landed it as a side effect would have left phase 4 unable to prove it did
// anything (SSP-010). This cell was the record that phase 2 did not.
//
// WHY IT CANNOT RUN NOW. Phase 4 (AC-4.1) landed prohibition traversal, which
// makes this control FALSE BY DESIGN. AC-4.6 requires its retirement in the
// same change, for a reason worth stating: left executing, it would be a
// permanent red on phase 2 that the first person to see it would "fix" by
// reverting the newer work. It would also poison the mutation harness, whose
// `atrisk-derived-from-struct` case takes the whole package as its peer set and
// refuses to measure anything against a red baseline — so one stale negative
// control would silently take phase 1's AC-1.2, phase 2's AC-2.6 and phase 3's
// AC-3.7 down with it.
//
// It is a SKIP, not an inversion. Rewriting the body to assert that the arm IS
// reached would leave AC-2.4 green while its statement is false; the positive
// assertion belongs to AC-4.1 and lives in TestProhibitionPinsTraversed, with
// the same four-field dropped-entry shape this test used. AC-2.4's own verify
// goes red from here, which is the honest reading: the criterion is retired,
// not passing.
func TestProhibitionArm_StillUnreached(t *testing.T) {
	t.Skip("RETIRED by SPEC-023 AC-4.6: phase 4's prohibition traversal (AC-4.1) makes this negative " +
		"control false by design. The behaviour it guarded — a dropped prohibition reporting every pin " +
		"it carried — is now asserted positively by TestProhibitionPinsTraversed. Do not 'fix' this by " +
		"reverting prohibition traversal.")
}
