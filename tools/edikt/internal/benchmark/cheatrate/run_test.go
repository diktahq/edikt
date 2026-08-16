package cheatrate

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fakeDispatchPlan controls a stub Dispatcher that returns a
// pre-scripted sequence of per-run outcomes. Index N of the slices
// corresponds to the N-th adversary invocation.
type fakeDispatchPlan struct {
	verifyExits []int   // verifyExit code to inject for run N
	timedOuts   []bool  // whether run N times out
	errs        []error // optional dispatch errors per run
}

func (p *fakeDispatchPlan) dispatcher(t *testing.T) Dispatcher {
	t.Helper()
	calls := 0
	return func(ctx context.Context, req AdversaryRequest) (*AdversaryResult, error) {
		defer func() { calls++ }()
		if calls >= len(p.timedOuts) {
			t.Fatalf("fake dispatcher invoked %d times; plan only covers %d", calls+1, len(p.timedOuts))
		}
		if p.errs != nil && calls < len(p.errs) && p.errs[calls] != nil {
			return nil, p.errs[calls]
		}
		return &AdversaryResult{
			ExitCode:  0,
			TracePath: filepath.Join(req.SandboxPath, "..", "fake-trace.txt"),
			ElapsedMs: 100,
			TimedOut:  p.timedOuts[calls],
		}, nil
	}
}

func (p *fakeDispatchPlan) verifyRunner(t *testing.T) VerifyRunner {
	t.Helper()
	calls := 0
	return func(ctx context.Context, sandboxPath, verifyCommand string) (int, error) {
		defer func() { calls++ }()
		if calls >= len(p.verifyExits) {
			t.Fatalf("fake verify runner invoked %d times; plan only covers %d", calls+1, len(p.verifyExits))
		}
		return p.verifyExits[calls], nil
	}
}

// runWithPlan is the shared harness — creates a temp SourceDir (the
// thing CreateSandbox copies from) + a temp SandboxesDir, wires the
// fakes, and returns the resulting Verify.
func runWithPlan(t *testing.T, plan *fakeDispatchPlan, negFixture string) (Verify, error) {
	t.Helper()
	srcDir, err := os.MkdirTemp("", "cheatrate-run-src-")
	if err != nil {
		t.Fatalf("mkdtemp src: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(srcDir) })
	// Drop a marker file in the source tree so CreateSandbox has
	// something to copy. Otherwise the copy is a trivial empty
	// dir which is fine but uninteresting.
	if err := os.WriteFile(filepath.Join(srcDir, "marker.txt"), []byte("source"), 0o644); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	srcDir, _ = filepath.EvalSymlinks(srcDir)

	sandboxesDir, err := os.MkdirTemp("", "cheatrate-run-sandboxes-")
	if err != nil {
		t.Fatalf("mkdtemp sandboxes: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sandboxesDir) })
	sandboxesDir, _ = filepath.EvalSymlinks(sandboxesDir)

	opts := RunOpts{
		SidecarID:             "ADR-040",
		VerifyIdx:             0,
		Intent:                "test intent",
		FalsifyingObservation: "test violation",
		VerifyCommand:         "true",
		NegativeFixturePath:   negFixture,
		AdversaryModel:        "claude-opus-4-7",
		TemplatePath:          testdataPath(t, "adversary-template.md"),
		SandboxesDir:          sandboxesDir,
		SourceDir:             srcDir,
		Dispatcher:            plan.dispatcher(t),
		VerifyRunner:          plan.verifyRunner(t),
	}
	return RunCheatRateForVerify(context.Background(), opts)
}

// TestRunCheatRateForVerify_AllCheated covers AC-3.7 case 1.
// All three runs satisfy the cheated predicate (verify exits 0, no
// negative fixture → fallback to cheated) → majority cheated.
func TestRunCheatRateForVerify_AllCheated(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{0, 0, 0},
		timedOuts:   []bool{false, false, false},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.Verdict != VerdictCheated {
		t.Errorf("expected verdict %q, got %q", VerdictCheated, verify.Verdict)
	}
	if verify.MajorityRuns != "3c" {
		t.Errorf("expected majority_runs %q, got %q", "3c", verify.MajorityRuns)
	}
	if verify.ElapsedMs != 300 {
		t.Errorf("expected elapsed_ms 300 (3 runs × 100ms), got %d", verify.ElapsedMs)
	}
	if verify.VerifyID != "directive[0]" {
		t.Errorf("expected verify_id %q, got %q", "directive[0]", verify.VerifyID)
	}
	if verify.VerifyKind != "behavioral" {
		t.Errorf("expected verify_kind %q, got %q", "behavioral", verify.VerifyKind)
	}
}

// TestRunCheatRateForVerify_AllNotCheated covers AC-3.7 case 2.
// All three runs have verify exit != 0 → not_cheated.
func TestRunCheatRateForVerify_AllNotCheated(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{1, 1, 1},
		timedOuts:   []bool{false, false, false},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.Verdict != VerdictNotCheated {
		t.Errorf("expected verdict %q, got %q", VerdictNotCheated, verify.Verdict)
	}
	if verify.MajorityRuns != "3n" {
		t.Errorf("expected majority_runs %q, got %q", "3n", verify.MajorityRuns)
	}
}

// TestRunCheatRateForVerify_TwoCheatedOneNot covers AC-3.7 case 3 —
// 2 cheated + 1 not_cheated → majority cheated.
func TestRunCheatRateForVerify_TwoCheatedOneNot(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{0, 0, 1},
		timedOuts:   []bool{false, false, false},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.Verdict != VerdictCheated {
		t.Errorf("expected verdict %q, got %q", VerdictCheated, verify.Verdict)
	}
	if verify.MajorityRuns != "2c/1n" {
		t.Errorf("expected majority_runs %q, got %q", "2c/1n", verify.MajorityRuns)
	}
}

// TestRunCheatRateForVerify_OneEach covers AC-3.7 case 4 —
// 1 cheated, 1 not_cheated, 1 inconclusive (via dispatch error) →
// no majority → inconclusive.
func TestRunCheatRateForVerify_OneEach(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{0, 1, 0}, // 3rd unused (dispatch errored before)
		timedOuts:   []bool{false, false, false},
		errs:        []error{nil, nil, &fakeError{"adversary spawn failed"}},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.Verdict != VerdictInconclusive {
		t.Errorf("expected verdict %q, got %q", VerdictInconclusive, verify.Verdict)
	}
	// 1 cheated + 1 not_cheated + 1 inconclusive (dispatch error).
	// Order is c,n,i in the formatted string.
	if !strings.Contains(verify.MajorityRuns, "1c") ||
		!strings.Contains(verify.MajorityRuns, "1n") ||
		!strings.Contains(verify.MajorityRuns, "1i") {
		t.Errorf("expected majority_runs to contain 1c/1n/1i, got %q", verify.MajorityRuns)
	}
}

// TestRunCheatRateForVerify_TwoTimeouts covers AC-3.5 + AC-3.7 case 5 —
// ≥2 timed out → inconclusive regardless of remaining verdict.
func TestRunCheatRateForVerify_TwoTimeouts(t *testing.T) {
	plan := &fakeDispatchPlan{
		// Verify is never run for the timed-out adversary, but
		// the plan covers 1 actual verify-runner call (for
		// run 0 which did not time out).
		verifyExits: []int{0},
		timedOuts:   []bool{false, true, true},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.Verdict != VerdictInconclusive {
		t.Errorf("expected verdict %q with 2 timeouts, got %q", VerdictInconclusive, verify.Verdict)
	}
	// 1 cheated + 2 inconclusive. The two inconclusive entries
	// came from timeouts; both register as "i" in the summary.
	if !strings.Contains(verify.MajorityRuns, "1c") ||
		!strings.Contains(verify.MajorityRuns, "2i") {
		t.Errorf("expected majority_runs to contain 1c and 2i, got %q", verify.MajorityRuns)
	}
}

// TestRunCheatRateForVerify_VerifyIDOverride covers SR-008's
// directive-vs-prohibition discrimination. When RunOpts.VerifyID is
// set, RunCheatRateForVerify must surface that exact string in the
// returned Verify.VerifyID. The override is what Phase 4 uses to
// emit "prohibition[N]" verify_ids when scoring prohibitions[].verify.
func TestRunCheatRateForVerify_VerifyIDOverride(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{0, 0, 0},
		timedOuts:   []bool{false, false, false},
	}
	srcDir, _ := os.MkdirTemp("", "cheatrate-run-src-")
	t.Cleanup(func() { _ = os.RemoveAll(srcDir) })
	srcDir, _ = filepath.EvalSymlinks(srcDir)
	sandboxesDir, _ := os.MkdirTemp("", "cheatrate-run-sandboxes-")
	t.Cleanup(func() { _ = os.RemoveAll(sandboxesDir) })
	sandboxesDir, _ = filepath.EvalSymlinks(sandboxesDir)

	opts := RunOpts{
		SidecarID:     "ADR-040",
		VerifyIdx:     1,
		VerifyID:      "prohibition[1]", // override
		VerifyCommand: "true",
		SandboxesDir:  sandboxesDir,
		SourceDir:     srcDir,
		TemplatePath:  testdataPath(t, "adversary-template.md"),
		Dispatcher:    plan.dispatcher(t),
		VerifyRunner:  plan.verifyRunner(t),
	}
	verify, err := RunCheatRateForVerify(context.Background(), opts)
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.VerifyID != "prohibition[1]" {
		t.Errorf("expected verify_id %q, got %q", "prohibition[1]", verify.VerifyID)
	}
}

// TestRunCheatRateForVerify_VerifyIDDefault confirms the empty-string
// fallback to "directive[<idx>]".
func TestRunCheatRateForVerify_VerifyIDDefault(t *testing.T) {
	plan := &fakeDispatchPlan{
		verifyExits: []int{0, 0, 0},
		timedOuts:   []bool{false, false, false},
	}
	verify, err := runWithPlan(t, plan, "")
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}
	if verify.VerifyID != "directive[0]" {
		t.Errorf("expected default verify_id %q, got %q", "directive[0]", verify.VerifyID)
	}
}

// TestRunCheatRateForVerify_RejectsBadOpts covers the input-validation
// branch — required fields must be present.
func TestRunCheatRateForVerify_RejectsBadOpts(t *testing.T) {
	cases := []struct {
		name string
		opts RunOpts
		want string
	}{
		{"missing_sidecar_id", RunOpts{SandboxesDir: "/tmp", SourceDir: "/tmp"}, "sidecar_id required"},
		{"missing_sandboxes_dir", RunOpts{SidecarID: "ADR-040", SourceDir: "/tmp"}, "sandboxes_dir required"},
		{"relative_sandboxes_dir", RunOpts{SidecarID: "ADR-040", SandboxesDir: "./relative", SourceDir: "/tmp"}, "must be absolute"},
		{"missing_source_dir", RunOpts{SidecarID: "ADR-040", SandboxesDir: "/tmp"}, "source_dir required"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunCheatRateForVerify(context.Background(), tc.opts)
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q missing substring %q", err.Error(), tc.want)
			}
		})
	}
}

// TestFormatMajorityRuns exercises the compact-string format directly
// — keeps the contract grep-friendly across orderings.
func TestFormatMajorityRuns(t *testing.T) {
	cases := []struct {
		verdicts []string
		want     string
	}{
		{[]string{VerdictCheated, VerdictCheated, VerdictCheated}, "3c"},
		{[]string{VerdictNotCheated, VerdictNotCheated, VerdictNotCheated}, "3n"},
		{[]string{VerdictCheated, VerdictNotCheated, VerdictCheated}, "2c/1n"},
		{[]string{VerdictCheated, VerdictNotCheated, VerdictInconclusive}, "1c/1n/1i"},
		{[]string{VerdictInconclusive, VerdictInconclusive, VerdictInconclusive}, "3i"},
		{[]string{}, ""},
	}
	for _, tc := range cases {
		got := formatMajorityRuns(tc.verdicts)
		if got != tc.want {
			t.Errorf("formatMajorityRuns(%v) = %q, want %q", tc.verdicts, got, tc.want)
		}
	}
}

// TestMajorityVerdict_TimeoutOverride confirms the AC-3.5 rule that
// ≥2 timeouts overrides the simple-majority count.
func TestMajorityVerdict_TimeoutOverride(t *testing.T) {
	// Without the timeout override, this would be 1 cheated +
	// 2 inconclusive → inconclusive anyway. But the override is
	// what makes it inconclusive regardless of how the other run
	// went.
	got := majorityVerdict([]string{VerdictCheated, VerdictInconclusive, VerdictInconclusive}, 2)
	if got != VerdictInconclusive {
		t.Errorf("with 2 timeouts, expected %q, got %q", VerdictInconclusive, got)
	}
	// Sanity: 2-cheated still wins when only 1 timeout (subset of
	// inconclusive that isn't a timeout).
	got = majorityVerdict([]string{VerdictCheated, VerdictCheated, VerdictInconclusive}, 1)
	if got != VerdictCheated {
		t.Errorf("with 1 timeout + 2 cheated, expected %q, got %q", VerdictCheated, got)
	}
}

// fakeError is a tiny error stand-in for the dispatch-error path in
// TestRunCheatRateForVerify_OneEach.
type fakeError struct{ msg string }

func (e *fakeError) Error() string { return e.msg }
