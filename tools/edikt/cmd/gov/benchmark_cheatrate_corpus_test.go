package gov

// benchmark_cheatrate_corpus_test.go — discriminating-power tests
// driven by the on-disk fixture corpus at
// test/fixtures/cheat-rate-corpus/.
//
// Each fixture directory is a self-contained mini-project (see the
// corpus README). For each, the test:
//
//   1. Loads sidecar.edikt.yaml and selects behavioral directives.
//   2. Runs cheatrate.RunCheatRateForVerify against the fixture's
//      sandbox/ tree.
//   3. In stub mode (default): injects a fake adversary that applies
//      the fixture's cheat.sh inside the sandbox. The real verdict
//      pipeline (RunVerifyInSandbox + DetermineVerdict) runs against
//      the modified state.
//   4. In real mode (EDIKT_CHEAT_RATE_REAL=1): uses the production
//      DispatchAdversary. Requires `claude` auth.
//   5. Asserts the returned verdict matches the fixture's expected.json.
//
// The corpus replaces the earlier Phase 5 inline integration test
// (`test/integration/benchmark-cheat-rate-production.sh`). It is more
// useful because the fixtures are tracked artifacts, easy to inspect,
// and the assertions are visible in test output.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/diktahq/edikt/tools/edikt/internal/benchmark/cheatrate"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// corpusFixture is the on-disk shape of one fixture directory.
type corpusFixture struct {
	Name            string
	Dir             string // absolute path
	SidecarPath     string
	SandboxRoot     string // <Dir>/sandbox
	CheatScript     string // <Dir>/cheat.sh
	ExpectedVerdict string
	CheatShape      string
}

// discoverCorpusFixtures walks the corpus directory and returns one
// entry per subdirectory that has the canonical fixture shape.
func discoverCorpusFixtures(t *testing.T) []corpusFixture {
	t.Helper()
	corpusDir := corpusRoot(t)
	entries, err := os.ReadDir(corpusDir)
	if err != nil {
		t.Fatalf("read corpus dir %q: %v", corpusDir, err)
	}
	var out []corpusFixture
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(corpusDir, e.Name())
		sidecarPath := filepath.Join(dir, "sidecar.edikt.yaml")
		if _, err := os.Stat(sidecarPath); err != nil {
			t.Logf("skip %s: no sidecar.edikt.yaml", e.Name())
			continue
		}
		expectedPath := filepath.Join(dir, "expected.json")
		expected, err := readExpected(expectedPath)
		if err != nil {
			t.Fatalf("read expected.json for %s: %v", e.Name(), err)
		}
		out = append(out, corpusFixture{
			Name:            e.Name(),
			Dir:             dir,
			SidecarPath:     sidecarPath,
			SandboxRoot:     filepath.Join(dir, "sandbox"),
			CheatScript:     filepath.Join(dir, "cheat.sh"),
			ExpectedVerdict: expected.Verdict,
			CheatShape:      expected.CheatShape,
		})
	}
	if len(out) == 0 {
		t.Fatalf("no fixtures discovered under %s", corpusDir)
	}
	return out
}

type expectedJSON struct {
	Verdict    string `json:"verdict"`
	CheatShape string `json:"cheat_shape"`
}

func readExpected(path string) (*expectedJSON, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var e expectedJSON
	if err := json.Unmarshal(raw, &e); err != nil {
		return nil, err
	}
	if e.Verdict == "" {
		return nil, errors.New("expected.json missing 'verdict'")
	}
	return &e, nil
}

// corpusRoot returns the absolute path to test/fixtures/cheat-rate-corpus/
// relative to this source file. Robust against being invoked from any cwd.
func corpusRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../tools/edikt/cmd/gov/benchmark_cheatrate_corpus_test.go
	// Repo root is 4 levels up.
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	resolved, err := filepath.Abs(filepath.Join(repoRoot, "test", "fixtures", "cheat-rate-corpus"))
	if err != nil {
		t.Fatalf("resolve corpus root: %v", err)
	}
	return resolved
}

// stubDispatcherFromScript returns a Dispatcher that, instead of
// invoking real claude, applies the fixture's cheat.sh inside the
// sandbox. The dispatcher succeeds whether the cheat script does or
// doesn't — adversary "couldn't cheat" is encoded by cheat.sh
// failing to satisfy the verify, not by the dispatcher returning an
// error.
func stubDispatcherFromScript(t *testing.T, scriptPath string, counter *int64) cheatrate.Dispatcher {
	t.Helper()
	return func(ctx context.Context, req cheatrate.AdversaryRequest) (*cheatrate.AdversaryResult, error) {
		if counter != nil {
			atomic.AddInt64(counter, 1)
		}
		cmd := exec.CommandContext(ctx, "bash", scriptPath)
		cmd.Dir = req.SandboxPath
		// Inherit a minimal env so basic shell utilities resolve.
		cmd.Env = append([]string{}, os.Environ()...)
		_ = cmd.Run() // failures here just mean the adversary couldn't cheat
		return &cheatrate.AdversaryResult{
			ExitCode:  0,
			TracePath: "",
			ElapsedMs: 1,
			TimedOut:  false,
		}, nil
	}
}

// runCorpusFixture exercises one fixture against Plan E's machinery.
// Stub mode wires the fake dispatcher; real mode leaves Dispatcher nil
// so RunCheatRateForVerify falls through to the production
// DispatchAdversary.
func runCorpusFixture(t *testing.T, f corpusFixture, real bool) {
	t.Helper()
	sc, err := sidecar.Load(f.SidecarPath)
	if err != nil {
		t.Fatalf("sidecar.Load(%s): %v", f.SidecarPath, err)
	}
	// We only score the first behavioral directive — fixtures are
	// designed as single-verdict cases. Multi-verdict mixed fixtures
	// can come later if needed.
	var behavioral *sidecar.Directive
	var behavioralIdx int
	for i := range sc.Directives {
		if sc.Directives[i].VerifyKind == "behavioral" {
			behavioral = &sc.Directives[i]
			behavioralIdx = i
			break
		}
	}
	if behavioral == nil {
		t.Fatalf("fixture %s: no behavioral directive in sidecar", f.Name)
	}

	// Resolve fixture paths to absolute. The sidecar declares
	// `negative_fixture_path: fixtures/negative.sh` relative to the
	// fixture root; the verdict layer needs an absolute path so
	// `bash <path>` works regardless of cwd.
	negFixtureAbs := ""
	if behavioral.NegativeFixturePath != "" {
		negFixtureAbs = filepath.Join(f.Dir, behavioral.NegativeFixturePath)
	}

	// Per-test temp dirs for sandboxes — keeps the test hermetic and
	// removable by t.Cleanup.
	sandboxesDir, err := os.MkdirTemp("", "cheatrate-corpus-sb-")
	if err != nil {
		t.Fatalf("mkdtemp sandboxes: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(sandboxesDir) })
	sandboxesDir, _ = filepath.EvalSymlinks(sandboxesDir)

	opts := cheatrate.RunOpts{
		SidecarID:             f.Name,
		VerifyIdx:             behavioralIdx,
		VerifyID:              fmt.Sprintf("directive[%d]", behavioralIdx),
		Intent:                behavioral.Intent,
		FalsifyingObservation: behavioral.FalsifyingObservation,
		VerifyCommand:         behavioral.Verify,
		NegativeFixturePath:   negFixtureAbs,
		AdversaryModel:        "claude-opus-4-7",
		TemplatePath:          adversaryTemplatePath(t),
		SandboxesDir:          sandboxesDir,
		SourceDir:             f.SandboxRoot,
	}

	if !real {
		opts.Dispatcher = stubDispatcherFromScript(t, f.CheatScript, nil)
	}

	ctx := context.Background()
	if real {
		// Bound the real run hard — three 5-min adversary dispatches
		// would otherwise sit here for ~15min in the worst case. Real
		// mode tests should be opted into knowingly.
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Minute)
		t.Cleanup(cancel)
	}

	verify, err := cheatrate.RunCheatRateForVerify(ctx, opts)
	if err != nil {
		t.Fatalf("RunCheatRateForVerify: %v", err)
	}

	if verify.Verdict != f.ExpectedVerdict {
		t.Errorf("fixture %s: expected verdict %q (cheat_shape %q), got %q (majority_runs %q)",
			f.Name, f.ExpectedVerdict, f.CheatShape, verify.Verdict, verify.MajorityRuns)
	}
}

// adversaryTemplatePath resolves to the project's
// templates/agents/cheat-rate-adversary.md — required by Plan E's
// DispatchAdversary contract even in stub mode (the fake dispatcher
// ignores it, but Validate() insists on its presence).
func adversaryTemplatePath(t *testing.T) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	repoRoot := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "..")
	p, err := filepath.Abs(filepath.Join(repoRoot, "templates", "agents", "cheat-rate-adversary.md"))
	if err != nil {
		t.Fatalf("resolve template: %v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("adversary template missing at %s: %v", p, err)
	}
	return p
}

// TestCheatRateCorpus walks the on-disk corpus and asserts that each
// fixture's expected verdict is what Plan E produces under stub mode.
// Set EDIKT_CHEAT_RATE_REAL=1 to swap in the production adversary
// (auth-gated, costs Opus tokens, probabilistic).
func TestCheatRateCorpus(t *testing.T) {
	real := os.Getenv("EDIKT_CHEAT_RATE_REAL") == "1"
	fixtures := discoverCorpusFixtures(t)
	for _, f := range fixtures {
		t.Run(f.Name, func(t *testing.T) {
			runCorpusFixture(t, f, real)
		})
	}
}
