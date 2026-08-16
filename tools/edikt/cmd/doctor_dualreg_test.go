package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-050 §5 — dual-scope hook registration: doctor detects, OFFERS removal
// on consent, and NEVER silently edits the user-level settings.

func stageDual(t *testing.T) (claudeRoot, projectRoot string) {
	t.Helper()
	claudeRoot, projectRoot = t.TempDir(), t.TempDir()
	user := `{"hooks": {"Stop": [{"hooks": [
	  {"type": "command", "command": "/x/hooks/stop-hook.sh"},
	  {"type": "command", "command": "/x/hooks/phase-end-detector.sh"}]}],
	  "SubagentStop": [{"hooks": [{"type": "command", "command": "/x/hooks/subagent-stop.sh"}]}]},
	  "permissions": {"allow": ["Bash(git :*)"]}}`
	proj := `{"hooks": {"Stop": [{"hooks": [{"type": "command", "command": "${EDIKT_HOOK_DIR}/stop-hook.sh"}]}]}}`
	if err := os.WriteFile(filepath.Join(claudeRoot, "settings.json"), []byte(user), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(projectRoot, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectRoot, ".claude", "settings.json"), []byte(proj), 0o644); err != nil {
		t.Fatal(err)
	}
	return
}

func TestDualReg_DetectsOverlapOnly(t *testing.T) {
	claudeRoot, projectRoot := stageDual(t)
	dups := dualHookRegistrations(claudeRoot, projectRoot)
	if len(dups) != 1 || dups[0] != "Stop/stop-hook.sh" {
		t.Fatalf("expected exactly Stop/stop-hook.sh, got %v", dups)
	}
}

func TestDualReg_NonInteractiveNeverEdits(t *testing.T) {
	claudeRoot, projectRoot := stageDual(t)
	before, _ := os.ReadFile(filepath.Join(claudeRoot, "settings.json"))
	var out strings.Builder
	n := runDualRegistrationCheck(claudeRoot, projectRoot, strings.NewReader(""), false, &out)
	if n != 1 {
		t.Fatalf("expected 1 warning, got %d", n)
	}
	after, _ := os.ReadFile(filepath.Join(claudeRoot, "settings.json"))
	if string(before) != string(after) {
		t.Fatal("non-interactive run MUST NOT edit user settings")
	}
	if !strings.Contains(out.String(), "dual: Stop/stop-hook.sh") {
		t.Fatalf("warning must name the dual registration:\n%s", out.String())
	}
}

func TestDualReg_DeclineLeavesFile(t *testing.T) {
	claudeRoot, projectRoot := stageDual(t)
	before, _ := os.ReadFile(filepath.Join(claudeRoot, "settings.json"))
	var out strings.Builder
	runDualRegistrationCheck(claudeRoot, projectRoot, strings.NewReader("n\n"), true, &out)
	after, _ := os.ReadFile(filepath.Join(claudeRoot, "settings.json"))
	if string(before) != string(after) {
		t.Fatal("declined prompt MUST leave user settings untouched")
	}
}

func TestDualReg_ConsentRemovesOnlyDuplicates(t *testing.T) {
	claudeRoot, projectRoot := stageDual(t)
	var out strings.Builder
	runDualRegistrationCheck(claudeRoot, projectRoot, strings.NewReader("y\n"), true, &out)

	raw, _ := os.ReadFile(filepath.Join(claudeRoot, "settings.json"))
	var d map[string]any
	if err := json.Unmarshal(raw, &d); err != nil {
		t.Fatalf("user settings no longer parse: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, "stop-hook.sh") {
		t.Fatal("duplicated Stop/stop-hook.sh must be removed from user scope")
	}
	for _, keep := range []string{"phase-end-detector.sh", "subagent-stop.sh", "permissions"} {
		if !strings.Contains(s, keep) {
			t.Fatalf("non-duplicated content %q must be preserved:\n%s", keep, s)
		}
	}
	if _, err := os.Stat(filepath.Join(claudeRoot, "settings.json.bak")); err != nil {
		t.Fatal("a .bak must be written before rewriting user settings")
	}
}
