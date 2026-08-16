package cmd

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// _testBinDir is a persistent dir that survives the full test binary run.
var _testBinDir string

func TestMain(m *testing.M) {
	// Approve verify execution for the whole cmd test binary (ADR-041). Tests
	// scaffold throwaway projects in t.TempDir() that are never recorded in the
	// trust store, so without this bypass every verify/compile subprocess would
	// fail closed (exit 4). The bypass is inherited by exec'd subprocesses
	// (runVerify/runEdikt set no curated cmd.Env). Tests that exercise the
	// trust deny-path clear it explicitly with t.Setenv.
	os.Setenv("EDIKT_VERIFY_TRUST", "1")

	// INV-007: resolveClaudeRoot honours CLAUDE_CONFIG_DIR, so a value leaked
	// from a multi-profile host shell would redirect any test that sandboxes
	// via HOME alone into the developer's real Claude profile. Tests that need
	// it set it explicitly with t.Setenv.
	os.Unsetenv("CLAUDE_CONFIG_DIR")

	// Build the binary once into a directory that is NOT cleaned up by
	// t.TempDir() — it must survive all test functions.
	var err error
	_testBinDir, err = os.MkdirTemp("", "edikt-test-bin-")
	if err != nil {
		panic("cannot create binary temp dir: " + err.Error())
	}
	defer os.RemoveAll(_testBinDir)

	_, thisFile, _, _ := runtime.Caller(0)
	modRoot := filepath.Join(filepath.Dir(thisFile), "..")
	bin := filepath.Join(_testBinDir, "edikt")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("build binary: " + err.Error() + "\n" + string(out))
	}
	_builtBinary = bin

	os.Exit(m.Run())
}
