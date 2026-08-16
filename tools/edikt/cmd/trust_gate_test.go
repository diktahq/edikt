package cmd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// runVerifyIsolated runs the built binary's `verify` subcommand with a curated
// environment: HOME points at an isolated trust-store home; EDIKT_VERIFY_TRUST
// and EDIKT_VERIFY_TRUST_MODE are whatever the caller passes (default: absent,
// i.e. warn posture, untrusted). This overrides TestMain's process-wide bypass
// so the ADR-041 gate is actually exercised. PATH is preserved so the verify
// runner can find bash.
func runVerifyIsolated(t *testing.T, root, home string, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	bin := buildBinary(t)
	full := append([]string{"verify"}, args...)
	cmd := exec.Command(bin, full...)
	cmd.Dir = root
	cmd.Env = append([]string{"HOME=" + home, "PATH=" + os.Getenv("PATH")}, extraEnv...)
	out, err := cmd.CombinedOutput()
	if err == nil {
		return string(out), 0
	}
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	}
	t.Fatalf("exec: %v", err)
	return "", -1
}

const passingPhase = `phases:
  - id: "1"
    name: pass
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`

// Default (warn) posture: an untrusted project RUNS its verify (no block, exit 0),
// prints a one-time trust-on-first-use notice, and records the root so a second
// run is silent. Zero friction.
func TestTrustGate_warnDefault_runsRecordsWarns(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", passingPhase)
	home := t.TempDir()

	out, code := runVerifyIsolated(t, root, home, nil, "demo", "--phase", "1")
	if code != 0 {
		t.Fatalf("warn-posture verify: got exit %d, want 0\n%s", code, out)
	}
	if !strings.Contains(out, "trust-on-first-use") {
		t.Fatalf("expected a one-time TOFU notice, got:\n%s", out)
	}

	// Second run: recorded → silent, no notice.
	out2, code2 := runVerifyIsolated(t, root, home, nil, "demo", "--phase", "1")
	if code2 != 0 {
		t.Fatalf("second run: got exit %d, want 0", code2)
	}
	if strings.Contains(out2, "trust-on-first-use") {
		t.Fatalf("TOFU notice should fire once, not on the recorded second run:\n%s", out2)
	}
}

// Block posture (opt-in): an untrusted project is refused with exit 4 and an
// actionable message — never silently executes repo-defined shell.
func TestTrustGate_blockMode_denies(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", passingPhase)
	home := t.TempDir()
	out, code := runVerifyIsolated(t, root, home, []string{"EDIKT_VERIFY_TRUST_MODE=block"}, "demo", "--phase", "1")
	if code != 4 {
		t.Fatalf("block-posture untrusted verify: got exit %d, want 4\n%s", code, out)
	}
	if !strings.Contains(out, "not an approved edikt project") {
		t.Fatalf("expected actionable block message, got:\n%s", out)
	}
}

// Disabled posture: proceed silently — no block, no notice.
func TestTrustGate_disabledMode_silent(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", passingPhase)
	home := t.TempDir()
	out, code := runVerifyIsolated(t, root, home, []string{"EDIKT_VERIFY_TRUST_MODE=disabled"}, "demo", "--phase", "1")
	if code != 0 {
		t.Fatalf("disabled-posture verify: got exit %d, want 0\n%s", code, out)
	}
	if strings.Contains(out, "trust-on-first-use") {
		t.Fatalf("disabled posture must not warn:\n%s", out)
	}
}

// EDIKT_VERIFY_TRUST=1 grants ephemeral trust even under block posture.
func TestTrustGate_envBypassAllows(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", passingPhase)
	home := t.TempDir()
	out, code := runVerifyIsolated(t, root, home,
		[]string{"EDIKT_VERIFY_TRUST=1", "EDIKT_VERIFY_TRUST_MODE=block"}, "demo", "--phase", "1")
	if code != 0 {
		t.Fatalf("env-bypass verify: got exit %d, want 0\n%s", code, out)
	}
}

// --trust records the project (even under block posture), so the run proceeds
// AND a later run with no trust signal also proceeds (persistent approval).
func TestTrustGate_trustFlagRecordsAndPersists(t *testing.T) {
	root := scaffoldVerifyProject(t, "demo", passingPhase)
	home := t.TempDir()

	_, code := runVerifyIsolated(t, root, home,
		[]string{"EDIKT_VERIFY_TRUST_MODE=block"}, "demo", "--phase", "1", "--trust")
	if code != 0 {
		t.Fatalf("--trust verify: got exit %d, want 0", code)
	}

	// Second run, block posture, no --trust — the recorded entry alone satisfies it.
	_, code2 := runVerifyIsolated(t, root, home,
		[]string{"EDIKT_VERIFY_TRUST_MODE=block"}, "demo", "--phase", "1")
	if code2 != 0 {
		t.Fatalf("post-trust verify: got exit %d, want 0 (recorded trust should persist)", code2)
	}
}
