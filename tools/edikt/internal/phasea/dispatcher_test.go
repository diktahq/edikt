package phasea

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// fakeRunner records each Resync call and returns whatever the caller
// configured for that ArtifactID. It also tracks max concurrent calls so
// tests can assert the dispatcher honours the semaphore.
type fakeRunner struct {
	results map[string]error
	delay   time.Duration

	active    atomic.Int32
	maxActive atomic.Int32
	calls     atomic.Int32
}

// Preflight always succeeds — the fake has no external dependency to probe.
func (r *fakeRunner) Preflight() error { return nil }

func (r *fakeRunner) Resync(_ context.Context, t Task) error {
	cur := r.active.Add(1)
	for {
		old := r.maxActive.Load()
		if cur <= old || r.maxActive.CompareAndSwap(old, cur) {
			break
		}
	}
	defer r.active.Add(-1)
	r.calls.Add(1)
	if r.delay > 0 {
		time.Sleep(r.delay)
	}
	return r.results[t.ArtifactID]
}

func TestDispatcher_ContinueOnErrorAndAggregatesFailures(t *testing.T) {
	tmp := t.TempDir()
	logPath := filepath.Join(tmp, ".edikt", "state", "compile-errors.log")
	r := &fakeRunner{results: map[string]error{
		"ADR-001": nil,
		"ADR-002": errors.New("boom"),
		"ADR-003": nil,
		"ADR-004": errors.New("kapow"),
	}}
	d := &Dispatcher{
		Runner:       r,
		Concurrency:  2,
		ProgressOut:  &bytes.Buffer{},
		ErrorLogPath: logPath,
	}
	tasks := []Task{
		{ArtifactType: "adr", ArtifactID: "ADR-001"},
		{ArtifactType: "adr", ArtifactID: "ADR-002"},
		{ArtifactType: "adr", ArtifactID: "ADR-003"},
		{ArtifactType: "adr", ArtifactID: "ADR-004"},
	}
	res := d.Run(context.Background(), tasks)

	if got := r.calls.Load(); got != 4 {
		t.Errorf("Runner.Resync called %d times, want 4", got)
	}
	if len(res.Failures) != 2 {
		t.Errorf("expected 2 failures, got %d", len(res.Failures))
	}
	if max := r.maxActive.Load(); max > 2 {
		t.Errorf("concurrency cap violated: max active %d > 2", max)
	}
}

func TestDispatcher_NoTasksIsZero(t *testing.T) {
	r := &fakeRunner{}
	d := &Dispatcher{Runner: r, Concurrency: 4, ProgressOut: &bytes.Buffer{}}
	res := d.Run(context.Background(), nil)
	if res.TaskCount != 0 || len(res.Failures) != 0 {
		t.Errorf("zero-task run: got %+v", res)
	}
}

// slowRunner blocks each Resync long enough for heartbeats to fire.
type slowRunner struct{ d time.Duration }

func (r *slowRunner) Preflight() error { return nil }
func (r *slowRunner) Resync(_ context.Context, _ Task) error {
	time.Sleep(r.d)
	return nil
}

// Field bug (bok-services): a 24-task serial-feeling Phase A ran >10 minutes
// with no output between the banner and the first completion. The dispatcher
// must emit periodic heartbeat lines while tasks are in flight.
func TestDispatcher_HeartbeatWhileTasksInFlight(t *testing.T) {
	var buf bytes.Buffer
	d := &Dispatcher{
		Runner:         &slowRunner{d: 120 * time.Millisecond},
		Concurrency:    2,
		ProgressOut:    &buf,
		HeartbeatEvery: 20 * time.Millisecond,
	}
	d.Run(context.Background(), []Task{
		{ArtifactType: "adr", ArtifactID: "ADR-001", ParentPath: "a.md", SidecarPath: "a.yaml"},
	})
	if !strings.Contains(buf.String(), "still running") {
		t.Errorf("expected heartbeat lines during a slow run, got:\n%s", buf.String())
	}
}
