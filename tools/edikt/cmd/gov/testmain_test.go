package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "edikt-gov-test-bin-")
	if err != nil {
		panic("cannot create binary temp dir: " + err.Error())
	}
	defer os.RemoveAll(dir)

	_, thisFile, _, _ := runtime.Caller(0)
	modRoot := filepath.Join(filepath.Dir(thisFile), "../..")
	bin := filepath.Join(dir, "edikt")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = modRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		panic("build binary: " + err.Error() + "\n" + string(out))
	}
	_builtBinary = bin
	os.Exit(m.Run())
}
