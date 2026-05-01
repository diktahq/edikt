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
