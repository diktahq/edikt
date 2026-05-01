package cmd

import (
	"bytes"
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// runCmd executes the edikt binary with the given args, captures combined
// stdout+stderr, and returns (output, error). The binary is built once per
// test run into a temp dir via TestMain if needed — for now we exec the
// binary from the module root (build on demand).
func runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	bin := buildBinary(t)
	cmd := exec.Command(bin, args...)
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.String(), err
}

// buildBinary compiles the binary once per test binary run and returns
// the path. Uses t.TempDir for isolation.
// _builtBinary is set by TestMain before any test runs.
var _builtBinary string

func buildBinary(t *testing.T) string {
	t.Helper()
	if _builtBinary == "" {
		t.Fatal("_builtBinary not set — TestMain did not run")
	}
	return _builtBinary
}

// contains reports whether s contains substr.
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}

// isExitCode reports whether err is an *exec.ExitError with the given code.
func isExitCode(err error, code int) bool {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode() == code
	}
	return false
}
