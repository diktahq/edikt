package cmd

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// stageContractDrift writes an accepted ADR, an extractor agent definition,
// and a sidecar whose extractor_contract is pinned to the agent's digest AT
// STAGING TIME (before any later change to the agent file).
func stageContractDrift(t *testing.T, agentBody string) (root, agentDigest string) {
	t.Helper()
	root = t.TempDir()

	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	quote := "The extractor MUST quote the artifact byte-exactly."
	body := "# Title\n\n**Status:** Accepted\n\n" + quote + "\n" // quote sits at line 5
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-900-test.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	agentPath := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
	if err := os.MkdirAll(filepath.Dir(agentPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(agentPath, []byte(agentBody), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(agentBody))
	agentDigest = hex.EncodeToString(sum[:])

	sc := &sidecar.Sidecar{
		SchemaVersion:     2,
		Topic:             "testing",
		Path:              "docs/architecture/decisions/ADR-900-test.md",
		ExtractorModel:    "claude-sonnet-5",
		ExtractorContract: agentDigest,
		Directives: []sidecar.Directive{{
			Text:           "The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)",
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: quote}},
		}},
	}
	scBytes, merr := sidecar.Marshal(sc)
	if merr != nil {
		t.Fatal(merr)
	}
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-900-test.edikt.yaml"), scBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	return root, agentDigest
}

// AC-7.6 — a sidecar's extractor_contract no longer matching the resolved
// extractor agent's current bytes is reported as INFORMATION, naming both
// digests, and never counted against doctor's warn/error tally.
func TestDoctorContractDrift_IsInfo(t *testing.T) {
	agentV1 := "---\nname: sidecar-extractor\nmodel: claude-sonnet-5\nprompt_version: 4\n---\n\nbody v1\n"
	root, oldDigest := stageContractDrift(t, agentV1)

	// Patch the agent definition after staging — the extractor prompt
	// evolving is the ordinary case this check exists to describe.
	agentV2 := "---\nname: sidecar-extractor\nmodel: claude-sonnet-5\nprompt_version: 5\n---\n\nbody v2 (patched)\n"
	agentPath := filepath.Join(root, ".claude", "agents", "sidecar-extractor.md")
	if err := os.WriteFile(agentPath, []byte(agentV2), 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte(agentV2))
	newDigest := hex.EncodeToString(sum[:])
	if newDigest == oldDigest {
		t.Fatal("control: the patched agent body must hash differently from the staged one")
	}

	notes := checkExtractorContractDrift(root)
	if len(notes) != 1 {
		t.Fatalf("want 1 drift note, got %d: %+v", len(notes), notes)
	}
	if notes[0].Extracted != oldDigest || notes[0].Current != newDigest {
		t.Fatalf("note = %+v, want extracted=%s current=%s", notes[0], oldDigest, newDigest)
	}

	var out bytes.Buffer
	runExtractorContractDriftCheck(root, &out)
	got := out.String()
	if !strings.Contains(got, "INFO:") {
		t.Fatalf("drift must be reported as INFO, got:\n%s", got)
	}
	if !strings.Contains(got, oldDigest) || !strings.Contains(got, newDigest) {
		t.Fatalf("report must name both digests, got:\n%s", got)
	}
}

// Control: no drift when the sidecar's recorded contract still matches the
// resolved agent's current bytes.
func TestDoctorContractDrift_SilentWhenUnchanged(t *testing.T) {
	agentBody := "---\nname: sidecar-extractor\nmodel: claude-sonnet-5\nprompt_version: 4\n---\n\nbody v1\n"
	root, _ := stageContractDrift(t, agentBody)

	if notes := checkExtractorContractDrift(root); len(notes) != 0 {
		t.Fatalf("want no drift when the agent is unchanged, got: %+v", notes)
	}

	var out bytes.Buffer
	runExtractorContractDriftCheck(root, &out)
	if out.Len() != 0 {
		t.Fatalf("want no output when the agent is unchanged, got:\n%s", out.String())
	}
}
