package verify

import (
	"bytes"
	"context"
	"errors"
	"os/exec"
	"time"
)

// CriterionTimeout is the per-criterion subprocess timeout (Phase 12 spec).
const CriterionTimeout = 30 * time.Second

// ExcerptCap is the per-stream stdout/stderr cap recorded in the report.
const ExcerptCap = 4096

// Status values recorded in the report.
const (
	StatusPassed              = "passed"
	StatusFailed              = "failed"
	StatusTimeout             = "timeout"
	StatusSkippedOperational  = "skipped: operational"
	StatusSkippedInformational = "skipped: informational"
)

// Result is the per-criterion outcome captured by the runner.
type Result struct {
	ID             string `json:"id"`
	Statement      string `json:"statement"`
	Status         string `json:"status"`
	DurationMS     int64  `json:"duration_ms"`
	StdoutExcerpt  string `json:"stdout_excerpt"`
	StderrExcerpt  string `json:"stderr_excerpt"`
	ExitCode       int    `json:"exit_code"`
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
//	testable      → must have verify; execute it
//	operational   → verify optional; if present, run; else mark skipped
//	informational → never executed; marked skipped
//
// The function never returns an error for criterion failures — those are
// recorded in the returned Result. It only errors if the spawn itself
// fails before the subprocess started (e.g. bash missing).
func RunCriterion(phase Phase, c Criterion, opts RunOptions) Result {
	res := Result{ID: c.ID, Statement: c.Statement}

	switch phase.Classification {
	case ClassInformational:
		res.Status = StatusSkippedInformational
		return res
	case ClassOperational:
		if c.Verify == "" {
			res.Status = StatusSkippedOperational
			return res
		}
	case ClassTestable:
		// verify is guaranteed by Validate(); fall through.
	}

	timeout := opts.Timeout
	if timeout == 0 {
		timeout = CriterionTimeout
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "bash", "-c", c.Verify)
	if opts.Cwd != "" {
		cmd.Dir = opts.Cwd
	}
	// Inherit env, set EDIKT_VERIFY=1 so verifiers can detect runner-mode.
	cmd.Env = append(envWithVerify(), "EDIKT_VERIFY=1")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

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
	// Spawn-level failure (bash not found, etc.). Record as failed with
	// a synthetic exit code so the report shape stays uniform.
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
