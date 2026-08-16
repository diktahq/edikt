package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func settingsJSON(hookCommand string) string {
	return `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"` + hookCommand + `"}]}]}}`
}

func TestHookPathCheck_unsubstitutedPlaceholder(t *testing.T) {
	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json", settingsJSON("${EDIKT_HOOK_DIR}/pre-tool-use.sh"), &out)
	if errN != 1 || warnN != 0 {
		t.Fatalf("expected 1 error 0 warnings, got %d/%d\n%s", errN, warnN, out.String())
	}
	if !strings.Contains(out.String(), "unsubstituted placeholder") {
		t.Errorf("expected placeholder message:\n%s", out.String())
	}
}

func TestHookPathCheck_globalHomeForm_resolves(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir in this environment")
	}
	hookPath := filepath.Join(home, ".edikt-doctor-test-hook.sh")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(hookPath)

	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json", settingsJSON("$HOME/.edikt-doctor-test-hook.sh"), &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("expected a resolving $HOME hook to report clean, got %d/%d\n%s", errN, warnN, out.String())
	}
}

func TestHookPathCheck_globalHomeForm_missing(t *testing.T) {
	var out bytes.Buffer
	errN, _ := runHookPathCheck("settings.json", settingsJSON("$HOME/.edikt/hooks/does-not-exist.sh"), &out)
	if errN != 1 {
		t.Fatalf("expected 1 error for a missing $HOME-form hook, got %d\n%s", errN, out.String())
	}
}

// This is the positive case for the confirmed gap: a --project install
// substitutes ${EDIKT_HOOK_DIR} to a fully-resolved absolute project path
// at install time, so the settings.json contains neither $HOME nor the
// placeholder — invisible to a check that only recognized those two
// shapes. A missing hook in this shape must still be caught.
func TestHookPathCheck_projectModeAbsolutePath_missing(t *testing.T) {
	root := t.TempDir()
	missingHook := filepath.Join(root, ".edikt", "hooks", "pre-tool-use.sh")

	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(missingHook), &out)
	if errN != 1 || warnN != 0 {
		t.Fatalf("expected 1 error for a missing project-mode absolute hook path, got %d/%d\n%s", errN, warnN, out.String())
	}
	if !strings.Contains(out.String(), missingHook) {
		t.Errorf("expected the unresolved project-mode path to be named:\n%s", out.String())
	}
}

func TestHookPathCheck_projectModeAbsolutePath_resolves(t *testing.T) {
	root := t.TempDir()
	hookDir := filepath.Join(root, ".edikt", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hookDir, "pre-tool-use.sh")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(hookPath), &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("expected a resolving project-mode absolute hook to report clean, got %d/%d\n%s", errN, warnN, out.String())
	}
}
