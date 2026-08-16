package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeSettings writes a .claude/settings.json registering the given hook
// basenames under PreToolUse, in the shape hookBasenames reads.
func writeSettings(t *testing.T, root string, preToolUse ...string) {
	t.Helper()
	dir := filepath.Join(root, ".claude")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	var entries []string
	for _, h := range preToolUse {
		entries = append(entries,
			`{"type":"command","command":"${EDIKT_HOOK_DIR}/`+h+`"}`)
	}
	body := `{"hooks":{"PreToolUse":[{"matcher":"*","hooks":[` +
		strings.Join(entries, ",") + `]}]}}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// TestVerifyGate_UninstalledIsNotEnabled is the core A4c regression.
//
// The check branched solely on EDIKT_DISABLE_VERIFY_GATE: unset meant it
// printed "Verify Gate: ENABLED (PreToolUse hook active)". It never looked
// at whether the hook was installed, so a directory with no .claude/ at
// all — no settings.json, no hooks, edikt never initialised — was reported
// as having an active PreToolUse gate.
//
// That is the reported-a-measurement-it-never-made shape at its purest:
// the one thing the line asserts ("hook active") is the one thing it did
// not check.
func TestVerifyGate_UninstalledIsNotEnabled(t *testing.T) {
	t.Setenv("EDIKT_DISABLE_VERIFY_GATE", "")
	root := t.TempDir() // no .claude/ whatsoever

	var buf bytes.Buffer
	warns, _ := runVerifyGateCheck(root, &buf)
	out := buf.String()

	if strings.Contains(out, "ENABLED") {
		t.Errorf("reported ENABLED for a project with no .claude/ at all:\n%s", out)
	}
	if !strings.Contains(out, "NOT INSTALLED") {
		t.Errorf("output does not say the gate is uninstalled:\n%s", out)
	}
	if warns == 0 {
		t.Error("an uninstalled verify gate produced no warning")
	}
}

// TestVerifyGate_RegisteredButNotTheGateIsNotEnabled covers the subtler
// case: settings.json exists and registers PreToolUse hooks, just not this
// one. Reading "a settings file is present" as "the gate is active" would
// be the same unmade measurement one step in.
func TestVerifyGate_RegisteredButNotTheGateIsNotEnabled(t *testing.T) {
	t.Setenv("EDIKT_DISABLE_VERIFY_GATE", "")
	root := t.TempDir()
	writeSettings(t, root, "some-other-hook.sh")

	var buf bytes.Buffer
	_, _ = runVerifyGateCheck(root, &buf)
	out := buf.String()

	if strings.Contains(out, "ENABLED") {
		t.Errorf("reported ENABLED with no verify-gate hook registered:\n%s", out)
	}
}

// TestVerifyGate_RegisteredIsEnabled is the control. The fix must not turn
// a correctly-installed gate into a false alarm.
func TestVerifyGate_RegisteredIsEnabled(t *testing.T) {
	t.Setenv("EDIKT_DISABLE_VERIFY_GATE", "")
	root := t.TempDir()
	writeSettings(t, root, "verify-gate.sh")

	var buf bytes.Buffer
	warns, _ := runVerifyGateCheck(root, &buf)
	out := buf.String()

	if !strings.Contains(out, "ENABLED") {
		t.Errorf("a registered verify-gate hook was not reported ENABLED:\n%s", out)
	}
	if warns != 0 {
		t.Errorf("a correctly installed gate produced %d warning(s):\n%s", warns, out)
	}
}

// TestVerifyGate_BypassBeatsRegistration keeps ADR-038's escape hatch
// visible: an installed gate with the bypass env var set is BYPASSED, not
// ENABLED.
func TestVerifyGate_BypassBeatsRegistration(t *testing.T) {
	t.Setenv("EDIKT_DISABLE_VERIFY_GATE", "1")
	root := t.TempDir()
	writeSettings(t, root, "verify-gate.sh")

	var buf bytes.Buffer
	_, _ = runVerifyGateCheck(root, &buf)
	out := buf.String()

	if !strings.Contains(out, "BYPASSED") {
		t.Errorf("EDIKT_DISABLE_VERIFY_GATE=1 not reported as BYPASSED:\n%s", out)
	}
	if strings.Contains(out, "ENABLED") {
		t.Errorf("reported ENABLED while bypassed:\n%s", out)
	}
}
