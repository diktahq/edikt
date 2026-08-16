package phasea

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/redact"
)

// Dispatcher fans Tasks out to a Runner with a concurrency cap, captures
// failures continue-on-error, emits
// per-completion progress with running p50 ETA on stderr, and appends a
// timestamped record to ErrorLogPath for each failure.
type Dispatcher struct {
	Runner       Runner
	Concurrency  int
	ProgressOut  io.Writer
	ErrorLogPath string

	// HeartbeatEvery is the interval between "still running" lines while
	// no task has completed yet (and between completions). Real extractor
	// dispatches take minutes each, so without a heartbeat the first
	// output could be 10+ minutes after the "resyncing N sidecars" banner
	// — indistinguishable from a hang. Defaults to 30s; tests override.
	HeartbeatEvery time.Duration
}

// Failure links a Task to its Runner error.
type Failure struct {
	Task Task
	Err  error
}

// Result aggregates a Phase A run.
type Result struct {
	TaskCount int
	Failures  []Failure
	Wall      time.Duration
}

// Run dispatches every task and waits for all to complete. Returns when the
// last subagent finishes; never aborts mid-run (continue-on-error).
func (d *Dispatcher) Run(ctx context.Context, tasks []Task) Result {
	if d.Concurrency <= 0 {
		d.Concurrency = 8
	}
	if d.ProgressOut == nil {
		d.ProgressOut = os.Stderr
	}
	res := Result{TaskCount: len(tasks)}
	if len(tasks) == 0 {
		return res
	}

	start := time.Now()
	fmt.Fprintf(d.ProgressOut, "Phase A — resyncing %d stale sidecar(s) at concurrency=%d\n", len(tasks), d.Concurrency)

	sem := make(chan struct{}, d.Concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var durations []time.Duration
	completed := 0

	heartbeat := d.HeartbeatEvery
	if heartbeat <= 0 {
		heartbeat = 30 * time.Second
	}
	done := make(chan struct{})
	go func() {
		tick := time.NewTicker(heartbeat)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return
			case <-tick.C:
				mu.Lock()
				c := completed
				mu.Unlock()
				inFlight := len(tasks) - c
				if inFlight > d.Concurrency {
					inFlight = d.Concurrency
				}
				fmt.Fprintf(d.ProgressOut, "  … still running: %d/%d done, %d in flight, elapsed %s\n",
					c, len(tasks), inFlight, time.Since(start).Round(time.Second))
			}
		}
	}()
	defer close(done)

	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}
		go func(t Task) {
			defer wg.Done()
			defer func() { <-sem }()

			started := time.Now()
			err := d.Runner.Resync(ctx, t)
			took := time.Since(started)

			mu.Lock()
			defer mu.Unlock()
			completed++
			durations = append(durations, took)
			eta := projectETA(durations, len(tasks)-completed, d.Concurrency)
			status := "ok"
			if err != nil {
				status = "FAIL"
				res.Failures = append(res.Failures, Failure{Task: t, Err: err})
			}
			fmt.Fprintf(d.ProgressOut, "  [%d/%d] %s %s in %s (eta %s)\n",
				completed, len(tasks), t.ArtifactID, status,
				took.Round(time.Second), eta.Round(time.Second))
		}(t)
	}
	wg.Wait()
	res.Wall = time.Since(start)

	if len(res.Failures) > 0 {
		d.writeErrorLog(res.Failures)
	}
	return res
}

func (d *Dispatcher) writeErrorLog(failures []Failure) {
	if d.ErrorLogPath == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(d.ErrorLogPath), 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(d.ErrorLogPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	ts := time.Now().UTC().Format(time.RFC3339)
	for _, fail := range failures {
		// scrub credential-shaped substrings before
		// writing to the long-lived error log. The Phase A subagent's
		// captured output may contain attacker-influenceable content (the
		// claude session might surface a token from an env var, a leaked
		// API key, etc.); redact before persisting.
		errMsg := redact.Scrub(oneLine(fail.Err.Error()))
		fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n",
			ts, fail.Task.ArtifactType, fail.Task.ArtifactID, fail.Task.SidecarPath, errMsg)
	}
}

func projectETA(durations []time.Duration, remaining, concurrency int) time.Duration {
	if remaining <= 0 || len(durations) == 0 {
		return 0
	}
	sorted := append([]time.Duration(nil), durations...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	p50 := sorted[len(sorted)/2]
	if concurrency <= 0 {
		concurrency = 1
	}
	return p50 * time.Duration(remaining) / time.Duration(concurrency)
}

func oneLine(s string) string {
	out := make([]byte, 0, len(s))
	for _, b := range []byte(s) {
		if b == '\n' || b == '\r' {
			out = append(out, ' ')
			continue
		}
		out = append(out, b)
	}
	return string(out)
}
