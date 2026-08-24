package govrun

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
)

// mkTopicPair stages one sidecar declaring a topic with a proposed description.
func mkTopicPair(t *testing.T, root, id, topic string) sidecar.Pair {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+".edikt.yaml")
	sc := &sidecar.Sidecar{
		SchemaVersion:            2,
		Topic:                    topic,
		Path:                     "docs/architecture/decisions/" + id + ".md",
		Signals:                  []string{},
		Directives:               []sidecar.Directive{},
		ProposedTopicDescription: "touching " + topic + " internals",
		SourcePath:               p,
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{ArtifactID: id, SidecarPath: p, Sidecar: sc}
}

// The loop this closes: rejecting a pending description removed the pending
// file and recorded nothing, so the next extraction re-proposed the same
// description forever. The only escape a consumer project found was APPROVING
// a description for a topic they had already decided was dead.
func TestRouteProposals_RejectedTopicIsNotReProposed(t *testing.T) {
	root := t.TempDir()
	pendingDir := filepath.Join(root, ".edikt", "state", "pending-topic-descriptions")

	// Round 1: no rejection recorded — the description queues, as it should.
	pair := mkTopicPair(t, root, "ADR-001-test", "dissolved")
	if _, err := RouteProposals(root, []sidecar.Pair{pair}, topicregistry.Registry{}, time.Unix(0, 0)); err != nil {
		t.Fatalf("round 1: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir, "dissolved.yaml")); err != nil {
		t.Fatalf("round 1 should queue a proposal, got: %v", err)
	}

	// The human rejects it: pending file removed, rejection persisted.
	if err := os.Remove(filepath.Join(pendingDir, "dissolved.yaml")); err != nil {
		t.Fatal(err)
	}
	rejPath := topicregistry.RejectionsPathFor(root)
	if err := topicregistry.SaveRejections(rejPath, topicregistry.Rejections{
		"dissolved": {RejectedAt: "2026-08-24T00:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}

	// Round 2: re-extraction re-populates the proposal — the exact condition
	// that used to re-open the loop.
	pair2 := mkTopicPair(t, root, "ADR-001-test", "dissolved")
	if _, err := RouteProposals(root, []sidecar.Pair{pair2}, topicregistry.Registry{}, time.Unix(0, 0)); err != nil {
		t.Fatalf("round 2: %v", err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir, "dissolved.yaml")); !os.IsNotExist(err) {
		t.Fatalf("LOOP STILL OPEN: a rejected topic was re-proposed (stat err = %v)", err)
	}
}

// The negative control: without the rejection the proposal DOES return, so the
// test above proves the rejection is what closed the loop rather than some
// unrelated change in routing.
func TestRouteProposals_UnrejectedTopicStillReProposes(t *testing.T) {
	root := t.TempDir()
	pendingDir := filepath.Join(root, ".edikt", "state", "pending-topic-descriptions")

	pair := mkTopicPair(t, root, "ADR-001-test", "live")
	if _, err := RouteProposals(root, []sidecar.Pair{pair}, topicregistry.Registry{}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(pendingDir, "live.yaml")); err != nil {
		t.Fatal(err)
	}

	pair2 := mkTopicPair(t, root, "ADR-001-test", "live")
	if _, err := RouteProposals(root, []sidecar.Pair{pair2}, topicregistry.Registry{}, time.Unix(0, 0)); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(pendingDir, "live.yaml")); err != nil {
		t.Fatalf("an UNrejected topic must still re-propose after its pending file is removed: %v", err)
	}
}

// A malformed rejections file must fail routing, not degrade to "no
// rejections" — silently re-opening the loop is the failure mode this whole
// mechanism exists to close.
func TestRouteProposals_MalformedRejectionsFileFails(t *testing.T) {
	root := t.TempDir()
	pair := mkTopicPair(t, root, "ADR-001-test", "x")
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(topicregistry.RejectionsPathFor(root), []byte("{{{ not yaml"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := RouteProposals(root, []sidecar.Pair{pair}, topicregistry.Registry{}, time.Unix(0, 0)); err == nil {
		t.Fatal("expected routing to fail on an unreadable rejections file, got nil")
	}
}
