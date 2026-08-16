package cheatrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeScript writes a small bash script into the given dir and returns
// its absolute path. Used to synthesize negative fixtures whose exit
// behavior is controlled by the test.
func writeScript(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write script %s: %v", path, err)
	}
	return path
}

// sandboxDir reuses newSandbox from adversary_test.go (same package).

// TestRunVerifyInSandbox_HappyPath confirms a verify command that exits
// 0 returns (0, nil). The verify command is run via bash -c with
// cmd.Dir set to the sandbox.
func TestRunVerifyInSandbox_HappyPath(t *testing.T) {
	sandbox := newSandbox(t)
	// Write a marker file into the sandbox so the verify can grep
	// it — exercises cmd.Dir scoping.
	if err := os.WriteFile(filepath.Join(sandbox, "marker"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	code, err := RunVerifyInSandbox(context.Background(), sandbox, "grep -q ok marker")
	if err != nil {
		t.Fatalf("RunVerifyInSandbox: unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("expected exit 0, got %d", code)
	}
}

// TestRunVerifyInSandbox_NonZeroExit confirms a verify that exits
// non-zero is reported without error.
func TestRunVerifyInSandbox_NonZeroExit(t *testing.T) {
	sandbox := newSandbox(t)
	code, err := RunVerifyInSandbox(context.Background(), sandbox, "exit 7")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 7 {
		t.Errorf("expected exit 7, got %d", code)
	}
}

// TestRunVerifyInSandbox_Timeout uses a sleep that outlives the
// caller's 200ms deadline. The function must return -1 with a
// timeout-shaped error.
func TestRunVerifyInSandbox_Timeout(t *testing.T) {
	sandbox := newSandbox(t)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	code, err := RunVerifyInSandbox(ctx, sandbox, "sleep 10")
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if code != -1 {
		t.Errorf("expected exit -1 on timeout, got %d", code)
	}
	if elapsed > 2*time.Second {
		t.Errorf("timeout took too long: %v (subprocess not killed?)", elapsed)
	}
}

// TestRunVerifyInSandbox_RejectsBadInputs covers AC-2.1's input
// validation — absolute path required, command required.
func TestRunVerifyInSandbox_RejectsBadInputs(t *testing.T) {
	cases := []struct {
		name    string
		sandbox string
		cmd     string
		want    string
	}{
		{"empty_sandbox", "", "true", "sandbox_path required"},
		{"relative_sandbox", "./relative", "true", "sandbox_path must be absolute"},
		{"empty_command", "/tmp", "", "verify_command required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, err := RunVerifyInSandbox(context.Background(), tc.sandbox, tc.cmd)
			if err == nil {
				t.Fatalf("expected error, got nil (code=%d)", code)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestDetermineVerdict_VerifyFails covers AC-2.5 case 1 — verify
// returns non-zero → not_cheated regardless of fixture state.
func TestDetermineVerdict_VerifyFails(t *testing.T) {
	sandbox := newSandbox(t)
	negFixture := writeScript(t, sandbox, "neg.sh", "#!/bin/sh\nexit 0\n")
	got, err := DetermineVerdict(context.Background(), VerdictOpts{
		SandboxPath:         sandbox,
		VerifyExitCode:      1,
		NegativeFixturePath: negFixture,
	})
	if err != nil {
		t.Fatalf("DetermineVerdict: %v", err)
	}
	if got != VerdictNotCheated {
		t.Errorf("expected %q when verify fails, got %q", VerdictNotCheated, got)
	}
}

// TestDetermineVerdict_VerifyPassesNoFixture covers AC-2.5 case 2 —
// the ADR-040 §6.5 fallback: with no negative fixture, every passing
// verify is cheated.
func TestDetermineVerdict_VerifyPassesNoFixture(t *testing.T) {
	got, err := DetermineVerdict(context.Background(), VerdictOpts{
		SandboxPath:         "/tmp",
		VerifyExitCode:      0,
		NegativeFixturePath: "",
	})
	if err != nil {
		t.Fatalf("DetermineVerdict: %v", err)
	}
	if got != VerdictCheated {
		t.Errorf("expected %q on fallback, got %q", VerdictCheated, got)
	}
}

// TestDetermineVerdict_VerifyPassesFixtureAlsoPasses covers AC-2.5
// case 3 — both verify and negative fixture exit 0 → the fixture's
// detection logic was undermined → cheated.
func TestDetermineVerdict_VerifyPassesFixtureAlsoPasses(t *testing.T) {
	sandbox := newSandbox(t)
	negFixture := writeScript(t, sandbox, "neg.sh",
		"#!/bin/sh\n# fixture cannot detect this cheat (always-pass stub)\nexit 0\n")
	got, err := DetermineVerdict(context.Background(), VerdictOpts{
		SandboxPath:         sandbox,
		VerifyExitCode:      0,
		NegativeFixturePath: negFixture,
	})
	if err != nil {
		t.Fatalf("DetermineVerdict: %v", err)
	}
	if got != VerdictCheated {
		t.Errorf("expected %q when fixture also passes, got %q", VerdictCheated, got)
	}
}

// TestDetermineVerdict_VerifyPassesFixtureDetects covers AC-2.5
// case 4 — verify passes BUT negative fixture exits non-zero
// (detected a violation) → verify discriminates → not_cheated.
func TestDetermineVerdict_VerifyPassesFixtureDetects(t *testing.T) {
	sandbox := newSandbox(t)
	negFixture := writeScript(t, sandbox, "neg.sh",
		"#!/bin/sh\n# fixture detects the violation\nexit 1\n")
	got, err := DetermineVerdict(context.Background(), VerdictOpts{
		SandboxPath:         sandbox,
		VerifyExitCode:      0,
		NegativeFixturePath: negFixture,
	})
	if err != nil {
		t.Fatalf("DetermineVerdict: %v", err)
	}
	if got != VerdictNotCheated {
		t.Errorf("expected %q when fixture detects, got %q", VerdictNotCheated, got)
	}
}

// TestDetermineVerdict_FixtureTimeout covers AC-2.5 case 5 — fixture
// sleeps past the deadline → inconclusive.
func TestDetermineVerdict_FixtureTimeout(t *testing.T) {
	sandbox := newSandbox(t)
	negFixture := writeScript(t, sandbox, "neg.sh",
		"#!/bin/sh\nexec sleep 10\n")

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	start := time.Now()
	got, err := DetermineVerdict(ctx, VerdictOpts{
		SandboxPath:         sandbox,
		VerifyExitCode:      0,
		NegativeFixturePath: negFixture,
	})
	elapsed := time.Since(start)
	if err == nil {
		t.Errorf("expected timeout error from fixture; got nil")
	}
	if got != VerdictInconclusive {
		t.Errorf("expected %q on fixture timeout, got %q", VerdictInconclusive, got)
	}
	if elapsed > 2*time.Second {
		t.Errorf("fixture timeout took too long: %v", elapsed)
	}
}

// TestDetermineVerdict_SandboxOrFixtureError covers AC-2.5 case 6 —
// verify passed but the fixture path is missing → inconclusive.
func TestDetermineVerdict_SandboxOrFixtureError(t *testing.T) {
	sandbox := newSandbox(t)
	missing := filepath.Join(sandbox, "does-not-exist.sh")
	got, err := DetermineVerdict(context.Background(), VerdictOpts{
		SandboxPath:         sandbox,
		VerifyExitCode:      0,
		NegativeFixturePath: missing,
	})
	if err == nil {
		t.Errorf("expected error when fixture path is missing")
	}
	if got != VerdictInconclusive {
		t.Errorf("expected %q on missing fixture, got %q", VerdictInconclusive, got)
	}
}

// TestDetermineVerdict_VerifyExitNegativeSentinels confirms timeout
// and spawn-error sentinels from RunVerifyInSandbox map to
// inconclusive.
func TestDetermineVerdict_VerifyExitNegativeSentinels(t *testing.T) {
	for _, code := range []int{-1, -2} {
		got, err := DetermineVerdict(context.Background(), VerdictOpts{
			SandboxPath:    "/tmp",
			VerifyExitCode: code,
		})
		if err != nil {
			t.Fatalf("DetermineVerdict(code=%d): %v", code, err)
		}
		if got != VerdictInconclusive {
			t.Errorf("verify_exit=%d: expected %q, got %q", code, VerdictInconclusive, got)
		}
	}
}

// TestScrubbedEnv_RemovesCredentials confirms AC-2.6's env scrubbing.
// Set a credential-shaped env var, confirm it's absent from the
// returned slice, confirm a normal var (PATH) survives.
func TestScrubbedEnv_RemovesCredentials(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret-token-123")
	t.Setenv("GITHUB_TOKEN", "ghs_xyz")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "awssecret")
	t.Setenv("INNOCENT_VAR", "harmless")

	env := scrubbedEnv()
	for _, kv := range env {
		switch {
		case strings.HasPrefix(kv, "ANTHROPIC_API_KEY="):
			t.Errorf("scrubbedEnv leaked ANTHROPIC_API_KEY")
		case strings.HasPrefix(kv, "GITHUB_TOKEN="):
			t.Errorf("scrubbedEnv leaked GITHUB_TOKEN")
		case strings.HasPrefix(kv, "AWS_SECRET_ACCESS_KEY="):
			t.Errorf("scrubbedEnv leaked AWS_SECRET_ACCESS_KEY")
		}
	}
	// INNOCENT_VAR and PATH must survive.
	var sawInnocent, sawPath bool
	for _, kv := range env {
		if strings.HasPrefix(kv, "INNOCENT_VAR=") {
			sawInnocent = true
		}
		if strings.HasPrefix(kv, "PATH=") {
			sawPath = true
		}
	}
	if !sawInnocent {
		t.Error("scrubbedEnv dropped INNOCENT_VAR")
	}
	if !sawPath {
		t.Error("scrubbedEnv dropped PATH — verify commands need tool discovery")
	}
}

// TestRunVerifyInSandbox_EnvScrubbedAtRuntime confirms the scrubbing
// reaches the bash subprocess. The verify command echoes
// $ANTHROPIC_API_KEY; the captured exit code is non-zero (since the
// shell test fails) which confirms the var is empty in-process.
func TestRunVerifyInSandbox_EnvScrubbedAtRuntime(t *testing.T) {
	t.Setenv("ANTHROPIC_API_KEY", "secret-token-123")
	sandbox := newSandbox(t)
	// The verify exits 0 iff ANTHROPIC_API_KEY is empty in the
	// subprocess. If scrubbing failed, the var would be set and
	// the verify would exit 1.
	code, err := RunVerifyInSandbox(context.Background(), sandbox,
		`[ -z "$ANTHROPIC_API_KEY" ]`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if code != 0 {
		t.Errorf("ANTHROPIC_API_KEY leaked to verify subprocess (got exit %d)", code)
	}
}
