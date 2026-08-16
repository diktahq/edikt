package phaseb

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func mkPair(t *testing.T, projectRoot, basename, topic string, directives []sidecar.Directive) sidecar.Pair {
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
		// A DECLARED glob, so the topic stays at tier 2 and renders a rules
		// file. Without it the fixture is an UNSCOPED topic, which since
		// SPEC-011 stage 1 retires to tier 3 and writes no topic file at all —
		// and every merge/render/fingerprint/suppression test here would fail
		// on a missing file for a reason that has nothing to do with its
		// subject. The tier decision has its own tests in
		// twophase_scope_test.go; these fixtures represent a scoped topic
		// because that is the case they were written to exercise.
		Paths:      []string{"src/**/*.go"},
		Signals:    []string{"x"},
		Directives: directives,
		SourcePath: sidecarPath,
	}
	if err := os.WriteFile(sidecarPath, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{
		ParentPath:  parentPath,
		SidecarPath: sidecarPath,
		ArtifactID:  basename[:7], // "ADR-001"
		Sidecar:     sc,
	}
}

func TestMerge_WritesTopicAndIndex(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "architecture", []sidecar.Directive{
			{Text: "First directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	res, err := Merge(root, pairs, Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if !res.IndexWritten {
		t.Error("expected index to be written on first run")
	}
	if len(res.TopicsRendered) != 1 || res.TopicsRendered[0] != "architecture" {
		t.Errorf("expected ['architecture'] rendered, got %v", res.TopicsRendered)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "rules", "governance", "architecture.md")); err != nil {
		t.Errorf("topic file missing: %v", err)
	}
}

func TestMerge_IsIdempotent(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "architecture", []sidecar.Directive{
			{Text: "Same directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}

	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	first, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}

	res2, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatalf("second merge: %v", err)
	}
	second, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "architecture.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) {
		t.Error("byte-equal input must produce byte-equal output")
	}
	if len(res2.TopicsRendered) != 0 {
		t.Errorf("idempotent rerun should report 0 rendered, got %v", res2.TopicsRendered)
	}
	if len(res2.TopicsUnchanged) != 1 {
		t.Errorf("idempotent rerun should report 1 unchanged, got %v", res2.TopicsUnchanged)
	}
}

func TestMerge_OnlyAffectedTopicChanges(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "Alpha directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
		mkPair(t, root, "ADR-002-test", "beta", []sidecar.Directive{
			{Text: "Beta directive. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}

	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatal(err)
	}
	betaPath := filepath.Join(root, ".claude", "rules", "governance", "beta.md")
	betaBefore, _ := os.ReadFile(betaPath)

	pairs[0].Sidecar.Directives[0].Text = "Alpha modified. (ref: ADR-001)"
	res, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatal(err)
	}
	betaAfter, _ := os.ReadFile(betaPath)

	if string(betaBefore) != string(betaAfter) {
		t.Error("beta topic changed unexpectedly when only alpha sidecar mutated")
	}
	rendered := map[string]bool{}
	for _, n := range res.TopicsRendered {
		rendered[n] = true
	}
	if !rendered["alpha"] {
		t.Errorf("alpha must be rendered, got %v", res.TopicsRendered)
	}
	if rendered["beta"] {
		t.Errorf("beta must NOT be re-rendered, got %v", res.TopicsRendered)
	}
}

// TestMerge_FingerprintSkipsRender pins the Phase 8 invariant: when a topic's
// contributing-sidecar fingerprint hasn't changed, Merge MUST NOT re-render
// the topic file even if the timestamp / compiler version differ between
// runs. The pre-Phase-8 byte-equal check would re-write on every CompiledAt
// shift; the fingerprint short-circuit fixes that.
func TestMerge_FingerprintSkipsRender(t *testing.T) {
	// The cache short-circuit must prevent a rewrite when nothing changed.
	//
	// This test used to prove that by asserting the rendered `compiled_at`
	// stamp had not moved. SPEC-011 removed compiled_at from hashed surfaces —
	// a value that changes on every no-op compile defeats the determinism
	// ADR-020 requires — so that observable is gone. The PROPERTY is not:
	// re-pointed at mtime, which still distinguishes "not rewritten" from
	// "rewritten identically". Deleting the test would have discarded a real
	// decision along with its obsolete instrument.
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "Alpha v1. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}
	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("first merge: %v", err)
	}
	topic := filepath.Join(root, ".claude", "rules", "governance", "alpha.md")
	before, err := os.Stat(topic)
	if err != nil {
		t.Fatalf("stat after first merge: %v", err)
	}

	time.Sleep(10 * time.Millisecond)
	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatalf("second merge: %v", err)
	}
	after, err := os.Stat(topic)
	if err != nil {
		t.Fatalf("stat after second merge: %v", err)
	}
	if !after.ModTime().Equal(before.ModTime()) {
		t.Error("topic file was rewritten on an unchanged corpus; the cache short-circuit failed")
	}
}

func TestMerge_FingerprintBustsOnContentChange(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "Alpha v1. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	opts := Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}

	if _, err := Merge(root, pairs, opts); err != nil {
		t.Fatal(err)
	}

	pairs[0].Sidecar.Directives[0].Text = "Alpha v2. (ref: ADR-001)"
	res, err := Merge(root, pairs, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.TopicsRendered) != 1 || res.TopicsRendered[0] != "alpha" {
		t.Errorf("expected alpha to re-render after content change, got %v", res.TopicsRendered)
	}
}

func TestMerge_FingerprintEmbeddedInFrontmatter(t *testing.T) {
	root := t.TempDir()
	pairs := []sidecar.Pair{
		mkPair(t, root, "ADR-001-test", "alpha", []sidecar.Directive{
			{Text: "Alpha directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
		}),
	}
	if _, err := Merge(root, pairs, Options{CompiledAt: "2026-05-02T00:00:00Z", CompilerVersion: "0.6.0-test"}); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "alpha.md"))
	if !contains(body, "_fingerprint:") {
		t.Errorf("topic file missing _fingerprint frontmatter:\n%s", string(body))
	}

	want := TopicRenderFingerprint([]*sidecar.Sidecar{pairs[0].Sidecar}, "")
	if !contains(body, want) {
		t.Errorf("fingerprint %s not present in topic file:\n%s", want, string(body))
	}
}

func contains(haystack []byte, needle string) bool {
	return len(haystack) > 0 && len(needle) > 0 && indexOf(string(haystack), needle) >= 0
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// mkINVPair builds a sidecar.Pair for an invariant artifact.
func mkINVPair(t *testing.T, projectRoot, basename, topic string, directives []sidecar.Directive, reminders, verification []string) sidecar.Pair {
	t.Helper()
	dir := filepath.Join(projectRoot, "docs", "architecture", "invariants")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentPath := filepath.Join(dir, basename+".md")
	sidecarPath := filepath.Join(dir, basename+".edikt.yaml")
	if err := os.WriteFile(parentPath, []byte("# placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	vEntries := make([]sidecar.VerificationEntry, len(verification))
	for i, t := range verification {
		vEntries[i] = sidecar.VerificationEntry{Text: t}
	}
	sc := &sidecar.Sidecar{
		SchemaVersion: 1,
		Topic:         topic,
		Path:          "docs/architecture/invariants/" + basename + ".md",
		Signals:       []string{"inv-signal"},
		Directives:    directives,
		Reminders:     reminders,
		Verification:  vEntries,
		SourcePath:    sidecarPath,
	}
	if err := os.WriteFile(sidecarPath, []byte("placeholder\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{
		ParentPath:  parentPath,
		SidecarPath: sidecarPath,
		ArtifactID:  basename[:7], // "INV-001"
		Sidecar:     sc,
	}
}

// TestMerge_INVDirectivesNotInTopicFiles pins the routing fix: INV directives
// must appear in governance.md Non-Negotiable Constraints ONLY. They must
// never appear in a topic file, even when the INV shares a topic with ADRs.
//
// The ADR directive's home moved with ADR-066: a scoped ADR sidecar (the
// ordinary shape, declaring paths:) now renders its directive text into
// directive-index.yaml only, never into the topic file — so the "ADR
// directive lands somewhere real" half of this test checks the index, not
// ai.md. The "INV directive never reaches the topic file" half is
// unaffected: it was already excluded before this ADR, for a different
// reason (routed to the ambient core instead), and stays excluded now.
func TestMerge_INVDirectivesNotInTopicFiles(t *testing.T) {
	root := t.TempDir()
	adrPair := mkPair(t, root, "ADR-001-test", "ai", []sidecar.Directive{
		{Text: "ADR directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	adrPair.ArtifactID = "ADR-001"

	invPair := mkINVPair(t, root, "INV-001-test", "ai", []sidecar.Directive{
		{Text: "INV directive. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	}, nil, nil)
	invPair.ArtifactID = "INV-001"

	_, err := Merge(root, []sidecar.Pair{adrPair, invPair}, Options{
		CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	topicBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "ai.md"))
	if err != nil {
		t.Fatalf("read topic file: %v", err)
	}
	if contains(topicBody, "INV directive") {
		t.Error("INV directive must NOT appear in topic file ai.md — it belongs in governance.md constraints only")
	}
	if contains(topicBody, "ADR directive") {
		t.Error("scoped ADR directive must NOT appear in topic file ai.md since ADR-066 — it belongs in directive-index.yaml only")
	}
	if !contains([]byte(readDirectiveIndex(t, root)), "ADR directive") {
		t.Error("ADR directive must appear in directive-index.yaml")
	}

	indexBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !contains(indexBody, "INV directive") {
		t.Error("INV directive must appear in governance.md Non-Negotiable Constraints")
	}
}

// TestMerge_INVOnlyTopicNotWritten verifies that when a topic has ONLY INV
// sidecars (no ADRs/guidelines), no topic file is written for it. An empty
// topic file would be noise in the routing table.
func TestMerge_INVOnlyTopicNotWritten(t *testing.T) {
	root := t.TempDir()
	invPair := mkINVPair(t, root, "INV-005-test", "frontend", []sidecar.Directive{
		{Text: "UI invariant. (ref: INV-005)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "z"}},
	}, nil, nil)
	invPair.ArtifactID = "INV-005"

	res, err := Merge(root, []sidecar.Pair{invPair}, Options{
		CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}
	if len(res.TopicsRendered) != 0 {
		t.Errorf("INV-only topic must not produce a topic file; got TopicsRendered=%v", res.TopicsRendered)
	}
	if _, err := os.Stat(filepath.Join(root, ".claude", "rules", "governance", "frontend.md")); err == nil {
		t.Error("frontend.md must not be written when all contributors are INVs")
	}

	// The INV directive must still appear in governance.md.
	indexBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}
	if !contains(indexBody, "UI invariant") {
		t.Error("INV-only topic directive must appear in governance.md Non-Negotiable Constraints")
	}
}

// TestMerge_RemindersAndVerificationAggregated pins the reminders/verification
// fix: items from all sidecars (ADRs and INVs) must appear in governance.md
// and must be empty when no sidecars provide them.
func TestMerge_RemindersAndVerificationAggregated(t *testing.T) {
	root := t.TempDir()

	adrPair := mkPair(t, root, "ADR-001-test", "ai", []sidecar.Directive{
		{Text: "ADR directive. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	adrPair.ArtifactID = "ADR-001"
	adrPair.Sidecar.Reminders = []string{"Before acting on ADR-001 → check the constraint (ref: ADR-001)"}
	adrPair.Sidecar.Verification = []sidecar.VerificationEntry{{Text: "[ ] ADR-001 handler accepts only AI client (ref: ADR-001)"}}

	invPair := mkINVPair(t, root, "INV-001-test", "ai", []sidecar.Directive{
		{Text: "INV directive. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	},
		[]string{"Before AI derivation → verify confidence is draft or ghost (ref: INV-001)"},
		[]string{"[ ] AI derivation sets confidence draft or ghost (ref: INV-001)"},
	)
	invPair.ArtifactID = "INV-001"

	_, err := Merge(root, []sidecar.Pair{adrPair, invPair}, Options{
		CompiledAt: "2026-05-03T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("Merge: %v", err)
	}

	indexBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("read index: %v", err)
	}

	// WHAT THIS ASSERTS, AND WHY IT MOVED AGAIN.
	//
	// D37 split reminders: INV-sourced stayed always-on in governance.md, ADR-
	// and guideline-sourced moved to topic files. SPEC-011 stage 1 goes
	// further — governance.md carries NO Reminders section at all, because a
	// list that loads on every edit is the ambient cost this release exists to
	// remove.
	//
	// The reasoning that kept INV reminders always-on ("an invariant's
	// reminder qualifies a non-negotiable, and scoping it would load the
	// constraint while its qualifier stayed behind a glob") is answered rather
	// than overruled: the qualifier is now delivered at WRITE-TOUCH time via
	// the directive index, and for pathless artifacts via the topic's skill
	// package. It reaches the reader at the moment it applies instead of on
	// every unrelated edit.
	//
	// So the assertion inverts for governance.md and MUST be paired with a
	// destination check. Asserting only "absent from the core" would pass if
	// the content had been DROPPED — which is exactly the silent loss this
	// suite has to be able to catch.
	if contains(indexBody, "Before AI derivation") {
		t.Error("governance.md must no longer carry a Reminders section (SPEC-011 stage 1)")
	}

	// The destination check: it moved, it did not vanish.
	//
	// THREE candidate homes, and the set had to grow. A pathless artifact has
	// no glob, so the glob-keyed directive index has no key for its reminders;
	// its skill package was the only home until scoped topics' skills became
	// POINTER STUBS (AC-2.2, strong form). At that point the reminder reached
	// nothing — so the topic file, the scoped topic's one home, took it.
	//
	// The check is a disjunction over every surface a reminder may legitimately
	// land on, because WHICH one is a tier decision that has now moved twice.
	// What must never change is that it lands somewhere.
	skill, serr := os.ReadFile(filepath.Join(root, ".claude", "skills", "edikt-ai", "SKILL.md"))
	idx, ierr := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "directive-index.yaml"))
	topic, terr := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "ai.md"))
	if serr != nil && ierr != nil && terr != nil {
		t.Fatalf("no destination surface exists at all: skill=%v index=%v topic=%v", serr, ierr, terr)
	}
	if !contains(skill, "Before AI derivation") &&
		!contains(idx, "Before AI derivation") &&
		!contains(topic, "Before AI derivation") {
		t.Error("the INV reminder reached NO surface — moved out of the ambient core and dropped, " +
			"which is the silent loss the split must not cause")
	}

	// ADR-sourced: no longer always-on.
	if contains(indexBody, "Before acting on ADR-001") {
		t.Error("ADR reminder must NOT be in governance.md — it is artifact-scoped and belongs behind the topic file's paths: glob")
	}
	if contains(indexBody, "[ ] ADR-001 handler accepts only AI client") {
		t.Error("ADR verification item must NOT be in governance.md")
	}

	// THE CONSERVATION CHECK (BRAIN-002 §7.10). A split is a claim that
	// nothing was lost, and the only way to check that claim is to count both
	// sides. Without this block, "ADR content is not in governance.md" passes
	// IDENTICALLY whether the content moved or was DROPPED — which is exactly
	// what happened on the first cut of this change: 335 lines left
	// governance.md, 95 landed nowhere, totals went 1168 -> 1073, and every
	// test passed. It was caught by accounting, not by an assertion.
	topics, err := filepath.Glob(filepath.Join(root, ".claude", "rules", "governance", "*.md"))
	if err != nil || len(topics) == 0 {
		t.Fatalf("no topic files rendered: %v", err)
	}
	var foundReminder, foundVerify bool
	for _, tf := range topics {
		b, rerr := os.ReadFile(tf)
		if rerr != nil {
			continue
		}
		if contains(b, "Before acting on ADR-001") {
			foundReminder = true
		}
		if contains(b, "[ ] ADR-001 handler accepts only AI client") {
			foundVerify = true
		}
	}
	if !foundReminder {
		t.Error("ADR reminder left governance.md but landed in no topic file — content was DROPPED")
	}
	if !foundVerify {
		t.Error("ADR verification item left governance.md but landed in no topic file — content was DROPPED")
	}
}

// TestMerge_PathlessInvariantDoesNotRetireItsTopic is the named regression
// test for a defect the tier work introduced.
//
// The scope union counted EVERY contributing sidecar, including a pathless
// invariant. But a pathless invariant contributes only its canonical
// statement to the ambient core and nothing at all to the topic file — so a
// topic whose ADRs were properly scoped was still reported unscoped, and
// since SPEC-011 stage 1 would be RETIRED TO TIER 3. Scoped tier-2 rules
// would leave the reader's path on account of content that is not in the
// file, and declaring globs on the invariant is not even the fix, because a
// scoped invariant changes tier itself.
//
// RED BEFORE GREEN: with the `contributesToTopicFile` guard removed from
// merge.go's union, this fails on the first assertion — the topic file does
// not exist, because the topic retired.
//
// The dogfood corpus is UNAFFECTED (its six retired topics have genuinely
// undeclared ADRs, and the measured ambient figure is identical either way),
// which is exactly why this needs its own test: nothing else in the suite
// would have reported the regression returning.
func TestMerge_PathlessInvariantDoesNotRetireItsTopic(t *testing.T) {
	root := t.TempDir()

	adr := mkPair(t, root, "ADR-001-test", "arch", []sidecar.Directive{
		{Text: "Scoped ADR rule. (ref: ADR-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	adr.ArtifactID = "ADR-001"

	inv := mkINVPair(t, root, "INV-001-test", "arch", []sidecar.Directive{
		{Text: "Pathless invariant MUST hold. (ref: INV-001)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	}, nil, nil)
	inv.ArtifactID = "INV-001"
	inv.Sidecar.Paths = nil // pathless: routes to the ambient core

	res, err := Merge(root, []sidecar.Pair{adr, inv}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	topic := filepath.Join(root, ".claude", "rules", "governance", "arch.md")
	if _, rerr := os.Stat(topic); rerr != nil {
		t.Fatalf("scoped topic was retired because a PATHLESS INVARIANT shares its "+
			"topic; the invariant contributes nothing to this file: %v (retired=%v)",
			rerr, res.TopicsRetiredToSkill)
	}
	// ADR-066: a scoped ADR's directive text renders into directive-index.yaml,
	// not the topic file — the topic file's job here is just to still EXIST
	// (proving the pathless invariant didn't retire it), which the stat above
	// already checked.
	if !strings.Contains(readDirectiveIndex(t, root), "Scoped ADR rule.") {
		t.Errorf("scoped ADR directive missing from directive-index.yaml")
	}
	for _, n := range res.TopicsRetiredToSkill {
		if n == "arch" {
			t.Errorf("topic published as retired despite being fully scoped by its contributors")
		}
	}

	// CONTROL — the guard must not disable retirement generally. An ADR with
	// no globs DOES hold its topic open, because its directives are in the
	// file. Without this, deleting the union entirely would pass the test
	// above (isolation, per GL-002).
	root2 := t.TempDir()
	openAdr := mkPair(t, root2, "ADR-002-test", "arch", []sidecar.Directive{
		{Text: "Undeclared ADR rule. (ref: ADR-002)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "z"}},
	})
	openAdr.ArtifactID = "ADR-002"
	openAdr.Sidecar.Paths = nil

	res2, err := Merge(root2, []sidecar.Pair{openAdr}, Options{
		CompiledAt: "2026-08-12T00:00:00Z", CompilerVersion: "0.6.0-test",
	})
	if err != nil {
		t.Fatalf("merge (control): %v", err)
	}
	retired := false
	for _, n := range res2.TopicsRetiredToSkill {
		if n == "arch" {
			retired = true
		}
	}
	if !retired {
		t.Errorf("control failed: an UNDECLARED ADR must still retire its topic to tier 3; got %v",
			res2.TopicsRetiredToSkill)
	}
}

// TestMerge_TopicsNewlyUnreachable_FlagsARealRegression — F-115/A3. A topic
// with a tier-2 rules file ALREADY ON DISK (simulating a prior compile),
// whose only contributor loses its declared paths — the shape of a topic
// reassignment where the artifact's new topic ends up with no scoped
// contributor at all. TopicsRetiredToSkill fires either way (it always did);
// TopicsNewlyUnreachable must fire ONLY here, because this run is what took
// reachability away.
func TestMerge_TopicsNewlyUnreachable_FlagsARealRegression(t *testing.T) {
	root := t.TempDir()

	govDir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(govDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Simulate "this topic had a tier-2 file before this compile" —
	// PathsGoverned, per merge.go's own header contract, is the file whose
	// prior existence TopicsNewlyUnreachable checks for.
	stalePath := filepath.Join(govDir, "collaboration.md")
	if err := os.WriteFile(stalePath, []byte("# stub, simulating a prior compile's output\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The only contributor to "collaboration" in THIS run has no declared
	// paths — exactly the annotakt shape: an artifact moved into a topic
	// with no scoped contributor.
	adr := mkPair(t, root, "ADR-039-test", "collaboration", []sidecar.Directive{
		{Text: "Some collaboration rule. (ref: ADR-039)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "x"}},
	})
	adr.ArtifactID = "ADR-039"
	adr.Sidecar.Paths = nil

	res, err := Merge(root, []sidecar.Pair{adr}, Options{
		CompiledAt: "2026-08-16T00:00:00Z", CompilerVersion: "0.7.0-test",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	found := false
	for _, n := range res.TopicsNewlyUnreachable {
		if n == "collaboration" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a topic with a tier-2 file on disk before this run, now unscoped, was not reported as "+
			"newly unreachable — the reachability-regression signal this test exists to prove: %v",
			res.TopicsNewlyUnreachable)
	}
	if _, statErr := os.Stat(stalePath); statErr == nil {
		t.Error("the stale topic file should have been removed by this compile (retirement itself still happens)")
	}
}

// TestMerge_TopicsNewlyUnreachable_OmitsAlreadyUnreachable — the control.
// A topic that was NEVER on disk (a fresh clone, or a topic that was always
// skill-only) must not appear in TopicsNewlyUnreachable even though it is
// unscoped and DOES appear in TopicsRetiredToSkill (unchanged, existing
// behavior). Without this control, a version of the fix that reported EVERY
// unscoped topic as "newly unreachable" would pass the regression test above
// and still be wrong — it would alarm on every fresh clone.
func TestMerge_TopicsNewlyUnreachable_OmitsAlreadyUnreachable(t *testing.T) {
	root := t.TempDir()
	// Deliberately no pre-existing .claude/rules/governance/ directory at
	// all — a genuinely fresh clone.

	adr := mkPair(t, root, "ADR-004-test", "data-access", []sidecar.Directive{
		{Text: "Some data-access rule. (ref: ADR-004)", SourceExcerpt: sidecar.SourceExcerpt{LineStart: 1, LineEnd: 1, Quote: "y"}},
	})
	adr.ArtifactID = "ADR-004"
	adr.Sidecar.Paths = nil

	res, err := Merge(root, []sidecar.Pair{adr}, Options{
		CompiledAt: "2026-08-16T00:00:00Z", CompilerVersion: "0.7.0-test",
	})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}

	retired := false
	for _, n := range res.TopicsRetiredToSkill {
		if n == "data-access" {
			retired = true
		}
	}
	if !retired {
		t.Fatalf("control precondition failed: data-access should still appear in TopicsRetiredToSkill "+
			"(unchanged behavior): %v", res.TopicsRetiredToSkill)
	}
	for _, n := range res.TopicsNewlyUnreachable {
		if n == "data-access" {
			t.Fatalf("a topic that never had a tier-2 file (fresh clone) was reported as a reachability "+
				"REGRESSION — false alarm on the common case: %v", res.TopicsNewlyUnreachable)
		}
	}
}
