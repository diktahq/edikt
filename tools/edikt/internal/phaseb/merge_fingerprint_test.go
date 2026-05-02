package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// TestFingerprint_RoundTrips pins the full _fingerprint contract for a
// topic file (Phase 8 of PLAN-sidecar-architecture; #17 of
// PLAN-sidecar-review-fixes):
//
//  1. The fingerprint computed from in-memory sidecars is the value that
//     ends up in the rendered topic file's frontmatter.
//  2. Mutating any contributing sidecar bumps the fingerprint AND
//     triggers a re-render (cache busts on real change).
//  3. A no-op re-render is byte-equal on disk, mtime-stable, and
//     reported as TopicsUnchanged — i.e. writeAtomicIfChanged
//     short-circuits and the cache hit fires.
//
// This test is the regression gate for template changes that strip or
// rename `_fingerprint:`. If the contract drifts, the assertions on
// frontmatter content fail immediately.
func TestFingerprint_RoundTrips(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "First. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
		mkPair(t, root, "ADR-002-test", "alpha", []sidecar.Directive{
			{Text: "Second. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
		mkPair(t, root, "ADR-003-test", "alpha", []sidecar.Directive{
			{Text: "Third. (ref: ADR-003)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}
	topicPath := filepath.Join(root, ".claude", "rules", "governance", "alpha.md")

	// (1) The fingerprint embedded in the rendered topic matches the
	//     value computed from the in-memory sidecar set.
	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	body, err := os.ReadFile(topicPath)
	if err != nil {
		t.Fatalf("read topic: %v", err)
	}
	want := TopicFingerprint([]*sidecar.Sidecar{pairs[0].Sidecar, pairs[1].Sidecar, pairs[2].Sidecar})
	if !strings.Contains(string(body), want) {
		t.Errorf("rendered topic missing computed fingerprint %s in body:\n%s", want, body)
	}
	if !strings.Contains(string(body), "_fingerprint:") {
		t.Error("rendered topic missing _fingerprint frontmatter key")
	}

	// (2) Mutating one contributing sidecar must bump the fingerprint
	//     AND trigger a re-render of the topic file.
	pairs[1].Sidecar.Directives[0].Text = "Second-mutated. (ref: ADR-002)"
	res2, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatalf("mutated merge: %v", err)
	}
	rerendered := false
	for _, n := range res2.TopicsRendered {
		if n == "alpha" {
			rerendered = true
			break
		}
	}
	if !rerendered {
		t.Errorf("alpha not re-rendered after mutation: TopicsRendered=%v Unchanged=%v",
			res2.TopicsRendered, res2.TopicsUnchanged)
	}
	bodyAfter, _ := os.ReadFile(topicPath)
	if string(bodyAfter) == string(body) {
		t.Error("topic body unchanged after sidecar mutation; fingerprint bump did not propagate to disk")
	}
	wantAfter := TopicFingerprint([]*sidecar.Sidecar{pairs[0].Sidecar, pairs[1].Sidecar, pairs[2].Sidecar})
	if wantAfter == want {
		t.Error("fingerprint did not change after mutating a contributing sidecar")
	}
	if !strings.Contains(string(bodyAfter), wantAfter) {
		t.Errorf("post-mutation topic missing new fingerprint %s in body:\n%s", wantAfter, bodyAfter)
	}

	// (3) A no-op re-render is byte-equal on disk, mtime-stable, and
	//     reports the topic as unchanged. This pins the
	//     writeAtomicIfChanged short-circuit.
	info1, err := os.Stat(topicPath)
	if err != nil {
		t.Fatal(err)
	}
	res3, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatalf("noop merge: %v", err)
	}
	bodyNoop, _ := os.ReadFile(topicPath)
	if string(bodyNoop) != string(bodyAfter) {
		t.Error("topic body diverged on no-op re-render; determinism broken")
	}
	for _, n := range res3.TopicsRendered {
		if n == "alpha" {
			t.Errorf("noop merge re-rendered alpha; short-circuit failed (TopicsRendered=%v)", res3.TopicsRendered)
		}
	}
	foundUnchanged := false
	for _, n := range res3.TopicsUnchanged {
		if n == "alpha" {
			foundUnchanged = true
			break
		}
	}
	if !foundUnchanged {
		t.Errorf("noop merge should report alpha unchanged, got %v", res3.TopicsUnchanged)
	}
	info2, err := os.Stat(topicPath)
	if err != nil {
		t.Fatal(err)
	}
	if !info1.ModTime().Equal(info2.ModTime()) {
		t.Errorf("topic mtime changed on no-op merge: before=%v after=%v",
			info1.ModTime(), info2.ModTime())
	}
}
