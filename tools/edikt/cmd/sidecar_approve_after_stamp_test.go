package cmd

// sidecar_approve_after_stamp_test.go — AC-7.10: a pending-verify proposal
// queued BEFORE phasea verify's post-verify extractor_model/extractor_contract
// stamp must still resolve correctly after the stamp lands. The stamp adds
// two top-level keys via sidecar.InsertKeyInPlace; approve's own write path
// is a full sidecar.Load -> mutate directives[index] -> sidecar.Marshal
// round trip, which must carry the stamped fields through untouched.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func TestSidecarApprove_AfterStamp(t *testing.T) {
	dir := t.TempDir()
	_ = seedEdiktRoot(t, dir)

	artifactID := "ADR-998"
	sidecarPath := filepath.Join(dir, artifactID+"-example.edikt.yaml")
	// Pre-stamped, as if phasea verify's post-verify stamp already ran
	// before this pending-verify proposal was resolved.
	sidecarBody := `schema_version: 2
topic: "testing"
path: "` + artifactID + `-example.md"
extractor_model: sonnet
extractor_contract: afd03a749b35758360fdbede6c9d525088273d729cad0445b4cb6c7ec06a524d
signals:
  - "example signal"
directives:
  - text: 'An example rule MUST hold. (ref: ` + artifactID + `)'
    source_excerpts:
      - line_start: 1
        line_end: 1
        quote: 'An example rule holds.'
`
	if err := os.WriteFile(sidecarPath, []byte(sidecarBody), 0o644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}

	pendingID := artifactID + "-d00"
	pendingPath := filepath.Join(dir, ".edikt", "state", "pending-verifies", pendingID+".yaml")
	pendingBody := `id: ` + pendingID + `
sidecar_path: ` + sidecarPath + `
directive_index: 0
proposed_verify: "true"
intent: "example intent"
falsifying_observation: "the example rule is violated"
proposed_at: "2026-08-14T00:00:00Z"
`
	if err := os.WriteFile(pendingPath, []byte(pendingBody), 0o644); err != nil {
		t.Fatalf("write pending verify: %v", err)
	}

	positive := filepath.Join(dir, "positive.sh")
	negative := filepath.Join(dir, "negative.sh")
	for _, p := range []string{positive, negative} {
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}

	withCWD(t, dir)

	out, runErr := runCmd(t, "sidecar", "approve", pendingID,
		"--decision=approve", "--positive-fixture="+positive, "--negative-fixture="+negative)
	if runErr != nil {
		t.Fatalf("want success, got: %v\noutput: %s", runErr, out)
	}
	if !strings.Contains(out, "ok: approved") {
		t.Fatalf("expected an approval confirmation, got: %s", out)
	}

	sc, err := sidecar.Load(sidecarPath)
	if err != nil {
		t.Fatalf("loading the sidecar after approve: %v", err)
	}
	if sc.Directives[0].VerifyKind != "behavioral" {
		t.Fatalf("verify_kind = %q, want behavioral", sc.Directives[0].VerifyKind)
	}
	if sc.Directives[0].HumanApprovedAt == "" {
		t.Fatal("human_approved_at was not set")
	}
	if sc.ExtractorModel != "sonnet" {
		t.Fatalf("extractor_model = %q, want it preserved through approve's write", sc.ExtractorModel)
	}
	if sc.ExtractorContract != "afd03a749b35758360fdbede6c9d525088273d729cad0445b4cb6c7ec06a524d" {
		t.Fatalf("extractor_contract = %q, want it preserved through approve's write", sc.ExtractorContract)
	}

	body, err := os.ReadFile(sidecarPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "extractor_model: sonnet") {
		t.Fatalf("the stamped field must survive on disk in readable form, got:\n%s", body)
	}
}
