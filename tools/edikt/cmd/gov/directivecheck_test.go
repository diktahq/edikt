package gov

import (
	"bytes"
	"os/exec"
	"strings"
	"testing"
)

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
	cmd := exec.Command(bin, "gov", "directive-check")
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
	cmd := exec.Command(bin, "gov", "directive-check")
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
