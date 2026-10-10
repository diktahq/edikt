package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// CriterionTimeout is the per-criterion subprocess timeout (Phase 12 spec).
const CriterionTimeout = 30 * time.Second

// ExcerptCap is the per-stream stdout/stderr cap recorded in the report.
const ExcerptCap = 4096

// Status values recorded in the report.
const (
	StatusPassed  = "passed"
	StatusFailed  = "failed"
	StatusTimeout = "timeout"

	// StatusUnmeasured marks a verify whose SUBJECT could not be read, so the
	// command's exit code says nothing about the rule (SR-005... SPEC-018 SR-004).  edikt-guard:allow
	//
	// 42 of 158 verify commands begin with `!`, which flips a non-zero exit into
	// a pass. `! rg -l 'FORBIDDEN' src/*.go` exits 0 when src/ is missing and 0
	// when src/ is unreadable — both proven by execution, both previously
	// recorded as passed. Deleting the tree a rule protects was indistinguishable
	// from complying with it.
	//
	// Unmeasured is NOT failed. The rule may well hold; nothing here established
	// that it does, and INV-013's line is that a measured zero and a failed  edikt-guard:allow
	// measurement must not look the same to a reader.
	StatusUnmeasured           = "unmeasured: subject unreadable"
	StatusSkippedOperational   = "skipped: operational"
	StatusSkippedInformational = "skipped: informational"
	// StatusSkippedSuppressed marks a directive listed in the sidecar's own
	// suppressed_directives — excluded from the compiled corpus at render
	// time, so its verify: (if any) is stale by construction and MUST NOT
	// be executed. Distinct from StatusSkippedOperational (no verify: was
	// ever declared): here a verify: exists but the directive it checks no
	// longer counts.
	StatusSkippedSuppressed = "skipped: suppressed"

	// StatusSkippedOwner marks a criterion under an owner-gated phase: never
	// executed, because its completion is a human decision rather than a
	// command. Distinct from StatusSkippedInformational on purpose —
	// informational means nothing needed checking, owner means a person must
	// act and the report should still be able to say so.
	StatusSkippedOwner = "skipped: owner-gated"
)

// Result is the per-criterion outcome captured by the runner.
type Result struct {
	ID            string `json:"id"`
	Statement     string `json:"statement"`
	Status        string `json:"status"`
	DurationMS    int64  `json:"duration_ms"`
	StdoutExcerpt string `json:"stdout_excerpt"`
	StderrExcerpt string `json:"stderr_excerpt"`
	ExitCode      int    `json:"exit_code"`
}

// RunOptions configures a single runner invocation. Cwd is the directory
// passed to bash; Timeout overrides CriterionTimeout when non-zero (used
// by tests so we can exercise the timeout path quickly).
type RunOptions struct {
	Cwd     string
	Timeout time.Duration
}

// RunCriterion executes one criterion under bash -c, honoring the phase's
// classification:
//
//	testable → must have verify; execute it
//	operational → verify optional; if present, run; else mark skipped
//	informational → never executed; marked skipped
//
// The function never returns an error for criterion failures — those are
// recorded in the returned Result. It only errors if the spawn itself
// fails before the subprocess started (e.g. bash missing).
func RunCriterion(phase Phase, c Criterion, opts RunOptions) Result {
	switch phase.Classification {
	case ClassInformational:
		return Result{ID: c.ID, Statement: c.Statement, Status: StatusSkippedInformational}
	case ClassOwner:
		return Result{ID: c.ID, Statement: c.Statement, Status: StatusSkippedOwner}
	case ClassOperational:
		if c.Verify == "" {
			return Result{ID: c.ID, Statement: c.Statement, Status: StatusSkippedOperational}
		}
	case ClassTestable:
		// verify is guaranteed by Validate(); fall through.
	default:
		// Unreachable through LoadCriteria, which rejects any classification
		// outside the enum. Reached only by a caller constructing a Phase
		// directly — so it FAILS rather than executing or silently skipping.
		// Executing would run criteria under semantics nobody defined;
		// skipping would be a control that cannot fail. Failing names the
		// cause and cannot be mistaken for either.
		return Result{
			ID:            c.ID,
			Statement:     c.Statement,
			Status:        StatusFailed,
			StderrExcerpt: fmt.Sprintf("unrecognised phase classification %q — refusing to guess whether this criterion should run", phase.Classification),
		}
	}
	if c.TimeoutS > 0 {
		opts.Timeout = time.Duration(c.TimeoutS) * time.Second
	}
	return RunOne(c.ID, c.Statement, c.Verify, opts)
}

// RunOne executes a single verify command and returns the recorded Result.
// Lower-level entry point used by both the plan-criteria flow (via
// RunCriterion, which adds classification semantics) and the gov/prd/spec
// flows (which carry no phase classification — every item with a verify
// is runnable; items without verify are recorded as skipped by the
// caller).
//
// If verifyCmd is empty, the result is recorded as skipped:operational
// (the caller decides the policy for missing-verify items).
func RunOne(id, statement, verifyCmd string, opts RunOptions) Result {
	res := Result{ID: id, Statement: statement}
	if verifyCmd == "" {
		res.Status = StatusSkippedOperational
		return res
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = CriterionTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", verifyCmd)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	cmd.Env = append(envWithVerify(), "EDIKT_VERIFY=1", childDepthEnv())
	// A criterion runs under the verify runner, which is the door for the
	// walled verbs it may invoke (SPEC-020 SR-009): export the invoker when  edikt-guard:allow
	// the caller has not, so a criterion is never refused as an undeclared
	// non-terminal call.
	if os.Getenv("EDIKT_INVOKER") == "" {
		cmd.Env = append(cmd.Env, "EDIKT_INVOKER=/edikt:verify")
	}

	// Run the verify command in its own process group so a timeout can reap
	// everything it forked. CommandContext's default kill signals the direct
	// child only; a runaway verify's grandchildren survive it and keep
	// spawning. Setpgid + kill(-pgid) is the difference between "the command
	// stopped" and "the tree stopped".
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcessGroup(cmd) }

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	// SUBJECT PRE-FLIGHT (SPEC-018 SR-004). A command whose subject is absent  edikt-guard:allow
	// cannot report on the rule, whatever it exits with.
	if missing := unreadableSubjects(opts.Cwd, verifyCmd); missing != "" {
		res.Status = StatusUnmeasured
		res.ExitCode = -3
		res.StderrExcerpt = truncate([]byte("subject not readable: "+missing), ExcerptCap)
		return res
	}

	start := time.Now()
	err := cmd.Run()
	res.DurationMS = time.Since(start).Milliseconds()

	res.StdoutExcerpt = truncate(stdout.Bytes(), ExcerptCap)
	res.StderrExcerpt = truncate(stderr.Bytes(), ExcerptCap)

	if ctx.Err() == context.DeadlineExceeded {
		res.Status = StatusTimeout
		res.ExitCode = -1
		return res
	}
	if err == nil {
		res.Status = StatusPassed
		res.ExitCode = 0
		return res
	}
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		res.Status = StatusFailed
		res.ExitCode = ee.ExitCode()
		return res
	}
	res.Status = StatusFailed
	res.ExitCode = -2
	res.StderrExcerpt = truncate([]byte(err.Error()+"\n"+res.StderrExcerpt), ExcerptCap)
	return res
}

// truncate returns the first cap bytes of b as a string.
func truncate(b []byte, cap int) string {
	if len(b) <= cap {
		return string(b)
	}
	return string(b[:cap])
}
