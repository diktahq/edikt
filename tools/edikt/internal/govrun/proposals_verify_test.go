package govrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/internal/topicregistry"
)

// scratchSidecar writes a sidecar and returns the Pair the routing step sees.
func scratchSidecar(t *testing.T, root, id string, ds []sidecar.Directive) sidecar.Pair {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Anchors are required for a sidecar to load at all: an ungrounded item
	// cannot be drift-checked. The parent .md is written to match, so the
	// fixture is a real artifact pair rather than a shape that only survives
	// because nothing looked.
	parent := filepath.Join(dir, id+".md")
	if err := os.WriteFile(parent, []byte("---\nid: "+id+"\nstatus: accepted\n---\n\n# "+id+"\n\nA rule lives here.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := range ds {
		if len(ds[i].SourceExcerpts) == 0 {
			ds[i].SourceExcerpts = []sidecar.SourceExcerpt{{LineStart: 7, LineEnd: 7, Quote: "A rule lives here."}}
		}
	}
	path := filepath.Join(dir, id+".edikt.yaml")
	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/" + id + ".md",
		Directives:    ds,
		SourcePath:    path,
	}
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return sidecar.Pair{ArtifactID: id, SidecarPath: path, Sidecar: sc}
}

func reload(t *testing.T, path string) *sidecar.Sidecar {
	t.Helper()
	sc, err := sidecar.Load(path)
	if err != nil {
		t.Fatalf("reload %s: %v", path, err)
	}
	return sc
}

// TestRouteProposals_UnapprovedBehavioralVerifyIsQueued — a freshly extracted
// behavioral verify with no fixture would otherwise fail Phase B for the WHOLE
// project. It belongs in the ceremony, not in the output.
func TestRouteProposals_UnapprovedBehavioralVerifyIsQueued(t *testing.T) {
	root := t.TempDir()
	p := scratchSidecar(t, root, "ADR-900", []sidecar.Directive{{
		Text:                  "Layer 2 tests MUST gate on authentication. (ref: ADR-900)",
		Verify:                "pytest test/integration --collect-only",
		VerifyKind:            "behavioral",
		Intent:                "unauthenticated collection must fail loudly",
		FalsifyingObservation: "collection succeeds with no credentials present",
	}})

	rep, err := RouteProposals(root, []sidecar.Pair{p}, topicregistry.Registry{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.VerifiesQueued) != 1 {
		t.Fatalf("queued %v; want exactly one behavioral verify proposal", rep.VerifiesQueued)
	}

	pending := filepath.Join(root, ".edikt", "state", "pending-verifies", "ADR-900-d00.yaml")
	body, err := os.ReadFile(pending)
	if err != nil {
		t.Fatalf("no pending file written: %v", err)
	}
	for _, want := range []string{"proposed_verify", "pytest test/integration", "directive_index: 0", "ADR-900"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("pending file omits %q:\n%s", want, body)
		}
	}

	// STRIPPED FROM THE SIDECAR, or Phase B still refuses and the routing
	// achieved nothing.
	after := reload(t, p.SidecarPath)
	if after.Directives[0].Verify != "" || after.Directives[0].VerifyKind != "" {
		t.Fatalf("verify survived in the sidecar: verify=%q kind=%q",
			after.Directives[0].Verify, after.Directives[0].VerifyKind)
	}
	if !strings.Contains(rep.Report(), "behavioral verify proposal") {
		t.Errorf("report does not mention the queued verify: %s", rep.Report())
	}
}

// TestRouteProposals_ApprovedVerifyIsUntouched is the criterion AC-4.4 depends
// on: human_approved_at records that the ceremony happened. Re-queueing it
// would ask a human to re-approve their own decision every compile, and
// stripping it would destroy the pinned state re-extraction must preserve
// byte-intact.
func TestRouteProposals_ApprovedVerifyIsUntouched(t *testing.T) {
	root := t.TempDir()
	p := scratchSidecar(t, root, "ADR-901", []sidecar.Directive{{
		Text:                "The gate MUST refuse an unsigned payload. (ref: ADR-901)",
		Verify:              "bash test/fixtures/behavioral/ADR-901/positive.sh",
		VerifyKind:          "behavioral",
		PositiveFixturePath: "test/fixtures/behavioral/ADR-901/positive.sh",
		HumanApprovedAt:     "2026-08-01T00:00:00Z",
	}})

	rep, err := RouteProposals(root, []sidecar.Pair{p}, topicregistry.Registry{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.VerifiesQueued) != 0 {
		t.Fatalf("an approved verify was queued: %v", rep.VerifiesQueued)
	}
	after := reload(t, p.SidecarPath)
	d := after.Directives[0]
	if d.Verify == "" || d.VerifyKind != "behavioral" || d.HumanApprovedAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("approved verify was altered: %+v", d)
	}
}

// TestRouteProposals_StructuralVerifyIsNotQueued — structural and tooling
// verifies inspect the tree and need no fixture. Queueing them would put every
// mechanical check through a human ceremony and make the queue unreadable,
// which is how a queue stops being a control (F-005).
func TestRouteProposals_StructuralVerifyIsNotQueued(t *testing.T) {
	root := t.TempDir()
	p := scratchSidecar(t, root, "ADR-902", []sidecar.Directive{{
		Text:       "The loader MUST use KnownFields(true). (ref: ADR-902)",
		Verify:     "rg -q 'KnownFields\\(true\\)' tools/edikt/internal/sidecar/sidecar.go",
		VerifyKind: "structural",
	}})

	rep, err := RouteProposals(root, []sidecar.Pair{p}, topicregistry.Registry{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.VerifiesQueued) != 0 {
		t.Fatalf("a structural verify was queued: %v", rep.VerifiesQueued)
	}
	if reload(t, p.SidecarPath).Directives[0].Verify == "" {
		t.Fatal("a structural verify was stripped; only unapproved behavioral verifies are proposals")
	}
}

// TestRouteProposals_BehavioralWithFixtureIsNotQueued — a behavioral verify
// that already carries its positive fixture satisfies Phase B's gate and has
// nothing left for a human to supply.
func TestRouteProposals_BehavioralWithFixtureIsNotQueued(t *testing.T) {
	root := t.TempDir()
	p := scratchSidecar(t, root, "ADR-903", []sidecar.Directive{{
		Text:                "The installer MUST verify the signature. (ref: ADR-903)",
		Verify:              "bash test/fixtures/behavioral/ADR-903/positive.sh",
		VerifyKind:          "behavioral",
		PositiveFixturePath: "test/fixtures/behavioral/ADR-903/positive.sh",
	}})

	rep, err := RouteProposals(root, []sidecar.Pair{p}, topicregistry.Registry{}, time.Unix(1700000000, 0))
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.VerifiesQueued) != 0 {
		t.Fatalf("a fixture-backed behavioral verify was queued: %v", rep.VerifiesQueued)
	}
	if reload(t, p.SidecarPath).Directives[0].Verify == "" {
		t.Fatal("a fixture-backed behavioral verify was stripped")
	}
}

// TestRouteProposals_ExistingProposalIsNotOverwritten — a human mid-review must
// not have the file replaced underneath them by the next compile.
func TestRouteProposals_ExistingProposalIsNotOverwritten(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".edikt", "state", "pending-verifies")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	existing := filepath.Join(dir, "ADR-904-d00.yaml")
	if err := os.WriteFile(existing, []byte("id: ADR-904-d00\nunder_review_by_a_human: true\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	p := scratchSidecar(t, root, "ADR-904", []sidecar.Directive{{
		Text:       "A rule. (ref: ADR-904)",
		Verify:     "a different command than the one under review",
		VerifyKind: "behavioral",
	}})
	if _, err := RouteProposals(root, []sidecar.Pair{p}, topicregistry.Registry{}, time.Unix(1700000000, 0)); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(existing)
	if !strings.Contains(string(body), "under_review_by_a_human") {
		t.Fatalf("the in-review proposal was overwritten:\n%s", body)
	}
}
