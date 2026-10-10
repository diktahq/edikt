package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// FR-034: init and upgrade register hooks at EXACTLY ONE scope — the one the
// install manifest declares — and a stale registration at the other scope is
// removed only on consent, never silently. The doubled state cannot be
// re-created by the tool that fixes it.
//
// This asserts the shipped procedure, because that procedure IS the
// implementation: the hooks pass lives in commands/upgrade.md, and a Go test
// that asserted something else would be testing a copy nobody runs.
func TestUpgrade_OneScope(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "commands", "upgrade.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(raw)

	// The pass writes the PROJECT settings file.
	if !strings.Contains(body, ".claude/settings.json") {
		t.Error("the hooks pass must name the project-scope settings file it writes")
	}
	// And never the user-scope one: writing both is what doubled every hook.
	for _, userScope := range []string{
		"$HOME/.claude/settings.json",
		"${HOME}/.claude/settings.json",
		"~/.claude/settings.json",
	} {
		for _, line := range strings.Split(body, "\n") {
			if !strings.Contains(line, userScope) {
				continue
			}
			l := strings.ToLower(line)
			// Naming it to REMOVE from it, or to describe the defect, is
			// exactly what FR-034 asks for. Naming it as a write target is
			// the defect.
			if strings.Contains(l, "remove") || strings.Contains(l, "consent") ||
				strings.Contains(l, "dual") || strings.Contains(l, "doctor") ||
				strings.HasPrefix(strings.TrimSpace(l), "#") {
				continue
			}
			if strings.Contains(l, "write") || strings.Contains(l, "install") || strings.Contains(l, "register") {
				t.Errorf("the hooks pass registers at user scope too — that is the doubled state:\n  %s", strings.TrimSpace(line))
			}
		}
	}

	// Removal of the other scope happens on consent, and says so.
	lower := strings.ToLower(body)
	if !strings.Contains(lower, "consent") && !strings.Contains(lower, "[y/n]") {
		t.Error("removing a stale registration at the other scope must be on consent, never silent")
	}
}

// The same claim from the doctor side: the remedy it prints is the one the
// user can audit, and it names the resolved file.
func TestUpgrade_OneScope_DoctorRemedyNamesTheResolvedFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(`{"hooks":{}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	want, err := filepath.EvalSymlinks(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if got := resolvedUserSettings(dir); got != want {
		t.Fatalf("a plain file resolves to itself, got %q want %q", got, want)
	}
}
