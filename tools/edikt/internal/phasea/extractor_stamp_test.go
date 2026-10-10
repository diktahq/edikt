package phasea

import (
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// SPEC-020 SR-010/SR-011 — an ok item's receipt AND sidecar both carry the
// extraction's provenance: which model actually performed it, and against
// which exact contract bytes. Precedence: a validated --extractor-model
// self-report beats the installed frontmatter's model: pin.
func TestExtractorModelRecorded(t *testing.T) {
	t.Run("self-report takes precedence over frontmatter", func(t *testing.T) {
		f := newFixture(t, "")
		f.extractGood(t)
		out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID, ExtractorModel: "claude-sonnet-5"})
		it := onlyItem(t, out)
		if it.Status != StatusOK {
			t.Fatalf("control: want ok, got %q (%s)", it.Status, it.Diagnostic)
		}
		if it.ExtractorModel != "claude-sonnet-5" {
			t.Fatalf("extractor_model = %q, want the self-reported %q (frontmatter pins claude-opus-5)", it.ExtractorModel, "claude-sonnet-5")
		}
		if len(it.ExtractorContract) != 64 {
			t.Fatalf("extractor_contract = %q, want a 64-char hex digest", it.ExtractorContract)
		}

		sc, err := sidecar.Load(f.scPath)
		if err != nil {
			t.Fatalf("loading the stamped sidecar: %v", err)
		}
		if sc.ExtractorModel != "claude-sonnet-5" {
			t.Fatalf("sidecar extractor_model = %q, want %q", sc.ExtractorModel, "claude-sonnet-5")
		}
		if sc.ExtractorContract != it.ExtractorContract {
			t.Fatalf("sidecar extractor_contract = %q, want %q (must match the receipt)", sc.ExtractorContract, it.ExtractorContract)
		}
	})

	t.Run("falls back to the installed frontmatter when nothing is self-reported", func(t *testing.T) {
		f := newFixture(t, "")
		f.extractGood(t)
		out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
		it := onlyItem(t, out)
		if it.ExtractorModel != "claude-opus-5" {
			t.Fatalf("extractor_model = %q, want the frontmatter pin %q", it.ExtractorModel, "claude-opus-5")
		}
	})

	t.Run("a not-ok item carries neither field", func(t *testing.T) {
		f := newFixture(t, "")
		// No sidecar written: output-missing.
		out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
		it := onlyItem(t, out)
		if it.Status == StatusOK {
			t.Fatal("control: expected a non-ok item")
		}
		if it.ExtractorModel != "" || it.ExtractorContract != "" {
			t.Fatalf("a non-ok item must not carry provenance: model=%q contract=%q", it.ExtractorModel, it.ExtractorContract)
		}
	})
}

// An invalid --extractor-model self-report refuses the WHOLE verify call —
// it never silently degrades to "unrecorded", because a shape-invalid value
// reaching InsertKeyInPlace would write untrusted text into the sidecar
// (INV-006).
func TestStampRefusesUnvalidatedModel(t *testing.T) {
	f := newFixture(t, "")
	f.extractGood(t)
	out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID, ExtractorModel: "Not A Valid Model!!"})
	if err == nil {
		t.Fatal("want an error refusing the malformed --extractor-model")
	}
	if out.ExitCode != ExitVerifyMalformed {
		t.Fatalf("exit = %d, want %d (ExitVerifyMalformed)", out.ExitCode, ExitVerifyMalformed)
	}
	if !strings.Contains(out.Message, "extractor-model") {
		t.Fatalf("message must name the offending flag; got: %s", out.Message)
	}
	if f.receiptExists() {
		t.Fatal("a malformed invocation must write no receipt")
	}
}
