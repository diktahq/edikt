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
)

// Dispatcher fans Tasks out to a Runner with a concurrency cap, captures
// failures continue-on-error per ADR-028 §"Failure semantics", emits
// per-completion progress with running p50 ETA on stderr, and appends a
// timestamped record to ErrorLogPath for each failure.
type Dispatcher struct {
	Runner       Runner
	Concurrency  int
	ProgressOut  io.Writer
	ErrorLogPath string
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
		fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n",
			ts, fail.Task.ArtifactType, fail.Task.ArtifactID, fail.Task.SidecarPath, oneLine(fail.Err.Error()))
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
