package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// scratchProject builds a minimal, self-contained project tree: a .edikt/
// marker, one governance artifact with a v2 sidecar, and a couple of real
// source files for globs to match against.
//
// It returns the project root. The caller chdirs into it, because every
// resolver in the ceremony anchors on CWD.
func scratchProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	mustMkdir(t, filepath.Join(root, ".edikt", "state", "pending-paths"))
	mustMkdir(t, filepath.Join(root, "docs", "architecture", "decisions"))
	mustMkdir(t, filepath.Join(root, "src", "engine"))

	mustWrite(t, filepath.Join(root, "src", "engine", "run.go"), "package engine\n")
	mustWrite(t, filepath.Join(root, "src", "engine", "state.go"), "package engine\n")

	mustWrite(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.md"),
		"---\ntype: adr\nid: ADR-900\nstatus: accepted\n---\n\n## Decision\n\nThe engine MUST record state.\n")

	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "compile",
		Path:          "docs/architecture/decisions/ADR-900-scratch.md",
		Signals:       []string{"scratch engine"},
		Directives: []sidecar.Directive{{
			Text: "The engine MUST record state. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{
				LineStart: 8, LineEnd: 8, Quote: "The engine MUST record state.",
			}},
		}},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal scratch sidecar: %v", err)
	}
	mustWrite(t, filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.edikt.yaml"), string(body))

	return root
}

// mustMkdir / mustWrite are the package's existing helpers (mustMkAll,
// mustWrite in doctor_routed_sources_test.go). Aliased here rather than
// redeclared.
func mustMkdir(t *testing.T, p string) { t.Helper(); mustMkAll(t, p) }

func writePendingPaths(t *testing.T, root, id, body string) string {
	t.Helper()
	p := filepath.Join(root, ".edikt", "state", "pending-paths", id+".yaml")
	mustWrite(t, p, body)
	return p
}

// resetApproveFlags restores the package-level flag vars between subtests.
// Cobra flag vars are global; a test that leaves --edited-content set would
// silently change the meaning of the next one.
func resetApproveFlags(t *testing.T) {
	t.Helper()
	prevKind, prevDecision, prevEdited, prevList := approveKind, approveDecision, approveEditedContent, approveList
	t.Cleanup(func() {
		approveKind, approveDecision, approveEditedContent, approveList = prevKind, prevDecision, prevEdited, prevList
	})
	approveKind, approveDecision, approveEditedContent, approveList = kindPaths, "", "", false
}

// TestProposedPathsApproveCeremonyPromotesGlobs is the ceremony half of
// AC-1.8: an approved proposal's globs land in the sidecar's enforced paths:,
// the transient proposed_paths field does not survive, and the pending file is
// consumed.
func TestProposedPathsApproveCeremonyPromotesGlobs(t *testing.T) {
	resetApproveFlags(t)
	root := scratchProject(t)
	t.Chdir(root)

	pending := writePendingPaths(t, root, "ADR-900", `id: ADR-900
sidecar_path: docs/architecture/decisions/ADR-900-scratch.edikt.yaml
proposed_at: "2026-08-12T00:00:00Z"
proposed_paths:
  - glob: "src/engine/**/*.go"
    evidence: "ADR-900's decision names the engine package as the thing that records state."
`)

	if err := runApprovePaths("ADR-900", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	sc, err := sidecar.Load(filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.edikt.yaml"))
	if err != nil {
		t.Fatalf("reload sidecar: %v", err)
	}
	if len(sc.Paths) != 1 || sc.Paths[0] != "src/engine/**/*.go" {
		t.Fatalf("paths after approve = %v, want [src/engine/**/*.go]", sc.Paths)
	}
	if len(sc.ProposedPaths) != 0 {
		t.Errorf("proposed_paths survived approval (%d entries) — the transient field must be stripped", len(sc.ProposedPaths))
	}
	if _, err := os.Stat(pending); !os.IsNotExist(err) {
		t.Errorf("pending file still present after approval: %v", err)
	}
}

// TestProposedPathsApproveRefusesInvalidProposal is the ceremony's fail-closed
// half. The mechanical validation re-runs at approval time against the live
// tree, so a human cannot rubber-stamp a glob that matches nothing — and the
// refusal must NAME the failing glob rather than reporting a bare error.
//
// The isolation control matters as much as the refusal: the sidecar must be
// byte-identical afterwards. A validator that refuses but has already written
// half the globs has not refused.
func TestProposedPathsApproveRefusesInvalidProposal(t *testing.T) {
	resetApproveFlags(t)
	root := scratchProject(t)
	t.Chdir(root)

	scPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.edikt.yaml")
	before, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}

	pending := writePendingPaths(t, root, "ADR-900", `id: ADR-900
sidecar_path: docs/architecture/decisions/ADR-900-scratch.edikt.yaml
proposed_paths:
  - glob: "src/engine/**/*.go"
    evidence: "This one is fine and would be approved on its own."
  - glob: "src/nonexistent/**/*.go"
    evidence: "This names a directory that was never created in this project."
`)

	err = runApprovePaths("ADR-900", "approve")
	if err == nil {
		t.Fatal("approve accepted a proposal containing a glob that matches nothing")
	}
	ece, ok := err.(*exitCodeError)
	if !ok {
		t.Fatalf("expected exitCodeError, got %T: %v", err, err)
	}
	if ece.code != 1 {
		t.Errorf("exit code = %d, want 1 (validation error)", ece.code)
	}
	if !strings.Contains(ece.msg, "src/nonexistent/**/*.go") {
		t.Errorf("refusal does not name the offending glob:\n%s", ece.msg)
	}
	if !strings.Contains(ece.msg, "no-match") {
		t.Errorf("refusal does not name the rule that fired:\n%s", ece.msg)
	}

	after, err := os.ReadFile(scPath)
	if err != nil {
		t.Fatalf("re-read sidecar: %v", err)
	}
	if string(after) != string(before) {
		t.Error("sidecar was mutated by a refused approval")
	}
	if _, err := os.Stat(pending); err != nil {
		t.Errorf("pending file was consumed by a refused approval: %v", err)
	}
}

// TestProposedPathsApprovePreservesUnrelatedSidecarProposals is F-077's
// regression: a sidecar can carry its own proposed_paths (written directly by
// an extraction dispatch) that are entirely independent of a pending file
// approved through the ceremony for that same sidecar — ADR-062's own
// extraction proposed two paths, and a later, differently-sourced manual
// proposal approved for a third path discarded both extractor originals,
// including one nobody had reviewed or rejected. Approving one glob must not
// silently erase proposals it was never asked about.
func TestProposedPathsApprovePreservesUnrelatedSidecarProposals(t *testing.T) {
	resetApproveFlags(t)
	root := scratchProject(t)
	t.Chdir(root)

	scPath := filepath.Join(root, "docs", "architecture", "decisions", "ADR-900-scratch.edikt.yaml")
	sc, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("load scratch sidecar: %v", err)
	}
	// Simulate an extraction dispatch that already wrote its own
	// proposed_paths directly onto the sidecar, unrelated to anything a
	// pending-paths file will separately propose.
	sc.ProposedPaths = []sidecar.ProposedPath{
		{Glob: "src/engine/state.go", Evidence: "extractor's own untouched proposal, never reviewed"},
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshal sidecar with proposed_paths: %v", err)
	}
	if err := os.WriteFile(scPath, body, 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	// A separate, differently-sourced pending proposal for a DIFFERENT glob,
	// approved through the normal ceremony.
	writePendingPaths(t, root, "ADR-900", `id: ADR-900
sidecar_path: docs/architecture/decisions/ADR-900-scratch.edikt.yaml
proposed_paths:
  - glob: "src/engine/run.go"
    evidence: "an independent, hand-authored proposal for a different file"
`)

	if err := runApprovePaths("ADR-900", "approve"); err != nil {
		t.Fatalf("approve: %v", err)
	}

	after, err := sidecar.Load(scPath)
	if err != nil {
		t.Fatalf("reload sidecar: %v", err)
	}
	if len(after.Paths) != 1 || after.Paths[0] != "src/engine/run.go" {
		t.Fatalf("paths after approve = %v, want [src/engine/run.go]", after.Paths)
	}
	if len(after.ProposedPaths) != 1 || after.ProposedPaths[0].Glob != "src/engine/state.go" {
		t.Fatalf("the sidecar's own unrelated proposed_paths entry was lost — got %v, want [src/engine/state.go] to survive (F-077)", after.ProposedPaths)
	}
}
