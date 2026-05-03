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
		SchemaVersion:    1,
		Topic:            topic,
		Path:             "docs/architecture/decisions/" + basename + ".md",
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

// TestRender_ManualInterleaved — manual directives appear in the directives
// region sorted by ref tag with the *(manual)* marker.
func TestRender_ManualInterleaved(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-a", "alpha",
		[]sidecar.Directive{
			{Text: "Auto rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		[]string{"Stage 2 MUST enable prompt caching."},
		nil,
	)
	if _, err := Merge(root, []sidecar.Pair{pair}, Options{CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body := readTopic(t, root, "alpha")
	if !strings.Contains(body, "[edikt:directives:start]: #") {
		t.Errorf("missing directives start sentinel:\n%s", body)
	}
	if !strings.Contains(body, "Auto rule. (ref: ADR-001)") {
		t.Error("auto directive missing")
	}
	if !strings.Contains(body, "Stage 2 MUST enable prompt caching. (ref: ADR-001 + manual) *(manual)*") {
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
func TestRender_ProhibitionsOwnSection(t *testing.T) {
	root := t.TempDir()
	pair := mkPairWith(t, root, "ADR-001-b", "alpha",
		[]sidecar.Directive{
			{Text: "Auto rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		},
		nil,
		[]sidecar.Prohibition{
			{Text: "MUST NOT use Cursor. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
			{Text: "NEVER bundle a build step. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "z"}},
		},
	)
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
	} else if !strings.Contains(err.Error(), "INV-005 violation") || !strings.Contains(err.Error(), "overlap") {
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
	fp := TopicFingerprint([]*sidecar.Sidecar{pair.Sidecar})
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

// TestRegionSHA_EmptyEqualsSHA256OfEmpty — the empty-region anchor is
// sha256("") so a freshly bootstrapped empty region carries a known value.
func TestRegionSHA_EmptyEqualsSHA256OfEmpty(t *testing.T) {
	want := sha256.Sum256(nil)
	got := regionSHA(nil, false)
	if got != hex.EncodeToString(want[:]) {
		t.Errorf("empty regionSHA: got %q want %q", got, hex.EncodeToString(want[:]))
	}
}
