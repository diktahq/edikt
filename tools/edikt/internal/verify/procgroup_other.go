//go:build !unix

package verify

import "os/exec"

// Non-unix fallback. edikt ships darwin/linux binaries only (ADR-021), so this
// exists to keep the package building under `go vet`/`go build` on other
// platforms — not as a supported target. Process-group containment is
// unavailable here, so a timeout degrades to killing the direct child.
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
