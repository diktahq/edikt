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

// TestUnwrittenCycleLeavesBodyDriftReported — PLAN-v090-issue-fixes Phase 3
// (#8, c2-b). Prose was added to an artifact after its last extraction, so
// the body-drift signal is up. A re-extraction is requested and the extractor
// writes nothing. Restore used to stamp the body digest anyway, against prose
// no extraction read, and the signal went quiet with nothing regenerated.
func TestUnwrittenCycleLeavesBodyDriftReported(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	t.Setenv("CLAUDE_HOME", t.TempDir())
	root := t.TempDir()

	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quote := "The extractor MUST quote the artifact byte-exactly."
	adr := filepath.Join(adrDir, "ADR-900-test.md")
	body := "# Title\n\n**Status:** Accepted\n\n" + quote + "\n" // quote at line 5
	if err := os.WriteFile(adr, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	stageExtractorAgent(t, root, "claude-sonnet-5")

	sc := filepath.Join(adrDir, "ADR-900-test.edikt.yaml")
	raw, err := sidecar.Marshal(&sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives: []sidecar.Directive{{
			Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: quote}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sc, raw, 0o644); err != nil {
		t.Fatal(err)
	}
	// The last real extraction read this body.
	if err := sidecar.StampBodyDigest(sc, adr, os.ReadFile,
		func(p string, b []byte) error { return os.WriteFile(p, b, 0o644) }); err != nil {
		t.Fatal(err)
	}
	// Prose added since; the anchor at line 5 still matches.
	if err := os.WriteFile(adr, []byte(body+"\nA paragraph no extraction has read. It MUST be extracted.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	drifted := func(what string) int {
		t.Helper()
		var errBuf, outBuf bytes.Buffer
		res, _ := RunTwoPhase(TwoPhaseOptions{
			ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept", CheckOnly: true,
		}, model.RealClock{})
		if res == nil || res.BodyDrift == nil {
			t.Fatalf("%s: --check reported no body-drift summary\nstderr:\n%s", what, errBuf.String())
		}
		return res.BodyDrift.Drifted
	}
	if n := drifted("before"); n != 1 {
		t.Fatalf("setup: body drift %d before the cycle, want 1", n)
	}

	planOut, err := phasea.Plan(phasea.PlanOptions{ProjectRoot: root, Artifact: adr})
	if err != nil || planOut.ExitCode != phasea.ExitPlanEmitted {
		t.Fatalf("Plan(--artifact): exit %d, err %v: %s", planOut.ExitCode, err, planOut.Message)
	}
	// The extractor writes nothing. Restore and verify run as the command does.
	if r, rerr := phasea.Restore(phasea.RestoreOptions{ProjectRoot: root, PlanID: planOut.Plan.PlanID}); r == nil || r.ExitCode != phasea.ExitRestoreOK {
		t.Fatalf("Restore(): %+v %v", r, rerr)
	}
	_, _ = phasea.Verify(phasea.VerifyOptions{ProjectRoot: root, PlanID: planOut.Plan.PlanID})

	if n := drifted("after"); n != 1 {
		t.Fatalf("body drift is %d after a cycle in which nothing was extracted, want 1 — the baseline was "+
			"stamped over prose no extraction read", n)
	}
}
