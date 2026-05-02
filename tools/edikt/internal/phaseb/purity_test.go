// purity_test.go — Phase B purity gate (ADR-028 §"Phase B").
//
// Phase B (this package) is the deterministic merge step of gov:compile.
// It MUST NOT dispatch subagents, shell out, or make network calls. The
// gate runs `go list -deps` so it sees the full transitive import closure
// rather than just the package's own source — that catches a forbidden
// symbol leaking in via a helper package too.
//
// This Go test is the authoritative purity gate. The bash script at
// tools/edikt/check/no-llm-in-merge.sh is kept as a CI convenience but
// is no longer the gate of record.
package phaseb_test

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

func TestPhaseBPurity(t *testing.T) {
	const pkgPath = "github.com/diktahq/edikt/tools/edikt/internal/phaseb"
	forbidden := []string{
		"os/exec",
		"net/http",
		"net/rpc",
		"github.com/diktahq/edikt/tools/edikt/internal/phasea",
	}

	var stdout, stderr bytes.Buffer
	cmd := exec.Command("go", "list", "-deps", "-f", "{{.ImportPath}}", pkgPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("go list: %v\nstderr: %s", err, stderr.String())
	}

	deps := make(map[string]bool)
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line != "" {
			deps[line] = true
		}
	}
	if !deps[pkgPath] {
		t.Fatalf("go list -deps did not return %s; stdout:\n%s", pkgPath, stdout.String())
	}

	for _, imp := range forbidden {
		if deps[imp] {
			t.Errorf("phaseb has forbidden transitive import %q (Phase B must remain pure per ADR-028)", imp)
		}
	}
}
