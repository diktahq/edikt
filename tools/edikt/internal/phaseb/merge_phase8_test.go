package phaseb

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// mkPairWith builds a sidecar.Pair populated with optional manual_directives
// and prohibitions for Phase 8 render tests.
func mkPairWith(t *testing.T, projectRoot, basename, topic string,
	directives []sidecar.Directive,
	manual []string,
	prohibitions []sidecar.Prohibition,
) sidecar.Pair {
	t.Helper()
	dir := filepath.Join(projectRoot, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, basename+".md")
	sidecarPath := filepath.Join(dir, basename+".edikt.yaml")
	if err := os.WriteFile(parentPath, []byte("# placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         topic,
		Path:          "docs/architecture/decisions/" + basename + ".md",
		// Scoped, for the reason documented on mkPair in merge_test.go: an
		// undeclared glob now means tier 3 and no rules file.
		Paths:            []string{"src/**/*.go"},
		Signals:          []string{"x"},
		Directives:       directives,
		ManualDirectives: manual,
		Prohibitions:     prohibitions,
		SourcePath:       sidecarPath,
	}
	if err := os.WriteFile(sidecarPath, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{
		ParentPath:  parentPath,
		SidecarPath: sidecarPath,
		ArtifactID:  basename[:7],
		Sidecar:     sc,
	}
}

func readTopic(t *testing.T, root, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", name+".md"))
	if err != nil {
		t.Fatalf("read topic %s: %v", name, err)
	}
	return string(b)
}

// assertNoDirectiveIndex pins the retirement of the write-time surface
// (ADR-083): the compiler renders no directive-index.yaml, so a scoped
// sidecar's directive and prohibition text has exactly one home — the topic
// file (or the skill package for a tier-3 topic).
func readFileString(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

func assertNoDirectiveIndex(t *testing.T, root string) {
	t.Helper()
	p := filepath.Join(root, ".claude", "rules", "governance", "directive-index.yaml")
	if _, err := os.Stat(p); err == nil {
		t.Fatalf("directive-index.yaml was rendered at %s — the write-time surface is retired (ADR-083)", p)
	}
}

// TestRender_ManualInterleaved — manual directives appear in the directives
// region sorted by ref tag with the *(manual)* marker.
//
// The "auto" side uses a PATHLESS invariant's second directive rather than a
// scoped ADR: since ADR-066, a sidecar declaring paths: (the ordinary ADR
// shape) renders its directive text into the topic file, as every shape does since ADR-083 — the topic file's compiled-directives region is this test's
// subject, so its auto content has to come from the one artifact shape that
// still lands there. The first directive is the invariant's canonical
// statement and goes to the ambient core instead (merge.go), which is why
// there are two.
func TestRender_ManualInterleaved(t *testing.T) {
	root := t.TempDir()
	pair := mkINVPair(t, root, "INV-001-a", "alpha",
		[]sidecar.Directive{
			{Text: "Canonical statement. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "c"}},
			{Text: "Auto rule. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "x"}},
		}, nil, nil)
	pair.ArtifactID = "INV-001"
	pair.Sidecar.ManualDirectives = []string{"Stage 2 MUST enable prompt caching."}
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")
	if !strings.Contains(body, "[edikt:directives:start]: #") {
		t.Errorf("missing directives start sentinel:\n%s", body)
	}
	if !strings.Contains(body, "Auto rule. (ref: INV-001)") {
		t.Error("auto directive missing")
	}
	if !strings.Contains(body, "Stage 2 MUST enable prompt caching. (ref: INV-001 + manual) *(manual)*") {
		t.Errorf("manual directive not annotated correctly:\n%s", body)
	}
	// Auto first by ref tag tiebreak (extracted before manual on equal ref).
	idxAuto := strings.Index(body, "Auto rule.")
	idxManual := strings.Index(body, "Stage 2 MUST")
	if idxAuto < 0 || idxManual < 0 || idxAuto > idxManual {
		t.Errorf("expected auto before manual on equal ref tag; auto=%d manual=%d", idxAuto, idxManual)
	}
}

// TestRender_ProhibitionsOwnSection — prohibitions render in their own
// sentinel-bracketed region with MUST NOT preserved verbatim.
//
// PATHLESS invariant, per the same ADR-066 reasoning as
// TestRender_ManualInterleaved: prohibitions on a scoped sidecar (the
// ordinary ADR shape) now render into directive-index.yaml only. Pathless-
// invariant prohibitions are the one shape still delivered into the topic
// file unconditionally (merge.go: no "canonical first" cutoff applies to
// prohibitions).
func TestRender_ProhibitionsOwnSection(t *testing.T) {
	root := t.TempDir()
	pair := mkINVPair(t, root, "INV-001-b", "alpha",
		[]sidecar.Directive{
			{Text: "Canonical statement. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "c"}},
		}, nil, nil)
	pair.ArtifactID = "INV-001"
	pair.Sidecar.Prohibitions = []sidecar.Prohibition{
		{Text: "MUST NOT use Cursor. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
		{Text: "NEVER bundle a build step. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "z"}},
	}
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")
	startIdx := strings.Index(body, "[edikt:prohibitions:start]: #")
	endIdx := strings.Index(body, "[edikt:prohibitions:end]: #")
	if startIdx < 0 || endIdx < 0 || endIdx < startIdx {
		t.Fatalf("prohibitions sentinels missing or inverted:\n%s", body)
	}
	region := body[startIdx:endIdx]
	if !strings.Contains(region, "## Prohibitions") {
		t.Errorf("prohibitions region missing heading:\n%s", region)
	}
	if !strings.Contains(region, "MUST NOT use Cursor.") {
		t.Errorf("MUST NOT prohibition missing:\n%s", region)
	}
	if !strings.Contains(region, "NEVER bundle a build step.") {
		t.Errorf("NEVER prohibition missing:\n%s", region)
	}
}

// TestRender_ManualSection — manual_directives ALSO appear in the dedicated
// [edikt:manual:...] region as a faithful copy.
func TestRender_ManualSection(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-c", "alpha",
		[]sidecar.Directive{
			{Text: "Auto rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		[]string{"Manual entry one.", "Manual entry two."},
		nil,
	)
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")
	startIdx := strings.Index(body, "[edikt:manual:start]: #")
	endIdx := strings.Index(body, "[edikt:manual:end]: #")
	if startIdx < 0 || endIdx < 0 {
		t.Fatalf("manual region sentinels missing:\n%s", body)
	}
	region := body[startIdx:endIdx]
	if !strings.Contains(region, "Manual entry one.") {
		t.Errorf("manual entry one missing from manual region:\n%s", region)
	}
	if !strings.Contains(region, "Manual entry two.") {
		t.Errorf("manual entry two missing from manual region:\n%s", region)
	}
}

// TestRender_AnchorScheme — every region carries its sha256 anchor.
func TestRender_AnchorScheme(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-d", "alpha",
		[]sidecar.Directive{
			{Text: "Auto. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		[]string{"Manual."},
		[]sidecar.Prohibition{
			{Text: "MUST NOT do X. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
		},
	)
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")
	for _, kind := range []string{"directives", "prohibitions", "manual"} {
		anchor := "[edikt:" + kind + ":sha256]: # "
		idx := strings.Index(body, anchor)
		if idx < 0 {
			t.Fatalf("anchor for %s missing:\n%s", kind, body)
		}
		// Hex must be 64 chars after the anchor prefix (newline-terminated).
		rest := body[idx+len(anchor):]
		nl := strings.Index(rest, "\n")
		if nl < 0 {
			nl = len(rest)
		}
		hexv := strings.TrimSpace(rest[:nl])
		if len(hexv) != 64 {
			t.Errorf("%s sha256 anchor not 64 chars: %q", kind, hexv)
		}
		if _, err := hex.DecodeString(hexv); err != nil {
			t.Errorf("%s sha256 not hex: %v", kind, err)
		}
	}
}

// TestRender_OverlapDetection — synthetic overlap → compile error.
func TestRender_OverlapDetection(t *testing.T) {
	body := "head\n[edikt:directives:start]: #\n- a\n[edikt:prohibitions:start]: #\n- p\n[edikt:directives:end]: #\n[edikt:prohibitions:end]: #\n"
	if err := assertNoRegionOverlap("alpha", body); err == nil {
		t.Error("expected overlap error, got nil")
	} else if !strings.Contains(err.Error(), "sentinel-region violation") || !strings.Contains(err.Error(), "overlap") {
		t.Errorf("unexpected error: %v", err)
	}
	// Non-overlapping sequential regions must pass.
	ok := "head\n[edikt:directives:start]: #\n- a\n[edikt:directives:end]: #\n[edikt:prohibitions:start]: #\n- p\n[edikt:prohibitions:end]: #\n"
	if err := assertNoRegionOverlap("alpha", ok); err != nil {
		t.Errorf("non-overlapping body rejected: %v", err)
	}
}

// TestCompile_BootstrapAnchors — a v0.6.0-rc4-shape file (only directives
// region present, or in fact no managed regions at all) gets all three
// anchors written on first compile after upgrade.
func TestCompile_BootstrapAnchors(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-e", "alpha",
		[]sidecar.Directive{
			{Text: "Auto. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		nil, nil,
	)
	// Pre-write a v0.6.0-rc4 shaped topic file with the matching fingerprint —
	// the cache short-circuit would otherwise skip render. Bootstrap MUST
	// re-render anyway because the new prohibitions/manual regions are absent.
	fp := TopicRenderFingerprint([]*sidecar.Sidecar{pair.Sidecar}, "")
	preBody := "---\npaths:\n  - \"x\"\ncompile_schema_version: 2\n_fingerprint: \"" + fp + "\"\n---\n# Alpha\n\n- Auto. (ref: ADR-001)\n"
	dest := filepath.Join(root, ".claude", "rules", "governance", "alpha.md")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dest, []byte(preBody), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TopicsRendered) != 1 {
		t.Errorf("expected bootstrap to re-render alpha; got rendered=%v unchanged=%v", res.TopicsRendered, res.TopicsUnchanged)
	}
	body := readTopic(t, root, "alpha")
	for _, kind := range []string{"directives", "prohibitions", "manual"} {
		if !strings.Contains(body, "[edikt:"+kind+":start]: #") {
			t.Errorf("bootstrap missed %s region", kind)
		}
		if !strings.Contains(body, "[edikt:"+kind+":sha256]: #") {
			t.Errorf("bootstrap missed %s sha256 anchor", kind)
		}
	}
}

// TestCompile_Determinism — two consecutive compiles produce byte-equal
// topic files for the same input (ADR-020 contract).
func TestCompile_Determinism(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-f", "alpha",
		[]sidecar.Directive{
			{Text: "Auto. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		[]string{"Manual A.", "Manual B."},
		[]sidecar.Prohibition{
			{Text: "MUST NOT do X. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
		},
	)
	opts := Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}
	if _, err := Merge(root, []sidecar.Pair{pair}, opts); err != nil {
		t.Fatal(err)
	}
	first := readTopic(t, root, "alpha")
	if _, err := Merge(root, []sidecar.Pair{pair}, opts); err != nil {
		t.Fatal(err)
	}
	second := readTopic(t, root, "alpha")
	if first != second {
		t.Errorf("non-deterministic output:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestCompile_DeterminismExtended — multi-sidecar permutations produce
// stable output, exercising the cross-sidecar sort comparator.
func TestCompile_DeterminismExtended(t *testing.T) {
	root := t.TempDir()
	p1 := mkPairWith(t, root, "ADR-002-x", "alpha",
		[]sidecar.Directive{{Text: "B-rule. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}}},
		[]string{"manual-2"},
		[]sidecar.Prohibition{{Text: "MUST NOT b. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}}},
	)
	p2 := mkPairWith(t, root, "ADR-001-x", "alpha",
		[]sidecar.Directive{{Text: "A-rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}}},
		[]string{"manual-1"},
		[]sidecar.Prohibition{{Text: "MUST NOT a. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}}},
	)
	opts := Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}
	if _, err := Merge(root, []sidecar.Pair{p1, p2}, opts); err != nil {
		t.Fatal(err)
	}
	first := readTopic(t, root, "alpha")
	// Reverse pair order, retry. Should yield byte-equal result.
	root2 := t.TempDir()
	q2 := mkPairWith(t, root2, "ADR-001-x", "alpha",
		[]sidecar.Directive{{Text: "A-rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}}},
		[]string{"manual-1"},
		[]sidecar.Prohibition{{Text: "MUST NOT a. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}}},
	)
	q1 := mkPairWith(t, root2, "ADR-002-x", "alpha",
		[]sidecar.Directive{{Text: "B-rule. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}}},
		[]string{"manual-2"},
		[]sidecar.Prohibition{{Text: "MUST NOT b. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}}},
	)
	if _, err := Merge(root2, []sidecar.Pair{q2, q1}, opts); err != nil {
		t.Fatal(err)
	}
	second := readTopic(t, root2, "alpha")
	// The two roots have different `paths:` frontmatter only via Sidecar.Path
	// strings — both pairs use the same path forms, so the bodies should be
	// byte-equal.
	if first != second {
		t.Errorf("permutation produced different output:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
	// Sanity: the sha256 anchors are the same hex (deterministic across runs).
	for _, kind := range []string{"directives", "prohibitions", "manual"} {
		anc := "[edikt:" + kind + ":sha256]: # "
		extract := func(s string) string {
			i := strings.Index(s, anc)
			rest := s[i+len(anc):]
			nl := strings.Index(rest, "\n")
			return strings.TrimSpace(rest[:nl])
		}
		if extract(first) != extract(second) {
			t.Errorf("%s anchor differs across permutations: %q vs %q", kind, extract(first), extract(second))
		}
	}
}

// TestCompile_AnchorsDescribeTheRegionsOnDisk — every anchor in a compiled
// topic file is the sha256 of the bytes THAT FILE carries between that
// region's own markers.
//
// It replaces TestRegionSHA_EmptyEqualsSHA256OfEmpty, which asserted that
// phaseb's own regionSHA(nil, false) equalled sha256("") — a check on this
// package's RESTATEMENT of the renderer's layout, not on anything that
// reaches disk. It survived the mutation that mattered: making regionSHA stop
// prepending the prohibitions heading, which makes every prohibitions anchor
// in every compiled file wrong, left the suite green. regionSHA is gone
// (D357) and the renderer anchors what it emits; what is asserted here is the
// artifact.
//
// FALSIFIED BY MUTATING THE WRITE-PATH, never the renderer (D359). Under D357
// the renderer emits RegionSHA(body) for the body it just emitted, so an
// in-memory comparison of the declared anchor against RegionSHA(body) is
// f(x) == f(x) — true by construction, and green under the very mutation that
// drops a directive. That identity is the WIN of option D, which is exactly
// why it is not a control. What remains falsifiable is the ROUND TRIP, so
// both mutations were applied to writeAtomicIfChanged (merge.go), the one
// function standing between the rendered string and the file. Measured
// 2026-09-06, `go test -count=1 ./internal/phaseb/ ./internal/render/`; each
// mutation was reverted from a byte-for-byte backup (merge.go sha256
// 36187ae0… before and after):
//
//	W1 the writer "tidies" the in-marker heading —
//	   ReplaceAll(content, "## Prohibitions\n",
//	   "## Prohibitions\n\n")                                    → RED
//	   region "prohibitions": the file declares anchor df4ce315…
//	   but its own bytes hash to f463878b…
//	W2 the writer drops the first bullet on its way to disk —
//	   Replace(content, "\n- ", "\n", 1)                         → RED
//	   region "directives": the file declares anchor bcb240bc…
//	   but its own bytes hash to 5a7b651e…
//
// Under each write-path mutation this was the ONLY test in package phaseb
// that moved, and the whole internal/render suite stayed GREEN: an in-memory
// identity cannot see a writer that transforms bytes on the way out. That
// asymmetry is why this assertion lives here, on the write-path, rather than
// beside the renderer.
func TestCompile_AnchorsDescribeTheRegionsOnDisk(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-anchors", "alpha",
		[]sidecar.Directive{
			{Text: "Auto. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		// No manual entries: the empty-region case, which must land on
		// sha256("") by HASHING an empty body, not by defaulting to a constant.
		nil,
		[]sidecar.Prohibition{
			{Text: "MUST NOT do X. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
		},
	)
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")

	for _, kind := range []string{"directives", "prohibitions", "manual"} {
		region := regionBytesOnDisk(t, body, kind)
		sum := sha256.Sum256([]byte(region))
		if got, want := regionAnchorOnDisk(t, body, kind), hex.EncodeToString(sum[:]); got != want {
			t.Errorf("region %q: the file declares anchor %s but its own bytes hash to %s\nbody was %q",
				kind, got, want, region)
		}
	}

	// The prohibitions region is the one whose body is NEVER empty — the
	// template emits its heading inside the markers. Naming it here so the
	// next reader sees why "hash the bullets" is not the same scheme.
	if proh := regionBytesOnDisk(t, body, "prohibitions"); !strings.HasPrefix(proh, "## Prohibitions\n") {
		t.Errorf("prohibitions region no longer opens with its in-marker heading: %q", proh)
	}
	// And the empty region reaches sha256("") by hashing nothing.
	empty := sha256.Sum256(nil)
	if got, want := regionAnchorOnDisk(t, body, "manual"), hex.EncodeToString(empty[:]); got != want {
		t.Errorf("empty manual region anchored %s, want sha256(\"\") = %s", got, want)
	}
}

// regionBytesOnDisk returns the bytes a compiled topic file carries strictly
// between one region's start-marker line and its end marker.
func regionBytesOnDisk(t *testing.T, file, kind string) string {
	t.Helper()
	start := "[edikt:" + kind + ":start]: #\n"
	i := strings.Index(file, start)
	if i < 0 {
		t.Fatalf("region %q: no start marker in:\n%s", kind, file)
	}
	rest := file[i+len(start):]
	j := strings.Index(rest, "[edikt:"+kind+":end]: #")
	if j < 0 {
		t.Fatalf("region %q: no end marker after the start marker in:\n%s", kind, file)
	}
	return rest[:j]
}

// regionAnchorOnDisk returns the hex a region's sha256 anchor line declares.
func regionAnchorOnDisk(t *testing.T, file, kind string) string {
	t.Helper()
	anchor := "[edikt:" + kind + ":sha256]: # "
	i := strings.Index(file, anchor)
	if i < 0 {
		t.Fatalf("region %q: no sha256 anchor line in:\n%s", kind, file)
	}
	rest := file[i+len(anchor):]
	nl := strings.Index(rest, "\n")
	if nl < 0 {
		nl = len(rest)
	}
	return strings.TrimSpace(rest[:nl])
}
