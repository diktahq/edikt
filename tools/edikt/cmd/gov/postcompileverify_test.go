package gov

import (
	"path/filepath"
	"testing"
)

// TestRunPostCompileVerify_LaunchFailureIsNotAPass pins the gate's
// unobserved case.
//
// runPostCompileVerify's contract already reserves exit 3 for "invocation
// problem", but the function returned 0 on every path where the subprocess
// never started: an unresolvable binary path, or a cmd.Run error that is
// not an *exec.ExitError (chdir failure, binary missing, permission
// denied). The caller does `if rc != 0 { os.Exit(rc) }`, so a gate that
// could not run at all was indistinguishable from a gate that ran and
// found nothing wrong — and gov compile exited 0 announcing verified
// output it had never verified.
//
// A stderr warning does not close this. The exit code is what CI reads.
//
// The launch failure here is a real production shape rather than an
// injected fault: cmd.Dir is set to the project root, and a project root
// that has been removed or is unreadable makes exec fail before the child
// exists. Nothing in the process is stubbed to produce it.
func TestRunPostCompileVerify_LaunchFailureIsNotAPass(t *testing.T) {
	// A path that cannot be chdir'd into. exec returns a *fs.PathError
	// here, not an *exec.ExitError — there is no child to have exited.
	missingRoot := filepath.Join(t.TempDir(), "does-not-exist")

	rc := runPostCompileVerify(missingRoot)

	if rc == 0 {
		t.Fatal("gate returned 0 after failing to launch: an unrun gate must " +
			"not report the same result as a gate that ran and passed")
	}
	if rc != 3 {
		t.Errorf("launch failure should map to the documented invocation-problem "+
			"code 3, got %d", rc)
	}
}
