package gov

// benchmark_cheatrate_production_test.go — Phase 4 (Plan E) tests for
// the production dispatch path of `bin/edikt gov benchmark cheat-rate`.
//
// These run in-process so they can inject dispatcherForTests +
// verifyRunnerForTests + nowForTests and avoid spawning real `claude`.
// The existing benchmark_cheatrate_test.go covers exit-code branches
// via the built binary subprocess; this file covers the verdict-loop
// + cache + inconclusive-gate paths that the binary subprocess can't
// observe.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/benchmark/cheatrate"
	"github.com/spf13/cobra"
)

// productionTestEnv groups the per-test temp dirs + the cobra Command
// used to invoke runCheatRateProduction. The struct exists so the
// individual tests stay short.
type productionTestEnv struct {
	projectRoot string
	cmd         *cobra.Command
	out         *bytes.Buffer
}

// setupProductionEnv creates a temp project root containing one
// behavioral-directive sidecar plus the cheat-rate-adversary
// template, changes the test's cwd to it (with cleanup), and returns
// a cobra Command wired with a stdout buffer.
func setupProductionEnv(t *testing.T, sidecars map[string]string) *productionTestEnv {
	t.Helper()
	root, err := os.MkdirTemp("", "cheatrate-production-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	root, _ = filepath.EvalSymlinks(root)

	// Adversary template — small interpolatable stub.
	tmplDir := filepath.Join(root, "templates", "agents")
	if err := os.MkdirAll(tmplDir, 0o755); err != nil {
		t.Fatalf("mkdir tmplDir: %v", err)
	}
	tmplBody := "INTENT: {{DIRECTIVE_INTENT}}\nSANDBOX: {{SANDBOX_PATH}}\n"
	if err := os.WriteFile(filepath.Join(tmplDir, "cheat-rate-adversary.md"), []byte(tmplBody), 0o644); err != nil {
		t.Fatalf("write template: %v", err)
	}

	// .edikt/config.yaml pointing at the standard governance dirs.
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	cfg := "paths:\n  decisions: docs/architecture/decisions\n  invariants: docs/architecture/invariants\n  guidelines: docs/architecture/guidelines\n"
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	// Each sidecar in `sidecars` is keyed by ID ("ADR-001") with its
	// YAML body as the value. We also drop a stub parent .md so
	// sidecar.Discover walks the directory and picks it up.
	decDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(decDir, 0o755); err != nil {
		t.Fatalf("mkdir decisions: %v", err)
	}
	for id, body := range sidecars {
		parentPath := filepath.Join(decDir, id+"-test.md")
		if err := os.WriteFile(parentPath, []byte("# "+id+"\n\nStub parent for tests.\n"), 0o644); err != nil {
			t.Fatalf("write parent %s: %v", id, err)
		}
		sidecarPath := filepath.Join(decDir, id+"-test.edikt.yaml")
		if err := os.WriteFile(sidecarPath, []byte(body), 0o644); err != nil {
			t.Fatalf("write sidecar %s: %v", id, err)
		}
	}

	// Chdir so runCheatRateProduction picks up `root` via os.Getwd().
	// t.Chdir restores the previous cwd automatically on cleanup.
	t.Chdir(root)

	cobraCmd := &cobra.Command{Use: "cheat-rate"}
	out := &bytes.Buffer{}
	cobraCmd.SetOut(out)
	cobraCmd.SetErr(out)
	return &productionTestEnv{projectRoot: root, cmd: cobraCmd, out: out}
}

// behavioralSidecar returns a minimal v1.2 sidecar YAML body with one
// behavioral directive carrying the given verify command. The
// directive declares a negative_fixture_path that the verdict layer
// will fall through (no fixture script, → no_fixture path) so the
// cheated determination collapses to the verify exit code.
func behavioralSidecar(verifyCmd string) string {
	return `schema_version: 1
topic: test
path: docs/architecture/decisions/ADR-001-test.edikt.yaml
signals: ["test"]
paths: ["**/*.go"]
scope: ["design"]
directives:
  - text: "Test directive must hold."
    source_excerpt:
      line_start: 1
      line_end: 1
      quote: "test"
    verify: ` + fmt.Sprintf("%q", verifyCmd) + `
    verify_kind: behavioral
    intent: "Test directive holds in the post-adversary sandbox."
    falsifying_observation: "Violation observed when X."
    human_approved_at: "2026-05-23T14:00:00Z"
`
}

// installFakes wires dispatcherForTests + verifyRunnerForTests to
// deterministic stubs, registering cleanup to nil them out so other
// tests aren't affected.
func installFakes(t *testing.T, dispatch cheatrate.Dispatcher, verify cheatrate.VerifyRunner) {
	t.Helper()
	dispatcherForTests = dispatch
	verifyRunnerForTests = verify
	t.Cleanup(func() {
		dispatcherForTests = nil
		verifyRunnerForTests = nil
	})
}

// alwaysSucceedDispatcher returns a Dispatcher that always returns a
// clean adversary result (ExitCode 0, no timeout).
func alwaysSucceedDispatcher(counter *int64) cheatrate.Dispatcher {
	return func(ctx context.Context, req cheatrate.AdversaryRequest) (*cheatrate.AdversaryResult, error) {
		atomic.AddInt64(counter, 1)
		return &cheatrate.AdversaryResult{
			ExitCode:  0,
			TracePath: filepath.Join(req.SandboxPath, "..", "fake-trace.txt"),
			ElapsedMs: 1,
			TimedOut:  false,
		}, nil
	}
}

// alwaysTimeoutDispatcher returns a Dispatcher that always reports
// TimedOut: true — drives the inconclusive-rate gate test.
func alwaysTimeoutDispatcher() cheatrate.Dispatcher {
	return func(ctx context.Context, req cheatrate.AdversaryRequest) (*cheatrate.AdversaryResult, error) {
		return &cheatrate.AdversaryResult{
			ExitCode:  0,
			TracePath: "",
			ElapsedMs: 1,
			TimedOut:  true,
		}, nil
	}
}

// constantVerifyRunner returns a VerifyRunner that always returns the
// given exit code with no error.
func constantVerifyRunner(exit int) cheatrate.VerifyRunner {
	return func(ctx context.Context, sandboxPath, verifyCommand string) (int, error) {
		return exit, nil
	}
}

// TestProductionDispatch_SidecarNotFound covers AC-4.1 — a positional
// sidecar id that doesn't match any discovered sidecar returns exit 2.
func TestProductionDispatch_SidecarNotFound(t *testing.T) {
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
	})
	cheatRateAll = false
	cheatRateRefresh = false
	cheatRateAdversaryModel = "claude-opus-4-7"

	err := runCheatRateProduction(env.cmd, []string{"ADR-999"})
	if err == nil {
		t.Fatalf("expected error, got nil; out=%q", env.out.String())
	}
	exitE, ok := err.(*exitErr)
	if !ok {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if exitE.code != 2 {
		t.Errorf("expected exit code 2, got %d (msg: %s)", exitE.code, exitE.msg)
	}
	if !strings.Contains(exitE.msg, "ADR-999") {
		t.Errorf("error should name the missing sidecar; got %q", exitE.msg)
	}
}

// TestProductionDispatch_CacheHit covers AC-4.3 — a second invocation
// against the same sidecar reuses the cached verdict without
// dispatching the adversary. The dispatcher counter increments only
// on the first run.
func TestProductionDispatch_CacheHit(t *testing.T) {
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
	})
	cheatRateAll = false
	cheatRateRefresh = false
	cheatRateAdversaryModel = "claude-opus-4-7"

	var dispatchCalls int64
	installFakes(t, alwaysSucceedDispatcher(&dispatchCalls), constantVerifyRunner(0))

	// First run — dispatcher runs 3 times (3-run majority).
	if err := runCheatRateProduction(env.cmd, []string{"ADR-001"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	firstCalls := atomic.LoadInt64(&dispatchCalls)
	if firstCalls != 3 {
		t.Errorf("first run: expected 3 dispatcher calls, got %d", firstCalls)
	}

	// Second run — same args, cache should hit and skip dispatch.
	env.out.Reset()
	if err := runCheatRateProduction(env.cmd, []string{"ADR-001"}); err != nil {
		t.Fatalf("second run: %v", err)
	}
	secondCalls := atomic.LoadInt64(&dispatchCalls) - firstCalls
	if secondCalls != 0 {
		t.Errorf("second run: expected 0 additional dispatcher calls (cache hit), got %d", secondCalls)
	}
}

// TestProductionDispatch_RefreshBypassesCache covers the --refresh
// flag — even with a warm cache, the second run dispatches afresh.
func TestProductionDispatch_RefreshBypassesCache(t *testing.T) {
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
	})
	cheatRateAll = false
	cheatRateRefresh = false
	cheatRateAdversaryModel = "claude-opus-4-7"

	var dispatchCalls int64
	installFakes(t, alwaysSucceedDispatcher(&dispatchCalls), constantVerifyRunner(0))

	// Warm the cache.
	if err := runCheatRateProduction(env.cmd, []string{"ADR-001"}); err != nil {
		t.Fatalf("first run: %v", err)
	}
	atomic.StoreInt64(&dispatchCalls, 0)
	env.out.Reset()

	// --refresh forces re-dispatch.
	cheatRateRefresh = true
	t.Cleanup(func() { cheatRateRefresh = false })
	if err := runCheatRateProduction(env.cmd, []string{"ADR-001"}); err != nil {
		t.Fatalf("refresh run: %v", err)
	}
	calls := atomic.LoadInt64(&dispatchCalls)
	if calls != 3 {
		t.Errorf("--refresh: expected 3 dispatcher calls, got %d", calls)
	}
}

// TestProductionDispatch_InconclusiveRateGate covers AC-4.6 — an
// --all run with all verifies inconclusive (every adversary times
// out) triggers exit 1 with the threshold message.
func TestProductionDispatch_InconclusiveRateGate(t *testing.T) {
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
		"ADR-002": behavioralSidecar("true"),
	})
	cheatRateAll = true
	cheatRateRefresh = false
	cheatRateAdversaryModel = "claude-opus-4-7"
	t.Cleanup(func() { cheatRateAll = false })

	installFakes(t, alwaysTimeoutDispatcher(), constantVerifyRunner(0))

	err := runCheatRateProduction(env.cmd, nil)
	if err == nil {
		t.Fatalf("expected exit 1 from inconclusive gate; out=%q", env.out.String())
	}
	exitE, ok := err.(*exitErr)
	if !ok {
		t.Fatalf("expected *exitErr, got %T: %v", err, err)
	}
	if exitE.code != 1 {
		t.Errorf("expected exit code 1, got %d (msg: %s)", exitE.code, exitE.msg)
	}
	if !strings.Contains(exitE.msg, "inconclusive_rate") {
		t.Errorf("error should mention inconclusive_rate; got %q", exitE.msg)
	}
}

// TestProductionDispatch_WritesReport confirms AC-4.5 — a clean run
// emits a JSON report file under .edikt/state/benchmark/, schema 1.
func TestProductionDispatch_WritesReport(t *testing.T) {
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
	})
	cheatRateAll = false
	cheatRateRefresh = false
	cheatRateAdversaryModel = "claude-opus-4-7"

	var dispatchCalls int64
	installFakes(t, alwaysSucceedDispatcher(&dispatchCalls), constantVerifyRunner(0))
	// Freeze the clock so the ran_at timestamp is deterministic.
	nowForTests = func() time.Time { return time.Unix(1717000000, 0).UTC() }
	t.Cleanup(func() { nowForTests = nil })

	if err := runCheatRateProduction(env.cmd, []string{"ADR-001"}); err != nil {
		t.Fatalf("runCheatRateProduction: %v", err)
	}

	// Path shape from cheatrate.ReportPath:
	//   <stateDir>/benchmark/<sidecarID>/<timestamp>.json
	sidecarReportDir := filepath.Join(env.projectRoot, ".edikt", "state", "benchmark", "ADR-001")
	entries, err := os.ReadDir(sidecarReportDir)
	if err != nil {
		t.Fatalf("read sidecarReportDir: %v", err)
	}
	var reportPath string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			reportPath = filepath.Join(sidecarReportDir, e.Name())
			break
		}
	}
	if reportPath == "" {
		t.Fatalf("no .json report under %s; entries=%v", sidecarReportDir, entries)
	}

	raw, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("read report: %v", err)
	}
	var report cheatrate.Report
	if err := json.Unmarshal(raw, &report); err != nil {
		t.Fatalf("parse report: %v", err)
	}
	if report.SchemaVersion != 1 {
		t.Errorf("expected schema_version 1, got %d", report.SchemaVersion)
	}
	if report.SidecarID != "ADR-001" {
		t.Errorf("expected sidecar_id ADR-001, got %s", report.SidecarID)
	}
	if len(report.Verifies) != 1 {
		t.Errorf("expected 1 verify, got %d", len(report.Verifies))
	}
	if len(report.Verifies) > 0 && report.Verifies[0].VerifyID != "directive[0]" {
		t.Errorf("expected verify_id 'directive[0]', got %q", report.Verifies[0].VerifyID)
	}
}

// TestProductionDispatch_StubModeShortCircuit confirms AC-4.7 — when
// EDIKT_CHEAT_RATE_STUB=1 is set, the production path is never
// reached. The fake dispatcher's call counter stays at zero.
func TestProductionDispatch_StubModeShortCircuit(t *testing.T) {
	t.Setenv("EDIKT_CHEAT_RATE_STUB", "1")
	env := setupProductionEnv(t, map[string]string{
		"ADR-001": behavioralSidecar("true"),
	})
	cheatRateAll = false
	cheatRateAdversaryModel = "claude-opus-4-7"

	var dispatchCalls int64
	installFakes(t, alwaysSucceedDispatcher(&dispatchCalls), constantVerifyRunner(0))

	// Use ADR-001 — there's a stub fixture for it. Stub mode reads
	// the fixture rather than running production. We use the runner
	// the binary uses (runCheatRate) so the env-var short-circuit
	// at the top fires.
	cobraCmd := &cobra.Command{}
	cobraCmd.SetOut(env.out)
	cobraCmd.SetErr(env.out)

	// runCheatRate is the top-level handler that branches on the
	// env var. The stub flow reads test/fixtures/benchmark-stubs/.
	// Our temp project root has no such directory, so stub mode
	// will exit 2 (no fixture) — but the production dispatcher
	// must NOT have been called.
	_ = runCheatRate(cobraCmd, []string{"ADR-001"})
	if calls := atomic.LoadInt64(&dispatchCalls); calls != 0 {
		t.Errorf("EDIKT_CHEAT_RATE_STUB=1 should short-circuit; dispatcher called %d times", calls)
	}
}
