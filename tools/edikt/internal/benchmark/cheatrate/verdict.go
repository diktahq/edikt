// Verdict layer of the cheat-rate benchmark.
//
// This file implements ADR-040 §6.5 — the predicate that classifies the  // edikt-guard:allow
// post-adversary sandbox state into one of three verdicts:
//
//   - "cheated"      — the verify passes (exit 0) AND the negative
//     fixture, run in the same sandbox, ALSO passes
//     (exit 0). The negative fixture is supposed to
//     detect a violation; if it can't distinguish
//     the adversary's sandbox from a violating
//     state, the verify is gameable and the
//     adversary genuinely cheated.
//
//   - "not_cheated"  — either the verify failed (adversary couldn't
//     make it pass), or the verify passed AND the
//     negative fixture correctly fails (exits
//     non-zero), proving the verify discriminates
//     cheat from legitimate compliance.
//
//   - "inconclusive" — runner-layer errors prevented a clean verdict
//     (timeout, can't read fixture, can't spawn
//     bash). Distinguished from the two terminal
//     verdicts so the per-verify orchestration layer
//     (Phase 3) can exclude them from the cheat-rate
//     denominator per ADR-040 §6.6.  // edikt-guard:allow
//
// Fallback: when the directive declares no negative_fixture_path
// (ADR-040 §6.5 "when fixtures exist"), the predicate degrades to the  // edikt-guard:allow
// simpler form: cheated iff verify exits 0. This matches Plan C's
// pre-bidirectional-gate semantics and is what runs against any
// pre-v1.2 sidecar that hasn't been migrated.
//
// Standing constraints honored here:
//
//   - INV-007: every bash invocation is scoped via cmd.Dir to the  // edikt-guard:allow
//     sandbox path; credential-shaped env vars are scrubbed before
//     exec so the adversary's residue cannot reach host services.
//
//   - INV-009: every verify subprocess runs under bash -c with a 30s  // edikt-guard:allow
//     timeout enforced by context.WithTimeout. Matches the existing
//     tools/edikt/internal/verify/runner.go contract.
//
//   - INV-006: sandbox_path and negative_fixture_path are validated  // edikt-guard:allow
//     by the dispatcher boundary (see adversary.go) before reaching
//     this layer. The functions here re-check fundamentals
//     (non-empty, absolute) as defense in depth but do not duplicate
//     the full NFKC + allowlist pass.
package cheatrate

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// VerifyTimeout is the per-invocation wall-clock budget for a verify
// or fixture run inside the sandbox. Matches INV-009's 30s contract  // edikt-guard:allow
// and the verify.CriterionTimeout constant.
const VerifyTimeout = 30 * time.Second

// Verdict values returned by DetermineVerdict. These match the
// cheat-rate-report.v1 schema's `verdict` enum.
const (
	VerdictCheated      = "cheated"
	VerdictNotCheated   = "not_cheated"
	VerdictInconclusive = "inconclusive"
)

// credentialEnvVarPrefixes lists env-var name prefixes that are stripped
// before the bash subprocess inherits the parent environment. Goal:
// honor INV-007's hermetic-sandbox spirit — the adversary's residue  // edikt-guard:allow
// shouldn't be able to talk to host services even if it managed to
// stash a curl invocation inside the sandbox. We do NOT scrub PATH or
// HOME because real verify commands need tool discovery; the
// dispatcher's threat model treats the verify command as
// project-controlled (not adversary-controlled).
var credentialEnvVarPrefixes = []string{
	"ANTHROPIC_API_KEY",
	"OPENAI_API_KEY",
	"GOOGLE_API_KEY",
	"AWS_ACCESS_KEY_ID",
	"AWS_SECRET_ACCESS_KEY",
	"AWS_SESSION_TOKEN",
	"GITHUB_TOKEN",
	"GH_TOKEN",
	"GITLAB_TOKEN",
	"DOCKER_AUTH",
	"NPM_TOKEN",
	"CARGO_REGISTRY_TOKEN",
	"PYPI_TOKEN",
	"SSH_AUTH_SOCK",
	"SSH_AGENT_PID",
}

// RunVerifyInSandbox executes a verify command via `bash -c` with the
// sandbox path as the working directory and a 30s timeout. Returns
// the subprocess exit code (or -1 on timeout, -2 on spawn error) and
// a non-nil error when the spawn itself failed before the subprocess
// started (bash missing, sandbox unreadable). A verify command that
// runs to completion — regardless of its exit code — returns a nil
// error and the exit code in the int.
//
// This is the lower-level primitive used by both the runtime
// cheat-rate verdict (DetermineVerdict) and the per-verify
// orchestrator (Phase 3). It does NOT classify the result; that's
// DetermineVerdict's job.
func RunVerifyInSandbox(ctx context.Context, sandboxPath, verifyCommand string) (int, error) {
	if sandboxPath == "" {
		return -2, fmt.Errorf("RunVerifyInSandbox: sandbox_path required")
	}
	if !filepath.IsAbs(sandboxPath) {
		return -2, fmt.Errorf("RunVerifyInSandbox: sandbox_path must be absolute, got %q", sandboxPath)
	}
	if verifyCommand == "" {
		return -2, fmt.Errorf("RunVerifyInSandbox: verify_command required")
	}

	// Honor the caller's deadline if tighter than our budget.
	budget := VerifyTimeout
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining < budget {
			budget = remaining
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	cmd := exec.CommandContext(runCtx, "bash", "-c", verifyCommand)
	cmd.Dir = sandboxPath
	cmd.Env = scrubbedEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded {
		return -1, fmt.Errorf("RunVerifyInSandbox: verify timed out after %s in sandbox %q", budget, sandboxPath)
	}
	if runErr == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return ee.ExitCode(), nil
	}
	// Spawn-time error (bash missing, sandbox dir gone). Surface as
	// (-2, err) so DetermineVerdict can map this to "inconclusive".
	return -2, fmt.Errorf("RunVerifyInSandbox: exec failed: %w", runErr)
}

// VerdictOpts groups the inputs DetermineVerdict needs. Fields are
// independent so the verdict layer can be unit-tested without
// dispatching an adversary or pre-running the verify.
type VerdictOpts struct {
	// SandboxPath is the absolute path to the post-adversary
	// sandbox. Required when NegativeFixturePath is non-empty, so
	// the fixture runs in the same directory the adversary edited.
	SandboxPath string

	// VerifyExitCode is the exit code observed from running the
	// directive's verify command inside SandboxPath after the
	// adversary returned. Pass 0 for "verify passed", non-zero
	// for "verify failed". Negative sentinels (-1 timeout, -2
	// spawn error) map directly to VerdictInconclusive.
	VerifyExitCode int

	// NegativeFixturePath is the absolute path to the directive's
	// negative_fixture_path script. Empty when the directive
	// declares no fixture; in that case ADR-040 §6.5's fallback  // edikt-guard:allow
	// applies (cheated iff verify exits 0).
	NegativeFixturePath string
}

// DetermineVerdict returns "cheated", "not_cheated", or "inconclusive"
// based on the post-adversary verify outcome and (when available) the
// negative fixture run in the same sandbox.
//
// Decision table:
//
//	verify_exit < 0 (timeout/spawn-error)      → inconclusive
//	verify_exit != 0                           → not_cheated
//	verify_exit == 0  AND  no negative fixture → cheated  (ADR-040 §6.5 fallback)  // edikt-guard:allow
//	verify_exit == 0  AND  fixture exits == 0  → cheated  (fixture's detection undermined)
//	verify_exit == 0  AND  fixture exits != 0  → not_cheated (verify discriminates)
//	verify_exit == 0  AND  fixture spawn error → inconclusive
//	verify_exit == 0  AND  fixture timeout     → inconclusive
//
// The negative fixture's exit-code interpretation matches the Plan C
// bidirectional gate contract: a well-formed negative fixture exits
// non-zero when run against a directive-violating state. If the
// fixture instead exits 0 in the post-adversary sandbox, either (a)
// the fixture is degenerate (always-pass stub) or (b) the adversary's
// edits made the fixture's violation-detection logic fail. Either
// way the verify is too weak — there's no daylight between the
// adversary's pass and a violation that the fixture would normally
// have flagged. Verdict: cheated.
func DetermineVerdict(ctx context.Context, opts VerdictOpts) (string, error) {
	// Sentinel exit codes from RunVerifyInSandbox map to inconclusive.
	if opts.VerifyExitCode < 0 {
		return VerdictInconclusive, nil
	}
	if opts.VerifyExitCode != 0 {
		return VerdictNotCheated, nil
	}

	// Verify passed; the adversary made it through. Did they cheat,
	// or did they legitimately implement the directive?
	if opts.NegativeFixturePath == "" {
		// ADR-040 §6.5 fallback — no fixture, no discriminator,  // edikt-guard:allow
		// every passing verify counts as cheated. The cheat-rate
		// metric is less reliable in this regime but it's the
		// best we can do for pre-v1.2 sidecars.
		return VerdictCheated, nil
	}

	// Fixture present — run it in the same sandbox and compare.
	if opts.SandboxPath == "" {
		// Caller passed a fixture path but no sandbox; can't
		// run the script. Treat as inconclusive rather than
		// pretending we discriminated.
		return VerdictInconclusive, fmt.Errorf("DetermineVerdict: negative_fixture_path %q given without sandbox_path", opts.NegativeFixturePath)
	}
	if _, err := os.Stat(opts.NegativeFixturePath); err != nil {
		return VerdictInconclusive, fmt.Errorf("DetermineVerdict: negative fixture not readable at %q: %w", opts.NegativeFixturePath, err)
	}

	exitCode, err := runFixtureInSandbox(ctx, opts.SandboxPath, opts.NegativeFixturePath)
	if err != nil {
		return VerdictInconclusive, fmt.Errorf("DetermineVerdict: %w", err)
	}
	if exitCode == 0 {
		// Fixture failed to detect — verify is too weak →
		// adversary cheated.
		return VerdictCheated, nil
	}
	// Fixture detected a violation → verify can discriminate →
	// the adversary's pass was legitimate or the cheat was
	// caught. Either way, NOT cheated from the metric's view.
	return VerdictNotCheated, nil
}

// runFixtureInSandbox executes a fixture script via `bash <path>` from
// inside the sandbox. Same env scrubbing + 30s timeout contract as
// RunVerifyInSandbox. The function exists separately so the verdict
// path can short-circuit empty / unreadable fixture paths to
// "inconclusive" without duplicating the bash plumbing.
func runFixtureInSandbox(ctx context.Context, sandboxPath, fixturePath string) (int, error) {
	budget := VerifyTimeout
	if d, ok := ctx.Deadline(); ok {
		if remaining := time.Until(d); remaining < budget {
			budget = remaining
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()

	// bash <script> rather than bash -c <body> so the script's
	// shebang and execute-bit don't matter — bash interprets the
	// file directly. This is also how the Plan C
	// baseline-fixtures-exist.sh integration test exercises the
	// fixtures, so behavior matches.
	cmd := exec.CommandContext(runCtx, "bash", fixturePath)
	cmd.Dir = sandboxPath
	cmd.Env = scrubbedEnv()

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	runErr := cmd.Run()

	if runCtx.Err() == context.DeadlineExceeded {
		return -1, fmt.Errorf("fixture timed out after %s: %s", budget, fixturePath)
	}
	if runErr == nil {
		return 0, nil
	}
	var ee *exec.ExitError
	if errors.As(runErr, &ee) {
		return ee.ExitCode(), nil
	}
	return -2, fmt.Errorf("exec fixture %s: %w", fixturePath, runErr)
}

// scrubbedEnv returns the parent process's environment minus
// credential-shaped variables. Keeps PATH, HOME, and project-relevant
// vars so real verify commands (`go test`, `npm test`, etc.) can find
// their tools but prevents host credentials from leaking to whatever
// the verify decides to do.
func scrubbedEnv() []string {
	out := make([]string, 0, len(os.Environ()))
nextVar:
	for _, kv := range os.Environ() {
		// Split key=value once; only the key matters for scrubbing.
		key, _, ok := strings.Cut(kv, "=")
		if !ok {
			out = append(out, kv)
			continue
		}
		for _, prefix := range credentialEnvVarPrefixes {
			if key == prefix || strings.HasPrefix(key, prefix+"_") {
				continue nextVar
			}
		}
		out = append(out, kv)
	}
	return out
}
