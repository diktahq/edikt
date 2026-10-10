package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/verify"
)

// covSidecar is a valid sidecar whose one directive carries a verify:
// command. If doctor ever executes it, the marker file appears.
func covSidecar(relMdPath, verifyCmd string) string {
	return `schema_version: 1
topic: hooks
path: ` + relMdPath + `
signals:
  - hook
directives:
  - text: "Hooks must emit JSON. (ref: INV-003)"
    verify: "` + verifyCmd + `"
    source_excerpt:
      line_start: 3
      line_end: 3
      quote: "Hooks must emit JSON."
`
}

// D38 defect 2: doctor never executes a verify. With no persisted report the
// group says NOT MEASURED with the remedy — never a pass (INV-013).
func TestVerifyCoverage_NeverExecutes_ReportsNotMeasured(t *testing.T) {
	root := scaffoldProject(t)
	marker := filepath.Join(root, "executed.marker")
	mdRel := "docs/architecture/decisions/ADR-500-verify.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-500-verify.edikt.yaml"),
		covSidecar(mdRel, "touch "+marker))
	t.Setenv("EDIKT_VERIFY_TRUST", "1")

	var buf bytes.Buffer
	_, warns, ran := runVerifyCoverageChecks(root, &buf)
	if !ran {
		t.Fatalf("a project with a sidecar must run the group; got:\n%s", buf.String())
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("doctor executed a sidecar verify — the marker exists:\n%s", buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "NOT MEASURED") || !strings.Contains(out, "edikt verify all") {
		t.Fatalf("expected NOT MEASURED with the remedy; got:\n%s", out)
	}
	if warns == 0 {
		t.Fatalf("an unmeasured corpus is a warning, not silence; got:\n%s", out)
	}
	if strings.Contains(out, "[ok]") {
		t.Fatalf("nothing was measured, so nothing may read as ok:\n%s", out)
	}
}

// A persisted failing report is reported as failing with the remedy, and the
// summary counts it as measured.
func TestVerifyCoverage_ReadsPersistedReport(t *testing.T) {
	root := scaffoldProject(t)
	mdRel := "docs/architecture/decisions/ADR-501-verify.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-501-verify.edikt.yaml"),
		covSidecar(mdRel, "true"))
	stateDir := filepath.Join(root, ".edikt", "state", "verify")
	r := verify.NewReport("gov-ADR-501", "all", "deadbeef", []verify.Result{
		{ID: "directives[0]", Statement: "Hooks must emit JSON.", Status: verify.StatusFailed, ExitCode: 1},
		{ID: "directives[1]", Statement: "x", Status: verify.StatusPassed},
	})
	if _, err := verify.WriteReports(stateDir, r); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	errs, _, _ := runVerifyCoverageChecks(root, &buf)
	out := buf.String()
	if !strings.Contains(out, "gov/ADR-501 — 1 of 2 verify(s) failing") || !strings.Contains(out, "edikt verify gov ADR-501") {
		t.Fatalf("expected the failing sidecar with its remedy; got:\n%s", out)
	}
	if !strings.Contains(out, "measured 1/1 sidecars") {
		t.Fatalf("expected the measured count; got:\n%s", out)
	}
	// A verify that RAN and FAILED is an ERROR now (FR-035), not a warning:
	// it described a broken rule in the same shade as "never measured".
	if errs != 1 {
		t.Fatalf("expected exactly the failing verify as an error, got %d:\n%s", errs, out)
	}
}

// D38 defect 2, end to end: `edikt doctor` with stdin from /dev/null exits
// well under 60 s on a project whose only verify would sleep for 120 s, and
// leaves that verify unexecuted. Before this fix doctor ran every verify
// (137 s of 168 s on this repo) and, under a 2-minute tool timeout, looked
// like a hang.
func TestDoctor_StdinDevNull_ExitsUnder60s(t *testing.T) {
	bin := buildBinary(t)
	root := scaffoldProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(root, ".edikt", "config.yaml"), "version: 1\n")
	marker := filepath.Join(root, "executed.marker")
	mdRel := "docs/architecture/decisions/ADR-502-slow.md"
	validMd(t, root, mdRel)
	writeFile(t, filepath.Join(root, "docs/architecture/decisions/ADR-502-slow.edikt.yaml"),
		covSidecar(mdRel, "sleep 120 && touch "+marker))

	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	cmd := exec.Command(bin, "doctor")
	cmd.Dir = root
	cmd.Stdin = devnull
	cmd.Env = append(os.Environ(), "EDIKT_VERIFY_TRUST=1", "EDIKT_ROOT=", "EDIKT_HOME="+t.TempDir(), "HOME="+t.TempDir(), "CLAUDE_HOME="+t.TempDir())
	start := time.Now()
	out, _ := cmd.CombinedOutput() // doctor's exit code reflects findings; the timing is the assertion
	elapsed := time.Since(start)
	if elapsed > 60*time.Second {
		t.Fatalf("doctor took %s with stdin from /dev/null — it must exit under 60 s:\n%s", elapsed, out)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("doctor executed the sidecar verify:\n%s", out)
	}
	if !strings.Contains(string(out), "NOT MEASURED") {
		t.Fatalf("doctor must say the verify coverage was not measured:\n%s", out)
	}
}
