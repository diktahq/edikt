package phaseb

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ADR-079 (supersedes ADR-072's digest, amends ADR-066) — delivery is split by
// GRADE, and by nothing else:
//
//	advisory -> the topic file, in full, loaded on a paths: match BEFORE a write
//	must     -> directive-index.yaml, injected at write time by PreToolUse
//
// Neither grade is duplicated. This file replaces the ADR-072 dual-channel
// suite, which asserted the opposite for its subset: that an entry carrying a
// per-directive paths override renders into the topic body IN ADDITION TO the
// index. That subset is a subset of advisory, so the general rule replaced the
// narrow one rather than layering on it, and the "in addition to" assertion is
// now exactly wrong. The tests were not deleted — they were re-pointed at the
// rule that replaced theirs, so the surface stays covered rather than losing
// its tests along with its decision.

// mkGradeMixedPair builds a sidecar whose entries span both grades under the
// ADR-071 class table, without setting `enforcement` anywhere:
//
//	ADR + directive   -> advisory  (two of them, one also carrying an override)
//	ADR + prohibition -> must
//
// The paths override on the first directive is retained from the ADR-072
// fixture deliberately. Under ADR-079 it must no longer change WHERE the
// directive is delivered — only which globs it would be indexed under if it
// were must-grade. Keeping it proves the override stopped being a routing
// signal instead of merely going untested.
func mkGradeMixedPair(t *testing.T, projectRoot, basename, topic string) sidecar.Pair {
	t.Helper()
	p := mkPair(t, projectRoot, basename, topic, nil)
	p.Sidecar.Paths = []string{"a/**/*.go", "b/**/*.go", "c/**/*.go", "d/**/*.go", "e/**/*.go"}
	p.Sidecar.Directives = []sidecar.Directive{
		{
			Text:          "Advisory directive with its own override. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"},
			Paths:         []string{"a/**/*.go"},
		},
		{
			Text:          "Advisory directive, no override. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 2, LineEnd: 2, Quote: "z"},
		},
	}
	p.Sidecar.Prohibitions = []sidecar.Prohibition{
		{
			Text:          "MUST NOT do the prohibited thing. (ref: " + basename[:7] + ")",
			SourceExcerpt: sidecar.SourceExcerpt{LineStart: 3, LineEnd: 3, Quote: "y"},
			Paths:         []string{"a/**/*.go"},
			// enforcement: deny keeps this must after SR-003 removed the class-table
			// promotion. The fixture needs one must entry to test the split at all.
			Enforcement: "deny",
		},
	}
	return p
}

func mergeGradeMixed(t *testing.T) (root string, pairs []sidecar.Pair, topicBody string) {
	t.Helper()
	root = t.TempDir()
	pairs = []sidecar.Pair{mkGradeMixedPair(t, root, "ADR-001-test", "architecture")}
	if _, err := Merge(root, pairs, Options{
		CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test",
	}); err != nil {
		t.Fatalf("Merge: %v", err)
	}
	topicBody = readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	return root, pairs, topicBody
}

// Since ADR-083 there is one home per directive and it is the topic file (or
// the skill package for a tier-3 topic): grade no longer selects a surface,
// because the write-time surface that must-grade selected is retired. These
// tests pin the re-homing (PLAN-v090-release Phase 0, AC-0.3): both grades
// render into the topic file, nothing renders into an index, and no delivery
// note claims an empty region is "by design".

func TestMerge_AdvisoryRendersInTopic(t *testing.T) {
	_, _, body := mergeGradeMixed(t)
	for _, want := range []string{
		"Advisory directive with its own override",
		"Advisory directive, no override",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("advisory directive %q missing from the topic file body:\n%s", want, body)
		}
	}
}

func TestMerge_MustRendersInTopicToo(t *testing.T) {
	_, pairs, body := mergeGradeMixed(t)
	if !strings.Contains(body, "MUST NOT do the prohibited thing") {
		t.Errorf("must-grade prohibition missing from the topic file body — with the write-time surface retired (ADR-083) the topic file is its only home:\n%s", body)
	}
	dm := BuildDeliveryMap(pairs, map[string]bool{})
	wantID := pairs[0].ArtifactID + ":p01"
	var found bool
	for glob, entries := range dm {
		for _, e := range entries {
			if e.ID != wantID {
				continue
			}
			found = true
			if e.Grade != gradeMust {
				t.Errorf("grade must stay pinned on the map entry: got %q", e.Grade)
			}
			if e.Surface != SurfaceTopicFile {
				t.Errorf("must-grade prohibition routed to %q under %s, want %q", e.Surface, glob, SurfaceTopicFile)
			}
			if glob != "a/**/*.go" {
				t.Errorf("must-grade prohibition under %q, want its own override glob a/**/*.go only", glob)
			}
		}
	}
	if !found {
		t.Fatal("delivery map does not record the must-grade prohibition at all")
	}
}

func TestMerge_NoDirectiveIndexRendered(t *testing.T) {
	root, _, _ := mergeGradeMixed(t)
	assertNoDirectiveIndex(t, root)
	manifest := readFileString(t, filepath.Join(root, ".claude", "rules", "governance", "manifest.yaml"))
	if strings.Contains(manifest, "directive-index") {
		t.Errorf("manifest still declares a directive-index surface:\n%s", manifest)
	}
}

func TestMerge_NoDeliveryNoteOnTopicFile(t *testing.T) {
	_, _, body := mergeGradeMixed(t)
	for _, phrase := range []string{"empty by design", "EMPTY BY DESIGN", "directive-index.yaml"} {
		if strings.Contains(body, phrase) {
			t.Errorf("topic file carries retired write-time wording %q:\n%s", phrase, body)
		}
	}
}
