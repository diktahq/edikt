package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// AC-7.4 — the compile manifest fingerprint and `gov compile --check`
// freshness are computed over the STAMPED bytes: extractor_model /
// extractor_contract land on the sidecar via the real phasea verify stamp
// (sidecar.InsertKeyInPlace), a full compile then renders the tree from that
// stamped sidecar, and a second, --check-only compile must report it fresh.
func TestCompileCheckGreenAfterStamp(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	t.Setenv("CLAUDE_HOME", t.TempDir())
	root := t.TempDir()

	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quote := "The extractor MUST quote the artifact byte-exactly."
	body := "# Title\n\n**Status:** Accepted\n\n" + quote + "\n" // quote sits at line 5
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-900-test.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stageExtractorAgent(t, root, "claude-sonnet-5")

	// Real Plan -> (staged extractor output) -> Restore -> Verify cycle, so
	// the sidecar carries the ACTUAL stamp verify.go writes, not a hand-rolled
	// approximation of it.
	planOut, err := phasea.Plan(phasea.PlanOptions{ProjectRoot: root})
	if err != nil || planOut.ExitCode != phasea.ExitPlanEmitted || len(planOut.Plan.Items) != 1 {
		t.Fatalf("Plan(): exit %d, err %v: %s", planOut.ExitCode, err, planOut.Message)
	}

	sc := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{{
			Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: quote}},
		}},
	}
	scBytes, merr := sidecar.Marshal(sc)
	if merr != nil {
		t.Fatal(merr)
	}
	sidecarPath := filepath.Join(adrDir, "ADR-900-test.edikt.yaml")
	if err := os.WriteFile(sidecarPath, scBytes, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, rerr := phasea.Restore(phasea.RestoreOptions{ProjectRoot: root, PlanID: planOut.Plan.PlanID}); rerr != nil {
		t.Fatalf("Restore(): %v", rerr)
	}
	verifyOut, verr := phasea.Verify(phasea.VerifyOptions{
		ProjectRoot: root, PlanID: planOut.Plan.PlanID, ExtractorModel: "claude-sonnet-5",
	})
	if verr != nil || verifyOut.Receipt == nil || len(verifyOut.Receipt.Items) != 1 || verifyOut.Receipt.Items[0].Status != phasea.StatusOK {
		t.Fatalf("Verify(): err %v, outcome %+v", verr, verifyOut)
	}
	if verifyOut.Receipt.Items[0].ExtractorModel != "claude-sonnet-5" {
		t.Fatalf("control: stamp did not land as expected: %+v", verifyOut.Receipt.Items[0])
	}

	// Full compile renders the live tree from the now-stamped sidecar.
	var errBuf, outBuf bytes.Buffer
	if _, cerr := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept",
	}, model.RealClock{}); cerr != nil {
		t.Fatalf("full compile after the stamp: %v\nstderr:\n%s", cerr, errBuf.String())
	}

	// `--check` against that same stamped sidecar must report the tree
	// fresh — the fingerprint/freshness comparison is computed over the
	// bytes InsertKeyInPlace actually wrote, not over some pre-stamp view.
	errBuf.Reset()
	outBuf.Reset()
	if _, cerr := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept", CheckOnly: true,
	}, model.RealClock{}); cerr != nil {
		t.Fatalf("--check after the stamp reported unclean: %v\nstderr:\n%s", cerr, errBuf.String())
	}
}
