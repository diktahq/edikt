package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// scratchProjectForRemoval builds a project with a sidecar whose paths[] is
// exactly what the caller supplies, plus real files under test/ for globs to
// match against. When directiveNamesFile is true, the sidecar's one
// directive literally names test/integration/benchmarks/runner.py — the
// fixture the refusal test needs to lose coverage of, and the fixture the
// success/preserve tests need to keep covered.
func scratchProjectForRemoval(t *testing.T, paths []string, directiveNamesFile bool) (root, scPath string) {
	t.Helper()
	root = t.TempDir()

	mustMkAll(t, filepath.Join(root, ".edikt", "state", "pending-paths"))
	mustMkAll(t, filepath.Join(root, "docs", "architecture", "invariants"))
	mustMkAll(t, filepath.Join(root, "test", "integration", "benchmarks"))
	mustMkAll(t, filepath.Join(root, "test", "unit", "hooks"))

	mustWrite(t, filepath.Join(root, "test", "integration", "benchmarks", "runner.py"), "# runner\n")
	mustWrite(t, filepath.Join(root, "test", "unit", "hooks", "test_unrelated.sh"), "#!/bin/sh\n")

	mustWrite(t, filepath.Join(root, "docs", "architecture", "invariants", "INV-900-scratch.md"),
		"---\ntype: invariant\nid: INV-900\nstatus: accepted\n---\n\n## Rule\n\nSandboxes MUST be hermetic.\n")

	directiveText := "Sandboxes created by the harness MUST be hermetic. (ref: INV-900)"
	if directiveNamesFile {
		directiveText = "The benchmark runner MUST be maintained at `test/integration/benchmarks/runner.py`. (ref: INV-900)"
	}

	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/invariants/INV-900-scratch.md",
		Signals:       []string{"scratch hermetic sandboxes"},
		Paths:         paths,
		Directives: []sidecar.Directive{{
			Text: directiveText,
			SourceExcerpts: []sidecar.SourceExcerpt{{
				LineStart: 8, LineEnd: 8, Quote: "Sandboxes MUST be hermetic.",
			}},
		}},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal scratch sidecar: %v", err)
	}
	scPath = filepath.Join(root, "docs", "architecture", "invariants", "INV-900-scratch.edikt.yaml")
	mustWrite(t, scPath, string(body))

	return root, scPath
}

// TestProposedRemovalApproveRefusesNamedFileLoss is the ceremony's one hard
// block: a removal that would drop write-time coverage of a file the
// sidecar's own directive text names must be refused, not silently
// approved. The remaining glob here (test/unit/hooks/**/*) does not cover
// the named file test/integration/benchmarks/runner.py, so removing
// test/**/* strips its only coverage.
func TestProposedRemovalApproveRefusesNamedFileLoss(t *testing.T) {
	resetApproveFlags(t)
	root, scPath := scratchProjectForRemoval(t, []string{"test/**/*", "test/unit/hooks/**/*"}, true)
	t.Chdir(root)

	before, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}

	pending := writePendingPaths(t, root, "INV-900", `id: INV-900
sidecar_path: docs/architecture/invariants/INV-900-scratch.edikt.yaml
proposed_removals:
  - glob: "test/**/*"
    evidence: "narrowing to only the hooks-specific glob"
`)

	err = runApprovePaths("INV-900", "approve")
	if err == nil {
		t.Fatal("approve accepted a removal that drops a directive-named file")
	}
	ece, ok := err.(*exitCodeError)
	if !ok {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if ece.code != 1 {
		t.Errorf("exit code = %d, want 1", ece.code)
	}
	if !strings.Contains(ece.msg, "test/integration/benchmarks/runner.py") {
		t.Errorf("refusal does not name the lost file the directive text mentions:\n%s", ece.msg)
	}

	after, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("re-read sidecar: %v", err)
	}
	if string(after) != string(before) {
		t.Error("sidecar was mutated by a refused removal")
	}
	if _, err := os.Stat(pending); err != nil {
		t.Errorf("pending file was consumed by a refused removal: %v", err)
	}
}

// TestProposedRemovalApproveSucceedsAndWritesReceipt is the ceremony's
// positive half: a narrowing that does NOT drop any directive-named file
// succeeds, the glob is removed from paths[], and the receipt (same
// PathsApproval shape the additive path already uses) is written over the
// post-removal glob set. Here the remaining glob
// (test/integration/benchmarks/**/*) still covers the named file
// runner.py, so removing test/**/* is safe.
func TestProposedRemovalApproveSucceedsAndWritesReceipt(t *testing.T) {
	resetApproveFlags(t)
	root, scPath := scratchProjectForRemoval(t, []string{"test/**/*", "test/integration/benchmarks/**/*"}, true)
	t.Chdir(root)

	writePendingPaths(t, root, "INV-900", `id: INV-900
sidecar_path: docs/architecture/invariants/INV-900-scratch.edikt.yaml
proposed_removals:
  - glob: "test/**/*"
    evidence: "narrowing to the benchmarks-specific glob the directive text actually names"
`)

	if err := runApprovePaths("INV-900", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("reload sidecar: %v", err)
	}
	if len(sc.Paths) != 1 || sc.Paths[0] != "test/integration/benchmarks/**/*" {
		t.Fatalf("paths after removal = %v, want [test/integration/benchmarks/**/*]", sc.Paths)
	}
	if len(sc.ProposedRemovals) != 0 {
		t.Errorf("proposed_removals survived approval (%d entries) — the transient field must be stripped", len(sc.ProposedRemovals))
	}
	if sc.PathsApproval == nil {
		t.Fatal("paths_approval receipt was not written for a removal approval")
	}
	wantHash := sidecar.HashGlobs(sc.Paths)
	if sc.PathsApproval.GlobsSHA256 != wantHash {
		t.Errorf("paths_approval.globs_sha256 = %s, want %s (hash over the POST-removal glob set)", sc.PathsApproval.GlobsSHA256, wantHash)
	}
}

// TestProposedRemovalApproveConsidersSimultaneousAdditions covers the
// common real-world shape: a single approval both drops a broad glob AND
// adds its narrower evidenced replacements. The starting sidecar has ONLY
// "test/**/*" — so a remove-only preview would see nothing left covering
// the named file and wrongly refuse. The replacement glob proposed in the
// SAME pending file must count as coverage.
func TestProposedRemovalApproveConsidersSimultaneousAdditions(t *testing.T) {
	resetApproveFlags(t)
	root, scPath := scratchProjectForRemoval(t, []string{"test/**/*"}, true)
	t.Chdir(root)

	writePendingPaths(t, root, "INV-900", `id: INV-900
sidecar_path: docs/architecture/invariants/INV-900-scratch.edikt.yaml
proposed_paths:
  - glob: "test/integration/benchmarks/**/*"
    evidence: "replacement glob for the named file, added in the same approval as the removal below"
proposed_removals:
  - glob: "test/**/*"
    evidence: "narrowing test/**/* down to just its evidenced replacement, added above"
`)

	if err := runApprovePaths("INV-900", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("reload sidecar: %v", err)
	}
	if len(sc.Paths) != 1 || sc.Paths[0] != "test/integration/benchmarks/**/*" {
		t.Fatalf("paths after approve = %v, want [test/integration/benchmarks/**/*]", sc.Paths)
	}
}

// TestProposedRemovalApprovePreservesUnrelatedSidecarProposals is F-077's
// defect shape, run against the removal path specifically: an unrelated
// proposed_paths and proposed_removals entry the sidecar already carries —
// never mentioned by this pending removal — must survive the approval
// untouched. runApprovePaths was rewritten hours earlier to fix exactly
// this class of clobber on the additive path; this pins the same
// discipline against the new removal code sharing the same function.
func TestProposedRemovalApprovePreservesUnrelatedSidecarProposals(t *testing.T) {
	resetApproveFlags(t)
	root, scPath := scratchProjectForRemoval(t, []string{"test/**/*", "test/integration/benchmarks/**/*"}, false)
	t.Chdir(root)

	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("load scratch sidecar: %v", err)
	}
	// An extraction dispatch already wrote its own proposed_paths AND
	// proposed_removals directly onto the sidecar — both unrelated to what
	// the pending file below will propose.
	sc.ProposedPaths = []sidecar.ProposedPath{
		{Glob: "test/security/**/*", Evidence: "extractor's own untouched add-proposal, never reviewed"},
	}
	sc.ProposedRemovals = []sidecar.ProposedRemoval{
		{Glob: "test/integration/benchmarks/**/*", Evidence: "a DIFFERENT, unrelated removal proposal never reviewed"},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal sidecar: %v", err)
	}
	if err := os.WriteFile(scPath, body, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	// A separate, differently-sourced pending removal for the OTHER glob.
	writePendingPaths(t, root, "INV-900", `id: INV-900
sidecar_path: docs/architecture/invariants/INV-900-scratch.edikt.yaml
proposed_removals:
  - glob: "test/**/*"
    evidence: "an independent, hand-authored removal for a different glob"
`)

	if err := runApprovePaths("INV-900", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	after, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("reload sidecar: %v", err)
	}
	if len(after.Paths) != 1 || after.Paths[0] != "test/integration/benchmarks/**/*" {
		t.Fatalf("paths after approve = %v, want [test/integration/benchmarks/**/*]", after.Paths)
	}
	if len(after.ProposedPaths) != 1 || after.ProposedPaths[0].Glob != "test/security/**/*" {
		t.Fatalf("the sidecar's own unrelated proposed_paths entry was lost — got %v (F-077)", after.ProposedPaths)
	}
	if len(after.ProposedRemovals) != 1 || after.ProposedRemovals[0].Glob != "test/integration/benchmarks/**/*" {
		t.Fatalf("the sidecar's own unrelated proposed_removals entry was lost — got %v (F-077, removal path)", after.ProposedRemovals)
	}
}
