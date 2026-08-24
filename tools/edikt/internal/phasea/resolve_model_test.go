package phasea

import "testing"

// TestResolveExtractorModel_Precedence pins the resolution order so a
// caller can predict which value wins without reading the implementation.
//
// Rescued from runner_model_test.go, which was deleted with ClaudeRunner in
// PLAN-v080 Phase 4. Its other three tests exercised ClaudeRunner.Resync and
// went with their subject; this one does not — ResolveExtractorModel survives
// the deletion, so its test had to as well.
func TestResolveExtractorModel_Precedence(t *testing.T) {
	t.Setenv("EDIKT_EXTRACTOR_MODEL", "claude-sonnet-5")

	got, err := ResolveExtractorModel("claude-haiku-4-5")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got != "claude-haiku-4-5" {
		t.Errorf("explicit argument must outrank the environment; got %q", got)
	}

	if got, err = ResolveExtractorModel(""); err != nil || got != "claude-sonnet-5" {
		t.Errorf("environment must outrank the default; got %q err %v", got, err)
	}

	t.Setenv("EDIKT_EXTRACTOR_MODEL", "")
	if got, err = ResolveExtractorModel(""); err != nil || got != DefaultExtractorModel {
		t.Errorf("default must apply when nothing is set; got %q err %v", got, err)
	}
}
