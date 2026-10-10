package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Issue #14. /edikt:adr:new writes an ADR as `status: proposed` and its
// acceptance gate compiles it (step 6c: `phasea plan --artifact … --bootstrap`)
// BEFORE step 6d may flip it to accepted, on 6c's verify exiting 0. plan
// refused every proposed ADR as excluded from the corpus, so 6c could never
// pass and no ADR created by the command could ever be accepted.
//
// The admission is narrow: --bootstrap (the artifact was just written and has
// never had a sidecar) on a PROPOSED ADR with NO sidecar on disk. A rejected
// or superseded ADR, a proposed ADR without --bootstrap, and a proposed ADR
// that already has a sidecar are all still refused.

func writeStatusADR(t *testing.T, root, id, status string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash("docs/architecture/decisions"))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, id+"-test.md")
	body := "---\ntype: adr\nid: " + id + "\nstatus: " + status + "\n---\n\n# Title\n\n**Status:** " + status + "\n\nSome prose line one.\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlan_BootstrapAdmitsProposedADR(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	adr := writeStatusADR(t, root, "ADR-012", "proposed")

	out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: adr, Bootstrap: true})
	if err != nil && out == nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("adr:new step 6c compiles a just-written PROPOSED ADR with --bootstrap; plan must emit.\nexit %d, message %s",
			out.ExitCode, out.Message)
	}

	// The rest of 6c: the extractor writes the sidecar, restore and verify close the plan.
	writeSidecarFile(t, strings.TrimSuffix(adr, ".md")+".edikt.yaml",
		&sidecar.Sidecar{SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-012-test.md"})
	if rout, rerr := Restore(RestoreOptions{ProjectRoot: root, PlanID: out.Plan.PlanID}); rout == nil || rout.ExitCode != 0 {
		t.Fatalf("Restore() on the proposed ADR's plan: %+v, %v", rout, rerr)
	}
	vout, verr := Verify(VerifyOptions{ProjectRoot: root, PlanID: out.Plan.PlanID})
	if vout == nil || vout.ExitCode != 0 {
		t.Fatalf("Verify() on the proposed ADR's plan must exit 0 so step 6d can flip the status: %+v, %v", vout, verr)
	}
}

func TestPlan_ProposedADRStillRefusedOutsideBootstrap(t *testing.T) {
	cases := []struct {
		name        string
		status      string
		bootstrap   bool
		withSidecar bool
	}{
		{name: "proposed without --bootstrap", status: "proposed"},
		{name: "proposed with a sidecar already on disk", status: "proposed", bootstrap: true, withSidecar: true},
		{name: "rejected with --bootstrap", status: "rejected", bootstrap: true},
		{name: "superseded with --bootstrap", status: "superseded", bootstrap: true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			isolate(t)
			writeExtractorAgent(t, root, 3)
			adr := writeStatusADR(t, root, "ADR-012", c.status)
			if c.withSidecar {
				writeSidecarFile(t, strings.TrimSuffix(adr, ".md")+".edikt.yaml",
					&sidecar.Sidecar{SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/ADR-012-test.md"})
			}
			out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: adr, Bootstrap: c.bootstrap})
			if err != nil && out == nil {
				t.Fatalf("Plan(): %v", err)
			}
			if out.ExitCode != ExitRefused {
				t.Fatalf("exit %d, want %d (refused)\nmessage: %s", out.ExitCode, ExitRefused, out.Message)
			}
			if !strings.Contains(out.Message, "excluded from the compiled corpus") {
				t.Errorf("refusal does not name the status exclusion: %s", out.Message)
			}
		})
	}
}
