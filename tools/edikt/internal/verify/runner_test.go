package verify

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRunCriterion_pass(t *testing.T) {
	p := Phase{ID: "1", Classification: ClassTestable}
	c := Criterion{ID: "1.1", Statement: "exit 0", Verify: "exit 0"}
	r := RunCriterion(p, c, RunOptions{})
	if r.Status != StatusPassed {
		t.Fatalf("status: got %q, want %q", r.Status, StatusPassed)
	}
	if r.ExitCode != 0 {
		t.Fatalf("exit_code: got %d, want 0", r.ExitCode)
	}
	if r.ID != "1.1" {
		t.Fatalf("id: got %q, want 1.1", r.ID)
	}
}

func TestRunCriterion_fail(t *testing.T) {
	p := Phase{ID: "1", Classification: ClassTestable}
	c := Criterion{ID: "1.2", Statement: "exit 7", Verify: "exit 7"}
	r := RunCriterion(p, c, RunOptions{})
	if r.Status != StatusFailed {
		t.Fatalf("status: got %q, want %q", r.Status, StatusFailed)
	}
	if r.ExitCode != 7 {
		t.Fatalf("exit_code: got %d, want 7", r.ExitCode)
	}
}

func TestRunCriterion_timeout(t *testing.T) {
	p := Phase{ID: "1", Classification: ClassTestable}
	c := Criterion{ID: "1.3", Statement: "sleep 60", Verify: "sleep 60"}
	r := RunCriterion(p, c, RunOptions{Timeout: 200 * time.Millisecond})
	if r.Status != StatusTimeout {
		t.Fatalf("status: got %q, want %q", r.Status, StatusTimeout)
	}
}

func TestRunCriterion_excerptTruncation(t *testing.T) {
	p := Phase{ID: "1", Classification: ClassTestable}
	// Print 5 KiB of 'a' characters; ExcerptCap is 4 KiB.
	c := Criterion{
		ID: "1.4", Statement: "large stdout",
		Verify: "head -c 5120 /dev/zero | tr '\\0' a",
	}
	r := RunCriterion(p, c, RunOptions{})
	if r.Status != StatusPassed {
		t.Fatalf("status: got %q, want %q", r.Status, StatusPassed)
	}
	if len(r.StdoutExcerpt) != ExcerptCap {
		t.Fatalf("stdout_excerpt: got %d bytes, want %d", len(r.StdoutExcerpt), ExcerptCap)
	}
}

func TestRunCriterion_classification(t *testing.T) {
	t.Run("informational never executes", func(t *testing.T) {
		p := Phase{ID: "10", Classification: ClassInformational}
		c := Criterion{ID: "10.1", Statement: "info", Verify: "exit 1"}
		r := RunCriterion(p, c, RunOptions{})
		if r.Status != StatusSkippedInformational {
			t.Fatalf("status: got %q, want %q", r.Status, StatusSkippedInformational)
		}
		if r.DurationMS != 0 {
			t.Fatalf("duration_ms: informational skip should not measure time, got %d", r.DurationMS)
		}
	})
	t.Run("operational without verify is skipped", func(t *testing.T) {
		p := Phase{ID: "11", Classification: ClassOperational}
		c := Criterion{ID: "11.1", Statement: "no verify"}
		r := RunCriterion(p, c, RunOptions{})
		if r.Status != StatusSkippedOperational {
			t.Fatalf("status: got %q, want %q", r.Status, StatusSkippedOperational)
		}
	})
	t.Run("operational with verify is executed", func(t *testing.T) {
		p := Phase{ID: "11", Classification: ClassOperational}
		c := Criterion{ID: "11.2", Statement: "verify present", Verify: "exit 0"}
		r := RunCriterion(p, c, RunOptions{})
		if r.Status != StatusPassed {
			t.Fatalf("status: got %q, want %q", r.Status, StatusPassed)
		}
	})
}

func TestRunCriterion_cwdAndEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "marker"), []byte("ok"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	p := Phase{ID: "1", Classification: ClassTestable}
	// Verify both cwd resolution and EDIKT_VERIFY=1 injection.
	c := Criterion{
		ID: "1.x", Statement: "cwd+env",
		Verify: "test -f marker && test \"$EDIKT_VERIFY\" = 1",
	}
	r := RunCriterion(p, c, RunOptions{Cwd: dir})
	if r.Status != StatusPassed {
		t.Fatalf("status: got %q (stderr=%q), want %q", r.Status, r.StderrExcerpt, StatusPassed)
	}
}

func TestReport_jsonShape(t *testing.T) {
	results := []Result{
		{ID: "1.1", Statement: "ok", Status: StatusPassed, DurationMS: 5},
	}
	r := NewReport("demo", "1", "abcdef0", results)
	if r.PlanID != "demo" {
		t.Fatalf("plan_id: %q", r.PlanID)
	}
	if r.Phase != "1" {
		t.Fatalf("phase: %q", r.Phase)
	}
	if r.GitSHA != "abcdef0" {
		t.Fatalf("git_sha: %q", r.GitSHA)
	}
	if r.RanAt == "" {
		t.Fatal("ran_at: empty")
	}
	if r.RanBy == "" {
		t.Fatal("ran_by: empty")
	}

	body, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, key := range []string{
		`"plan_id"`, `"phase"`, `"ran_at"`, `"ran_by"`, `"git_sha"`,
		`"summary"`, `"criteria"`, `"passed"`, `"failed"`, `"skipped"`,
		`"timeout"`, `"total"`,
	} {
		if !bytes.Contains(body, []byte(key)) {
			t.Errorf("expected key %s in JSON", key)
		}
	}
}

func TestReport_summaryCounts(t *testing.T) {
	results := []Result{
		{ID: "1", Status: StatusPassed},
		{ID: "2", Status: StatusPassed},
		{ID: "3", Status: StatusFailed},
		{ID: "4", Status: StatusTimeout},
		{ID: "5", Status: StatusSkippedOperational},
		{ID: "6", Status: StatusSkippedInformational},
	}
	r := NewReport("demo", "all", "sha", results)
	if r.Summary.Passed != 2 {
		t.Errorf("passed: got %d, want 2", r.Summary.Passed)
	}
	if r.Summary.Failed != 1 {
		t.Errorf("failed: got %d, want 1", r.Summary.Failed)
	}
	if r.Summary.Timeout != 1 {
		t.Errorf("timeout: got %d, want 1", r.Summary.Timeout)
	}
	if r.Summary.Skipped != 2 {
		t.Errorf("skipped: got %d, want 2", r.Summary.Skipped)
	}
	if r.Summary.Total != 6 {
		t.Errorf("total: got %d, want 6", r.Summary.Total)
	}
	if !r.AnyFailures() {
		t.Error("AnyFailures: got false, want true")
	}
}

func TestReport_writeFiles(t *testing.T) {
	dir := t.TempDir()
	results := []Result{{ID: "1", Status: StatusPassed}}
	r := NewReport("demo", "1", "sha", results)
	jsonPath, err := WriteReports(dir, r)
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if !strings.HasPrefix(filepath.Base(jsonPath), "demo-phase-1-") {
		t.Errorf("json filename: %s", filepath.Base(jsonPath))
	}
	if !strings.HasSuffix(jsonPath, ".json") {
		t.Errorf("json extension missing: %s", jsonPath)
	}
	txtPath := strings.TrimSuffix(jsonPath, ".json") + ".txt"
	if _, err := os.Stat(txtPath); err != nil {
		t.Errorf("text report missing: %v", err)
	}
}
