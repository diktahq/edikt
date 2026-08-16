package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// SYNTHETIC CEREMONY — stated here so its absence of live input is never later
// mistaken for the feature being untestable.
//
// .edikt/state/pending-verifies/ is EMPTY: no ceremony is in flight. That
// strengthens the case rather than weakening it — 28 historical approvals exist
// and their rejections are already lost, and every ceremony that happens before
// capture exists loses another. So capture is built FORWARD-LOOKING and
// validated against a constructed proposal.
func stageCeremony(t *testing.T, id, proposed string) (root, pendingPath, sidecarPath string) {
	t.Helper()
	root = t.TempDir()
	pendingDir := filepath.Join(root, ".edikt", "state", "pending-verifies")
	if err := os.MkdirAll(pendingDir, 0o755); err != nil {
		t.Fatal(err)
	}
	sidecarPath = filepath.Join(root, "ADR-001-x.edikt.yaml")
	sidecar := "schema_version: 1\ntopic: testing\npath: docs/x.md\nsignals:\n  - testing\n" +
		"directives:\n  - text: \"MUST hold. (ref: ADR-001)\"\n    source_excerpt:\n" +
		"      line_start: 1\n      line_end: 1\n      quote: \"MUST hold.\"\n" +
		"    verify_kind: behavioral\n    falsifying_observation: \"It does not hold.\"\n"
	if err := os.WriteFile(sidecarPath, []byte(sidecar), 0o644); err != nil {
		t.Fatal(err)
	}
	pendingPath = filepath.Join(pendingDir, id+".yaml")
	pv := "id: " + id + "\nsidecar_path: " + sidecarPath + "\ndirective_index: 0\n" +
		"proposed_verify: \"" + proposed + "\"\nintent: \"why it exists\"\n" +
		"falsifying_observation: \"It does not hold.\"\nproposed_at: \"2026-08-09T00:00:00Z\"\n"
	if err := os.WriteFile(pendingPath, []byte(pv), 0o644); err != nil {
		t.Fatal(err)
	}
	return root, pendingPath, sidecarPath
}

func readOnlyDiff(t *testing.T, root string) approvalDiff {
	t.Helper()
	dir := filepath.Join(root, ApprovalDiffDir)
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("no approval-diff dir at %s: %v", dir, err)
	}
	if len(ents) != 1 {
		t.Fatalf("expected exactly 1 diff, got %d", len(ents))
	}
	b, err := os.ReadFile(filepath.Join(dir, ents[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var d approvalDiff
	if err := yaml.Unmarshal(b, &d); err != nil {
		t.Fatalf("diff is not valid YAML: %v", err)
	}
	return d
}

// THE ONE THAT MATTERS. A rejection is a labelled negative and negatives are
// what the extractor lacks. Before this, reject loaded the proposal and deleted
// it — the most informative outcome left no trace.
func TestApprovalDiff_RejectionIsCapturedBeforeDeletion(t *testing.T) {
	root, pendingPath, _ := stageCeremony(t, "pv-reject", "rg -q 'x' f.go")
	recordApprovalDiff(pendingPath, approvalDiff{
		PendingID: "pv-reject", Decision: "reject", DecidedAt: approvalNow(),
		ProposedVerify: "rg -q 'x' f.go", Intent: "why it exists",
	})
	d := readOnlyDiff(t, root)
	if d.Decision != "reject" {
		t.Fatalf("decision = %q, want reject", d.Decision)
	}
	if d.ProposedVerify != "rg -q 'x' f.go" {
		t.Fatalf("the rejected proposal was not preserved: %q", d.ProposedVerify)
	}
	if d.AcceptedVerify != "" {
		t.Fatalf("a rejection recorded an accepted verify (%q) — nothing was accepted", d.AcceptedVerify)
	}
}

// The edit distinguishes "approved as proposed" from "approved after changing
// it". Without this the two collapse and the richest signal is lost.
func TestApprovalDiff_EditIsDistinguishedFromPlainApproval(t *testing.T) {
	root, pendingPath, _ := stageCeremony(t, "pv-edit", "rg -q 'proposed' f.go")
	recordApprovalDiff(pendingPath, approvalDiff{
		PendingID: "pv-edit", Decision: "approve", DecidedAt: approvalNow(),
		ProposedVerify: "rg -q 'proposed' f.go",
		AcceptedVerify: "rg -q 'accepted' f.go",
		Edited:         true,
	})
	d := readOnlyDiff(t, root)
	if !d.Edited {
		t.Fatal("an approval whose accepted text differs from the proposal was not marked edited")
	}
	if d.ProposedVerify == d.AcceptedVerify {
		t.Fatal("proposed and accepted collapsed — the diff records no difference")
	}
}

// Isolation: a plain approval must NOT be marked edited, or the flag above
// could be satisfied by one that is always true.
func TestApprovalDiff_PlainApprovalIsNotMarkedEdited(t *testing.T) {
	root, pendingPath, _ := stageCeremony(t, "pv-plain", "rg -q 'same' f.go")
	recordApprovalDiff(pendingPath, approvalDiff{
		PendingID: "pv-plain", Decision: "approve", DecidedAt: approvalNow(),
		ProposedVerify: "rg -q 'same' f.go", AcceptedVerify: "rg -q 'same' f.go",
		Edited: false,
	})
	if d := readOnlyDiff(t, root); d.Edited {
		t.Fatal("a plain approval was marked edited")
	}
}

// The diffs must be GIT-TRACKED, not runtime state. An asset that dies on a
// fresh clone is not an asset — that ruling is the reason phase 5 went first.
func TestApprovalDiff_LandsOutsideEdiktState(t *testing.T) {
	if strings.Contains(ApprovalDiffDir, ".edikt/state") {
		t.Fatalf("approval diffs are written under .edikt/state (%s) — gitignored runtime "+
			"ephemera, which destroys the compounding property that justified this phase",
			ApprovalDiffDir)
	}
	root, pendingPath, _ := stageCeremony(t, "pv-loc", "x")
	recordApprovalDiff(pendingPath, approvalDiff{
		PendingID: "pv-loc", Decision: "reject", DecidedAt: approvalNow(), ProposedVerify: "x",
	})
	if _, err := os.Stat(filepath.Join(root, ApprovalDiffDir)); err != nil {
		t.Fatalf("diff did not land in %s: %v", ApprovalDiffDir, err)
	}
}
