package phasea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// D414. bok's round-5 field run: a sidecar was deleted, `/edikt:adr:compile`
// regenerated it, and FOUR `manual_directives` were destroyed — including a
// SQL-parameterisation rule. plan, restore and verify all exited 0, the receipt
// read `status: ok`, and restore printed `0 unrestorable pin(s)` over the loss.
//
// The cause is in capturePinSnapshot's own doc (plan.go): a nil beforeSidecar
// is "the ordinary bootstrap case" and "succeeds vacuously". That is correct
// when no sidecar has ever existed. It is indistinguishable, from disk alone,
// from a sidecar that existed, carried hand-authored prose, and is now ABSENT —
// and the pipeline reported success over both.
//
// `manual_directives` and `suppressed_directives` are exactly the fields the
// extractor can NEVER regenerate: they are prose a human wrote into the
// sidecar, present nowhere in the parent .md. pinpreserve carries them as
// whole-list fields, and restoreAuthoredList has no unrestorable arm because
// "nothing has to find a directive again, so nothing can fail to" — true when a
// pin exists, and precisely why nothing fires when one does not.
//
// INV-015: a directive body MUST NOT leave the compiled corpus except through
// an explicitly recorded removal carrying a stated reason. Four left silently.
// INV-013: absence is never a passing measurement.
//
// THE DISCRIMINATOR THIS ASSERTS. A machine cannot tell bootstrap from loss by
// looking at an absent file, so it must not guess — it asks. The refusal fires
// on the `requested` path only (`--artifact`, which plans an artifact "whether
// or not the stale detector would have selected it"), and never on the corpus
// path, where ReasonMissing IS the bootstrap case and refusing would break
// every legitimate first compile. Case D below is that guard, and it is the
// reason this fix is narrow rather than a blanket refusal.

func writeSidecarWithManualDirectives(t *testing.T, root, adrPath, id string) string {
	t.Helper()
	scPath := strings.TrimSuffix(adrPath, ".md") + ".edikt.yaml"
	sc := &sidecar.Sidecar{
		SchemaVersion:    2,
		Topic:            "testing",
		ManualDirectives: []string{"Never interpolate a query string into SQL. (ref: " + id + ")"},
	}
	writeSidecarFile(t, scPath, sc)
	return scPath
}

// CASE A — the defect. A sidecar that EXISTED is gone, and the caller asks for
// this one artifact by name. Planning it as a bootstrap destroys authored prose.
func TestPlan_RequestedWithAbsentSidecarIsRefused(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	adr := writeADR(t, root, "ADR-010", "Some prose line one.")
	sc := writeSidecarWithManualDirectives(t, root, adr, "ADR-010")
	if err := os.Remove(sc); err != nil {
		t.Fatal(err)
	}

	out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: adr})
	if err != nil && out == nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitRefused {
		t.Fatalf("a requested extraction of an artifact whose sidecar is ABSENT must be REFUSED (exit %d), got exit %d.\n"+
			"Planning it silently destroys every manual_directive and suppressed_directive: the extractor cannot\n"+
			"regenerate them, they live only in the sidecar, and nothing downstream reports their loss.\nmessage: %s",
			ExitRefused, out.ExitCode, out.Message)
	}
	for _, want := range []string{"manual_directives", "bootstrap"} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("the refusal must name %q so the reader knows what is at risk and how to proceed.\nmessage: %s", want, out.Message)
		}
	}
}

// CASE B — the declared bootstrap. A human asserting "this never had a sidecar"
// is the one thing that can tell the two cases apart, so it must be accepted.
func TestPlan_RequestedWithAbsentSidecarProceedsWhenBootstrapDeclared(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	adr := writeADR(t, root, "ADR-010", "Some prose line one.")

	out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: adr, Bootstrap: true})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("a DECLARED bootstrap must plan normally — the refusal exists to make the human choose, not to block them.\nexit %d, message %s", out.ExitCode, out.Message)
	}
}

// CASE C — attribution. With the sidecar PRESENT the same call must proceed, so
// case A's refusal is caused by the absence and by nothing else.
func TestPlan_RequestedWithPresentSidecarIsUnaffected(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	adr := writeADR(t, root, "ADR-010", "Some prose line one.")
	writeSidecarWithManualDirectives(t, root, adr, "ADR-010")

	out, err := Plan(PlanOptions{ProjectRoot: root, Artifact: adr})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted {
		t.Fatalf("a requested extraction with its sidecar PRESENT must still plan; case A proves nothing otherwise.\nexit %d, message %s", out.ExitCode, out.Message)
	}
}

// CASE D — THE GUARD AGAINST THIS FIX. The corpus path plans absent sidecars as
// ReasonMissing, which IS the bootstrap case. If the refusal reaches here it
// breaks every legitimate first compile, which is far worse than the defect.
func TestPlan_CorpusBootstrapIsUnaffected(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	writeADR(t, root, "ADR-010", "Some prose line one.")
	writeADR(t, root, "ADR-020", "Some prose line two.")

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("the CORPUS path must still bootstrap missing sidecars — ReasonMissing is the legitimate case.\nexit %d, message %s", out.ExitCode, out.Message)
	}
	if len(out.Plan.Items) != 2 {
		t.Fatalf("expected both missing sidecars planned, got %d item(s)", len(out.Plan.Items))
	}
	for _, it := range out.Plan.Items {
		if it.Reason != ReasonMissing {
			t.Errorf("item %s: expected reason %q, got %q", it.ItemID, ReasonMissing, it.Reason)
		}
	}
}

// CASE E — THE CORPUS PATH, WHICH IS WHERE THE LOSS ACTUALLY HAPPENS.
//
// A clean reviewer refuted the first version of this fix here, and the finding
// was decisive: the `--artifact` refusal guards one entry point, and
// `/edikt:gov:compile` does not use it. It runs a bare `phasea plan`, where a
// deleted sidecar arrives as ReasonMissing — indistinguishable from a genuine
// first compile — so the loss stayed fully reachable with every report at
// exit 0.
//
// Prevention is NOT possible there and this test does not pretend otherwise:
// refusing a corpus compile because an artifact has no sidecar would break
// every greenfield adoption, and edikt keeps no record of which artifacts once
// HAD a sidecar (the manifest lists rendered surfaces, and the delivery map
// omits artifacts whose directives are all suppressed — exactly the population
// whose suppressed_directives are at risk). So the corpus path REPORTS.
//
// What this asserts is that the report is per-item and NAMES the artifact. The
// first version was a whole-plan boolean, true only when EVERY item was
// unpinned, so one ordinary stale sidecar beside a deleted one silenced it —
// and a reviewer showed that mutating it to a constant, in either direction,
// left all five tests and the whole package green. This test fails on both.
func TestRestore_NamesEveryItemThatTookNoPin(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)

	// ADR-010: had a sidecar carrying authored prose, now deleted. THE LOSS.
	lost := writeADR(t, root, "ADR-010", "the current line of prose.")
	lostSC := writeSidecarWithManualDirectives(t, root, lost, "ADR-010")
	if err := os.Remove(lostSC); err != nil {
		t.Fatal(err)
	}

	// ADR-020: an ordinary stale sidecar. Its presence is the entire point —
	// it is what silenced the first version of this report.
	stale := writeADR(t, root, "ADR-020", "the current line of prose.")
	writeSidecarFile(t, strings.TrimSuffix(stale, ".md")+".edikt.yaml", &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-020-test.md",
		Directives: []sidecar.Directive{{
			Text:           "Test directive. (ref: ADR-020)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: "a quote that no longer matches"}},
		}},
	})

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("corpus plan must be emitted: exit %d, %s", out.ExitCode, out.Message)
	}
	if len(out.Plan.Items) != 2 {
		t.Fatalf("expected 2 items (one missing, one stale), got %d", len(out.Plan.Items))
	}

	// The marker is a per-item fact and must be set on the corpus path too —
	// setting it only on the --artifact path is how the first version came to
	// describe half the runs that needed it.
	for _, it := range out.Plan.Items {
		want := it.ItemID == "ADR-010"
		if it.BootstrapNoPin != want {
			t.Errorf("item %s: BootstrapNoPin = %v, want %v", it.ItemID, it.BootstrapNoPin, want)
		}
	}

	// Simulate the extractor: both sidecars are rewritten, and the regenerated
	// ADR-010 has no manual_directives — the extractor cannot invent them.
	for _, id := range []string{"ADR-010", "ADR-020"} {
		writeSidecarFile(t, filepath.Join(root, "docs", "architecture", "decisions", id+"-test.edikt.yaml"),
			&sidecar.Sidecar{SchemaVersion: 2, Topic: "testing", Path: "docs/architecture/decisions/" + id + "-test.md"})
	}

	rout, rerr := Restore(RestoreOptions{ProjectRoot: root, PlanID: out.Plan.PlanID})
	if rout == nil {
		t.Fatalf("Restore() returned no outcome: %v", rerr)
	}
	if len(rout.UnpinnedItems) != 1 || rout.UnpinnedItems[0] != "ADR-010" {
		t.Fatalf("restore must NAME the item that took no pin, and only that one.\n"+
			"got %v, want [ADR-010]. A whole-plan boolean reports the same value here as it does for a\n"+
			"greenfield compile where nothing is at risk, which is why the first version of this said\n"+
			"nothing about the run that destroyed four authored directives.", rout.UnpinnedItems)
	}
	if !strings.Contains(rout.Message, "ADR-010") {
		t.Errorf("the operator-facing message must name ADR-010; a fact recorded on a struct that no\n"+
			"output carries is not a report.\nmessage: %s", rout.Message)
	}
}
