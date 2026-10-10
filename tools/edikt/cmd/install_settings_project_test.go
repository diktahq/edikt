package cmd

// install_settings_project_test.go — `install settings --repair --project`
// (#11 comment item 7, PLAN-v090-issue-fixes phase 9 step 4).
//
// doctor checks the PROJECT's .claude/settings.json and named
// `edikt install settings --repair` as the remedy; that verb resolved the
// USER-scope file through the claude root and never touched the project
// file. The remedy ran, reported "nothing to repair" or repaired the wrong
// file, and the finding stayed. --project points the repair at the file the
// finding was about.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const projectScopeBroken = `{"statusLine":{"command":"x"},"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"${EDIKT_HOOK_DIR}/pre-tool-use.sh"}]}]}}`

// isolateSettingsEnv points every root the verb can resolve at a temp tree
// and returns (home, userSettingsPath, sidecarPath).
func isolateSettingsEnv(t *testing.T) (string, string, string) {
	t.Helper()
	home := t.TempDir()
	claudeRoot := filepath.Join(home, ".claude")
	ediktHome := filepath.Join(home, ".edikt")
	for _, d := range []string{claudeRoot, ediktHome} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("HOME", home)
	t.Setenv("CLAUDE_HOME", claudeRoot)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("EDIKT_HOME", ediktHome)
	t.Setenv("EDIKT_HOOK_DIR", "/opt/edikt/hooks")
	user := filepath.Join(claudeRoot, "settings.json")
	if err := os.WriteFile(user, []byte(projectScopeBroken), 0o644); err != nil {
		t.Fatal(err)
	}
	return home, user, filepath.Join(ediktHome, "state", "settings-managed.json")
}

func TestInstallSettingsRepair_ProjectScope(t *testing.T) {
	_, user, sidecar := isolateSettingsEnv(t)
	userBefore, _ := os.ReadFile(user)

	proj := t.TempDir()
	for _, d := range []string{filepath.Join(proj, ".edikt"), filepath.Join(proj, ".claude"), filepath.Join(proj, "sub")} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	projSettings := filepath.Join(proj, ".claude", "settings.json")
	if err := os.WriteFile(projSettings, []byte(projectScopeBroken), 0o644); err != nil {
		t.Fatal(err)
	}
	// From a subdirectory: the project is found by walking up, as every
	// other project-scoped verb finds it.
	t.Chdir(filepath.Join(proj, "sub"))

	if err := runInstallSettingsRepair(true); err != nil {
		t.Fatalf("--repair --project: %v", err)
	}
	got, _ := os.ReadFile(projSettings)
	if strings.Contains(string(got), "${EDIKT_HOOK_DIR}") || !strings.Contains(string(got), "/opt/edikt/hooks/pre-tool-use.sh") {
		t.Errorf("the project file was not repaired (placeholder):\n%s", got)
	}
	if !strings.Contains(string(got), `"type": "command"`) {
		t.Errorf("the project file was not repaired (statusLine.type):\n%s", got)
	}
	if userAfter, _ := os.ReadFile(user); string(userAfter) != string(userBefore) {
		t.Errorf("--project touched the user-scope settings file:\n%s", userAfter)
	}
	// D-P (b): the INV-005 managed sidecar is per-user state about the
	// user-scope file; a project-scope repair does not write it.
	if _, err := os.Stat(sidecar); !os.IsNotExist(err) {
		t.Errorf("--project wrote the user-scope managed sidecar at %s (stat err=%v)", sidecar, err)
	}

	// The control: without --project the same verb repairs the user file,
	// unchanged from before the flag existed.
	if err := runInstallSettingsRepair(false); err != nil {
		t.Fatalf("--repair (user scope): %v", err)
	}
	if userAfter, _ := os.ReadFile(user); string(userAfter) == string(userBefore) {
		t.Error("control: plain --repair no longer repairs the user-scope file")
	}
}

func TestInstallSettingsRepair_ProjectOutsideProjectRefused(t *testing.T) {
	home, user, _ := isolateSettingsEnv(t)
	userBefore, _ := os.ReadFile(user)

	assertRefused := func(t *testing.T, where string) {
		t.Helper()
		err := runInstallSettingsRepair(true)
		var ec *exitCodeError
		if !errors.As(err, &ec) {
			t.Fatalf("%s: --project outside a project must refuse with an exit code, got %v", where, err)
		}
		if ec.code != installSettingsExitToShellEx(installSettingsExitBadInput) {
			t.Errorf("%s: exit %d, want the bad-input exit %d", where, ec.code, installSettingsExitToShellEx(installSettingsExitBadInput))
		}
		if !strings.Contains(ec.msg, "--project") {
			t.Errorf("%s: the refusal does not name the flag: %q", where, ec.msg)
		}
		if userAfter, _ := os.ReadFile(user); string(userAfter) != string(userBefore) {
			t.Fatalf("%s: a refused --project repair wrote the user-scope file", where)
		}
	}

	t.Chdir(t.TempDir())
	assertRefused(t, "a directory with no project above it")

	// $HOME carries .edikt/ — the GLOBAL edikt home, not a project. A walk
	// that stopped there would treat $HOME/.claude/settings.json, the user
	// file, as the project's — the very confusion --project exists to end.
	sub := filepath.Join(home, "notes")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(sub)
	assertRefused(t, "a directory under $HOME whose only .edikt is the global home")
}
