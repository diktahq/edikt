package hook

// taskcreated_test.go — direct coverage for task-created.sh's PLAN_FILE
// resolution and in-progress-phase extraction, which test/fixtures/
// hook-corpus/task-created/matrix.yaml deliberately does not exercise:
// every recorded scenario takes the "no plan directory reachable" path
// (phase/plan both resolve null) — see that matrix's own header comment.
// This file seeds a REAL PLAN-*.md, the same technique
// userpromptsubmit_test.go's own end-to-end test uses for the analogous gap
// in user-prompt-submit.sh.

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hook/adapter"
)

// TestTaskCreatedLogsPhaseAndPlanFromRealPlanFile runs the full handler —
// config, plan-directory resolution, PLAN-*.md selection, in-progress-phase
// extraction — proving the detection logic reaches the logged event, not
// just that it is correct in isolation.
func TestTaskCreatedLogsPhaseAndPlanFromRealPlanFile(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")
	writeTestFile(t, filepath.Join(cwd, "docs", "plans", "PLAN-001-test.md"),
		"# Plan: Test Plan\n\n## Progress\n\n"+
			"| Phase | Theme | Status | Updated |\n"+
			"|-------|-------|--------|---------|\n"+
			"| 1 | Setup | done | 2026-03-20 |\n"+
			"| 2 | Build | in progress | 2026-03-20 |\n"+
			"| 3 | Ship | not started | — |\n")

	inv := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{TaskID: "t-1", TaskName: "Do X"}}
	res := taskCreated(inv)

	if res.Response.Text != `{"continue": true}` {
		t.Fatalf("stdout = %q, want the literal continue-true reply", res.Response.Text)
	}

	events := readEventsJSONL(t, filepath.Join(home, ".edikt", "events.jsonl"))
	if len(events) != 1 {
		t.Fatalf("events.jsonl has %d lines, want 1: %v", len(events), events)
	}
	var e struct {
		Event    string  `json:"event"`
		TaskID   string  `json:"task_id"`
		TaskName string  `json:"task_name"`
		Phase    *string `json:"phase"`
		Plan     *string `json:"plan"`
	}
	if err := json.Unmarshal([]byte(events[0]), &e); err != nil {
		t.Fatalf("unmarshal logged event: %v (line: %s)", err, events[0])
	}
	if e.Phase == nil || *e.Phase != "2" {
		t.Errorf("phase = %v, want \"2\"", e.Phase)
	}
	if e.Plan == nil || *e.Plan != "PLAN-001-test.md" {
		t.Errorf("plan = %v, want \"PLAN-001-test.md\"", e.Plan)
	}
}

// TestTaskCreatedNoPlanDirectoryLogsNullPhaseAndPlan is the negative
// control: with NO docs/plans/ directory at all (matching the corpus
// sandbox's own shape), phase and plan both resolve null — proving the
// positive test above actually depends on the seeded plan file, not on
// some other unconditional default.
func TestTaskCreatedNoPlanDirectoryLogsNullPhaseAndPlan(t *testing.T) {
	cwd := t.TempDir()
	t.Chdir(cwd)
	home := t.TempDir()
	t.Setenv("HOME", home)

	writeTestFile(t, filepath.Join(cwd, ".edikt", "config.yaml"), "base: docs\n")

	inv := &Invocation{Stderr: io.Discard, Payload: adapter.Payload{TaskID: "t-2", TaskName: "Do Y"}}
	taskCreated(inv)

	events := readEventsJSONL(t, filepath.Join(home, ".edikt", "events.jsonl"))
	if len(events) != 1 {
		t.Fatalf("events.jsonl has %d lines, want 1: %v", len(events), events)
	}
	var e struct {
		Phase *string `json:"phase"`
		Plan  *string `json:"plan"`
	}
	if err := json.Unmarshal([]byte(events[0]), &e); err != nil {
		t.Fatalf("unmarshal logged event: %v", err)
	}
	if e.Phase != nil {
		t.Errorf("phase = %v, want null", *e.Phase)
	}
	if e.Plan != nil {
		t.Errorf("plan = %v, want null", *e.Plan)
	}
}

func readEventsJSONL(t *testing.T, path string) []string {
	t.Helper()
	body, err := os.ReadFile(path) //nolint:gosec // test fixture path
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	start := 0
	for i, b := range body {
		if b == '\n' {
			if i > start {
				lines = append(lines, string(body[start:i]))
			}
			start = i + 1
		}
	}
	return lines
}
