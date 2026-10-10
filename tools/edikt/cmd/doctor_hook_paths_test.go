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

// TestDoctorRetiredHookRegistered — a 0.8.x settings.json still registering
// inject-directives-pre.sh after the binary upgraded and before
// /edikt:upgrade ran: the shim no longer exists, and doctor must say the hook
// is RETIRED with the remedy, not report a missing path as RED.
func TestDoctorRetiredHookRegistered(t *testing.T) {
	root := repoRootForDoctorTests(t)
	hookDir := t.TempDir() // no shims here: the file genuinely does not exist
	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json",
		settingsJSON(filepath.Join(hookDir, "inject-directives-pre.sh")), root, &out)
	if errN != 0 {
		t.Fatalf("a retired hook must not be a RED missing path; got %d error(s):\n%s", errN, out.String())
	}
	if warnN != 1 {
		t.Fatalf("want exactly one WARN for the retired registration, got %d:\n%s", warnN, out.String())
	}
	if !strings.Contains(out.String(), "registered hook retired in this version — run /edikt:upgrade") {
		t.Fatalf("remedy missing from the report:\n%s", out.String())
	}
	// Control: a hook the registry never knew stays a missing-path error.
	out.Reset()
	errN, warnN = runHookPathCheck("settings.json",
		settingsJSON(filepath.Join(hookDir, "never-existed.sh")), root, &out)
	if errN != 1 || warnN != 0 {
		t.Fatalf("an unknown missing hook must still be RED; got %d/%d:\n%s", errN, warnN, out.String())
	}
}

// repoRootForDoctorTests walks up from the test's working directory to the
// checkout root, so ResolveRegistryPath finds templates/hooks/registry.yaml.
func repoRootForDoctorTests(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for d := wd; d != "/"; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "templates", "hooks", "registry.yaml")); err == nil {
			return d
		}
	}
	t.Fatal("checkout root with templates/hooks/registry.yaml not found above the test")
	return ""
}

// A registered hook whose command path does not exist is a RED, not a
// warning: the host reports a missing command and continues, so every event
// the hook was registered for silently stops firing.
func TestDoctorHookPath_MissingIsRed(t *testing.T) {
	var out bytes.Buffer
	errN, warnN := runHookPathCheck("settings.json", settingsJSON("/nonexistent/edikt-hooks/gone.sh"), "", &out)
	if errN != 1 || warnN != 0 {
		t.Fatalf("missing hook path must count as 1 error, 0 warnings; got %d/%d\n%s", errN, warnN, out.String())
	}
	if !strings.Contains(out.String(), "🔴") {
		t.Fatalf("missing hook path must print as a RED finding:\n%s", out.String())
	}
}

// TestHookPathCheck_RemedyNamesTheFlaggedScope: doctor checks two settings
// files — the user-scope one under the claude root and the project's own —
// and used to name the same remedy for both, `install settings --repair`,
// which only ever acts on the user file. On a project finding the remedy ran
// against a different file and the finding stayed (#11, item 7). Each
// finding now names the verb that acts on the file it flagged, keeping the
// `install settings --repair` substring tooling greps for.
func TestHookPathCheck_RemedyNamesTheFlaggedScope(t *testing.T) {
	root := t.TempDir()
	body := settingsJSON("${EDIKT_HOOK_DIR}/pre-tool-use.sh")

	var proj bytes.Buffer
	runHookPathCheck(filepath.Join(root, ".claude", "settings.json"), body, root, &proj)
	if !strings.Contains(proj.String(), "install settings --repair --project") {
		t.Errorf("a project-file finding must name the project-scope repair:\n%s", proj.String())
	}

	var user bytes.Buffer
	runHookPathCheck(filepath.Join(t.TempDir(), "profile", "settings.json"), body, root, &user)
	if !strings.Contains(user.String(), "install settings --repair") || strings.Contains(user.String(), "--project") {
		t.Errorf("a user-file finding must name the user-scope repair, without --project:\n%s", user.String())
	}
}

// bok round 6 (build 9937d03a): a user hook registered as
// `bash '<path>/herdr-agent-state.sh' session` was reported as "does not resolve
// to a file" although the script exists — the whole command string was stat'd
// as one path. The host runs the command through a shell, so doctor must find
// the script the shell would run: split the command as the shell does, step
// over an interpreter and its flags, and stat that argument.
func TestHookPathCheck_interpreterWithArgs(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "My Hooks", "state.sh")
	if err := os.MkdirAll(filepath.Dir(script), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name    string
		command string
		wantErr int
	}{
		{"interpreter, single-quoted path with a space, trailing arg", "bash '" + script + "' session", 0},
		{"interpreter flag before the script", "bash -e \\\"" + script + "\\\" session", 0},
		{"env assignment then interpreter", "FOO=1 sh '" + script + "'", 0},
		{"script with trailing args", "'" + script + "' --verbose", 0},
		{"interpreter, missing script still caught", "bash '" + filepath.Join(dir, "gone.sh") + "' session", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			settings := settingsJSON(c.command)
			var out bytes.Buffer
			errN, _ := runHookPathCheck("settings.json", settings, dir, &out)
			if errN != c.wantErr {
				t.Fatalf("errors = %d, want %d\n%s", errN, c.wantErr, out.String())
			}
		})
	}
}
