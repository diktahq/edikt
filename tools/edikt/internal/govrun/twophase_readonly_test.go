package govrun

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/model"
)

// D51 (a): Phase B is read-only except the explicit routing edit, and that
// edit is minimal. A consumer's compile changed 316 lines of a committed
// sidecar to remove two keys because the router rewrote the file through
// Marshal. These two tests are the rule; run against the pre-fix router they
// fail (the clean corpus is reserialised, the routed file changes hundreds
// of lines).

func gitInitCommit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{{"init", "-q"}, {"add", "-A"}, {"-c", "user.name=t", "-c", "user.email=t@t", "commit", "-q", "-m", "fixture"}} {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitPorcelain(t *testing.T, root string, paths ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"status", "--porcelain", "--"}, paths...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git status: %v\n%s", err, out)
	}
	return string(out)
}

// A sidecar in the corpus's own written shape: quoted, wrapped, with a
// multi-line scalar — everything a reserialisation would touch.
const readonlyFixtureSidecar = `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/%s.md"
signals:
  - "fresh"
directives:
  - text: "Fresh rule MUST hold. (ref: %s)"
    intent: >-
      so that the rule
      is held
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Fresh rule MUST hold."
`

func writeCleanArtifact(t *testing.T, adrDir, id string) {
	t.Helper()
	md := "---\nstatus: accepted\n---\n\n# " + id + " — fresh\n\n## Decision\n\nFresh rule MUST hold.\n"
	if err := os.WriteFile(filepath.Join(adrDir, id+".md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := strings.ReplaceAll(readonlyFixtureSidecar, "%s", id)
	if err := os.WriteFile(filepath.Join(adrDir, id+".edikt.yaml"), []byte(sc), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunTwoPhase_CleanCorpusWritesNoSidecar(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCleanArtifact(t, adrDir, "ADR-080")
	writeCleanArtifact(t, adrDir, "ADR-081")
	stageExtractorAgent(t, root, "claude-sonnet-5")
	gitInitCommit(t, root)

	var errBuf, outBuf bytes.Buffer
	if _, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept"}, model.RealClock{}); err != nil {
		t.Fatalf("compile: %v\n%s%s", err, outBuf.String(), errBuf.String())
	}
	if p := gitPorcelain(t, root, "docs/architecture"); p != "" {
		t.Fatalf("a compile with nothing to route must leave every sidecar byte-identical; git status:\n%s", p)
	}
	if strings.Contains(outBuf.String()+errBuf.String(), "sidecar(s) rewritten") {
		t.Fatalf("nothing was rewritten, so the summary must not say so:\n%s", outBuf.String()+errBuf.String())
	}
}

func TestRunTwoPhase_RoutedSidecarIsNotWrittenAtAll(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeCleanArtifact(t, adrDir, "ADR-080")
	// ADR-082 carries one pairless behavioral on a prohibition.
	md := "---\nstatus: accepted\n---\n\n# ADR-082 — routed\n\n## Decision\n\nHooks MUST emit JSON and MUST NOT shell out.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-082.md"), []byte(md), 0o644); err != nil {
		t.Fatal(err)
	}
	routed := `schema_version: 2
topic: "testing"
path: "docs/architecture/decisions/ADR-082.md"
signals:
  - "bok signal"
directives:
  - text: "Hooks MUST emit JSON. (ref: ADR-082)"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Hooks MUST emit JSON and MUST NOT shell out."
prohibitions:
  - text: "MUST NOT shell out from a hook. (ref: ADR-082)"
    verify: "bash test/hooks/no-shell-out.sh"
    verify_kind: behavioral
    falsifying_observation: "a hook spawns a shell and the run passes"
    source_excerpts:
      - line_start: 9
        line_end: 9
        quote: "Hooks MUST emit JSON and MUST NOT shell out."
`
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-082.edikt.yaml"), []byte(routed), 0o644); err != nil {
		t.Fatal(err)
	}
	stageExtractorAgent(t, root, "claude-sonnet-5")
	gitInitCommit(t, root)

	var errBuf, outBuf bytes.Buffer
	if _, err := RunTwoPhase(TwoPhaseOptions{ProjectRoot: root, Stderr: &errBuf, Stdout: &outBuf, OnLoss: "accept"}, model.RealClock{}); err != nil {
		t.Fatalf("compile: %v\n%s%s", err, outBuf.String(), errBuf.String())
	}
	// COMPILE WRITES NOTHING TO A SOURCE ARTIFACT (D81 (d)). A consumer's
	// compile rewrote two committed sidecars and its own pre-commit gate
	// refused the result as "compiled surfaces staged by a non-compile
	// commit" — correctly: a read step had produced a diff.
	if p := strings.TrimSpace(gitPorcelain(t, root, "docs/architecture")); p != "" {
		t.Fatalf("compile must leave every source artifact untouched; git status:\n%q", p)
	}
	after, _ := os.ReadFile(filepath.Join(adrDir, "ADR-082.edikt.yaml"))
	if string(after) != routed {
		t.Fatalf("the sidecar on disk is not byte-identical to what was committed:\n%s", after)
	}
	// The proposal is queued and the summary says the sidecar was NOT touched.
	if _, err := os.Stat(filepath.Join(root, ".edikt", "state", "pending-verifies", "ADR-082-p00.yaml")); err != nil {
		t.Fatalf("the pairless behavioral must still be queued for approval: %v", err)
	}
	combined := outBuf.String() + errBuf.String()
	if !strings.Contains(combined, "the sidecar(s) are UNCHANGED on disk: ADR-082 — prohibitions[0] behavioral verify routed to pending (ADR-082-p00)") {
		t.Fatalf("the summary must name the routing and state that nothing was written:\n%s", combined)
	}
}
