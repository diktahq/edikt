package pinpreserve

// preserve_phase4_test.go — PLAN-SPEC-023 phase 4 (AC-4.1, AC-4.2, AC-4.5).
//
// Phase 4's subject is the exclusion nobody chose: `computePins` looped
// `before.Directives` and nothing else, so 13 prohibitions carrying 27 pinned
// values across 8 governance artifacts — and 15 more across 10 prohibitions in
// templates/sidecars/baseline/ — were outside the mechanism entirely.
//
// Three kinds of assertion live here, and they are deliberately different:
//
//	AC-4.1  the arm is REACHED, on a constructed fixture, in both directions —
//	        a dropped prohibition names every pin it carried, and a present one
//	        gets them back. Plus the guard the reflective projection owes:
//	        every row of entryPinTable survives a round trip through it.
//	AC-4.2  a LIVE-CORPUS demonstration on a COPY, asserted as a PROPERTY —
//	        every prohibition pin present is restored and named — never as a
//	        literal count, because one new prohibition pin moves the number and
//	        a literal then fails for a reason unrelated to the property.
//	AC-4.5  the DENOMINATOR that property needs. AC-4.2 alone is satisfied by a
//	        test that copies one artifact and asserts one pin; D327's standing
//	        rule is that a claim about N artifacts needs N observations or an
//	        explicit denominator. This one enumerates its corpus at run time and
//	        PRINTS the table it observed.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ---------------------------------------------------------------------------
// AC-4.1 — the prohibition arm is reached
// ---------------------------------------------------------------------------

// p4Prohibition is a prohibition carrying the four pins the retired negative
// control (TestProhibitionArm_StillUnreached) used, so the shape that proved
// the arm SILENT at phase 2's exit is the shape that proves it REACHED now.
//
// It deliberately avoids `negative_fixture_path`: the mutation manifest's
// `atrisk-derived-from-struct` case deletes that row and requires
// TestAtRiskSetDerivedFromStruct to be the ONLY new red in the whole package,
// so a phase-4 test pinning it would break that kill's isolation clause.
func p4Prohibition(text string) sidecar.Prohibition {
	p := tblProhibition(text)
	p.Verify = "bash check-cosign.sh"
	p.VerifyKind = "behavioral"
	p.HumanApprovedAt = "2026-08-04T00:00:00Z"
	p.PositiveFixturePath = "test/fixtures/behavioral/ADR-901/positive.sh"
	return p
}

// p4PinnedFields are the four field names p4Prohibition carries.
var p4PinnedFields = []string{"human_approved_at", "positive_fixture_path", "verify", "verify_kind"}

// TestProhibitionPinsTraversed is AC-4.1's package-level half: computePins and
// both restore paths reach prohibitions.
//
// It asserts the arm in BOTH directions on purpose. "The dropped prohibition is
// reported" alone is satisfied by a traversal that reports and never restores;
// "the present prohibition is restored" alone is satisfied by one that restores
// and never reports. The pair is the requirement — and the third sub-test is
// the one a naive widening gets wrong, because projecting prohibitions onto the
// Directive shape makes it newly POSSIBLE to weld a prohibition's approval onto
// a directive that happens to share its text.
func TestProhibitionPinsTraversed(t *testing.T) {
	const surviving = "The loader MUST reject unknown keys. (ref: ADR-901)"
	const prohibited = "The gate MUST NOT accept an unsigned payload. (ref: ADR-901)"

	t.Run("a dropped prohibition names every pin it carried", func(t *testing.T) {
		before := &sidecar.Sidecar{
			Directives:   []sidecar.Directive{anchored(surviving)},
			Prohibitions: []sidecar.Prohibition{p4Prohibition(prohibited)},
		}
		path := tblAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(surviving)}})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		named := tblNamed(res.Unrestorable)
		for _, f := range p4PinnedFields {
			hit, ok := named[f]
			if !ok {
				t.Errorf("a dropped prohibition lost %q without naming it. Before SPEC-023 phase 4 the "+
					"whole entry reported Unrestorable = 0 where a directive reported four (finding 39); "+
					"the report named %v", f, tblSorted(reportFieldSet(res.Unrestorable)))
				continue
			}
			if hit.EntryKind != KindProhibition {
				t.Errorf("%q is reported with EntryKind %q, want %q — an operator reading this report has "+
					"to be able to find the entry, and a sidecar may carry a directive and a prohibition "+
					"with identical text", f, hit.EntryKind, KindProhibition)
			}
			if hit.Value == "" {
				t.Errorf("%q is named with an empty value; a name with no value cannot be re-approved", f)
			}
			if hit.DirectiveText != prohibited {
				t.Errorf("%q names text %q, want the prohibition's own text", f, hit.DirectiveText)
			}
		}
	})

	t.Run("a surviving prohibition gets its pins back on disk", func(t *testing.T) {
		before := &sidecar.Sidecar{
			Directives:   []sidecar.Directive{anchored(surviving)},
			Prohibitions: []sidecar.Prohibition{p4Prohibition(prohibited)},
		}
		// The regenerated sidecar kept the prohibition and, as the extractor
		// always does, emitted it with none of the human-authored fields.
		path := tblAfter(t, &sidecar.Sidecar{
			Directives:   []sidecar.Directive{anchored(surviving)},
			Prohibitions: []sidecar.Prohibition{tblProhibition(prohibited)},
		})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		if !res.SidecarRewritten {
			t.Fatal("a prohibition whose pins were all missing produced no rewrite — the arm restores " +
				"nothing, so the report above would be the whole of the fix")
		}
		if res.DirectivePins != 1 {
			t.Errorf("DirectivePins = %d, want 1 — the restored prohibition was not counted as an entry",
				res.DirectivePins)
		}
		got := tblLoad(t, path).Prohibitions[0]
		want := p4Prohibition(prohibited)
		if got.Verify != want.Verify || got.VerifyKind != want.VerifyKind ||
			got.HumanApprovedAt != want.HumanApprovedAt || got.PositiveFixturePath != want.PositiveFixturePath {
			t.Errorf("the prohibition on disk is %+v; the pinned values did not survive the round trip "+
				"through the projection", got)
		}
		if got.Text != prohibited {
			t.Errorf("the prohibition's text on disk is %q, want %q — the write-back overwrote the "+
				"regenerated entry with the projected one", got.Text, prohibited)
		}
	})

	t.Run("a prohibition's pin is never welded onto a directive", func(t *testing.T) {
		// The regenerated sidecar dropped the prohibition and grew a DIRECTIVE
		// with byte-identical text. Matching across kinds would reattach a
		// human approval to an entry that human never saw it on — the exact
		// invention this package refuses to make between two directives, newly
		// reachable the moment both kinds share one matcher.
		before := &sidecar.Sidecar{
			Directives:   []sidecar.Directive{anchored(surviving)},
			Prohibitions: []sidecar.Prohibition{p4Prohibition(prohibited)},
		}
		path := tblAfter(t, &sidecar.Sidecar{
			Directives: []sidecar.Directive{anchored(surviving), anchored(prohibited)},
		})

		res, err := PreservePinned("ADR-901", before, nil, path)
		if err != nil {
			t.Fatal(err)
		}
		after := tblLoad(t, path)
		for _, d := range after.Directives {
			if d.Verify != "" || d.HumanApprovedAt != "" {
				t.Fatalf("a prohibition's pins were attached to the DIRECTIVE %q (verify=%q, "+
					"human_approved_at=%q). Cross-kind matching invents an approval",
					d.Text, d.Verify, d.HumanApprovedAt)
			}
		}
		// INV-013: the fixture must have been measured. Without the report the
		// assertion above would also pass on a traversal that did nothing.
		if len(res.Unrestorable) == 0 {
			t.Fatal("nothing was reported unrestorable, so the loop above observed a run in which the " +
				"prohibition arm may never have executed at all")
		}
	})
}

// TestProhibitionProjection_CarriesEveryTableRow is the guard the reflective
// projection owes.
//
// A prohibition is projected onto the Directive shape entryPinTable is typed
// to. If a row's field failed to project — a yaml key on one struct and not the
// other, or the same key with disagreeing Go types — that row would compile,
// pass every directive test, and silently do nothing on prohibitions: the exact
// class SPEC-023 exists to close, one level down. So the projection is driven
// with EVERY row rather than with the four a fixture happens to pin.
func TestProhibitionProjection_CarriesEveryTableRow(t *testing.T) {
	if mismatched := sharedYAMLTypeMismatches(); len(mismatched) > 0 {
		t.Fatalf("Directive and Prohibition declare %v with disagreeing Go types. copyByYAMLKey cannot "+
			"carry such a key, so a pin on one of them would vanish in the projection with nothing said",
			mismatched)
	}
	if len(entryPinTable) == 0 {
		t.Fatal("entryPinTable is empty — the loop below would assert nothing (INV-013)")
	}

	for _, f := range entryPinTable {
		f := f
		t.Run(f.name, func(t *testing.T) {
			// Write the row's field on a Directive, project it BACK onto a
			// prohibition, and project that forward again. A row that does not
			// survive both directions cannot be restored on a prohibition.
			var seed sidecar.Directive
			seed.Text = "The gate MUST NOT accept an unsigned payload. (ref: ADR-901)"
			f.write(&seed, pinValue{present: true, text: "x", list: []string{"tools/edikt/**/*.go"}})
			if !f.read(&seed).present {
				t.Fatalf("the row's own write/read pair does not round-trip on a Directive, so this " +
					"test cannot say anything about the projection")
			}

			var p sidecar.Prohibition
			writeDirectiveShapeInto(seed, &p)
			back := directiveShapeOf(p)
			if !f.read(&back).equal(f.read(&seed)) {
				t.Errorf("%q does not survive Directive -> Prohibition -> Directive. A pinnable row that "+
					"cannot project is restored on directives and silently dropped on prohibitions",
					f.name)
			}
			if back.Text != seed.Text {
				t.Errorf("the entry text does not survive the projection (%q -> %q) — matching would "+
					"never find the entry again", seed.Text, back.Text)
			}
		})
	}

	t.Run("prohibition-only fields survive a round trip", func(t *testing.T) {
		p := sidecar.Prohibition{
			Text:        "The gate MUST NOT accept an unsigned payload. (ref: ADR-901)",
			DerivedFrom: "ADR-901 directives[0]",
		}
		restored := p
		writeDirectiveShapeInto(directiveShapeOf(p), &restored)
		if restored.DerivedFrom != p.DerivedFrom {
			t.Errorf("derived_from was blanked by the round trip (%q -> %q). The Directive shape does "+
				"not declare it, so the write-back must leave it alone rather than clear it",
				p.DerivedFrom, restored.DerivedFrom)
		}
	})
}

// ---------------------------------------------------------------------------
// AC-4.2 / AC-4.5 — the live corpus, on a copy, with its denominator printed
// ---------------------------------------------------------------------------

// p4Population is one directory the demonstration sweeps. The BASELINE
// TEMPLATES are named explicitly: SPEC-023 finding 9 measured 15 pinned values
// across 10 prohibitions there, and no criterion before AC-4.5 named that
// population at all.
type p4Population struct {
	name string
	dir  string
}

var p4Populations = []p4Population{
	{name: "governance corpus", dir: "docs"},
	{name: "baseline templates", dir: filepath.Join("templates", "sidecars", "baseline")},
}

// p4Artifact is one sidecar the sweep read, with the prohibition pins it
// carries.
type p4Artifact struct {
	population string
	path       string
	sc         *sidecar.Sidecar
	// pins maps prohibition index -> field name -> rendered value.
	pins map[int]map[string]string
}

func (a p4Artifact) pinCount() int {
	n := 0
	for _, fields := range a.pins {
		n += len(fields)
	}
	return n
}

// p4RepoRoot walks up from the package directory to the repository root.
//
// WHEN THE CORPUS IS NOT THERE it SKIPS, and the skip is not a loophole. The
// phase-1 mutation harness runs this package from a scratch copy of the Go
// MODULE alone — `tools/edikt` and nothing above it — so docs/ and
// templates/sidecars/ genuinely do not exist there, and a t.Fatal would turn
// the harness's `atrisk-derived-from-struct` baseline red and silently take
// AC-1.2, AC-2.6 and AC-3.7 with it.
//
// What stops the skip becoming a free pass is the criterion's own command:
// AC-4.2 and AC-4.5 run through test/lib/gotest.sh, which FAILS when no
// `--- PASS:` line appears. A skipped run emits none. So in the real tree a
// corpus that went missing turns those criteria RED rather than green, which is
// the distinction INV-013 asks for — unmeasured must not read as measured.
func p4RepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		ok := true
		for _, p := range p4Populations {
			if st, err := os.Stat(filepath.Join(dir, p.dir)); err != nil || !st.IsDir() {
				ok = false
				break
			}
		}
		if ok {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Skipf("UNMEASURED, not passed: no repository root above %s carries every population %v, so the "+
		"live corpus is not reachable from here. That is the ordinary state inside the phase-1 "+
		"mutation harness's scratch copy of the module. In the real tree AC-4.2 and AC-4.5 run "+
		"through test/lib/gotest.sh, which fails when nothing reports `--- PASS:`, so this skip "+
		"turns those criteria RED rather than green.", dir, p4Populations)
	return ""
}

// p4Sweep reads every sidecar under every population and records the
// prohibition pins it finds. A population that yields no sidecar at all is a
// subject that vanished, and fails.
func p4Sweep(t *testing.T) (string, []p4Artifact) {
	t.Helper()
	root := p4RepoRoot(t)

	var out []p4Artifact
	for _, pop := range p4Populations {
		read := 0
		err := filepath.Walk(filepath.Join(root, pop.dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() || !strings.HasSuffix(path, ".edikt.yaml") {
				return nil
			}
			sc, lerr := sidecar.Load(path)
			if lerr != nil {
				// Not skipped. A corpus file this package cannot read is a
				// hole in the denominator, and a sweep that quietly steps
				// around one is reporting coverage it does not have.
				t.Fatalf("%s could not be loaded, so it is outside this demonstration's denominator: %v",
					path, lerr)
			}
			read++
			art := p4Artifact{population: pop.name, path: path, sc: sc, pins: map[int]map[string]string{}}
			for i := range sc.Prohibitions {
				d := directiveShapeOf(sc.Prohibitions[i])
				fields := map[string]string{}
				for _, f := range entryPinTable {
					if v := f.read(&d); v.present {
						fields[f.name] = v.String()
					}
				}
				if len(fields) > 0 {
					art.pins[i] = fields
				}
			}
			out = append(out, art)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", pop.dir, err)
		}
		if read == 0 {
			t.Fatalf("population %q (%s) yielded no sidecar — the sweep's subject is empty there, so "+
				"whatever it reports about that population is unmeasured (INV-013)", pop.name, pop.dir)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return root, out
}

// p4StripProhibitionPins writes a COPY of the artifact into dir with every
// prohibition pin cleared, and returns its path. The working tree is never
// touched — a live-corpus demonstration that edits the corpus it is
// demonstrating on is one bad interrupt away from destroying it.
func p4StripProhibitionPins(t *testing.T, dir string, a p4Artifact) string {
	t.Helper()
	stripped := *a.sc
	stripped.Prohibitions = append([]sidecar.Prohibition(nil), a.sc.Prohibitions...)
	for i := range stripped.Prohibitions {
		d := directiveShapeOf(stripped.Prohibitions[i])
		for _, f := range entryPinTable {
			f.write(&d, pinValue{})
		}
		writeDirectiveShapeInto(d, &stripped.Prohibitions[i])
	}
	body, err := sidecar.Marshal(&stripped)
	if err != nil {
		t.Fatalf("marshalling the stripped copy of %s: %v", a.path, err)
	}
	path := filepath.Join(dir, strings.ReplaceAll(a.path, string(os.PathSeparator), "_"))
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestLiveCorpus_ProhibitionPinsRestoredAndNamed is AC-4.2.
//
// TWO CALLS, one fixture, because neither instrument can show both halves:
// PreservePinned DISCARDS the reinstated list at its computePins call site, and
// only MissingPins returns it (SPEC-023 finding 37). So MissingPins runs BEFORE
// the restore for the NAMING half, and PreservePinned plus a fresh Load proves
// the VALUE half.
//
// Asserted as a property over whatever the sweep found — never as a literal
// count. One new prohibition pin in the corpus moves the number, a literal then
// fails for a reason unrelated to the property, and the usual repair is to edit
// the literal down until it asserts nothing.
func TestLiveCorpus_ProhibitionPinsRestoredAndNamed(t *testing.T) {
	_, artifacts := p4Sweep(t)

	observedPins, observedArtifacts := 0, 0
	for _, a := range artifacts {
		if a.pinCount() == 0 {
			continue
		}
		observedArtifacts++
		a := a
		t.Run(filepath.Base(a.path), func(t *testing.T) {
			dir := t.TempDir()
			after := p4StripProhibitionPins(t, dir, a)
			id := strings.TrimSuffix(filepath.Base(a.path), ".edikt.yaml")

			// HALF ONE — naming, from the instrument that can show it.
			missing, err := MissingPins(id, a.sc, nil, after)
			if err != nil {
				t.Fatalf("MissingPins on the stripped copy: %v", err)
			}
			named := map[string]bool{}
			for _, m := range missing {
				if m.EntryKind == KindProhibition {
					named[m.DirectiveText+"\x00"+m.Field] = true
				}
			}
			for i, fields := range a.pins {
				text := a.sc.Prohibitions[i].Text
				for name := range fields {
					if !named[text+"\x00"+name] {
						t.Errorf("%s prohibitions[%d] lost %q and MissingPins did not name it as a "+
							"prohibition pin", a.path, i, name)
					}
				}
			}

			// HALF TWO — values, through the mutating path and a fresh Load.
			if _, err := PreservePinned(id, a.sc, nil, after); err != nil {
				t.Fatalf("PreservePinned on the stripped copy: %v", err)
			}
			restored, err := sidecar.Load(after)
			if err != nil {
				t.Fatalf("reloading the restored copy: %v", err)
			}
			if len(restored.Prohibitions) != len(a.sc.Prohibitions) {
				t.Fatalf("the restored copy has %d prohibition(s), the original %d",
					len(restored.Prohibitions), len(a.sc.Prohibitions))
			}
			for i, fields := range a.pins {
				d := directiveShapeOf(restored.Prohibitions[i])
				for _, f := range entryPinTable {
					want, pinned := fields[f.name]
					if !pinned {
						continue
					}
					got := f.read(&d)
					if !got.present || got.String() != want {
						t.Errorf("%s prohibitions[%d] %q is %q after the restore, want %q",
							a.path, i, f.name, got.String(), want)
					}
				}
			}
		})
		observedPins += a.pinCount()
	}

	if observedPins == 0 {
		t.Fatalf("the sweep found ZERO prohibition pins across %d sidecar(s). SPEC-023 measured 27 in "+
			"the governance corpus and 15 in the baseline templates; a run observing none has asserted "+
			"nothing about either (INV-013)", len(artifacts))
	}
	t.Logf("asserted %d prohibition pin(s) across %d artifact(s) of %d swept",
		observedPins, observedArtifacts, len(artifacts))
}

// TestLiveCorpus_EnumeratesAndPrintsDenominator is AC-4.5 — SR-002's
// DENOMINATOR.
//
// AC-4.2 forbids a literal count, correctly. But "every prohibition pin present
// is restored and named" is satisfied, on its own terms, by a test that copies
// ONE artifact and asserts ONE pin: the quantifier has no denominator, and
// D327's standing rule is that a claim about N artifacts needs N observations
// or an explicit denominator. This test is the denominator. It enumerates the
// corpus at run time and PRINTS the table it observed, per population, so the
// number the sibling test asserts over is visible rather than implied.
func TestLiveCorpus_EnumeratesAndPrintsDenominator(t *testing.T) {
	root, artifacts := p4Sweep(t)

	type tally struct {
		sidecars      int
		withPins      int
		prohibitions  int
		pinnedEntries int
		pinValues     int
	}
	byPopulation := map[string]*tally{}
	byField := map[string]int{}
	order := []string{}
	for _, p := range p4Populations {
		byPopulation[p.name] = &tally{}
		order = append(order, p.name)
	}

	var rows []string
	for _, a := range artifacts {
		tl := byPopulation[a.population]
		tl.sidecars++
		tl.prohibitions += len(a.sc.Prohibitions)
		if a.pinCount() == 0 {
			continue
		}
		tl.withPins++
		tl.pinnedEntries += len(a.pins)
		tl.pinValues += a.pinCount()
		names := map[string]int{}
		for _, fields := range a.pins {
			for name := range fields {
				byField[name]++
				names[name]++
			}
		}
		var parts []string
		for _, name := range p4SortedKeys(names) {
			parts = append(parts, fmt.Sprintf("%s x%d", name, names[name]))
		}
		rel, err := filepath.Rel(root, a.path)
		if err != nil {
			rel = a.path
		}
		rows = append(rows, fmt.Sprintf("  %-18s %-72s %2d prohibition(s) pinned, %2d value(s): %s",
			a.population, rel, len(a.pins), a.pinCount(), strings.Join(parts, ", ")))
	}

	t.Log("prohibition-pin denominator, enumerated at run time:")
	for _, row := range rows {
		t.Log(row)
	}
	total := 0
	for _, name := range order {
		tl := byPopulation[name]
		total += tl.pinValues
		t.Logf("  %-18s %3d sidecar(s), %3d prohibition(s), %2d pinned entr(y/ies), %2d pinned value(s)",
			name, tl.sidecars, tl.prohibitions, tl.pinnedEntries, tl.pinValues)
	}
	for _, name := range p4SortedKeys(byField) {
		t.Logf("  field %-22s %3d value(s)", name, byField[name])
	}
	t.Logf("  TOTAL %d prohibition pin value(s) over %d sidecar(s)", total, len(artifacts))

	// The denominator is the point, so a zero denominator is the failure. Every
	// population must have contributed a subject, and the sweep as a whole must
	// have found something to protect.
	for _, name := range order {
		if byPopulation[name].sidecars == 0 {
			t.Errorf("population %q contributed no sidecar, so its share of the denominator is a "+
				"silent zero rather than a measured one (INV-013)", name)
		}
	}
	if total == 0 {
		t.Fatal("the enumerated denominator is ZERO prohibition pins. Every property this phase asserts " +
			"over the live corpus would then be vacuously true, which is the shape INV-013 forbids")
	}
}

func p4SortedKeys(m map[string]int) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
