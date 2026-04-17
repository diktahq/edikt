package gov

import (
	"testing"
)

func TestCompileCheckCleanProject(t *testing.T) {
	// Smoke: `edikt gov compile --check <repo>` must exit 0 (warnings OK).
	repoRoot := "../../../../../.." // tools/gov-compile/cmd/gov/ → repo root
	buf, err := runGovCmd(t, "gov", "compile", "--check", repoRoot)
	if err != nil {
		if isExitCode(err, 1) {
			t.Fatalf("gov compile --check returned errors:\n%s", buf)
		}
		t.Fatalf("gov compile --check: %v\n%s", err, buf)
	}
}
