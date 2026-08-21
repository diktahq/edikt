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
	errN, warnN := runHookPathCheck("settings.json", settingsJSON("${EDIKT_HOOK_DIR}/pre-tool-use.sh"), "", &out)
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
	errN, warnN := runHookPathCheck("settings.json", settingsJSON("$HOME/.edikt-doctor-test-hook.sh"), "", &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("expected a resolving $HOME hook to report clean, got %d/%d\n%s", errN, warnN, out.String())
	}
}

func TestHookPathCheck_globalHomeForm_missing(t *testing.T) {
	var out bytes.Buffer
	errN, _ := runHookPathCheck("settings.json", settingsJSON("$HOME/.edikt/hooks/does-not-exist.sh"), "", &out)
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
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(missingHook), "", &out)
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
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(hookPath), "", &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("expected a resolving project-mode absolute hook to report clean, got %d/%d\n%s", errN, warnN, out.String())
	}
}

// F9: tools/edikt/.claude/settings.json:7 registers a hook as
// `"$CLAUDE_PROJECT_DIR"/.claude/hooks/verikt-check.sh` — this repo's own
// dev tooling settings reproduce the defect live. The extraction regex
// this check used to use was escape-blind: `"([^"]+)"` stopped at the
// FIRST literal `"`, which for a quoted-variable command is the escaped
// quote right after `$CLAUDE_PROJECT_DIR`'s opening quote, so it captured
// only a lone backslash — degrading to `ERROR: ... references hook \
// which does not resolve to a file (\)` on a legitimate, existing,
// executable hook.
func TestHookPathCheck_claudeProjectDirQuotedForm_resolves(t *testing.T) {
	root := t.TempDir()
	hookDir := filepath.Join(root, ".claude", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hookPath := filepath.Join(hookDir, "verikt-check.sh")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// settingsJSON embeds this raw into a JSON string value with no
	// escaping of its own — a literal `"` here would produce malformed
	// JSON. Use \" (backslash-quote), matching the real on-disk shape
	// (tools/edikt/.claude/settings.json:7), so this exercises the
	// escaped-quote decode path, not a JSON parse failure.
	command := `\"$CLAUDE_PROJECT_DIR\"/.claude/hooks/verikt-check.sh`
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(command), root, &out)
	if errN != 0 || warnN != 0 {
		t.Fatalf("expected a resolving $CLAUDE_PROJECT_DIR-quoted hook to report clean, got %d/%d\n%s", errN, warnN, out.String())
	}
}

// Braced form and a genuinely missing file under the same shape.
func TestHookPathCheck_claudeProjectDirBracedForm_missing(t *testing.T) {
	root := t.TempDir()

	var out bytes.Buffer
	command := `\"${CLAUDE_PROJECT_DIR}\"/.claude/hooks/does-not-exist.sh`
	errN, warnN := runHookPathCheck("settings.json", settingsJSON(command), root, &out)
	if errN != 1 || warnN != 0 {
		t.Fatalf("expected 1 error for a missing $CLAUDE_PROJECT_DIR-form hook, got %d/%d\n%s", errN, warnN, out.String())
	}
	if strings.Contains(out.String(), `\`) && !strings.Contains(out.String(), "does-not-exist.sh") {
		t.Errorf("expected the actual unresolved path named, not a truncated backslash:\n%s", out.String())
	}
}
