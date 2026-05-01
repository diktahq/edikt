package gov

import (
	"bytes"
	"errors"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

var _builtBinary string

func buildBinary(t *testing.T) string {
	t.Helper()
	if _builtBinary != "" {
		return _builtBinary
	}
	_, thisFile, _, _ := runtime.Caller(0)
	modRoot := filepath.Join(filepath.Dir(thisFile), "../..")
	outDir := t.TempDir()
	bin := filepath.Join(outDir, "edikt")
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = modRoot
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	if err := cmd.Run(); err != nil {
		t.Fatalf("build binary: %v\n%s", err, buf.String())
	}
	_builtBinary = bin
	return bin
}

func runGovCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	bin := buildBinary(t)
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

func isExitCode(err error, code int) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == code
	}
	return false
}
