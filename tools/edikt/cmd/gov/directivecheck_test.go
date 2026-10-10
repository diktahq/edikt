package gov

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

// declareInvoker gives an exec'd `gov` the caller identity ADR-084's front door
// requires. These three tests predate the door (test written 2026-05-05, door
// landed 3caad16b on 2026-09-04) and had been failing ever since, invoking the
// binary with no declared caller and getting exit 4 instead of the exit each
// asserts. The subject here is what `directive-check` DOES with its stdin, not
// whether the door is shut.
//
// Set per call site, deliberately NOT in this package's TestMain: the door's
// refusal is itself under test in this package (losslesscheck_nothing_test.go
// expects exit 4), and a package-wide EDIKT_INVOKER is inherited by every
// exec'd subprocess, so it would quietly disarm the control that proves the
// door works. The value is the slash command this verb's own error message
// names, so the test takes the route a real caller takes.
func declareInvoker(cmd *exec.Cmd) *exec.Cmd {
	cmd.Env = append(os.Environ(), "EDIKT_INVOKER=/edikt:gov:directive-check")
	return cmd
}

func TestDirectiveCheck_Clean(t *testing.T) {
	out := runDirectiveCheckStdin(t, `{"adr_id":"ADR-001","directive_body":"Use repo layer.","canonical_phrases":[],"no_directives_reason":null}`, 0)
	if strings.TrimSpace(out) != "" {
		t.Errorf("expected silent output, got: %q", out)
	}
}

func TestDirectiveCheck_PhraseMissing(t *testing.T) {
	out := runDirectiveCheckStdin(t, `{
		"adr_id": "ADR-014",
		"directive_body": "Use os.Rename for state.",
		"canonical_phrases": ["atomic rename"],
		"no_directives_reason": null
	}`, 0)
	if !strings.Contains(out, `canonical_phrase "atomic rename" not found`) {
		t.Errorf("missing phrase warning: %s", out)
	}
}

func TestDirectiveCheck_BadJSON(t *testing.T) {
	bin := buildBinary(t)
	cmd := declareInvoker(exec.Command(bin, "gov", "directive-check"))
	cmd.Stdin = strings.NewReader("{not json")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if !isExitCode(err, 2) {
		t.Fatalf("expected exit 2 on bad JSON, got err=%v\noutput: %s", err, out.String())
	}
}

func runDirectiveCheckStdin(t *testing.T, payload string, wantCode int) string {
	t.Helper()
	bin := buildBinary(t)
	cmd := declareInvoker(exec.Command(bin, "gov", "directive-check"))
	cmd.Stdin = strings.NewReader(payload)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	if wantCode == 0 && err != nil {
		t.Fatalf("expected exit 0, got err=%v\noutput: %s", err, out.String())
	}
	if wantCode != 0 && !isExitCode(err, wantCode) {
		t.Fatalf("expected exit %d, got err=%v\noutput: %s", wantCode, err, out.String())
	}
	return out.String()
}
