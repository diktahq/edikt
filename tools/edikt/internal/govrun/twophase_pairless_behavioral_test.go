package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
	"github.com/diktahq/edikt/tools/edikt/model"
)

// D47 (2): a consumer's ADR-061 sidecar carried `verify_kind: behavioral` on
// a PROHIBITION with both fixture paths empty, and gov compile refused the
// whole corpus ("a behavioral verify requires a fixture PAIR"). The router
// walked only directives. Now every pairless behavioral — directive or
// prohibition, with or without a verify command — is routed, never refused:
// the compile succeeds, one proposal is written per entry that has a verify
// to propose, the entry renders without its verify, and the summary says
// how many are pending and how to approve them.
//
// Falsified by running this test against the pre-fix router (HEAD before the
// fix): RunTwoPhase returns the fixture-PAIR refusal and the test fails.
func TestRunTwoPhase_PairlessBehavioralIsRoutedNotRefused(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(adrDir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// Line 9 carries the quotable sentence for every anchor below.
	write("ADR-061-bok-shape.md",
		"---\nstatus: accepted\n---\n\n# ADR-061 — bok shape\n\n## Decision\n\nHooks MUST emit JSON and MUST NOT shell out.\n")
	write("ADR-061-bok-shape.edikt.yaml", `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-061-bok-shape.md"
signals:
  - "bok signal"
directives:
  - text: "Hooks MUST emit JSON. (ref: ADR-061)"
    verify_kind: behavioral
    falsifying_observation: "a hook emits non-JSON and nothing notices"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Hooks MUST emit JSON and MUST NOT shell out."
prohibitions:
  - text: "MUST NOT shell out from a hook. (ref: ADR-061)"
    verify: "bash test/hooks/no-shell-out.sh"
    verify_kind: behavioral
    falsifying_observation: "a hook spawns a shell and the run passes"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Hooks MUST emit JSON and MUST NOT shell out."
`)
	stageExtractorAgent(t, root, "claude-sonnet-5")

	var errBuf, outBuf bytes.Buffer
	_, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{})
	combined := outBuf.String() + errBuf.String()
	if err != nil {
		t.Fatalf("compile must succeed by routing the pairless behavioral, got: %v\n%s", err, combined)
	}

	// One proposal, for the prohibition (it has a verify to propose).
	pending := filepath.Join(root, ".edikt", "state", "pending-verifies", "ADR-061-p00.yaml")
	body, rerr := os.ReadFile(pending)
	if rerr != nil {
		t.Fatalf("expected the prohibition's proposal at %s: %v\n%s", pending, rerr, combined)
	}
	if !strings.Contains(string(body), "entry_kind: prohibition") || !strings.Contains(string(body), "no-shell-out.sh") {
		t.Fatalf("proposal must name the prohibition list and carry the proposed verify:\n%s", body)
	}
	if _, derr := os.Stat(filepath.Join(root, ".edikt", "state", "pending-verifies", "ADR-061-d00.yaml")); derr == nil {
		t.Fatalf("a behavioral marker with no verify command has nothing to propose; no directive proposal expected\n%s", combined)
	}

	// The COMPILED SURFACE carries neither verify — that is what routing is
	// for. The sidecar on disk is untouched: compile does not write to a
	// source artifact, and `sidecar approve` is the only writer (D81 (d)).
	var rendered string
	for _, f := range []string{
		filepath.Join(root, ".claude", "rules", "governance", "hooks.md"),
		filepath.Join(root, ".claude", "rules", "governance.md"),
	} {
		if b, rerr := os.ReadFile(f); rerr == nil {
			rendered += string(b)
		}
	}
	if strings.Contains(rendered, "no-shell-out.sh") {
		t.Fatalf("the routed verify reached a compiled surface:\n%s", rendered)
	}
	sc, lerr := sidecar.Load(filepath.Join(adrDir, "ADR-061-bok-shape.edikt.yaml"))
	if lerr != nil {
		t.Fatal(lerr)
	}
	if sc.Prohibitions[0].Verify == "" {
		t.Fatalf("compile wrote to the sidecar — the author's verify is gone from the file: %+v", sc.Prohibitions[0])
	}
	if sc.Directives[0].FalsifyingObservation == "" {
		t.Fatalf("the observation must survive: %+v", sc.Directives[0])
	}

	// The summary says what is pending and how to approve it; the
	// verify-less marker is reported, not silently dropped.
	if !strings.Contains(combined, "behavioral verify proposal(s) queued, pending approval — run /edikt:sidecar:approve") {
		t.Fatalf("summary must say the proposal is pending with the remedy:\n%s", combined)
	}
	if !strings.Contains(combined, "declared verify_kind behavioral with no verify command") {
		t.Fatalf("summary must report the dropped marker:\n%s", combined)
	}
	if strings.Contains(combined, "requires a fixture PAIR") {
		t.Fatalf("no corpus-wide refusal may remain:\n%s", combined)
	}
}
