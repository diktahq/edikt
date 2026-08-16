package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// ApprovalDiffDir is where approval-ceremony diffs are persisted, relative to
// the project root.
//
// GIT-TRACKED, NOT .edikt/state/. Ruled 2026-08-09 and the reason is the whole
// point of the phase: these diffs are an ASSET THAT COMPOUNDS — every ceremony
// deposits labelled ground truth about human judgment, and that corpus is what
// makes measuring extractor recall cheap forever rather than once.
//
// An asset that dies on a fresh clone is not an asset. .edikt/state/ is
// gitignored runtime ephemera; putting them there would preserve the mechanism
// and destroy the property that justified building it first.
const ApprovalDiffDir = "docs/internal/approval-diffs"

// approvalDiff is one record of what the extractor proposed against what the
// human accepted.
//
// WHY REJECTIONS MATTER MORE THAN APPROVALS. A rejection is a LABELLED
// NEGATIVE, and negatives are what the extractor lacks: a corpus of approvals
// teaches only what it already gets right. Before this, `reject` loaded the
// pending proposal and deleted it (sidecar_approve.go, reject branch) — the
// single most informative outcome of the ceremony was the one that left no
// trace.
type approvalDiff struct {
	PendingID string `yaml:"pending_id"`
	Decision  string `yaml:"decision"` // approve | reject | defer
	DecidedAt string `yaml:"decided_at"`

	SidecarPath    string `yaml:"sidecar_path,omitempty"`
	DirectiveIndex int    `yaml:"directive_index"`

	// What the extractor proposed.
	ProposedVerify        string `yaml:"proposed_verify"`
	Intent                string `yaml:"intent,omitempty"`
	FalsifyingObservation string `yaml:"falsifying_observation,omitempty"`
	ProposedAt            string `yaml:"proposed_at,omitempty"`

	// What the human accepted. Empty on reject and defer — and that is the
	// signal, not a gap: nothing was accepted.
	AcceptedVerify string `yaml:"accepted_verify,omitempty"`

	// Edited distinguishes "approved as proposed" from "approved after
	// changing it". The edit is the richest of the three outcomes: it is the
	// human saying "nearly, but not that", which neither a bare approve nor a
	// bare reject records.
	Edited bool `yaml:"edited"`
}

// approvalDiffProjectRoot derives the project root from the pending file's
// path: <root>/.edikt/state/pending-verifies/<id>.yaml
func approvalDiffProjectRoot(pendingPath string) string {
	return filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(pendingPath))))
}

// recordApprovalDiff persists one ceremony outcome.
//
// FAILURE IS NOT FATAL TO THE CEREMONY, and that is deliberate: a human has
// already made a decision, and refusing to honour it because an audit record
// could not be written would be the wrong trade. But it is NOT silent either —
// a capture that failed says so on stderr, because a diff corpus with silent
// holes is worse than one with known holes (INV-013: absence must be visible).  // edikt-guard:allow
func recordApprovalDiff(pendingPath string, d approvalDiff) {
	root := approvalDiffProjectRoot(pendingPath)
	dir := filepath.Join(root, ApprovalDiffDir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "warn: approval diff NOT captured (mkdir %s): %v\n", dir, err)
		return
	}
	name := fmt.Sprintf("%s-%s.yaml",
		strings.ReplaceAll(d.DecidedAt, ":", ""), sanitizeDiffID(d.PendingID))
	out, err := yaml.Marshal(&d)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: approval diff NOT captured (marshal): %v\n", err)
		return
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "warn: approval diff NOT captured (write %s): %v\n", path, err)
		return
	}
	fmt.Fprintf(os.Stdout, "    approval diff:   %s\n",
		filepath.Join(ApprovalDiffDir, name))
}

// sanitizeDiffID keeps the filename to an allowlist. The pending id reaches a
// file path, which INV-006 defines as externally-controlled.  // edikt-guard:allow
func sanitizeDiffID(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := b.String()
	if out == "" {
		out = "unknown"
	}
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}

func approvalNow() string { return time.Now().UTC().Format("20060102T150405Z") }
