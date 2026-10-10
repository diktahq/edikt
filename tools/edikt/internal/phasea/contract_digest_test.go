package phasea

import (
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// SPEC-020 SR-011 / D57(2) — phasea plan records the resolved extractor
// definition's body digest beside prompt_version, so two plans built under
// different extractor bytes never share a plan_id (config_hash's 5th
// input, see TestConfigHashIncludesContract).
func TestPlanRecordsContractDigest(t *testing.T) {
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	writeADR(t, root, "ADR-900", gateQuote)

	want, err := ResolveExtractorContractDigest(root)
	if err != nil {
		t.Fatalf("control: ResolveExtractorContractDigest: %v", err)
	}

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil || out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("Plan(): exit %d, err %v: %s", out.ExitCode, err, out.Message)
	}
	if got := out.Plan.Items[0].ContractDigest; got != want {
		t.Fatalf("contract_digest = %q, want %q (the resolved extractor agent's own digest)", got, want)
	}
}

// The receipt item for an ok extraction carries the same contract digest
// the plan pinned — SR-011's provenance chain, verified end to end from
// plan through the stamped receipt.
func TestReceiptRecordsContractDigest(t *testing.T) {
	f := newFixture(t, "")
	want, err := ResolveExtractorContractDigest(f.root)
	if err != nil {
		t.Fatalf("control: ResolveExtractorContractDigest: %v", err)
	}
	f.extractGood(t)
	out := f.verify(t)
	it := onlyItem(t, out)
	if it.Status != StatusOK {
		t.Fatalf("control: want ok, got %q", it.Status)
	}
	if it.ExtractorContract != want {
		t.Fatalf("receipt extractor_contract = %q, want %q", it.ExtractorContract, want)
	}
}

// The sidecar itself carries extractor_contract after the stamp, not just
// the in-memory receipt — Phase B and a later doctor read the sidecar, not
// the receipt.
func TestSidecarStampContract(t *testing.T) {
	f := newFixture(t, "")
	want, err := ResolveExtractorContractDigest(f.root)
	if err != nil {
		t.Fatalf("control: ResolveExtractorContractDigest: %v", err)
	}
	f.extractGood(t)
	f.verify(t)

	sc, err := sidecar.Load(f.scPath)
	if err != nil {
		t.Fatalf("loading the stamped sidecar: %v", err)
	}
	if sc.ExtractorContract != want {
		t.Fatalf("sidecar extractor_contract = %q, want %q", sc.ExtractorContract, want)
	}
}

// config_hash's 5th input is the contract digest: two configs identical in
// every other respect must hash differently when the extractor's bytes
// differ, so a plan built under one contract never collides with a plan
// built under another.
func TestConfigHashIncludesContract(t *testing.T) {
	cfg := &Config{
		DecisionsDir:       "docs/architecture/decisions",
		InvariantsDir:      "docs/architecture/invariants",
		GuidelinesDir:      "docs/guidelines",
		Ceiling:            DefaultCeiling,
		ForceCeiling:       DefaultForceCeiling,
		MaxAttemptsPerItem: DefaultMaxAttemptsPerItem,
		MaxConcurrency:     DefaultMaxConcurrency,
	}
	a := configHash(cfg, "v3", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	b := configHash(cfg, "v3", "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	if a == b {
		t.Fatal("configHash must change when only the contract digest changes")
	}
	again := configHash(cfg, "v3", "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	if a != again {
		t.Fatal("configHash must be deterministic for identical inputs")
	}
}
