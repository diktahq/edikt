package cmd

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// CLI ports of the c1 scratch repros (#11 body): the built binary, a sandbox
// HOME, and only the exit codes and stdout/stderr a caller sees.

// runSandboxed execs the built binary with every user-path root pointed at a
// per-test sandbox, so nothing resolves the real ~/.claude or ~/.edikt.
func runSandboxed(t *testing.T, root, home string, args ...string) (string, string, int) {
	t.Helper()
	c := exec.Command(buildBinary(t), args...)
	c.Dir = root
	c.Env = append(os.Environ(),
		"HOME="+home,
		"CLAUDE_HOME="+filepath.Join(home, ".claude"),
		"CLAUDE_CONFIG_DIR="+filepath.Join(home, ".claude"),
		"EDIKT_HOME="+filepath.Join(home, ".edikt"),
		"EDIKT_INVOKER=ci",
		"EDIKT_SKIP_VERSION_GATE=1",
	)
	var out, errb bytes.Buffer
	c.Stdout, c.Stderr = &out, &errb
	code := 0
	if err := c.Run(); err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			t.Fatalf("edikt %v did not run: %v", args, err)
		}
		code = ee.ExitCode()
	}
	return out.String(), errb.String(), code
}

type bbProject struct {
	root, home, adr string
}

func newBBProject(t *testing.T) *bbProject {
	t.Helper()
	p := &bbProject{root: t.TempDir(), home: t.TempDir()}
	bbWriteExtractorAgent(t, p.root, 11)
	bbWriteConfig(t, p.root, "")
	p.adr = bbWriteADR(t, p.root, "ADR-900", gateQuoteBB)
	return p
}

func (p *bbProject) run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	return runSandboxed(t, p.root, p.home, args...)
}

func (p *bbProject) planArtifact(t *testing.T, extra ...string) (string, string, int) {
	t.Helper()
	// Relative to the project root: the binary resolves the root through
	// symlinks (macOS /var -> /private/var), so an absolute temp path would
	// name no governed artifact.
	rel := filepath.ToSlash(filepath.Join("docs", "architecture", "decisions", filepath.Base(p.adr)))
	out, errOut, code := p.run(t, append([]string{"phasea", "plan", "--artifact", rel}, extra...)...)
	return bbPlanIDRe.FindString(out), out + errOut, code
}

func (p *bbProject) mustExit(t *testing.T, want int, args ...string) string {
	t.Helper()
	out, errOut, code := p.run(t, args...)
	if code != want {
		t.Fatalf("edikt %v exited %d, want %d\nstdout: %s\nstderr: %s", args, code, want, out, errOut)
	}
	return out + errOut
}

func stateFiles(t *testing.T, root, planID string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	dir := filepath.Join(root, ".edikt", "state", "phasea")
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.Contains(path, planID) {
			return nil
		}
		b, rerr := os.ReadFile(path)
		if rerr == nil {
			out[path] = b
		}
		return nil
	})
	return out
}

// repro.sh: bootstrap → extract → restore → verify (CONSUMED) → re-request the
// unchanged artifact. HEAD derived the consumed id again and rewrote its plan
// file and pins.
func TestPhaseaBlackbox_ReRequestAfterConsumeGetsDistinctID(t *testing.T) {
	p := newBBProject(t)
	first, msg, code := p.planArtifact(t, "--bootstrap")
	if code != 0 || first == "" {
		t.Fatalf("bootstrap plan: exit %d\n%s", code, msg)
	}
	bbWriteGoodSidecar(t, p.adr, gateQuoteBB)
	p.mustExit(t, phasea.ExitRestoreOK, "phasea", "restore", "--plan", first)
	p.mustExit(t, phasea.ExitVerifyOK, "phasea", "verify", "--plan", first)
	before := stateFiles(t, p.root, first)
	if len(before) < 3 {
		t.Fatalf("setup: expected plan, restore receipt and receipt for %s, found %d file(s)", first, len(before))
	}

	second, msg, code := p.planArtifact(t)
	if code != 0 || second == "" {
		t.Fatalf("re-request after consume: exit %d\n%s", code, msg)
	}
	if second == first {
		t.Fatalf("re-request after %s was consumed reused its plan_id:\n%s", first, msg)
	}
	after := stateFiles(t, p.root, first)
	for path, b := range before {
		if !bytes.Equal(b, after[path]) {
			t.Errorf("%s changed after the re-request — a consumed plan's state is closed", path)
		}
	}
	if len(after) != len(before) {
		t.Errorf("consumed plan %s gained or lost state files: %d -> %d", first, len(before), len(after))
	}
}

// repro_failed.sh: the artifact moves after planning, so verify ends FAILED
// with the extractor's output restored on disk. Reverting the artifact made
// HEAD re-derive the FAILED id; restore stamped the new output into it and
// verify carried the stale verdict forward. Now: restore refuses the FAILED
// plan, and a re-request refuses until the pin is put back.
func TestPhaseaBlackbox_RestoreRefusesFailedPlan(t *testing.T) {
	p := newBBProject(t)
	original, err := os.ReadFile(p.adr)
	if err != nil {
		t.Fatal(err)
	}
	id, msg, code := p.planArtifact(t, "--bootstrap")
	if code != 0 || id == "" {
		t.Fatalf("bootstrap plan: exit %d\n%s", code, msg)
	}
	bbWriteGoodSidecar(t, p.adr, gateQuoteBB)
	if err := os.WriteFile(p.adr, append(append([]byte{}, original...), []byte("\nAn edit after planning.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	p.mustExit(t, phasea.ExitRestoreOK, "phasea", "restore", "--plan", id)
	p.mustExit(t, phasea.ExitVerifyFailed, "phasea", "verify", "--plan", id)
	if err := os.WriteFile(p.adr, original, 0o644); err != nil { // revert the artifact
		t.Fatal(err)
	}

	out := p.mustExit(t, phasea.ExitRestorePlanRejected, "phasea", "restore", "--plan", id)
	if !strings.Contains(out, "FAILED") {
		t.Errorf("restore's refusal must say the plan is FAILED:\n%s", out)
	}

	re, msg, code := p.planArtifact(t)
	if code != phasea.ExitRefused {
		t.Fatalf("re-request over FAILED %s's output: exit %d (id %q), want %d\n%s", id, code, re, phasea.ExitRefused, msg)
	}
	if !strings.Contains(msg, id) {
		t.Errorf("the refusal must name the FAILED plan %s:\n%s", id, msg)
	}
}
