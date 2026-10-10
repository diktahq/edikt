package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// #7 (c3-c), through the built binary: an extractor re-wraps a quote, restore
// quarantines the sidecar, and verify used to report output-missing with no
// defect axis and no correction prompt — so the documented seeded retry had
// nothing to carry. The receipt a caller reads must now say defect_axis
// anchor and carry a correction_prompt with the parent's exact bytes.
func TestPhaseaBlackbox_WrappedQuoteCarriesAnchorCorrection(t *testing.T) {
	p := &bbProject{root: t.TempDir(), home: t.TempDir()}
	bbWriteExtractorAgent(t, p.root, 11)
	bbWriteConfig(t, p.root, "")
	// Lines 5-6 of the ADR bbWriteADR writes.
	p.adr = bbWriteADR(t, p.root, "ADR-900", "The extractor MUST quote the artifact\nbyte-exactly at the recorded range.")

	id, msg, code := p.planArtifact(t, "--bootstrap")
	if code != 0 || id == "" {
		t.Fatalf("bootstrap plan: exit %d\n%s", code, msg)
	}
	body, err := sidecar.Marshal(&sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/" + filepath.Base(p.adr),
		Directives: []sidecar.Directive{{
			Text: "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 6,
				Quote: "The extractor MUST quote the artifact byte-exactly at the recorded range."}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bbSidecarPath(p.adr), body, 0o644); err != nil {
		t.Fatal(err)
	}

	p.mustExit(t, phasea.ExitRestoreOK, "phasea", "restore", "--plan", id)
	out := p.mustExit(t, phasea.ExitVerifyIncomplete, "phasea", "verify", "--plan", id)

	raw, err := os.ReadFile(filepath.Join(p.root, ".edikt", "state", "phasea", id+".receipt.json"))
	if err != nil {
		t.Fatalf("no receipt after verify: %v\n%s", err, out)
	}
	var rec struct {
		Items []struct {
			Status           string `json:"status"`
			DefectAxis       string `json:"defect_axis"`
			CorrectionPrompt string `json:"correction_prompt"`
		} `json:"items"`
	}
	if err := json.Unmarshal(raw, &rec); err != nil || len(rec.Items) != 1 {
		t.Fatalf("receipt: %v %s", err, raw)
	}
	it := rec.Items[0]
	if it.Status != phasea.StatusAnchorInvalid || it.DefectAxis != "anchor" {
		t.Fatalf("status/defect_axis = %q/%q, want anchor-invalid/anchor\n%s", it.Status, it.DefectAxis, out)
	}
	for _, want := range []string{"lines 5-6", `"The extractor MUST quote the artifact\nbyte-exactly at the recorded range."`} {
		if !strings.Contains(it.CorrectionPrompt, want) {
			t.Errorf("correction_prompt omits %q:\n%s", want, it.CorrectionPrompt)
		}
	}
}
