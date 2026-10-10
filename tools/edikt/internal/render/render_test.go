// Package render had ZERO test reach until this file: no _test.go of its own,
// and no test anywhere in the tree referenced it. It is 298 lines sitting on
// the path that decides what text actually reaches the model, and its failure
// mode is the quiet one — a directive that should have been rendered and was
// not produces a compiled surface that reads as complete.
//
// That is globmatch's shape exactly (PLAN-test-reach-on-the-delivery-path):
// a package whose doc comment stated semantics the code did not implement,
// which nothing caught because nothing crashed. So the doc comments and the
// exported signatures of render.go are treated here as the specification, and
// each capability they claim is enumerated below and recorded covered or
// uncovered. The count of tests in this file is not the deliverable; the
// enumeration is.
//
// ── CAPABILITY ENUMERATION (source: render.go doc comments + exported API) ──
//
//	#   capability (as the package claims it)                       status
//	--- ---------------------------------------------------------- ---------
//	 1  pkg doc: "Deterministic: same input produces byte-equal      COVERED
//	    output" — including across map-valued input (Excluded)       (T9)
//	 2  EmptySHA == sha256("") in lower-case hex — a fact about  COVERED (T1)
//	    the hash, not a default anything falls back to (D357)
//	 3  RegionSHA(body) == sha256(body) hex; RegionSHA("")==EmptySHA COVERED (T2)
//	 4  RenderTopic sorts Paths and Sources — "template's iteration  COVERED (T3)
//	    order must not depend on insertion order"
//	 5  every managed region's declared anchor is the sha256 of the  COVERED (T4)
//	    bytes the renderer emitted between THAT region's markers,
//	    with prohibitions present and with every region empty. No
//	    caller can supply an anchor: the SHA fields are gone (D357)
//	 6  TopicView.Description is rendered VERBATIM, and an absent    COVERED (T5)
//	    description says so explicitly instead of omitting the line
//	    (INV-013: missing must not read the same as approved-blank).
//	    The remediation that absence line carries names the SLASH-
//	    COMMAND form and no repo-relative binary path: this line
//	    renders into every compiled .claude/rules/governance/ topic
//	    file lacking an approved description, which is a surface a
//	    USER reads, and `bin/edikt` does not exist in a user's
//	    project (D290 — the same removal already paid for across 32
//	    command bodies, reproduced here in a template)
//	 7  every directive/prohibition/manual line reaches its managed  COVERED (T6)
//	    region, in order — SILENT OMISSION IS CAUGHT, by TWO checks
//	    (the body byte-equal to the expected body; every line named
//	    individually, plus a bullet count for the other direction),
//	    not the three phase 1 recorded. The third — the body against
//	    the anchor the file declares — became true by construction
//	    under D357 and no longer kills an omission. It is kept as a
//	    region-BOUNDARY check and recorded as one. Re-measured, not
//	    reasoned: see the phase 6 record below
//	 8  Reminders / Verification render as their own sections on the COVERED (T7)
//	    TOPIC file (D37: they must not sit on the always-on core)
//	 9  topic frontmatter: paths block, compile_schema_version: 3,   COVERED (T8)
//	    and _fingerprint present only when the caller set one
//	10  IndexView.Excluded distinguishes UNMEASURED (nil) from a     COVERED (T10)
//	    measured zero, sorts kinds, labels the empty kind
//	    "unclassified", and singularises "artifact"
//	11  TopicIndexRow: every rendered topic gets a row including one COVERED (T11)
//	    whose description is still pending; SkillOnly points at the
//	    skill package instead of a rules file that does not exist
//	12  index renders each InvariantRule's Text, and its Intent only COVERED (T12)
//	    when the rule carries one (SPEC-018 SR-001)
//	13  index header carries the compiled counts and the directive / COVERED (T13)
//	    topic totals
//	14  IndexView.Reminders / .Verification do NOT reach the ambient COVERED (T14)
//	    core — they moved to the topic files under D37
//	15  `title`: "agent-rules" renders as the heading "Agent-Rules"  COVERED (T15)
//	16  RendererFingerprint hashes BOTH templates, domain-separated  COVERED (T16)
//	    and order-fixed, so a template edit changes the digest
//	17  CompiledAt is accepted by both views and deliberately NOT    COVERED (T17)
//	    rendered — the removal that RendererFingerprint's doc says
//	    reached zero of ten topic files before the fingerprint landed
//	18  TopicView.Rules ([]compile.Rule)                             UNCOVERED
//	    Accepted by the view and referenced by NO template: the
//	    topic body is built from DirectiveLines. Recorded rather
//	    than asserted — a test pinning "this field changes nothing"
//	    would freeze a dead field into the contract instead of
//	    reporting it. It is dead weight on the view, not a claim.
//	19  IndexView.InvariantRestated ([]compile.Rule)                 UNCOVERED
//	    Same shape: populated by phaseb (a copy of InvariantRules),
//	    referenced by no template. Recorded, not asserted.
//	20  changing the TEMPLATE's region structure moves that region's COVERED (T18)
//	    anchor with it — and moves no other region's. The property
//	    that separates "the renderer hashes what it emits" from
//	    "the anchor happens to be right today"
//	21  the ERROR HALF of both exported signatures: RenderTopic and  COVERED (T19)
//	    RenderIndex are (string, error), and a parse failure, an
//	    execute failure, or a region that cannot be anchored returns
//	    "" with a non-nil error — never partial output with a nil
//	    one. Phase 1 enumerated neither half; the gap was
//	    mutation-proven (returning buf.String() with a nil error on
//	    an Execute failure left the whole suite green)
//	22  TopicView.ScopeNote reaches the rendered scope comment, so   COVERED (T8)
//	    "unscoped because undeclared" cannot read as "unscoped by
//	    choice". Recorded because Phase 1 asserted it without
//	    enumerating it — an unlisted row is an unexamined one
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Every test below was falsified by mutation before it was trusted: the
// capability it covers was broken in render.go or in templates/, the suite was
// run, and the test went RED. A test that passes against both the working and
// the broken implementation is not reach. Each mutation was reverted from a
// byte-for-byte backup taken before it was applied. Measured 2026-09-06,
// `go test -count=1 -run TestRender ./internal/render/`:
//
// CITATION RULE for this record, and for anything that copies its format: a
// kill is cited by STABLE IDENTIFIER — the Go test name, e.g.
// TestRender_OmittedDirectiveIsCaught, and the assertion's own message text —
// NEVER by a line number into this file. A self-referential line number is
// stale by construction the moment anything above it moves, and it breaks
// SILENTLY: the T6 re-measurement below once carried three of them, and a
// later edit inserting 35 lines above the assertions left all three pointing
// at a comment, a string literal and a variable assignment while still reading
// as provenance. That is INV-016 at the citation layer.
//
//	T1  EmptySHA seeded with sha256("x")                      → RED
//	T2  RegionSHA hashes nil instead of its argument          → RED
//	T3  RenderTopic's sort.Strings(v.Paths) removed           → RED
//	T4  REPLACED in phase 6 — the phase 1 test compared each
//	    region's declared anchor against the EmptySHA
//	    constant, asserting the declaration where the outcome
//	    was available; see the phase 6 record below
//	T5  topic.md.tmpl drops the no-description fallback line  → RED
//	T6  topic.md.tmpl skips the FIRST directive line (the
//	    silent-omission mutation: output still looks whole)    → RED
//	T7  topic.md.tmpl drops the Verification section          → RED
//	T8  topic.md.tmpl emits _fingerprint unconditionally      → RED
//	T9  excludedNote's sort.Strings(kinds) removed            → RED
//	T10 excludedNote returns the zero note for a nil map      → RED
//	T11 index.md.tmpl drops the *(skill: ...)* marker         → RED
//	T12 index.md.tmpl drops the intent line                   → RED
//	T13 index.md.tmpl prints TopicCount where DirectiveCount
//	    belongs                                               → RED
//	T14 index.md.tmpl re-adds a Reminders section             → RED
//	T15 titleCase returns its argument unchanged              → RED
//	T16 RendererFingerprint stops hashing index.md.tmpl       → RED
//	T17 topic.md.tmpl adds compiled_at to the frontmatter     → RED
//
// The full working tree was green immediately before and after the sweep.
//
// ── PHASE 6 (D357): the tests added when the anchors moved here ─────────────
//
// Falsified the same way, 2026-09-06, `go test -count=1 -run TestRender
// ./internal/render/`. Each mutation was applied to render.go, measured, and
// reverted from a byte-for-byte backup (sha256 3258348d… before and after).
//
//	T4  anchorRegions hashes "" for every region instead of
//	    the body it emitted                                    → RED
//	    (T6 and T18 also RED — a constant anchor is visible
//	    from every direction)
//	T18 anchorRegions computes each anchor from a MODEL of the
//	    region — strip whatever heading the template emitted,
//	    prepend the "## Prohibitions\n" this function believes
//	    in — which is EXACTLY RIGHT under the shipped template  → RED
//	    T4, T6 and the phaseb suite stayed GREEN. T18 alone
//	    caught it, because T18 renders through an ALTERED
//	    template, where the model and the bytes part company
//	T19 the Execute-failure path returns buf.String() with a
//	    nil error: partial output, no error                     → RED
//	    Nothing else moved. This mutation survived Phase 1's
//	    entire suite
//
//	T6  RE-MEASURED, because option D changed what this test
//	    can kill and the phase 1 row was left claiming three
//	    legs. topic.md.tmpl skips the FIRST directive line —
//	    the silent-omission mutation — full suite run
//	    2026-09-06, template reverted from a byte-for-byte
//	    backup (sha256 7713c05d… before and after)              → RED
//	    in TestRender_OmittedDirectiveIsCaught, on all three of
//	    its per-region checks, cited by message text: `region %q
//	    body:\n got %q\nwant %q` (not byte-equal), `region %q
//	    silently omitted the line %q` (the line was "Cite
//	    ADR-NNN when a decision applies") and `region %q
//	    rendered %d bullet(s), want %d` (rendered 2, want 3).
//	    The declared-anchor comparison DID NOT FIRE and could
//	    not: under D357 the renderer hashes the bytes it
//	    emitted, so dropping a line moves body and anchor
//	    together. Two legs kill the omission, not three; the
//	    third is a region-boundary check and is recorded as
//	    one. No other test in the package moved
//
// T18's mutation is the one to read twice. It is the option-A shape — one
// component holding a copy of another's layout — and it is invisible to every
// test that only ever renders the template that ships. That is the drift
// phaseb.regionSHA carried, and why nothing could see it.
package render

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/compile"
)

// ── helpers ─────────────────────────────────────────────────────────────────

// emittedRegion returns the bytes a managed region encloses in the RENDERED
// OUTPUT: everything strictly between the region's start-marker line and its
// end-marker line. A missing marker is fatal rather than an empty string —
// "the region is not there" and "the region is empty" are different facts and
// must not both read as "".
//
// Deliberately a second, independent implementation rather than a call to the
// package's own regionBody: the renderer hashes what regionBody returns, so a
// test that read the body through the same function could not tell a
// mis-parsed region from a correctly parsed one. Reading the artifact is the
// test's own job.
func emittedRegion(t *testing.T, out, region string) string {
	t.Helper()
	start := "[edikt:" + region + ":start]: #\n"
	end := "[edikt:" + region + ":end]: #"
	i := strings.Index(out, start)
	if i < 0 {
		t.Fatalf("region %q: no start marker %q in rendered output:\n%s", region, start, out)
	}
	rest := out[i+len(start):]
	j := strings.Index(rest, end)
	if j < 0 {
		t.Fatalf("region %q: no end marker %q after the start marker in:\n%s", region, end, out)
	}
	return rest[:j]
}

// declaredAnchor reads the sha256 anchor line a region declares.
func declaredAnchor(t *testing.T, out, region string) string {
	t.Helper()
	re := regexp.MustCompile(`(?m)^\[edikt:` + regexp.QuoteMeta(region) + `:sha256\]: # (.*)$`)
	m := re.FindStringSubmatch(out)
	if m == nil {
		t.Fatalf("region %q declares no sha256 anchor in:\n%s", region, out)
	}
	return strings.TrimSpace(m[1])
}

// wantRegionBody states the bytes a region is EXPECTED to carry: an optional
// in-marker heading, then one "- text" bullet per input line.
//
// Its predecessor, anchorFor, was a hand-copy of phaseb.regionSHA whose
// comment claimed it caught renderer/caller drift. It did not: mutating the
// real regionSHA to stop prepending the prohibitions heading — which makes
// every anchor in every compiled topic file wrong — left this suite green,
// the only survivor of 29 mutations. A guard that restates the logic it
// guards cannot see that logic change (INV-014).
//
// This is not that. It states an expectation of the OUTPUT, and there is no
// second copy of the anchor formula left to drift from: nobody supplies an
// anchor any more (D357). What guards the hashing is T4 and T18, which hash
// the bytes actually emitted.
func wantRegionBody(heading string, lines []string) string {
	var b strings.Builder
	b.WriteString(heading)
	for _, l := range lines {
		b.WriteString("- ")
		b.WriteString(l)
		b.WriteByte('\n')
	}
	return b.String()
}

func mustRenderTopic(t *testing.T, v TopicView) string {
	t.Helper()
	out, err := RenderTopic(v)
	if err != nil {
		t.Fatalf("RenderTopic: %v", err)
	}
	return out
}

func mustRenderIndex(t *testing.T, v IndexView) string {
	t.Helper()
	out, err := RenderIndex(v)
	if err != nil {
		t.Fatalf("RenderIndex: %v", err)
	}
	return out
}

// ── T1: EmptySHA ────────────────────────────────────────────────────────────

func TestRender_EmptySHAIsTheHashOfNothing(t *testing.T) {
	want := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if EmptySHA != want {
		t.Errorf("EmptySHA = %q, want sha256(\"\") = %q", EmptySHA, want)
	}
	if EmptySHA != strings.ToLower(EmptySHA) {
		t.Errorf("EmptySHA = %q, doc says lower-case hex", EmptySHA)
	}
}

// ── T2: RegionSHA ───────────────────────────────────────────────────────────

func TestRender_RegionSHAHashesTheBodyItIsGiven(t *testing.T) {
	for _, body := range []string{"", "- a\n", "- a\n- b\n", "## Prohibitions\n- p\n"} {
		sum := sha256.Sum256([]byte(body))
		want := hex.EncodeToString(sum[:])
		if got := RegionSHA(body); got != want {
			t.Errorf("RegionSHA(%q) = %q, want %q", body, got, want)
		}
	}
	if got := RegionSHA(""); got != EmptySHA {
		t.Errorf("RegionSHA(\"\") = %q, want EmptySHA %q — the empty region's anchor", got, EmptySHA)
	}
	// Distinct bodies must not share an anchor, or omission is undetectable.
	if RegionSHA("- a\n") == RegionSHA("- a\n- b\n") {
		t.Error("RegionSHA collapsed two different region bodies to one digest")
	}
}

// ── T3: sorting for determinism ─────────────────────────────────────────────

func TestRender_TopicSortsPathsAndSources(t *testing.T) {
	out := mustRenderTopic(t, TopicView{
		Name:    "release",
		Paths:   []string{"z/**", "a/**", "m/**"},
		Sources: []string{"INV-009", "ADR-002", "ADR-001"},
	})
	pi := []int{strings.Index(out, `- "a/**"`), strings.Index(out, `- "m/**"`), strings.Index(out, `- "z/**"`)}
	for i, at := range pi {
		if at < 0 {
			t.Fatalf("path %d missing from frontmatter:\n%s", i, out)
		}
	}
	if !(pi[0] < pi[1] && pi[1] < pi[2]) {
		t.Errorf("paths not emitted in sorted order (offsets %v):\n%s", pi, out)
	}
	if want := "<!-- sources: ADR-001, ADR-002, INV-009 -->"; !strings.Contains(out, want) {
		t.Errorf("sources not sorted; want line %q in:\n%s", want, out)
	}
	// The same input in a different insertion order must render byte-equal.
	other := mustRenderTopic(t, TopicView{
		Name:    "release",
		Paths:   []string{"m/**", "z/**", "a/**"},
		Sources: []string{"ADR-002", "INV-009", "ADR-001"},
	})
	if out != other {
		t.Error("insertion order changed the rendered bytes — package doc claims determinism")
	}
}

// ── T4: every anchor describes its own region ───────────────────────────────

// The anchor a topic file declares for a region is the sha256 of the bytes
// that file carries between THAT region's markers. Asserted for a topic that
// has prohibitions and for one with no lines at all, because the empty case is
// where the scheme used to be wrong.
//
// What this replaces: a check that compared each region's declared anchor
// against the EmptySHA CONSTANT, for all three regions — the declaration
// standing in for the outcome, which is INV-014's shape. It passed while the prohibitions region
// declared sha256("") over a body of "## Prohibitions\n", froze that wrong
// value into the contract (fixing render.go would have broken it), and sat
// four hundred lines from the correct three-way check in T6.
//
// A caller-supplied anchor is not tested because a caller cannot supply one:
// the TopicView SHA fields are gone (D357). A field that is accepted and
// discarded looks identical to one that is honoured.
func TestRender_EveryAnchorDescribesItsRegion(t *testing.T) {
	for _, tc := range []struct {
		name string
		v    TopicView
	}{
		{"every region populated", TopicView{
			Name:             "compile",
			DirectiveLines:   []string{"cite ADR-NNN when a decision applies", "record why a glob widened"},
			ProhibitionLines: []string{"MUST NOT edit a compiled surface by hand"},
			ManualLines:      []string{"the extractor missed this one"},
		}},
		{"no lines at all", TopicView{Name: "compile"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := mustRenderTopic(t, tc.v)
			for _, region := range []string{"directives", "prohibitions", "manual"} {
				body := emittedRegion(t, out, region)
				if got, want := declaredAnchor(t, out, region), RegionSHA(body); got != want {
					t.Errorf("region %q declares anchor %s, but the bytes between its own markers hash to %s\nbody was %q",
						region, got, want, body)
				}
			}
		})
	}

	// The empty topic, stated as an outcome rather than as a constant. Two
	// regions have a genuinely empty body and therefore hash to the hash of
	// nothing; the prohibitions region does NOT, because the template emits
	// its heading inside the markers. That asymmetry is the whole defect —
	// one anchor scheme was applied to three regions of two different shapes.
	out := mustRenderTopic(t, TopicView{Name: "compile"})
	hashOfNothing := RegionSHA("")
	for _, region := range []string{"directives", "manual"} {
		if body := emittedRegion(t, out, region); body != "" {
			t.Errorf("region %q was expected to render an empty body, got %q", region, body)
		}
		if got := declaredAnchor(t, out, region); got != hashOfNothing {
			t.Errorf("empty region %q anchored %s, want the hash of an empty body %s", region, got, hashOfNothing)
		}
	}
	proh := emittedRegion(t, out, "prohibitions")
	if proh == "" {
		t.Fatalf("the prohibitions region rendered an empty body; this test is measuring nothing:\n%s", out)
	}
	if got := declaredAnchor(t, out, "prohibitions"); got == hashOfNothing {
		t.Errorf("the prohibitions region anchored to the hash of nothing (%s) while its body is %q — "+
			"that anchor describes a region this file does not contain", got, proh)
	}
}

// ── T5: the description is verbatim, and its absence is stated ──────────────

func TestRender_TopicDescriptionIsVerbatimAndAbsenceIsExplicit(t *testing.T) {
	// SSP-002: nothing in the package may rewrite, wrap, truncate or
	// re-derive the approved line. The input carries characters a
	// well-meaning renderer would be tempted to touch.
	desc := "Deciding how a rule reaches the model — extraction, `paths:` globs, {{ not a template }}"
	out := mustRenderTopic(t, TopicView{Name: "compile", Description: desc})
	if !strings.Contains(out, "> "+desc+"\n") {
		t.Errorf("approved description not rendered verbatim; want %q in:\n%s", desc, out)
	}

	// INV-013: an unapproved description must not render as an approved
	// blank one. The template has to SAY there is none.
	out = mustRenderTopic(t, TopicView{Name: "compile"})
	if !strings.Contains(out, "no approved topic description") {
		t.Errorf("an absent description rendered silently — nothing in the output says it is unapproved:\n%s", out)
	}
	if !strings.Contains(out, "/edikt:sidecar:approve --kind topic-description compile") {
		t.Errorf("the absence note does not say how to approve one:\n%s", out)
	}

	// D290: the remediation must name the SLASH-COMMAND form. A rendered
	// topic file is a surface a USER reads, and `bin/edikt` is a path that
	// exists in this repository and in no installed project. Asserted as a
	// prohibition over the whole rendered output rather than only over the
	// absence note, because the defect's shape is a repo-relative binary
	// path reaching a user, wherever in the file it is emitted from.
	if strings.Contains(out, "bin/edikt") {
		t.Errorf("a rendered topic file names the repo-relative binary path `bin/edikt`, "+
			"which does not exist in a user's project (D290):\n%s", out)
	}
}

// ── T6: SILENT OMISSION IS CAUGHT ───────────────────────────────────────────

// TestRender_OmittedDirectiveIsCaught is the phase's completion promise in one
// test. A directive that should appear in a rendered surface and does not is
// the failure shape that let globmatch ship broken: no panic, no error return,
// just a plausible-looking file with less in it than it should have.
//
// TWO independent ways to notice, because either one alone can be satisfied
// by output that is still wrong:
//
//	(a) the region body is byte-equal to the body the test states it should
//	    carry — catches an omitted line, a duplicated one, a reordered one,
//	    and a changed bullet prefix;
//	(b) every input line is present, asserted per line, so a failure names the
//	    directive that vanished rather than reporting a hash mismatch — plus a
//	    bullet count, which guards the other direction (a region that GAINED a
//	    line is as wrong as one that lost one).
//
// A third comparison follows in the body — the region against the anchor the
// file itself declares — and it is deliberately NOT counted among those two.
// Phase 1 recorded it as a third leg of the omission kill; option D (D357)
// made it true by construction, because the renderer now anchors the bytes it
// emitted, so dropping a line moves the body and the anchor together.
// Re-measured rather than reasoned: with topic.md.tmpl skipping the first
// directive line this test went RED at (a), at (b) and on the bullet count,
// and that comparison did not fire. It stays because it still falsifies one
// thing — a renderer whose own idea of where a region begins and ends parts
// company with the markers this test parses independently — and that, not
// omission, is what it now guards.
func TestRender_OmittedDirectiveIsCaught(t *testing.T) {
	dirs := []string{
		"Cite ADR-NNN when a decision applies",
		"Never widen a `paths:` glob without recording why",
		"A directive containing {{ .Name }} is data, not a template",
	}
	prohs := []string{"MUST NOT edit a compiled surface by hand"}
	mans := []string{"the extractor missed this one, so a human wrote it"}

	dBody := wantRegionBody("", dirs)
	pBody := wantRegionBody("## Prohibitions\n", prohs)
	mBody := wantRegionBody("", mans)

	out := mustRenderTopic(t, TopicView{
		Name:             "compile",
		DirectiveLines:   dirs,
		ProhibitionLines: prohs,
		ManualLines:      mans,
	})

	for _, tc := range []struct {
		region string
		lines  []string
		body   string
	}{
		{"directives", dirs, dBody},
		{"prohibitions", prohs, pBody},
		{"manual", mans, mBody},
	} {
		got := emittedRegion(t, out, tc.region)

		// (a) byte-equal to the body this test says the region carries.
		if got != tc.body {
			t.Errorf("region %q body:\n got %q\nwant %q", tc.region, got, tc.body)
		}
		// (b) every line present, named individually.
		for _, l := range tc.lines {
			if !strings.Contains(got, "- "+l+"\n") {
				t.Errorf("region %q silently omitted the line %q; region body was:\n%s", tc.region, l, got)
			}
		}
		// A bullet count guards the other direction: a region that gained a
		// line is as wrong as one that lost a line.
		wantBullets := len(tc.lines)
		if n := strings.Count(got, "\n- ") + boolToInt(strings.HasPrefix(got, "- ")); n != wantBullets {
			t.Errorf("region %q rendered %d bullet(s), want %d:\n%s", tc.region, n, wantBullets, got)
		}
		// A REGION-BOUNDARY check, not a third omission check: the anchor
		// the file declares against the body emittedRegion parsed out of
		// it. Under D357 both sides move together when a line is dropped
		// (measured — see the phase 6 record in the file header), so this
		// cannot catch an omission. What it can catch is the renderer's
		// own regionBody disagreeing with the markers about where the
		// region ends, since emittedRegion is an independent parse.
		if want, got := declaredAnchor(t, out, tc.region), RegionSHA(got); want != got {
			t.Errorf("region %q: the bytes between the markers hash to %s but the file declares %s — the renderer anchored a different span than the markers enclose",
				tc.region, got, want)
		}
	}

	// The empty case must still produce a REGION, not a hole: absence has to
	// be renderable, or "no directives" and "the region vanished" look alike.
	empty := mustRenderTopic(t, TopicView{Name: "compile"})
	for _, tc := range []struct {
		region string
		body   string
	}{
		{"directives", wantRegionBody("", nil)},
		{"prohibitions", wantRegionBody("## Prohibitions\n", nil)},
		{"manual", wantRegionBody("", nil)},
	} {
		body := emittedRegion(t, empty, tc.region)
		if body != tc.body {
			t.Errorf("empty region %q body = %q, want %q", tc.region, body, tc.body)
		}
		if got, want := RegionSHA(body), declaredAnchor(t, empty, tc.region); got != want {
			t.Errorf("empty region %q hashes to %s but declares %s", tc.region, got, want)
		}
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ── T7: reminders and verification live on the topic file ───────────────────

func TestRender_TopicCarriesRemindersAndVerification(t *testing.T) {
	out := mustRenderTopic(t, TopicView{
		Name:         "testing",
		Reminders:    []string{"a test that cannot fail is not evidence"},
		Verification: []string{"did you falsify it by mutation?"},
	})
	for _, want := range []string{
		"## Reminders",
		"- a test that cannot fail is not evidence",
		"## Verification Checklist",
		"- did you falsify it by mutation?",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("topic file missing %q:\n%s", want, out)
		}
	}
	// Empty lists must not leave an empty heading behind — a "## Reminders"
	// with nothing under it reads as guidance that was delivered.
	out = mustRenderTopic(t, TopicView{Name: "testing"})
	for _, unwanted := range []string{"## Reminders", "## Verification Checklist"} {
		if strings.Contains(out, unwanted) {
			t.Errorf("empty list still rendered the heading %q:\n%s", unwanted, out)
		}
	}
}

// ── T8: frontmatter and the fingerprint line ────────────────────────────────

func TestRender_TopicFrontmatterCarriesScopeAndFingerprint(t *testing.T) {
	out := mustRenderTopic(t, TopicView{
		Name:            "hooks",
		Paths:           []string{"tools/edikt/internal/hook/**"},
		ScopeNote:       "1 glob from 1 source",
		Sources:         []string{"ADR-070"},
		CompilerVersion: "0.9.0",
		Fingerprint:     "cafe1234",
	})
	for _, want := range []string{
		"paths:\n  - \"tools/edikt/internal/hook/**\"\n",
		"compile_schema_version: 3\n",
		"_fingerprint: \"cafe1234\"\n",
		"<!-- topic: hooks -->",
		"<!-- 1 glob from 1 source -->",
		"<!-- compiled_by: gov-compile v0.9.0 -->",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("topic frontmatter missing %q:\n%s", want, out)
		}
	}
	if !strings.HasPrefix(out, "---\n") {
		t.Errorf("topic body does not open with frontmatter:\n%s", out)
	}
	// No fingerprint set: the key must be ABSENT, not present-and-empty. A
	// `_fingerprint: ""` would make every cached file compare unequal to a
	// real digest and quietly re-render forever.
	out = mustRenderTopic(t, TopicView{Name: "hooks"})
	if strings.Contains(out, "_fingerprint") {
		t.Errorf("an unset fingerprint still emitted the key:\n%s", out)
	}
}

// ── T9: determinism, including over a map-valued field ──────────────────────

func TestRender_IsDeterministicAcrossRuns(t *testing.T) {
	tv := TopicView{
		Name:             "architecture",
		Description:      "tier boundaries",
		Paths:            []string{"c/**", "a/**", "b/**"},
		Sources:          []string{"INV-001", "ADR-003"},
		DirectiveLines:   []string{"one", "two"},
		ProhibitionLines: []string{"never"},
		ManualLines:      []string{"manual"},
		Reminders:        []string{"r"},
		Verification:     []string{"v"},
	}
	first := mustRenderTopic(t, tv)
	for i := 0; i < 50; i++ {
		if got := mustRenderTopic(t, tv); got != first {
			t.Fatalf("RenderTopic is not byte-stable: run %d differed", i)
		}
	}

	iv := IndexView{
		CompilerVersion: "0.9.0",
		// Four kinds: Go randomises map iteration, so an unsorted renderer
		// diverges within a handful of runs.
		Excluded:       map[string]int{"adr": 2, "invariant": 1, "guideline": 3, "": 1},
		DirectiveCount: 7,
		TopicCount:     3,
		InvariantRules: []compile.Rule{{Text: "a", Intent: "x"}, {Text: "b"}},
		TopicIndex:     []TopicIndexRow{{Topic: "compile", Description: "d"}, {Topic: "release"}},
	}
	firstIdx := mustRenderIndex(t, iv)
	for i := 0; i < 50; i++ {
		if got := mustRenderIndex(t, iv); got != firstIdx {
			t.Fatalf("RenderIndex is not byte-stable: run %d differed\n got %q\nwant %q", i, got, firstIdx)
		}
	}
}

// ── T10: excluded counts — unmeasured is not zero ───────────────────────────

func TestRender_IndexExcludedSeparatesUnmeasuredFromZero(t *testing.T) {
	// INV-013 in its original form: a nil map is "nobody counted", an empty
	// map is "counted, found none". A Go template cannot tell them apart,
	// which is why excludedNote exists at all.
	if got := excludedNote(nil); got != "retired-artifact exclusions UNMEASURED" {
		t.Errorf("excludedNote(nil) = %q, want the UNMEASURED note", got)
	}
	if got := excludedNote(map[string]int{}); got != "0 retired artifacts excluded" {
		t.Errorf("excludedNote(empty) = %q, want a measured zero", got)
	}
	if got := excludedNote(map[string]int{"adr": 0}); got != "0 retired artifacts excluded" {
		t.Errorf("excludedNote(all-zero) = %q, want a measured zero", got)
	}
	if got := excludedNote(map[string]int{"adr": 1}); got != "1 retired artifact excluded (adr 1)" {
		t.Errorf("excludedNote(one) = %q, want the singular noun", got)
	}
	// Sorted, and a kind the class contract has no entry for is NAMED rather
	// than dropped out of the total.
	got := excludedNote(map[string]int{"invariant": 2, "adr": 3, "": 1})
	if want := "6 retired artifacts excluded (unclassified 1, adr 3, invariant 2)"; got != want {
		t.Errorf("excludedNote = %q, want %q", got, want)
	}

	// And it reaches the rendered header, not just the helper.
	out := mustRenderIndex(t, IndexView{Excluded: nil})
	if !strings.Contains(out, "retired-artifact exclusions UNMEASURED") {
		t.Errorf("nil Excluded did not surface as UNMEASURED in the header:\n%s", out)
	}
	out = mustRenderIndex(t, IndexView{Excluded: map[string]int{}})
	if !strings.Contains(out, "0 retired artifacts excluded") {
		t.Errorf("measured-zero Excluded did not surface in the header:\n%s", out)
	}
}

// ── T11: the topic index ────────────────────────────────────────────────────

func TestRender_IndexListsEveryTopicIncludingPendingAndSkillOnly(t *testing.T) {
	out := mustRenderIndex(t, IndexView{
		TopicIndex: []TopicIndexRow{
			{Topic: "compile", Description: "when a rule must reach the model"},
			{Topic: "agent-rules", SkillOnly: true, Description: "when work is delegated"},
			{Topic: "release"}, // description still pending
		},
	})
	if !strings.Contains(out, "- **compile** — when a rule must reach the model\n") {
		t.Errorf("scoped topic row missing or rewritten:\n%s", out)
	}
	// A skill-only topic has NO .claude/rules/governance/<topic>.md. Pointing
	// a reader at a file that does not exist spends their attention and
	// returns nothing, so the row must name the skill package instead.
	if !strings.Contains(out, "- **agent-rules** *(skill: `edikt-agent-rules`)* — when work is delegated\n") {
		t.Errorf("skill-only topic row does not point at the skill package:\n%s", out)
	}
	if strings.Contains(out, "- **compile** *(skill") {
		t.Errorf("a topic with a rules file was marked skill-only:\n%s", out)
	}
	// Dropping the pending row would make the index under-report the corpus.
	if !strings.Contains(out, "- **release** — _(no approved description — pending)_\n") {
		t.Errorf("a topic with no approved description lost its row:\n%s", out)
	}
}

// ── T12: invariant text and intent ──────────────────────────────────────────

func TestRender_IndexRendersInvariantIntentOnlyWhenPresent(t *testing.T) {
	out := mustRenderIndex(t, IndexView{
		InvariantRules: []compile.Rule{
			{Text: "A control that observes nothing reports UNMEASURED", Source: "INV-013",
				Intent: "a silent no-op renders as a pass"},
			{Text: "Commands are markdown", Source: "INV-001"},
		},
	})
	if !strings.Contains(out, "- A control that observes nothing reports UNMEASURED\n") {
		t.Errorf("invariant text missing:\n%s", out)
	}
	// SPEC-018 SR-001: the intent travels with the statement, because a bare
	// constraint is what gets rationalised away under pressure.
	if !strings.Contains(out, "  intent: a silent no-op renders as a pass\n") {
		t.Errorf("invariant intent missing:\n%s", out)
	}
	if !strings.Contains(out, "- Commands are markdown\n") {
		t.Errorf("intent-less invariant missing:\n%s", out)
	}
	if strings.Contains(out, "intent: \n") {
		t.Errorf("an intent-less rule rendered an empty intent line:\n%s", out)
	}
}

// ── T13: the header counts ──────────────────────────────────────────────────

func TestRender_IndexHeaderCarriesTheCompiledCounts(t *testing.T) {
	out := mustRenderIndex(t, IndexView{
		ADRCompiled: 41, INVCompiled: 18, GuidelineCompiled: 6,
		Excluded:       map[string]int{"adr": 8},
		DirectiveCount: 335, TopicCount: 10,
		CompilerVersion: "0.9.0",
	})
	for _, want := range []string{
		"<!-- source: 41 ADRs, 18 invariants, 6 guidelines compiled; 8 retired artifacts excluded (adr 8) -->",
		"<!-- compiled_by: gov-compile v0.9.0 -->",
		"<!-- directives: 335 across 10 topics -->",
		"paths: \"**/*\"",
		"compile_schema_version: 3",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("index header missing %q:\n%s", want, out)
		}
	}
}

// ── T14: write-time guidance stays off the always-on core ───────────────────

// D37: governance.md declares `paths: "**/*"` and is therefore loaded on every
// single edit. Reminders and verification items are artifact-scoped guidance;
// carrying them here loaded all of them regardless of what was being touched,
// which is why they moved to the topic files (T7 asserts they arrive there).
// IndexView still ACCEPTS both fields — phaseb populates them — so nothing but
// a test stands between a template edit and their silent return.
func TestRender_IndexOmitsWriteTimeGuidance(t *testing.T) {
	out := mustRenderIndex(t, IndexView{
		Reminders:    []string{"REMINDER-CANARY-must-not-appear"},
		Verification: []string{"VERIFICATION-CANARY-must-not-appear"},
	})
	for _, canary := range []string{"REMINDER-CANARY-must-not-appear", "VERIFICATION-CANARY-must-not-appear"} {
		if strings.Contains(out, canary) {
			t.Errorf("write-time guidance reached the always-on core (%s):\n%s", canary, out)
		}
	}
	if strings.Contains(out, "## Reminders") || strings.Contains(out, "## Verification Checklist") {
		t.Errorf("the ambient core grew a write-time guidance section:\n%s", out)
	}
}

// ── T15: heading case ───────────────────────────────────────────────────────

func TestRender_TitleCasesTheTopicHeading(t *testing.T) {
	for in, want := range map[string]string{
		"agent-rules":          "Agent-Rules",
		"governance-authoring": "Governance-Authoring",
		"compile":              "Compile",
		"a_b c":                "A_B C",
		"":                     "",
		"1st-thing":            "1st-Thing",
	} {
		if got := titleCase(in); got != want {
			t.Errorf("titleCase(%q) = %q, want %q", in, got, want)
		}
	}
	out := mustRenderTopic(t, TopicView{Name: "agent-rules"})
	if !strings.Contains(out, "\n# Agent-Rules\n") {
		t.Errorf("topic heading not title-cased:\n%s", out)
	}
}

// ── T16: the renderer fingerprint ───────────────────────────────────────────

// The fingerprint exists because the topic-file cache was keyed on its INPUTS
// and never on the RENDERER: editing a template changed what render would
// produce while every cached file kept its old bytes. So the digest has to
// cover BOTH templates, and the test recomputes it from the template files on
// disk rather than from a frozen literal — a hard-coded expected digest would
// have to be updated by hand on every template edit, and "update the constant"
// is exactly how a fingerprint stops tracking what it fingerprints.
func TestRender_FingerprintCoversBothTemplates(t *testing.T) {
	topicSrc, err := os.ReadFile(filepath.Join("templates", "topic.md.tmpl"))
	if err != nil {
		t.Fatalf("read topic template: %v", err)
	}
	indexSrc, err := os.ReadFile(filepath.Join("templates", "index.md.tmpl"))
	if err != nil {
		t.Fatalf("read index template: %v", err)
	}

	h := sha256.New()
	h.Write([]byte("topic.md.tmpl\x00"))
	h.Write(topicSrc)
	h.Write([]byte("\x00index.md.tmpl\x00"))
	h.Write(indexSrc)
	want := hex.EncodeToString(h.Sum(nil))

	got := RendererFingerprint()
	if got != want {
		t.Errorf("RendererFingerprint() = %s, want %s — the digest does not cover both templates "+
			"as documented (domain-separated, topic then index)", got, want)
	}
	if got != RendererFingerprint() {
		t.Error("RendererFingerprint() is not stable across calls")
	}

	// Order-fixed and domain-separated: swapping the two bodies must not
	// produce the same digest, or a template rename could hide an edit.
	swapped := sha256.New()
	swapped.Write([]byte("topic.md.tmpl\x00"))
	swapped.Write(indexSrc)
	swapped.Write([]byte("\x00index.md.tmpl\x00"))
	swapped.Write(topicSrc)
	if hex.EncodeToString(swapped.Sum(nil)) == got {
		t.Error("swapping the template bodies produced the same fingerprint")
	}
}

// ── T17: compiled_at is deliberately not rendered ───────────────────────────

// Both views accept CompiledAt and neither template emits it. That is not an
// oversight: a timestamp in the rendered bytes makes "hash of what would
// render" differ from "hash on disk" on every run, which is the defect
// RendererFingerprint's doc comment records — the removal propagated to zero
// of ten topic files until the fingerprint covered the renderer.
func TestRender_CompiledAtNeverReachesTheOutput(t *testing.T) {
	a := mustRenderTopic(t, TopicView{Name: "compile", CompiledAt: "2026-01-01T00:00:00Z"})
	b := mustRenderTopic(t, TopicView{Name: "compile", CompiledAt: "2031-12-25T13:45:59Z"})
	if a != b {
		t.Error("CompiledAt changed the topic bytes — a timestamp in the output makes every compile a diff")
	}
	if strings.Contains(a, "2026-01-01") || strings.Contains(a, "compiled_at") {
		t.Errorf("topic output carries a compile timestamp:\n%s", a)
	}

	ia := mustRenderIndex(t, IndexView{CompiledAt: "2026-01-01T00:00:00Z"})
	ib := mustRenderIndex(t, IndexView{CompiledAt: "2031-12-25T13:45:59Z"})
	if ia != ib {
		t.Error("CompiledAt changed the index bytes")
	}
	if strings.Contains(ia, "2026-01-01") || strings.Contains(ia, "compiled_at") {
		t.Errorf("index output carries a compile timestamp:\n%s", ia)
	}
}

// ── T18: the anchor follows the template's region structure ─────────────────

// This is the criterion that separates "the renderer hashes what it emits"
// from "the anchor happens to be right today" (D357 option D vs option A).
//
// Under a scheme where the anchor is a constant, or is computed by anyone from
// a MODEL of what the template contains, editing the template silently
// desynchronises every anchor and nothing notices — that is precisely what
// happened: `## Prohibitions` lived in the template, in phaseb.regionSHA's
// withProhibitionsHeading flag, and in render.go's EmptySHA default, and the
// third copy was wrong. Under option D it cannot happen, and the way to show
// that is to render through an ALTERED template and watch the anchor move.
func TestRender_AnchorFollowsTemplateStructure(t *testing.T) {
	const heading = "## Prohibitions"
	altered := strings.Replace(topicTmplSrc, heading, "## Never Do This", 1)
	if altered == topicTmplSrc {
		t.Fatalf("topic.md.tmpl no longer contains %q — this test would measure nothing", heading)
	}

	v := TopicView{
		Name:             "compile",
		DirectiveLines:   []string{"cite ADR-NNN when a decision applies"},
		ProhibitionLines: []string{"MUST NOT edit a compiled surface by hand"},
		ManualLines:      []string{"the extractor missed this one"},
	}
	base := mustRenderTopic(t, v)
	alt, err := renderTopic(altered, v)
	if err != nil {
		t.Fatalf("renderTopic through the altered template: %v", err)
	}

	// The bytes inside the prohibitions markers changed...
	if emittedRegion(t, base, "prohibitions") == emittedRegion(t, alt, "prohibitions") {
		t.Fatal("the altered template rendered the same prohibitions body — the mutation did not take")
	}
	// ...so that region's anchor must have changed with them.
	if declaredAnchor(t, base, "prohibitions") == declaredAnchor(t, alt, "prohibitions") {
		t.Errorf("the in-marker heading changed but the prohibitions anchor did not (%s) — "+
			"the anchor is not derived from the bytes emitted",
			declaredAnchor(t, base, "prohibitions"))
	}
	// And no OTHER region's anchor moved: each anchor covers its own region,
	// not the file.
	for _, region := range []string{"directives", "manual"} {
		if declaredAnchor(t, base, region) != declaredAnchor(t, alt, region) {
			t.Errorf("editing the prohibitions region moved the %q anchor: %s → %s",
				region, declaredAnchor(t, base, region), declaredAnchor(t, alt, region))
		}
	}
	// Both renders remain self-describing: every anchor is the hash of the
	// region it names, under either template.
	for _, out := range []struct {
		name string
		body string
	}{{"shipped template", base}, {"altered template", alt}} {
		for _, region := range []string{"directives", "prohibitions", "manual"} {
			body := emittedRegion(t, out.body, region)
			if got, want := declaredAnchor(t, out.body, region), RegionSHA(body); got != want {
				t.Errorf("%s: region %q declares %s, its own bytes hash to %s", out.name, region, got, want)
			}
		}
	}
}

// ── T19: the error half of both exported signatures ─────────────────────────

// RenderTopic and RenderIndex are (string, error), and Phase 1 enumerated
// neither half of that pair — covered or uncovered. The gap was not
// theoretical: making the Execute-failure path return buf.String() with a NIL
// error left the whole suite green. Partial output and no error is the
// plausible-looking result, which is the failure mode this package's tests
// exist for. A truncated topic file reads as a complete one.
//
// The seams are the unexported renderTopic/renderIndex, because a broken
// template is the only way to reach these branches and the shipped templates
// are — correctly — not broken.
func TestRender_ErrorHalfNeverReturnsPartialOutput(t *testing.T) {
	const canary = "PARTIAL-OUTPUT-CANARY"

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"template does not parse", "{{ if }}"},
		{"execute fails after output was already buffered", canary + "\n{{ index .Paths 7 }}\n"},
		{"a managed region with no anchor line to write into",
			"[edikt:directives:start]: #\n- a\n[edikt:directives:end]: #\n" + canary + "\n"},
		{"two anchor lines for one region",
			"[edikt:directives:start]: #\n- a\n[edikt:directives:end]: #\n" +
				"[edikt:directives:sha256]: #\n[edikt:directives:sha256]: #\n"},
		{"an anchor line with no region to describe",
			canary + "\n[edikt:directives:sha256]: #\n"},
		{"a start marker with no end marker",
			"[edikt:directives:start]: #\n- a\n[edikt:directives:sha256]: #\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, err := renderTopic(tc.src, TopicView{Name: "compile"})
			if err == nil {
				t.Fatalf("renderTopic returned a nil error for a template that cannot render; output was:\n%s", out)
			}
			if out != "" {
				t.Errorf("renderTopic returned %d byte(s) alongside its error — a partial surface escaped:\n%s",
					len(out), out)
			}
			if strings.Contains(out, canary) {
				t.Errorf("the buffered prefix reached the caller despite the error:\n%s", out)
			}
		})
	}

	for _, tc := range []struct {
		name string
		src  string
	}{
		{"template does not parse", "{{ if }}"},
		{"execute fails after output was already buffered", canary + "\n{{ index .InvariantRules 7 }}\n"},
		{"an anchor line with no region to describe", canary + "\n[edikt:manual:sha256]: #\n"},
	} {
		t.Run("index/"+tc.name, func(t *testing.T) {
			out, err := renderIndex(tc.src, IndexView{})
			if err == nil {
				t.Fatalf("renderIndex returned a nil error for a template that cannot render; output was:\n%s", out)
			}
			if out != "" {
				t.Errorf("renderIndex returned %d byte(s) alongside its error:\n%s", len(out), out)
			}
		})
	}

	// The shipped templates render clean — otherwise the assertions above
	// would be measuring a renderer that always errors.
	if _, err := RenderTopic(TopicView{Name: "compile"}); err != nil {
		t.Errorf("RenderTopic (string, error) returned an error for the shipped template: %v", err)
	}
	if _, err := RenderIndex(IndexView{}); err != nil {
		t.Errorf("RenderIndex (string, error) returned an error for the shipped template: %v", err)
	}
}
