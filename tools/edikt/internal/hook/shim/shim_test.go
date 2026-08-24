package shim

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/trust"
)

const (
	testHookName  = "session-start"
	testHookEvent = "SessionStart"
)

// ── template shape ──────────────────────────────────────────────────────

// TestTemplateLineBudget — AC-8.1: the template is <=10 physical lines.
func TestTemplateLineBudget(t *testing.T) {
	n := LineCount()
	if n == 0 {
		t.Fatal("LineCount reports 0 — the template is empty, which is not a budget of anything")
	}
	if n > 10 {
		t.Fatalf("Template is %d physical lines, want <=10", n)
	}
	t.Logf("Template is %d physical lines", n)
}

// TestTemplateAnchorsOnPWDNotProjectRoot — the exact regression PLAN-v080
// Phase 8 names: $EDIKT_PROJECT_ROOT is a LOCAL variable stop-hook.sh sets
// for its own heredocs, not an ambient contract. A shim anchored on it would
// expand to "/.edikt/bin/edikt" and "/bin/edikt" in every install except
// stop-hook's own.
func TestTemplateAnchorsOnPWDNotProjectRoot(t *testing.T) {
	if !strings.Contains(Template, "${PWD}") {
		t.Fatal(`Template does not anchor on "${PWD}"`)
	}
	if strings.Contains(Template, "EDIKT_PROJECT_ROOT") {
		t.Fatal("Template references $EDIKT_PROJECT_ROOT — that variable is local to stop-hook.sh and is not exported anywhere else")
	}
}

// TestTemplateRungOrder — global root always precedes bare PATH, and the
// project-local dev-convention rung (bin/edikt) never outranks the
// unambiguous .edikt/bin/edikt marker.
func TestTemplateRungOrder(t *testing.T) {
	dotEdikt := strings.Index(Template, "${PWD}/.edikt/bin/edikt")
	projectBin := strings.Index(Template, "${PWD}/bin/edikt")
	// The FIRST "$_G" occurs earlier, inside _ok()'s own self-is-global
	// shortcut ([ "$1" = "$_G" ]) — that is a rung-order-agnostic comparison,
	// not the ladder itself. The ladder's actual rung 3 is the "$_G" that
	// appears in the `for c in ...` candidate list, which is the last "$_G"
	// in the template (nothing after it references the variable).
	global := strings.LastIndex(Template, `"$_G"`)
	pathFallback := strings.Index(Template, "command -v edikt")
	for name, idx := range map[string]int{
		".edikt/bin/edikt": dotEdikt, "bin/edikt": projectBin, "global ($_G)": global, "PATH fallback": pathFallback,
	} {
		if idx < 0 {
			t.Fatalf("rung %q not found in Template at all", name)
		}
	}
	if !(dotEdikt < projectBin && projectBin < global && global < pathFallback) {
		t.Fatalf("rung order wrong: .edikt/bin/edikt=%d bin/edikt=%d global=%d PATH=%d — want strictly increasing",
			dotEdikt, projectBin, global, pathFallback)
	}
}

// ── Render ───────────────────────────────────────────────────────────────

func TestRenderSubstitutesNameAndEvent(t *testing.T) {
	out, err := Render(testHookName, testHookEvent)
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if strings.Contains(out, "<name>") || strings.Contains(out, "<event>") {
		t.Fatalf("rendered output still carries a placeholder: %q", out)
	}
	if !strings.Contains(out, "hook run "+testHookName+" --event "+testHookEvent) {
		t.Fatalf("rendered output does not exec the substituted name/event: %q", out)
	}
	if !strings.Contains(out, `"hookEventName":"`+testHookEvent+`"`) {
		t.Fatalf("absence banner does not carry the substituted event: %q", out)
	}
}

func TestRenderIsDeterministic(t *testing.T) {
	a, err := Render(testHookName, testHookEvent)
	if err != nil {
		t.Fatal(err)
	}
	b, err := Render(testHookName, testHookEvent)
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatal("two renders of the same (name, event) produced different bytes")
	}
}

func TestRenderRejectsBadInput(t *testing.T) {
	cases := []struct {
		name, event string
	}{
		{"Not_A_Name", testHookEvent},
		{testHookName, ""},
		{"a<b", testHookEvent},
	}
	for _, tc := range cases {
		if _, err := Render(tc.name, tc.event); err == nil {
			t.Errorf("Render(%q, %q): want an error, got nil", tc.name, tc.event)
		}
	}
}

// ── trust-gated execution (AC-8.3 / AC-8.4) ─────────────────────────────

// writeShim renders (testHookName, testHookEvent) to <dir>/shim.sh (0755)
// and returns its path.
func writeShim(t *testing.T, dir string) string {
	t.Helper()
	rendered, err := Render(testHookName, testHookEvent)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	p := filepath.Join(dir, "shim.sh")
	if err := os.WriteFile(p, []byte(rendered), 0o755); err != nil {
		t.Fatalf("write shim: %v", err)
	}
	return p
}

// plantFakeBinary writes an executable at path that, when run, appends its
// OWN invoked path ($0) to markerFile and replies with an empty hook-protocol
// object. $0 (not file content) is what lets two BYTE-IDENTICAL copies (the
// sha256-match test plants one at both the local and the global rung) still
// report which one actually ran.
func plantFakeBinary(t *testing.T, path, markerFile string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	body := "#!/usr/bin/env bash\nprintf '%s\\n' \"$0\" >> '" + markerFile + "'\nprintf '{}'\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		t.Fatalf("write fake binary %s: %v", path, err)
	}
}

// isolatedEnv is the minimal environment a shim subprocess runs under: no
// inherited PATH, HOME, EDIKT_HOME or EDIKT_ROOT from the developer's real
// machine, so a test's pass/fail can never depend on what happens to be
// installed or trusted outside the test.
func isolatedEnv(home string) []string {
	return []string{
		"HOME=" + home,
		"PATH=/usr/bin:/bin:/sbin:/usr/sbin",
	}
}

func runShim(t *testing.T, shimPath, projectDir, home string) (stdout, stderr string, exitCode int) {
	t.Helper()
	cmd := exec.Command(shimPath)
	cmd.Dir = projectDir
	cmd.Env = isolatedEnv(home)
	cmd.Stdin = strings.NewReader("{}")
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("running shim: %v (stderr=%q)", err, errb.String())
	}
	return out.String(), errb.String(), code
}

// TestUntrustedLocalBinaryNotExecuted — PLAN-v080 Phase 8's named AC: a
// planted executable .edikt/bin/edikt in an untrusted project is not exec'd.
//
// The project carries neither an ADR-041 trust record nor a global install to
// sha256-match against, so BOTH trust paths ADR-041 offers are closed. If the
// gate held, the planted binary's marker is absent and the shim instead falls
// all the way through to the absence banner (nothing else resolves either).
func TestUntrustedLocalBinaryNotExecuted(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(project, "pwned")
	plantFakeBinary(t, filepath.Join(project, ".edikt", "bin", "edikt"), marker)
	// No trust.Record call: this project has no store entry. No global
	// binary is planted under $home/.edikt/bin/edikt either, so the sha256
	// fallback has nothing to compare against.

	shimPath := writeShim(t, t.TempDir())
	stdout, stderr, code := runShim(t, shimPath, project, home)

	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("the untrusted planted .edikt/bin/edikt WAS executed — trust gating did not hold.\nstdout=%q\nstderr=%q", stdout, stderr)
	}
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 (fail-open) — stdout=%q stderr=%q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, `"hookEventName":"`+testHookEvent+`"`) {
		t.Fatalf("absence banner missing or has the wrong event: stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "binary-absent") {
		t.Fatalf("absence banner missing its distinctly-typed cause: stdout=%q", stdout)
	}
}

// TestTrustedLocalBinaryViaTrustStoreIsExecuted is the positive twin: once
// ADR-041's own trust.Record marks the project trusted, the same local
// binary DOES resolve and run.
func TestTrustedLocalBinaryViaTrustStoreIsExecuted(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(project, "ran")
	plantFakeBinary(t, filepath.Join(project, ".edikt", "bin", "edikt"), marker)

	// Record trust into the SAME store the subprocess will read: point the
	// CURRENT process's HOME/EDIKT_HOME at `home` for the duration of the
	// Record call, matching runShim's isolated env for the child.
	t.Setenv("HOME", home)
	t.Setenv("EDIKT_HOME", "")
	if err := trust.Record(project); err != nil {
		t.Fatalf("trust.Record: %v", err)
	}

	shimPath := writeShim(t, t.TempDir())
	stdout, stderr, code := runShim(t, shimPath, project, home)

	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 — stdout=%q stderr=%q", code, stdout, stderr)
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("the trusted local binary was NOT executed (no marker written): stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(string(raw), project) {
		t.Fatalf("marker does not name the project-local invocation path: %q", raw)
	}
}

// TestTrustedLocalBinaryViaSha256MatchIsExecuted — the second ADR-041 path:
// no trust-store record needed when the local candidate is byte-identical to
// the global install.
func TestTrustedLocalBinaryViaSha256MatchIsExecuted(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	marker := filepath.Join(project, "ran")

	// Byte-IDENTICAL content at both the local and the global rung (the
	// marker path baked into the script is shared, so the bytes truly
	// match); $0 still differs per-invocation because it reflects argv[0],
	// not file content, which is how the observed marker can still say which
	// rung actually ran.
	localPath := filepath.Join(project, ".edikt", "bin", "edikt")
	globalPath := filepath.Join(home, ".edikt", "bin", "edikt")
	plantFakeBinary(t, localPath, marker)
	body, err := os.ReadFile(localPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(globalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(globalPath, body, 0o755); err != nil {
		t.Fatal(err)
	}
	// No trust.Record call — this test proves the sha256 path alone suffices.

	shimPath := writeShim(t, t.TempDir())
	stdout, stderr, code := runShim(t, shimPath, project, home)

	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 — stdout=%q stderr=%q", code, stdout, stderr)
	}
	raw, err := os.ReadFile(marker)
	if err != nil {
		t.Fatalf("neither copy ran (no marker written): stdout=%q stderr=%q", stdout, stderr)
	}
	if !strings.Contains(string(raw), project) {
		t.Fatalf("the LOCAL rung was not the one that ran (sha256-match must prefer rung 1 over falling through to rung 3): marker=%q", raw)
	}
}

// TestUntrustedLocalFallsThroughToGlobal proves the "silent skip, not a
// silent exec" half of the design: an untrusted local candidate does not
// block a working, trusted global install from being used, and it does not
// stop the write.
func TestUntrustedLocalFallsThroughToGlobal(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	localMarker := filepath.Join(project, "local-ran")
	globalMarker := filepath.Join(project, "global-ran")
	plantFakeBinary(t, filepath.Join(project, ".edikt", "bin", "edikt"), localMarker)
	plantFakeBinary(t, filepath.Join(home, ".edikt", "bin", "edikt"), globalMarker)
	// Local and global have DIFFERENT content (different marker targets), so
	// sha256 cannot match them, and no trust record exists either: rung 1 is
	// untrusted by both tests. Rung 3 (the global install) is always trusted.

	shimPath := writeShim(t, t.TempDir())
	stdout, stderr, code := runShim(t, shimPath, project, home)

	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 — stdout=%q stderr=%q", code, stdout, stderr)
	}
	if _, err := os.Stat(localMarker); err == nil {
		t.Fatal("the untrusted local binary ran — it should have been skipped")
	}
	if _, err := os.Stat(globalMarker); err != nil {
		t.Fatalf("the trusted global install did not run even though the local candidate was correctly skipped: stdout=%q stderr=%q", stdout, stderr)
	}
}

// TestPathRungIsNeverTrustGated — rung 4 (bare PATH) is not project-supplied,
// so it is exempt from the trust gate by design; it is also the rung the
// absence-banner tests rely on being reachable without any candidate
// resolving via .edikt/bin/edikt or the global root.
func TestPathRungIsNeverTrustGated(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	pathDir := t.TempDir()
	marker := filepath.Join(project, "ran")
	plantFakeBinary(t, filepath.Join(pathDir, "edikt"), marker)

	shimPath := writeShim(t, t.TempDir())
	cmd := exec.Command(shimPath)
	cmd.Dir = project
	cmd.Env = append(isolatedEnv(home), "PATH="+pathDir+":/usr/bin:/bin")
	cmd.Stdin = strings.NewReader("{}")
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		if _, ok := err.(*exec.ExitError); !ok {
			t.Fatalf("running shim: %v", err)
		}
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("the PATH-resolved binary did not run: stdout=%q stderr=%q", out.String(), errb.String())
	}
}

// TestAbsencePathWhenNothingResolves — no candidate anywhere: the shim exits
// 0 and emits the absence banner with the correct hookEventName.
func TestAbsencePathWhenNothingResolves(t *testing.T) {
	project := t.TempDir()
	home := t.TempDir()
	shimPath := writeShim(t, t.TempDir())

	stdout, stderr, code := runShim(t, shimPath, project, home)
	if code != 0 {
		t.Fatalf("exit code: got %d, want 0 — stderr=%q", code, stderr)
	}
	if !strings.Contains(stdout, `"hookEventName":"`+testHookEvent+`"`) {
		t.Fatalf("absence banner missing or wrong event: stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "additionalContext") {
		t.Fatalf("absence banner did not use hookSpecificOutput.additionalContext (the channel that actually reaches the model): stdout=%q", stdout)
	}
	if !strings.Contains(stdout, "binary-absent") {
		t.Fatalf("absence banner missing the distinctly-typed cause: stdout=%q", stdout)
	}
}
