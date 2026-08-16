package cheatrate

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// testdataPath returns an absolute path under the package's testdata
// directory. Used to point AdversaryRequest.TemplatePath at the
// minimal template fixture and EDIKT_LLM_BIN at the fake-claude
// scripts.
func testdataPath(t *testing.T, name string) string {
	t.Helper()
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	return filepath.Join(dir, "testdata", name)
}

// newSandbox creates a tempdir to act as the adversary sandbox and
// returns its absolute path. The dir is automatically removed when
// the test ends.
func newSandbox(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "cheatrate-adversary-sandbox-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	// Resolve symlinks so /var → /private/var on macOS doesn't trip
	// the sandbox_path validator's character allowlist with surprise
	// segments. The resolved path is what cmd.Dir ends up with anyway.
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatalf("evalsymlinks: %v", err)
	}
	return resolved
}

func validRequest(t *testing.T, sandbox string) AdversaryRequest {
	t.Helper()
	return AdversaryRequest{
		SidecarID:             "ADR-040",
		VerifyIdx:             0,
		SandboxPath:           sandbox,
		Intent:                "the directive describes a behavioral property",
		FalsifyingObservation: "a violation looks like ...",
		VerifyCommand:         "test -f marker",
		AdversaryModel:        "claude-opus-4-7",
		TemplatePath:          testdataPath(t, "adversary-template.md"),
	}
}

// Covers AC-1.4 + AC-1.6 case 3. Attacker-influenceable fields are rejected
// at the boundary. Formerly asserted through DispatchAdversary; that exec was
// deleted by ADR-044, so the same property is asserted against Validate
// directly — the check that actually enforces it.
func TestAdversaryRequest_ValidateRejectsInjection(t *testing.T) {
	sandbox := newSandbox(t)
	// ADR-044 deleted the in-binary dispatcher, so this exercises
	// AdversaryRequest.Validate directly. The INV-006 property is unchanged
	// and still worth guarding: attacker-influenceable fields (intent,
	// falsifying observation, sandbox path, model id, sidecar id) are
	// rejected at the boundary rather than reaching a prompt or a path.

	cases := []struct {
		name       string
		mutate     func(*AdversaryRequest)
		wantSubstr string
	}{
		{
			name: "null_byte_in_intent",
			mutate: func(r *AdversaryRequest) {
				r.Intent = "legitimate intent\x00; rm -rf /"
			},
			wantSubstr: "intent contains null byte",
		},
		{
			name: "control_char_in_falsifying_observation",
			mutate: func(r *AdversaryRequest) {
				r.FalsifyingObservation = "violation looks like\x07 bell"
			},
			wantSubstr: "falsifying_observation contains disallowed control character",
		},
		{
			name: "traversal_in_sandbox_path",
			mutate: func(r *AdversaryRequest) {
				r.SandboxPath = "/tmp/../etc"
			},
			wantSubstr: "sandbox_path must not contain '..'",
		},
		{
			name: "shell_meta_in_sandbox_path",
			mutate: func(r *AdversaryRequest) {
				r.SandboxPath = "/tmp/sandbox;rm -rf /"
			},
			wantSubstr: "sandbox_path contains disallowed character",
		},
		{
			name: "bad_model_id",
			mutate: func(r *AdversaryRequest) {
				r.AdversaryModel = "claude; rm -rf /"
			},
			wantSubstr: "invalid adversary_model",
		},
		{
			name: "bad_sidecar_id",
			mutate: func(r *AdversaryRequest) {
				r.SidecarID = "ADR-040; cat /etc/passwd"
			},
			wantSubstr: "invalid sidecar_id",
		},
		{
			name: "oversize_intent",
			mutate: func(r *AdversaryRequest) {
				r.Intent = strings.Repeat("A", maxIntentLen+1)
			},
			wantSubstr: "intent exceeds",
		},
		{
			name: "empty_verify_command",
			mutate: func(r *AdversaryRequest) {
				r.VerifyCommand = ""
			},
			wantSubstr: "verify_command required",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := validRequest(t, sandbox)
			tc.mutate(&req)
			err := req.Validate()
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSubstr) {
				t.Errorf("error message %q missing expected substring %q", err.Error(), tc.wantSubstr)
			}
		})
	}
}
