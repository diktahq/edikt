// Package surfaces had ZERO test reach until this file: no _test.go of its own,
// and no test anywhere in the tree referenced it. It is 187 lines standing
// between every consumer and the surface it needs to read, and its failure mode
// is the quiet one — a consumer that cannot find its input and carries on as
// though the input said nothing. `hook match` reports "no directives apply";
// doctor iterates an empty list and finds nothing wrong.
//
// That is globmatch's shape (PLAN-test-reach-on-the-delivery-path, phase 2):
// silence rather than a crash. So the doc comments and the exported signatures
// of surfaces.go are treated here as the specification, and each capability
// they claim is enumerated below and recorded covered or uncovered. The count
// of tests in this file is not the deliverable; the enumeration is.
//
// ── CAPABILITY ENUMERATION (source: surfaces.go doc comments + exported API) ─
//
//	#   capability (as the package claims it)                       status
//	--- ---------------------------------------------------------- ---------
//	 1  pkg doc: resolution goes through the MANIFEST, never an     COVERED (T3)
//	    assumed directory layout — "the manifest is the contract".
//	    Every surface in T3 sits at a path convention could not
//	    guess, so a resolution that fell back could not pass it
//	 2  pkg doc: "A MISSING SURFACE IS AN ERROR, NEVER AN EMPTY     COVERED (T4)
//	    PATH", half one — the manifest carries no entry of the
//	    requested kind, so the surface was NOT RENDERED: *ErrNoSurface
//	    and an empty path, never a guess
//	 3  the same claim, half two — the manifest NAMES a surface     COVERED (T5)
//	    that is not on disk. That is a corrupt render, and it is
//	    reported as a broken contract naming the kind and the path,
//	    NOT as ErrNoSurface: "the render is broken" and "this
//	    surface was not rendered" are different facts (INV-013)
//	 4  pkg doc: "FALLBACK IS DELIBERATE AND NARROW" — with NO      COVERED (T6)
//	    manifest at all, all four kinds resolve to the conventional
//	    path, because refusing there bricks every consumer on
//	    upgrade. A topic-scoped kind with no topic, and an unknown
//	    kind, have no conventional path and say so rather than
//	    return "". No manifest AND no file names the path it tried
//	 5  pkg doc: when a manifest EXISTS it is AUTHORITATIVE — there COVERED (T4)
//	    is no fallback. This is what makes row 2 a kill and not a
//	    preference: T4's conventional paths all EXIST on disk
//	 6  Load: an ABSENT manifest is not an error (Present=false),   COVERED (T1)
//	    and `surfaces: []` is a MEASURED ZERO (Present=true, no
//	    entries). Present is the flag the whole fallback keys off,
//	    so collapsing the two collapses rows 4 and 5
//	 7  Load: Manifest.Path is set even when nothing was there —    COVERED (T1)
//	    a consumer reporting where it looked needs the path most
//	    when the file was missing
//	 8  Load's ERROR HALF: a manifest that EXISTS and parses to     COVERED (T2)
//	    zero surfaces without declaring an empty set is refused,
//	    and a path that cannot be read as a file errors. Both
//	    return a NIL Manifest — a usable-looking empty value is an
//	    invitation to carry on
//	 9  ErrNoSurface has two message forms and the topic-scoped     COVERED (T7)
//	    one names the TOPIC: "no topic-file surface" cannot say
//	    which of ten topics went missing
//	10  ErrNoSurface is reachable by a caller through errors.As —   COVERED (T4,
//	    the discrimination in rows 2, 3 and 8 is worth nothing if   T5, T6, T8)
//	    the only way to make it is to match on message text. Every
//	    test here asks the way a consumer would
//	11  Resolve's topic filter: two topics of one kind do not       COVERED (T3)
//	    collapse into each other
//	12  ResolveKind is the one-shot form and must answer            COVERED (T4)
//	    IDENTICALLY to Load+Resolve. A helper that quietly took the
//	    other branch would be a second contract nothing compares
//	13  ResolveKind's ERROR HALF: a Load failure reaches the        COVERED (T8)
//	    caller and does not become ErrNoSurface. This is where it
//	    matters — hookmatch and doctor call ResolveKind precisely
//	    because they do not hold a Manifest
//	14  PathsOfKind returns every surface of the kind and none of   COVERED (T9)
//	    any other, root-joined and SORTED, so two runs over one
//	    unchanged tree cannot disagree
//	15  PathsOfKind's no-manifest fallback globs the conventional   COVERED (T10)
//	    layout for topic files and skill packages — the iterating
//	    consumers get the same upgrade grace as the resolving ones
//	16  the manifest phaseb WRITES is the manifest this package     COVERED (T11)
//	    READS: built by phaseb.BuildManifest, serialised by
//	    phaseb.RenderManifest, loaded and resolved here, including
//	    that the manifest excludes ITSELF and so must not resolve.
//	    The two halves live in different packages and agree only by
//	    convention, so every fixture in this file is written through
//	    phaseb's own writer rather than by hand
//	17  row 2's promise — never a path that is not there —          COVERED (T12)
//	    binds the ENUMERATING form too: PathsOfKind stats every
//	    entry it lists and reports EVERY absent one, in the same
//	    words Resolve uses, through the same helper. Recorded
//	    UNCOVERED by phase 2's first pass, which was right: Resolve
//	    statted and PathsOfKind did not, and a test asserting that
//	    would have frozen the divergence into the contract. Closed
//	    under AC-2.3 by making the code honour the doc rather than
//	    by narrowing the doc. The consumer half — doctor's routed-
//	    sources check, the caller that ITERATES — is asserted in
//	    cmd by TestRoutedSources_ManifestNamingAMissingSurfaceIsCounted
//	18  PathsOfKind's no-manifest fallback covers the SAME FOUR     COVERED (T12)
//	    KINDS Resolve falls back for. It returned nothing for
//	    ambient-core and delivery-map — a silent empty on the
//	    branch that exists to avoid one. Closed with row 17
//	19  the four Kind constants say they are "as written by the     COVERED (T13)
//	    renderer into the manifest", and T11 cannot see a rename
//	    because it feeds phaseb the kinds from these very
//	    constants. T13 runs a REAL compile — phaseb.Merge, whose
//	    kind literals sit inline in its own source — and asserts
//	    every kind it wrote has a constant here and every constant
//	    was written. phaseb also emits a fifth kind, "manifest",
//	    that this package has no constant for — correctly, because
//	    BuildManifest drops it, which T11 asserts and T13 inherits
//	20  Manifest.rootPath is unexported while Manifest is exported, UNCOVERED
//	    so a Manifest built as a struct literal outside this
//	    package resolves against "". Not a claimed capability;
//	    recorded because the exported struct invites the literal
//	21  filepath.Glob's error is discarded in the fallback.         UNCOVERED
//	    Glob errors only on a malformed pattern and both patterns
//	    are constants, so there is no input that reaches it
//	22  PathsOfKind returns an EMPTY SET rather than an error when  COVERED (T12)
//	    nothing of the kind is there, where Resolve errors. That is
//	    the one place the two forms legitimately differ, and it is
//	    the reason row 17's promise is "nothing handed back is
//	    absent" and not "never empty": a consumer that iterates can
//	    act on "none", and one that asked for a single surface
//	    cannot
//
// ── FALSIFICATION RECORD ─────────────────────────────────────────────────────
//
// Every test below was falsified by mutation before it was trusted: the
// capability it covers was broken in surfaces.go — or, for T11 and T13, in the
// OTHER package that produces the manifest — the suite was run, and the test
// went RED. A test that passes against both the working and the broken
// implementation is not reach. Each mutation was applied by a script that
// refuses unless the file matches its backup byte for byte, and reverted from
// that backup; the restored hash was checked every time.
//
// M1-M11 were first measured 2026-09-06 against surfaces.go sha256 461356d8….
// AC-2.3's close CHANGED that file, so that record described a revision no
// longer on disk. THE WHOLE SWEEP WAS RE-RUN, not re-cited: M1-M11 and the new
// M12-M19 were all measured 2026-09-07 against the files below, and every one
// is still RED. A falsification record naming a file it was never run against
// is prose, which is the failure AC-6.7 recorded against phase 1's record.
//
// Measured with `go test -count=1 ./internal/surfaces/ -run TestSurfaces` and,
// for the consumer-side mutations, `-run TestRoutedSources` in ./cmd/, before
// the sweep and after every one of the nineteen reverts:
//
//	surfaces.go              sha256 1d1ebed1…
//	phaseb/manifest.go       sha256 e7877491…
//	phaseb/merge.go          sha256 36187ae0…
//	cmd/doctor_routed_sources.go sha256 5b612e3e…
//
// CITATION RULE for this record, inherited from render_test.go (AC-6.8): a kill
// is cited by STABLE IDENTIFIER — the Go test name and the assertion's own
// message text — NEVER by a line number into this file. A self-referential line
// number is stale by construction the moment anything above it moves, and it
// breaks SILENTLY. That is INV-016 at the citation layer, and it is why the
// mutations below are named by what they changed rather than by where.
//
//	M1  Load reports a MISSING manifest as Present — absence
//	    collapses into a measured zero                         → RED
//	    TestSurfaces_LoadSeparatesAbsentFromMeasuredEmpty,
//	    _FallbackAppliesOnlyWhenNoManifestExists and
//	    _PathsOfKindGlobsTheConventionalLayoutWithNoManifest.
//	    Rows 4, 5 and 15 all hang off this one flag
//	M2  Load drops the zero-surfaces refusal and hands back an
//	    empty manifest for one it could not parse              → RED
//	    TestSurfaces_LoadRefusesAManifestItCannotRead and
//	    _ResolveKindPropagatesLoadFailure
//	M3  Resolve ignores the entry's Path and joins the
//	    conventional layout instead — the layout assumption
//	    this package exists to remove, reintroduced inside it   → RED
//	    TestSurfaces_ResolvesEveryKindThroughTheManifest and
//	    _MissingEntryIsCaught
//	M4  THE SILENT-OMISSION MUTATION (AC-2.2). A manifest with
//	    no entry for the kind falls through to the conventional
//	    path, so an unrendered surface resolves to a stale file
//	    left by an older compile: a believable answer, no error → RED
//	    TestSurfaces_MissingEntryIsCaught ALONE — nothing else
//	    in the package moved. It fired on all four kinds, cited
//	    by message text: `the manifest carries no %q entry for
//	    topic %q, yet Resolve returned %q with no error — a
//	    surface that was never rendered was reported as found`,
//	    for topic-file/"beta", skill-package/"beta",
//	    ambient-core and delivery-map
//	M5  a manifest entry whose file is missing reports
//	    ErrNoSurface — a corrupt render reads as "not rendered" → RED
//	    TestSurfaces_ManifestNamingAFileNotOnDiskIsABrokenContract
//	M6  the no-manifest fallback is removed                     → RED
//	    TestSurfaces_FallbackAppliesOnlyWhenNoManifestExists
//	M7  ErrNoSurface.Error drops the topic from its message     → RED
//	    TestSurfaces_ErrNoSurfaceNamesWhatItLookedFor
//	M8  ResolveKind swallows the Load failure and returns
//	    ErrNoSurface instead                                    → RED
//	    TestSurfaces_ResolveKindPropagatesLoadFailure
//	M9  PathsOfKind stops sorting                               → RED
//	    TestSurfaces_PathsOfKindListsEveryMatchOfThatKindSorted
//	M10 PathsOfKind's fallback stops globbing skill packages —
//	    an iterating consumer sees an empty corpus              → RED
//	    TestSurfaces_PathsOfKindGlobsTheConventionalLayoutWithNoManifest
//	M11 IN phaseb, NOT HERE: RenderManifest stops emitting the
//	    `topic:` line. Producer-side drift, invisible to this
//	    package's own source                                    → RED
//	    TestSurfaces_RoundTripsWhatTheCompilerWrote, and also
//	    _ResolvesEveryKindThroughTheManifest, _MissingEntryIsCaught
//	    and _ManifestNamingAFileNotOnDiskIsABrokenContract —
//	    because every fixture here is written through phaseb's
//	    writer. That breadth IS the result: a hand-built YAML
//	    fixture would have left all four green while the
//	    compiler's output moved away underneath them
//
//	── AC-2.3: the doc semantic the code did not implement ──
//
//	M12 PathsOfKind stops statting what the manifest lists and
//	    joins the root to the entry, as it did before this close  → RED
//	    TestSurfaces_PathsOfKindHonoursTheExistenceContract ALONE
//	    — nothing else in the package moved. This is the defect
//	    itself, replayed: `PathsOfKind returned [.../alpha.md
//	    .../beta.md .../gamma.md] with no error`
//	M13 the no-manifest fallback drops its ambient-core and
//	    delivery-map case, so the iterating form goes empty where
//	    the resolving one falls back                              → RED
//	    TestSurfaces_PathsOfKindHonoursTheExistenceContract, cited
//	    by message text: `no manifest, PathsOfKind("ambient-core")
//	    = [], want [...]` and the same for "delivery-map"
//	M14 only the FIRST absent entry is reported, not every one    → RED
//	    TestSurfaces_PathsOfKindHonoursTheExistenceContract:
//	    `does not name "out/gamma.md"`. Applied as
//	    errors.Join(problems[0]) rather than by deleting the join,
//	    because deleting it left "errors" unused and the package
//	    failed to BUILD — a build break is not a kill, and the
//	    first attempt at this mutation was discarded for that
//	M15 PathsOfKind returns the paths it did stat ALONGSIDE the
//	    error instead of nil — a partial list a ranging consumer
//	    reports on as though it were the whole corpus            → RED
//	    TestSurfaces_PathsOfKindHonoursTheExistenceContract:
//	    `returned [...] alongside its error, want nil`
//	M16 IN phaseb, NOT HERE: the compile writes its skill surface
//	    as kind "skill-pkg". A renamed kind is silent by
//	    construction — every consumer asking for the old name
//	    gets ErrNoSurface, which reads as "not rendered"          → RED
//	    TestSurfaces_KindConstantsAreWhatTheCompilerWrites, both
//	    halves: the unknown kind seen, and the constant unwritten
//	M17 IN cmd, NOT HERE: doctor's routed-sources check discards
//	    the PathsOfKind error and carries on with what it has     → RED
//	    TestRoutedSources_ManifestNamingAMissingSurfaceIsCounted
//	M18 IN cmd, NOT HERE: the all-sources-resolve path returns
//	    `0, 0, true`, discarding the errors it just printed — the
//	    broken manifest becomes a cosmetic message with a clean
//	    exit code                                                 → RED
//	    TestRoutedSources_ManifestNamingAMissingSurfaceIsCounted
//	M19 PathsOfKind grows its OWN wording for the refusal instead
//	    of going through the shared helper — a second spelling of
//	    one fact, which is the shape that produced this defect     → RED
//	    TestSurfaces_PathsOfKindHonoursTheExistenceContract:
//	    `the two exported forms are wording one fact differently`
//
// M4 is the one to read twice. It is the phase's whole question — would any
// test fail if this stopped working — and the answer before this file existed
// was no: the mutation compiles, the suite was green, and every consumer would
// have gone on reading a file the last compile did not produce.
//
// M12 is M4 one exported function along, and it is why AC-2.3 exists: the
// package doc made row 2's promise for the whole package, one of the two forms
// kept it, and the doc was believed for both.
//
// The full working tree was green immediately before and after the sweep.
package surfaces

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ── helpers ─────────────────────────────────────────────────────────────────

// writeFile creates a file and every parent it needs, and fails loudly rather
// than returning an error the caller might ignore: a fixture that silently
// failed to appear would make an absence test pass for the wrong reason.
func writeFile(t *testing.T, path, body string) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

// writeManifest serialises entries through phaseb.RenderManifest — the SAME
// writer the compiler uses — rather than through a hand-built YAML string in
// this file. A hand-built fixture would test this package against a format
// nothing produces, which is how a parser and its producer drift apart while
// both suites stay green.
func writeManifest(t *testing.T, root string, entries []phaseb.ManifestEntry) {
	t.Helper()
	writeFile(t, filepath.Join(root, ManifestRelPath), phaseb.RenderManifest(entries))
}

// conventional paths, spelled out here rather than read from the package's own
// conventionalPath. The fallback contract is "the path consumers hardcoded
// before the manifest existed"; asking the package where it thinks that is
// would let a mutation move both sides together (INV-014).
func convCore(root string) string { return filepath.Join(root, ".claude", "rules", "governance.md") }
func convMap(root string) string {
	return filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml")
}
func convTopic(root, topic string) string {
	return filepath.Join(root, ".claude", "rules", "governance", topic+".md")
}
func convSkill(root, topic string) string {
	return filepath.Join(root, ".claude", "skills", "edikt-"+topic, "SKILL.md")
}

func mustLoad(t *testing.T, root string) *Manifest {
	t.Helper()
	m, err := Load(root)
	if err != nil {
		t.Fatalf("Load(%s): unexpected error %v", root, err)
	}
	if m == nil {
		t.Fatalf("Load(%s): nil manifest and nil error", root)
	}
	return m
}

// isNoSurface reports whether err is the "the manifest says this surface does
// not exist" sentinel, as a CALLER would ask — through errors.As on the
// exported type, not by string-matching the message.
func isNoSurface(err error) bool {
	var e *ErrNoSurface
	return errors.As(err, &e)
}

// ── T1 ──────────────────────────────────────────────────────────────────────

// A manifest that is ABSENT and a manifest that measured ZERO surfaces are
// different facts, and Load must not collapse them: Present is the flag the
// entire fallback contract keys off.
func TestSurfaces_LoadSeparatesAbsentFromMeasuredEmpty(t *testing.T) {
	absent := t.TempDir()
	m := mustLoad(t, absent)
	if m.Present {
		t.Errorf("no manifest on disk: Present = true, want false")
	}
	if len(m.Entries) != 0 {
		t.Errorf("no manifest on disk: got %d entries, want 0", len(m.Entries))
	}
	if want := filepath.Join(absent, ManifestRelPath); m.Path != want {
		t.Errorf("Path = %q, want %q — a caller reporting where it looked needs the path even when nothing was there", m.Path, want)
	}

	empty := t.TempDir()
	writeManifest(t, empty, nil)
	m2 := mustLoad(t, empty)
	if !m2.Present {
		t.Errorf("a manifest declaring `surfaces: []`: Present = false, want true — a measured zero is not an absence (INV-013)")
	}
	if len(m2.Entries) != 0 {
		t.Errorf("`surfaces: []`: got %d entries, want 0", len(m2.Entries))
	}
}

// ── T2 ──────────────────────────────────────────────────────────────────────

// A manifest that EXISTS and cannot be understood is a tampered or corrupt
// contract, and Load refuses it. Returning an empty manifest here would make
// every consumer fall back to the layout assumption this package removes, on
// a project whose manifest is broken — the worst moment to guess.
func TestSurfaces_LoadRefusesAManifestItCannotRead(t *testing.T) {
	garbage := t.TempDir()
	writeFile(t, filepath.Join(garbage, ManifestRelPath), "this is not a manifest\nkind: topic-file\n")
	m, err := Load(garbage)
	if err == nil {
		t.Fatalf("a manifest that parsed to zero surfaces without declaring `surfaces: []`: got no error (manifest %+v), want a refusal", m)
	}
	if m != nil {
		t.Errorf("refused manifest: got a non-nil Manifest %+v, want nil — a usable-looking value invites the caller to carry on", m)
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("refusal message = %q, want it to say it is refusing rather than reporting an empty corpus", err.Error())
	}
	if isNoSurface(err) {
		t.Errorf("an unreadable manifest reported as ErrNoSurface — %q; that sentinel means the surface was not rendered, which is a different fact", err)
	}

	unreadable := t.TempDir()
	if err := os.MkdirAll(filepath.Join(unreadable, ManifestRelPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	m2, err2 := Load(unreadable)
	if err2 == nil {
		t.Fatalf("a manifest path that cannot be read as a file: got no error (manifest %+v), want one", m2)
	}
	if m2 != nil {
		t.Errorf("unreadable manifest: got %+v, want nil", m2)
	}
}

// ── T3 ──────────────────────────────────────────────────────────────────────

// The manifest is the contract, not the directory layout: every surface here
// sits at a path convention would never guess, so a resolution that happened
// to work by falling back to convention cannot pass this test.
func TestSurfaces_ResolvesEveryKindThroughTheManifest(t *testing.T) {
	root := t.TempDir()
	entries := []phaseb.ManifestEntry{
		{Kind: KindAmbientCore, Path: "elsewhere/core.md"},
		{Kind: KindDeliveryMap, Path: "elsewhere/routing.yaml"},
		{Kind: KindTopicFile, Path: "elsewhere/topics/agent-rules.md", Topic: "agent-rules"},
		{Kind: KindTopicFile, Path: "elsewhere/topics/security.md", Topic: "security"},
		{Kind: KindSkillPackage, Path: "elsewhere/skills/agent-rules.md", Topic: "agent-rules"},
	}
	for _, e := range entries {
		writeFile(t, filepath.Join(root, e.Path), "body\n")
	}
	writeManifest(t, root, entries)
	m := mustLoad(t, root)

	for _, e := range entries {
		got, err := m.Resolve(e.Kind, e.Topic)
		if err != nil {
			t.Errorf("Resolve(%q, %q): unexpected error %v", e.Kind, e.Topic, err)
			continue
		}
		if want := filepath.Join(root, e.Path); got != want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", e.Kind, e.Topic, got, want)
		}
	}

	// Two topics of one kind must not collapse into each other: the topic
	// filter is what makes a per-topic surface addressable at all.
	sec, err := m.Resolve(KindTopicFile, "security")
	if err != nil {
		t.Fatalf("Resolve(topic-file, security): %v", err)
	}
	if strings.Contains(sec, "agent-rules") {
		t.Errorf("Resolve(topic-file, security) = %q — it returned another topic's surface", sec)
	}
}

// ── T4 ──────────────────────────────────────────────────────────────────────

// THE SILENT-OMISSION TEST (AC-2.2).
//
// A compile that failed to render a surface leaves a manifest with no entry
// for it. Every conventional path a fallback would guess EXISTS on disk here,
// so a resolution that reaches for convention returns a plausible, readable,
// wrong file and no error — the shape that let globmatch ship: not a crash, a
// believable answer. The manifest is authoritative, and an absent entry means
// the surface was not rendered.
func TestSurfaces_MissingEntryIsCaught(t *testing.T) {
	root := t.TempDir()

	// Rendered, and listed.
	present := phaseb.ManifestEntry{Kind: KindTopicFile, Path: "out/alpha.md", Topic: "alpha"}
	writeFile(t, filepath.Join(root, present.Path), "alpha\n")
	writeManifest(t, root, []phaseb.ManifestEntry{present})

	// NOT rendered, NOT listed — but sitting at every path convention would
	// look for, left behind by an older compile.
	writeFile(t, convTopic(root, "beta"), "stale beta topic\n")
	writeFile(t, convSkill(root, "beta"), "stale beta skill\n")
	writeFile(t, convCore(root), "stale ambient core\n")
	writeFile(t, convMap(root), "stale delivery map\n")

	m := mustLoad(t, root)

	omitted := []struct {
		kind  string
		topic string
	}{
		{KindTopicFile, "beta"},
		{KindSkillPackage, "beta"},
		{KindAmbientCore, ""},
		{KindDeliveryMap, ""},
	}
	for _, o := range omitted {
		got, err := m.Resolve(o.kind, o.topic)
		if err == nil {
			t.Errorf("the manifest carries no %q entry for topic %q, yet Resolve returned %q with no error — a surface that was never rendered was reported as found", o.kind, o.topic, got)
			continue
		}
		if got != "" {
			t.Errorf("Resolve(%q, %q) returned path %q alongside its error — a caller that checks the path before the error gets a stale file", o.kind, o.topic, got)
		}
		if !isNoSurface(err) {
			t.Errorf("Resolve(%q, %q) error = %v, want *ErrNoSurface — the caller cannot tell 'not rendered' from 'could not read'", o.kind, o.topic, err)
		}
		// The one-shot form must answer identically. A helper that quietly
		// took the other branch would be an undetectable second contract.
		if _, err := ResolveKind(root, o.kind, o.topic); !isNoSurface(err) {
			t.Errorf("ResolveKind(%q, %q) error = %v, want *ErrNoSurface — it disagrees with Manifest.Resolve", o.kind, o.topic, err)
		}
	}

	// The listed surface still resolves: this test must fail on the omission,
	// not because resolution stopped working altogether.
	if got, err := m.Resolve(KindTopicFile, "alpha"); err != nil || got != filepath.Join(root, present.Path) {
		t.Errorf("Resolve(topic-file, alpha) = %q, %v — want the listed surface and no error", got, err)
	}
}

// ── T5 ──────────────────────────────────────────────────────────────────────

// The other half of "a missing surface is an error, never an empty path": the
// manifest names a surface that is not on disk. That is a BROKEN RENDER, and
// reporting it as ErrNoSurface would let a consumer treat a corrupt compile as
// a corpus with nothing to say.
func TestSurfaces_ManifestNamingAFileNotOnDiskIsABrokenContract(t *testing.T) {
	root := t.TempDir()
	e := phaseb.ManifestEntry{Kind: KindTopicFile, Path: "out/agent-rules.md", Topic: "agent-rules"}
	writeManifest(t, root, []phaseb.ManifestEntry{e}) // the entry, but never the file
	m := mustLoad(t, root)

	got, err := m.Resolve(e.Kind, e.Topic)
	if err == nil {
		t.Fatalf("manifest lists a surface that is not on disk, yet Resolve returned %q with no error", got)
	}
	if got != "" {
		t.Errorf("Resolve returned path %q alongside its error, want \"\"", got)
	}
	if isNoSurface(err) {
		t.Errorf("error = %v, want a broken-contract error rather than *ErrNoSurface — 'the render is corrupt' must not read as 'this surface was not rendered'", err)
	}
	for _, want := range []string{e.Path, "not on disk"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not name %q — the message is the only record of what it looked for", err.Error(), want)
		}
	}
}

// ── T6 ──────────────────────────────────────────────────────────────────────

// Fallback is deliberate and NARROW: it applies when there is no manifest at
// all, so an upgrade does not brick every consumer at once.
func TestSurfaces_FallbackAppliesOnlyWhenNoManifestExists(t *testing.T) {
	root := t.TempDir()
	want := map[[2]string]string{
		{KindAmbientCore, ""}:             writeFile(t, convCore(root), "core\n"),
		{KindDeliveryMap, ""}:             writeFile(t, convMap(root), "map\n"),
		{KindTopicFile, "agent-rules"}:    writeFile(t, convTopic(root, "agent-rules"), "topic\n"),
		{KindSkillPackage, "agent-rules"}: writeFile(t, convSkill(root, "agent-rules"), "skill\n"),
	}
	m := mustLoad(t, root)
	if m.Present {
		t.Fatalf("fixture error: a manifest exists where none was written")
	}
	for k, wantPath := range want {
		got, err := m.Resolve(k[0], k[1])
		if err != nil {
			t.Errorf("no manifest, Resolve(%q, %q): unexpected error %v — refusing here bricks every consumer on upgrade", k[0], k[1], err)
			continue
		}
		if got != wantPath {
			t.Errorf("no manifest, Resolve(%q, %q) = %q, want the conventional path %q", k[0], k[1], got, wantPath)
		}
	}

	// A topic-scoped kind with no topic has no conventional path to guess, and
	// an unknown kind has none either. Both must say so rather than return "".
	for _, k := range [][2]string{
		{KindTopicFile, ""},
		{KindSkillPackage, ""},
		{"directive-index", ""},
	} {
		got, err := m.Resolve(k[0], k[1])
		if err == nil {
			t.Errorf("no manifest, Resolve(%q, %q) = %q with no error — there is no conventional path for it to have found", k[0], k[1], got)
			continue
		}
		if !isNoSurface(err) {
			t.Errorf("no manifest, Resolve(%q, %q) error = %v, want *ErrNoSurface", k[0], k[1], err)
		}
	}

	// No manifest AND no file: an error naming the conventional path it tried.
	bare := t.TempDir()
	m2 := mustLoad(t, bare)
	got, err := m2.Resolve(KindAmbientCore, "")
	if err == nil {
		t.Fatalf("no manifest and no surface: Resolve returned %q with no error", got)
	}
	if got != "" {
		t.Errorf("Resolve returned %q alongside its error, want \"\"", got)
	}
	if !strings.Contains(err.Error(), ".claude/rules/governance.md") {
		t.Errorf("error %q does not name the conventional path it tried", err.Error())
	}
}

// ── T7 ──────────────────────────────────────────────────────────────────────

// "An error naming what it looked for." ErrNoSurface has two message forms and
// the topic-scoped one must carry the topic: "no topic-file surface" cannot
// tell a reader WHICH topic went missing, which is the whole question when ten
// topics compile and one does not.
func TestSurfaces_ErrNoSurfaceNamesWhatItLookedFor(t *testing.T) {
	withTopic := (&ErrNoSurface{Kind: KindTopicFile, Topic: "agent-rules"}).Error()
	for _, want := range []string{KindTopicFile, "agent-rules"} {
		if !strings.Contains(withTopic, want) {
			t.Errorf("topic-scoped message %q does not name %q", withTopic, want)
		}
	}
	withoutTopic := (&ErrNoSurface{Kind: KindAmbientCore}).Error()
	if !strings.Contains(withoutTopic, KindAmbientCore) {
		t.Errorf("message %q does not name the kind %q", withoutTopic, KindAmbientCore)
	}
	if strings.Contains(withoutTopic, `""`) {
		t.Errorf("message %q renders an empty topic as a quoted blank — an untopiced surface has no topic, it does not have a blank one", withoutTopic)
	}
}

// ── T8 ──────────────────────────────────────────────────────────────────────

// ResolveKind is the one-shot form, and a Load failure must reach the caller
// through it. Swallowing it would turn "the manifest is corrupt" into "this
// surface does not exist" at exactly the consumers that do not hold a
// Manifest — hookmatch and doctor.
func TestSurfaces_ResolveKindPropagatesLoadFailure(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, ManifestRelPath), "corrupt\n")
	got, err := ResolveKind(root, KindDeliveryMap, "")
	if err == nil {
		t.Fatalf("corrupt manifest: ResolveKind returned %q with no error", got)
	}
	if got != "" {
		t.Errorf("ResolveKind returned %q alongside its error, want \"\"", got)
	}
	if isNoSurface(err) {
		t.Errorf("ResolveKind error = %v, want the load failure rather than *ErrNoSurface", err)
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Errorf("ResolveKind error = %q, want the refusal Load produced", err.Error())
	}
}

// ── T9 ──────────────────────────────────────────────────────────────────────

// PathsOfKind serves the consumers that ITERATE. It must return every match of
// the requested kind and nothing of any other, sorted, so two runs over an
// unchanged tree cannot disagree.
func TestSurfaces_PathsOfKindListsEveryMatchOfThatKindSorted(t *testing.T) {
	root := t.TempDir()
	entries := []phaseb.ManifestEntry{
		{Kind: KindTopicFile, Path: "out/security.md", Topic: "security"},
		{Kind: KindSkillPackage, Path: "skills/edikt-security/SKILL.md", Topic: "security"},
		{Kind: KindTopicFile, Path: "out/agent-rules.md", Topic: "agent-rules"},
		{Kind: KindAmbientCore, Path: "out/core.md"},
		{Kind: KindTopicFile, Path: "out/hooks.md", Topic: "hooks"},
	}
	for _, e := range entries {
		writeFile(t, filepath.Join(root, e.Path), "body\n")
	}
	writeManifest(t, root, entries)
	m := mustLoad(t, root)

	got, err := m.PathsOfKind(KindTopicFile)
	if err != nil {
		t.Fatalf("PathsOfKind(topic-file): unexpected error %v — every listed surface is on disk", err)
	}
	want := []string{
		filepath.Join(root, "out/agent-rules.md"),
		filepath.Join(root, "out/hooks.md"),
		filepath.Join(root, "out/security.md"),
	}
	if !sort.StringsAreSorted(got) {
		t.Errorf("PathsOfKind(topic-file) = %v — not sorted; an unstable order makes two runs over one tree disagree", got)
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("PathsOfKind(topic-file):\n got %v\nwant %v", got, want)
	}
	for _, p := range got {
		if strings.Contains(p, "SKILL.md") || strings.HasSuffix(p, "core.md") {
			t.Errorf("PathsOfKind(topic-file) returned %q — a surface of another kind", p)
		}
	}
	s, serr := m.PathsOfKind(KindSkillPackage)
	if serr != nil {
		t.Fatalf("PathsOfKind(skill-package): unexpected error %v", serr)
	}
	if len(s) != 1 || s[0] != filepath.Join(root, "skills/edikt-security/SKILL.md") {
		t.Errorf("PathsOfKind(skill-package) = %v, want the one skill surface", s)
	}
}

// ── T10 ─────────────────────────────────────────────────────────────────────

// The iterating consumers get the same upgrade grace as the resolving ones:
// with no manifest at all, PathsOfKind globs the conventional layout.
func TestSurfaces_PathsOfKindGlobsTheConventionalLayoutWithNoManifest(t *testing.T) {
	root := t.TempDir()
	topicB := writeFile(t, convTopic(root, "b-topic"), "b\n")
	topicA := writeFile(t, convTopic(root, "a-topic"), "a\n")
	skillB := writeFile(t, convSkill(root, "b-topic"), "b\n")
	skillA := writeFile(t, convSkill(root, "a-topic"), "a\n")

	m := mustLoad(t, root)
	got, err := m.PathsOfKind(KindTopicFile)
	if err != nil {
		t.Fatalf("no manifest, PathsOfKind(topic-file): unexpected error %v — refusing here bricks every iterating consumer on upgrade", err)
	}
	if want := []string{topicA, topicB}; strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("no manifest, PathsOfKind(topic-file):\n got %v\nwant %v", got, want)
	}
	gotSkills, err := m.PathsOfKind(KindSkillPackage)
	if err != nil {
		t.Fatalf("no manifest, PathsOfKind(skill-package): unexpected error %v", err)
	}
	if want := []string{skillA, skillB}; strings.Join(gotSkills, "\n") != strings.Join(want, "\n") {
		t.Errorf("no manifest, PathsOfKind(skill-package):\n got %v\nwant %v", gotSkills, want)
	}
}

// ── T11 ─────────────────────────────────────────────────────────────────────

// End to end against the REAL producer: phaseb builds and renders the manifest,
// this package loads and resolves it. The two halves live in different packages
// and only agree by convention, so a fixture written by hand in this file would
// keep passing while the compiler's output moved out from under it.
func TestSurfaces_RoundTripsWhatTheCompilerWrote(t *testing.T) {
	root := t.TempDir()
	produced := []phaseb.Surface{
		{Kind: KindAmbientCore, Path: ".claude/rules/governance.md"},
		{Kind: KindDeliveryMap, Path: ".claude/rules/governance/delivery-map.yaml"},
		{Kind: KindTopicFile, Path: ".claude/rules/governance/agent-rules.md", Topic: "agent-rules"},
		{Kind: KindSkillPackage, Path: ".claude/skills/edikt-agent-rules/SKILL.md", Topic: "agent-rules"},
		// The manifest enumerates itself as a produced surface but cannot
		// contain its own hash, so BuildManifest drops it. Resolving it must
		// therefore fail — asserted below, because a consumer asking for it
		// and getting a path would be reading a file nothing hashed.
		{Kind: "manifest", Path: ManifestRelPath},
	}
	for _, s := range produced {
		writeFile(t, filepath.Join(root, s.Path), "body of "+s.Path+"\n")
	}
	entries, err := phaseb.BuildManifest(root, produced)
	if err != nil {
		t.Fatalf("phaseb.BuildManifest: %v", err)
	}
	writeFile(t, filepath.Join(root, ManifestRelPath), phaseb.RenderManifest(entries))

	m := mustLoad(t, root)
	if !m.Present {
		t.Fatalf("a manifest written by phaseb.RenderManifest did not load as present")
	}
	if len(m.Entries) != len(produced)-1 {
		t.Errorf("got %d entries from a %d-surface compile, want %d (the manifest itself is excluded)", len(m.Entries), len(produced), len(produced)-1)
	}
	for _, s := range produced[:len(produced)-1] {
		got, err := m.Resolve(s.Kind, s.Topic)
		if err != nil {
			t.Errorf("Resolve(%q, %q) after a real compile: %v", s.Kind, s.Topic, err)
			continue
		}
		if want := filepath.Join(root, s.Path); got != want {
			t.Errorf("Resolve(%q, %q) = %q, want %q", s.Kind, s.Topic, got, want)
		}
	}
	if got, err := m.Resolve("manifest", ""); !isNoSurface(err) {
		t.Errorf("Resolve(manifest) = %q, %v — want *ErrNoSurface: the manifest excludes itself, so nothing may resolve it", got, err)
	}
}

// ── T12 ─────────────────────────────────────────────────────────────────────

// THE DOC-SEMANTIC TEST (AC-2.3).
//
// The package doc says a missing surface is an error and never an empty path,
// and that EVERY resolution returns a path that exists or an error naming what
// it looked for. Resolve statted its result; PathsOfKind did not, and doctor's
// routed-sources check calls the unstatted one twice — so one exported form
// kept the promise the package doc made for both, and the other handed back
// paths to files no compile had written. That is the origin defect of this
// whole plan (globmatch: a doc comment stating a semantic the code does not
// implement, on a delivery path, failing silently) reproduced in the second
// package it examined.
//
// This test asserts the CLOSED contract, not the divergence. Phase 2 recorded
// the row UNCOVERED rather than pinning current behaviour precisely so this
// assertion could be written against the fixed code instead of freezing the
// gap into it.
func TestSurfaces_PathsOfKindHonoursTheExistenceContract(t *testing.T) {
	root := t.TempDir()
	onDisk := phaseb.ManifestEntry{Kind: KindTopicFile, Path: "out/alpha.md", Topic: "alpha"}
	gone := phaseb.ManifestEntry{Kind: KindTopicFile, Path: "out/beta.md", Topic: "beta"}
	alsoGone := phaseb.ManifestEntry{Kind: KindTopicFile, Path: "out/gamma.md", Topic: "gamma"}
	skill := phaseb.ManifestEntry{Kind: KindSkillPackage, Path: "skills/edikt-alpha/SKILL.md", Topic: "alpha"}
	writeFile(t, filepath.Join(root, onDisk.Path), "alpha\n")
	writeFile(t, filepath.Join(root, skill.Path), "skill\n")
	// beta.md and gamma.md are LISTED and never written: a compile that failed
	// part way, or a surface moved by hand afterwards.
	writeManifest(t, root, []phaseb.ManifestEntry{onDisk, gone, alsoGone, skill})
	m := mustLoad(t, root)

	got, err := m.PathsOfKind(KindTopicFile)
	if err == nil {
		t.Fatalf("the manifest lists two topic files that are not on disk, yet PathsOfKind returned %v with no error — an iterating consumer opens a path to nothing and reports on what is left as though that were the corpus", got)
	}
	if got != nil {
		t.Errorf("PathsOfKind returned %v alongside its error, want nil — a partial list makes a corrupt render read as a smaller one", got)
	}
	// EVERY missing entry, not the first: naming one of two sends a reader to
	// fix that one and re-run into the next.
	for _, want := range []string{gone.Path, alsoGone.Path, "not on disk"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("PathsOfKind error %q does not name %q", err.Error(), want)
		}
	}
	if isNoSurface(err) {
		t.Errorf("PathsOfKind error = %v, want a broken-contract error rather than *ErrNoSurface — the enumerating form must make the same discrimination the resolving one makes: 'the render is corrupt' is not 'this surface was not rendered'", err)
	}
	// ONE FACT, ONE PLACE: the two forms must not have grown two spellings of
	// the same refusal. A second message form is a second contract, and this is
	// the divergence that produced the defect in the first place.
	_, rerr := m.Resolve(gone.Kind, gone.Topic)
	if rerr == nil {
		t.Fatalf("fixture error: Resolve(%q, %q) succeeded for a surface that is not on disk", gone.Kind, gone.Topic)
	}
	if !strings.Contains(err.Error(), rerr.Error()) {
		t.Errorf("PathsOfKind error:\n %q\ndoes not carry Resolve's own message:\n %q\n— the two exported forms are wording one fact differently", err.Error(), rerr.Error())
	}

	// A kind whose every listed surface IS on disk is untouched: this test must
	// fail on the absent file, not because enumeration stopped working.
	skills, serr := m.PathsOfKind(KindSkillPackage)
	if serr != nil {
		t.Errorf("PathsOfKind(skill-package): unexpected error %v — every skill-package entry is on disk", serr)
	}
	if len(skills) != 1 || skills[0] != filepath.Join(root, skill.Path) {
		t.Errorf("PathsOfKind(skill-package) = %v, want the one listed surface", skills)
	}

	// ── the no-manifest fallback covers the SAME FOUR KINDS Resolve does ──
	//
	// It fell back for the two topic-scoped kinds and returned nothing for
	// ambient-core and delivery-map: a silent empty on the branch that exists
	// to avoid one, in the only exported form a consumer can iterate.
	fb := t.TempDir()
	for _, c := range []struct{ kind, topic, path string }{
		{KindAmbientCore, "", writeFile(t, convCore(fb), "core\n")},
		{KindDeliveryMap, "", writeFile(t, convMap(fb), "map\n")},
		{KindTopicFile, "alpha", writeFile(t, convTopic(fb, "alpha"), "topic\n")},
		{KindSkillPackage, "alpha", writeFile(t, convSkill(fb, "alpha"), "skill\n")},
	} {
		m2 := mustLoad(t, fb)
		if m2.Present {
			t.Fatalf("fixture error: a manifest exists where none was written")
		}
		paths, err := m2.PathsOfKind(c.kind)
		if err != nil {
			t.Errorf("no manifest, PathsOfKind(%q): unexpected error %v", c.kind, err)
			continue
		}
		if len(paths) != 1 || paths[0] != c.path {
			t.Errorf("no manifest, PathsOfKind(%q) = %v, want [%s] — the resolving form falls back for this kind, so the iterating one must too", c.kind, paths, c.path)
		}
		// And it agrees with what Resolve answers for the same kind.
		if r, rerr := m2.Resolve(c.kind, c.topic); rerr != nil || r != c.path {
			t.Errorf("fixture error: Resolve(%q, %q) = %q, %v — want %q", c.kind, c.topic, r, rerr, c.path)
		}
	}

	// Nothing on disk and no manifest: an EMPTY SET, not an error. "No surface
	// of this kind was rendered" is an answer an iterating consumer can act on;
	// Resolve errors on the same condition because a caller that asked for one
	// surface has nothing to carry on with.
	bare := t.TempDir()
	m3 := mustLoad(t, bare)
	for _, kind := range []string{KindAmbientCore, KindDeliveryMap, KindTopicFile, KindSkillPackage} {
		paths, err := m3.PathsOfKind(kind)
		if err != nil {
			t.Errorf("no manifest and nothing on disk, PathsOfKind(%q): error %v — an empty enumeration is a legal answer, and refusing it turns an uncompiled project into a failure", kind, err)
		}
		if len(paths) != 0 {
			t.Errorf("no manifest and nothing on disk, PathsOfKind(%q) = %v, want none", kind, paths)
		}
	}
}

// ── T13 ─────────────────────────────────────────────────────────────────────

// The four Kind constants are documented as "written by the renderer into the
// manifest". T11 proves the SERIALISATION round trip, but it feeds phaseb the
// kinds from these very constants — so it cannot see the renderer renaming one.
// The literals live inline in phaseb's compile function, and reaching them
// needs a whole compile, which is what this does: nothing here tells Merge what
// to call a surface.
//
// A renamed or added kind is silent by construction — a consumer asking for the
// old name gets ErrNoSurface forever, which reads exactly like "that surface
// was not rendered".
func TestSurfaces_KindConstantsAreWhatTheCompilerWrites(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	parentPath := writeFile(t, filepath.Join(dir, "ADR-001-test.md"), "# placeholder\n")
	sidecarPath := writeFile(t, filepath.Join(dir, "ADR-001-test.edikt.yaml"), "placeholder\n")
	pair := sidecar.Pair{
		ParentPath:  parentPath,
		SidecarPath: sidecarPath,
		ArtifactID:  "ADR-001",
		Sidecar: &sidecar.Sidecar{
			SchemaVersion: 1,
			Topic:         "architecture",
			Path:          "docs/architecture/decisions/ADR-001-test.md",
			// A declared glob keeps the topic at tier 2, so the compile renders
			// a topic file as well as a skill package — all four kinds in one
			// run.
			Paths:      []string{"src/**/*.go"},
			Signals:    []string{"x"},
			Directives: []sidecar.Directive{{Text: "A directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}}},
			SourcePath: sidecarPath,
		},
	}
	if _, err := phaseb.Merge(root, []sidecar.Pair{pair}, phaseb.Options{
		CompiledAt:      "2026-05-02T00:00:00Z",
		CompilerVersion: "0.9.0-test",
	}); err != nil {
		t.Fatalf("phaseb.Merge: %v", err)
	}

	m := mustLoad(t, root)
	if !m.Present {
		t.Fatalf("a real compile wrote no manifest this package can load")
	}
	known := map[string]bool{
		KindAmbientCore:  false,
		KindTopicFile:    false,
		KindSkillPackage: false,
		KindDeliveryMap:  false,
	}
	for _, e := range m.Entries {
		seen, ok := known[e.Kind]
		if !ok {
			t.Errorf("a real compile wrote surface kind %q at %s, and this package has no constant for it — every consumer asking for that surface by name gets ErrNoSurface forever, which reads as 'not rendered'", e.Kind, e.Path)
			continue
		}
		if seen {
			continue
		}
		known[e.Kind] = true
		if _, err := m.Resolve(e.Kind, e.Topic); err != nil {
			t.Errorf("a surface the compiler wrote as kind %q does not resolve through the constant of the same name: %v", e.Kind, err)
		}
	}
	for _, kind := range []string{KindAmbientCore, KindTopicFile, KindSkillPackage, KindDeliveryMap} {
		if !known[kind] {
			t.Errorf("constant %q is documented as a kind the renderer writes into the manifest, but a real compile wrote no surface of that kind — the constant names something nothing produces", kind)
		}
	}
}
