package gov

// Red-first record (v0.9.1 release, 2026-10-10). Mutation made to see
// TestCompile_RefusedPlanPrintsItsRefusalOnce go red: delete the
// govrun.IsReported check in compile.go so the returned error is printed again
// -> red (the summary line appears twice on stderr). Restoring the check -> green.

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestCompile_RefusedPlanPrintsItsRefusalOnce(t *testing.T) {
	work := stageNeverInitialisedProject(t)
	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "compile", work)
	cmd.Env = append(os.Environ(),
		"HOME="+t.TempDir(), "CLAUDE_HOME="+t.TempDir(), "EDIKT_HEADLESS=")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	err := cmd.Run()
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1 with no extractor installed; got %v\nstderr:\n%s", err, stderr.String())
	}
	out := stderr.String()
	const summary = "could not emit a dispatch plan"
	if n := strings.Count(out, summary); n != 1 {
		t.Fatalf("the refusal summary %q appears %d times on stderr, want exactly once:\n%s", summary, n, out)
	}
	// The one line the release script greps for stays on one line.
	found := false
	for _, l := range strings.Split(out, "\n") {
		if strings.Contains(l, summary) && strings.Contains(l, "extractor definition not found") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no single line carries both the summary and the cause:\n%s", out)
	}
}
