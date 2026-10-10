package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/govrun"
)

// writeArtifact lays down an artifact + sidecar pair with a given Status.
func writeArtifact(t *testing.T, root, id, status, sidecarExtra string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	base := filepath.Join(dir, id+"-a-test-artifact")
	md := "# " + id + ": a test artifact\n\n**Status:** " + status + "\n\n## Decision\n\nSomething.\n"
	if err := os.WriteFile(base+".md", []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	side := "schema_version: 2\ntopic: tooling\npath: x.md\nsignals: [a]\n" +
		"directives:\n  - text: \"X MUST be Y.\"\n    source_excerpts:\n" +
		"      - line_start: 1\n        line_end: 1\n        quote: \"Something.\"\n" + sidecarExtra
	if err := os.WriteFile(base+".edikt.yaml", []byte(side), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkcfg(t *testing.T, root string) govrun.Config {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := govrun.LoadConfig(root)
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

const proposalYAML = "proposed_paths:\n  - glob: commands/init.md\n    evidence: \"Because.\"\n"

// TestStrandedProposalOnASupersededArtifactIsReported — THE REGRESSION TEST.
//
// ADR-002 is `Superseded by ADR-006`, so discover.go keeps it out of the pairs
// RouteProposals iterates. Its proposed_paths therefore stay in the sidecar
// forever, never reach a pending queue, and are invisible to
// standingProposalQueue — which watches queues, not sidecars. Nothing in edikt
// reported it, and the finding that first noticed it misdiagnosed the cause as
// a routing failure.
func TestStrandedProposalOnASupersededArtifactIsReported(t *testing.T) {
	root := t.TempDir()
	cfg := mkcfg(t, root)
	writeArtifact(t, root, "ADR-002", "Superseded by ADR-006", proposalYAML)

	gaps := strandedProposals(root, cfg)
	if len(gaps) != 1 {
		t.Fatalf("gaps: got %d, want 1\n%+v", len(gaps), gaps)
	}
	g := gaps[0]
	if !strings.Contains(g.Title, "never be adjudicated") {
		t.Errorf("title must say the proposal is unadjudicable; got %q", g.Title)
	}
	if !strings.Contains(strings.Join(g.Detail, " "), "ADR-002") {
		t.Errorf("the artifact must be named by id; got %v", g.Detail)
	}
}

// TestNotYetAcceptedIsReportedAsWaitingNotStranded. The two states look
// identical in a sidecar and are not the same thing: a proposed ADR's proposal
// routes normally on the next compile after acceptance. Reporting it as
// stranded would manufacture a defect out of a routine state.
func TestNotYetAcceptedIsReportedAsWaitingNotStranded(t *testing.T) {
	root := t.TempDir()
	cfg := mkcfg(t, root)
	writeArtifact(t, root, "ADR-069", "Proposed. This ADR proposes a rename.", proposalYAML)

	gaps := strandedProposals(root, cfg)
	if len(gaps) != 1 {
		t.Fatalf("gaps: got %d, want 1\n%+v", len(gaps), gaps)
	}
	if strings.Contains(gaps[0].Title, "never be adjudicated") {
		t.Error("a not-yet-accepted artifact must NOT be reported as stranded")
	}
	if !strings.Contains(gaps[0].Title, "awaiting acceptance") {
		t.Errorf("title must say it is waiting; got %q", gaps[0].Title)
	}
}

// TestAcceptedArtifactWithAProposalIsNotReported — NEGATIVE CONTROL, and the
// one that matters: an ACCEPTED artifact's proposal is routed out by the next
// compile, so reporting it here would fire on every corpus between extraction
// and compile.
func TestAcceptedArtifactWithAProposalIsNotReported(t *testing.T) {
	root := t.TempDir()
	cfg := mkcfg(t, root)
	writeArtifact(t, root, "ADR-100", "Accepted", proposalYAML)

	if gaps := strandedProposals(root, cfg); len(gaps) != 0 {
		t.Errorf("an accepted artifact is routed on the next compile and must not "+
			"be reported; got %+v", gaps)
	}
}

// TestSupersededArtifactWithoutAProposalIsNotReported — the other negative
// control. Superseded artifacts are ordinary; only ones holding an
// unadjudicable proposal are a finding.
func TestSupersededArtifactWithoutAProposalIsNotReported(t *testing.T) {
	root := t.TempDir()
	cfg := mkcfg(t, root)
	writeArtifact(t, root, "ADR-003", "Superseded by ADR-006", "")

	if gaps := strandedProposals(root, cfg); len(gaps) != 0 {
		t.Errorf("a superseded artifact with no proposal must not be reported; got %+v", gaps)
	}
}
