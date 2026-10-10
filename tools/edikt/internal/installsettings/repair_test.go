package installsettings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeSettings(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "settings.json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRepair_SubstitutesPlaceholderAndKeepsTheRest(t *testing.T) {
	p := writeSettings(t, `{"permissions":{"deny":["Bash(rm -rf *)"]},"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"${EDIKT_HOOK_DIR}/pre-tool-use.sh"}]}]},"custom":{"mine":true}}`)
	rep, err := Repair(Options{SettingsPath: p, HookDir: "/home/u/.edikt/hooks"})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Substituted != 1 || rep.AddedType {
		t.Fatalf("report: %+v", rep)
	}
	out, _ := os.ReadFile(p)
	if strings.Contains(string(out), "${EDIKT_HOOK_DIR}") || !strings.Contains(string(out), "/home/u/.edikt/hooks/pre-tool-use.sh") {
		t.Fatalf("placeholder not substituted:\n%s", out)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["custom"].(map[string]interface{})["mine"] != true {
		t.Fatal("a user key was lost by the repair")
	}
}

func TestRepair_AddsStatusLineType(t *testing.T) {
	p := writeSettings(t, `{"statusLine":{"command":"~/.edikt/statusline.sh"}}`)
	rep, err := Repair(Options{SettingsPath: p, HookDir: "/home/u/.edikt/hooks"})
	if err != nil || !rep.AddedType || rep.Substituted != 0 {
		t.Fatalf("err=%v report=%+v", err, rep)
	}
	out, _ := os.ReadFile(p)
	var m map[string]interface{}
	_ = json.Unmarshal(out, &m)
	if m["statusLine"].(map[string]interface{})["type"] != "command" {
		t.Fatalf("statusLine.type not added:\n%s", out)
	}
}

func TestRepair_NothingToRepairLeavesTheFileUntouched(t *testing.T) {
	body := `{"statusLine":{"type":"command","command":"x"},"hooks":{}}` + "\n"
	p := writeSettings(t, body)
	rep, err := Repair(Options{SettingsPath: p, HookDir: "/home/u/.edikt/hooks"})
	if err != nil || !rep.Empty() {
		t.Fatalf("err=%v report=%+v", err, rep)
	}
	out, _ := os.ReadFile(p)
	if string(out) != body {
		t.Fatal("a no-op repair rewrote the file")
	}
}

func TestRepair_InvalidJSONIsAnError(t *testing.T) {
	p := writeSettings(t, `{"hooks": [`)
	if _, err := Repair(Options{SettingsPath: p, HookDir: "/home/u/.edikt/hooks"}); err == nil {
		t.Fatal("malformed settings must be an error, not a silent no-op")
	}
}

// TestRepair_StatusLineOnlyWithEmptyHookDir is #11 comment item 7's first
// half. A statusLine with no `type` needs no hook dir to repair, but Repair
// validated EDIKT_HOOK_DIR before reading the file, so the remedy doctor
// names (`install settings --repair`) refused with "EDIKT_HOOK_DIR is empty"
// on a file whose only defect had nothing to do with hooks. The hook dir is
// an input to ONE of the two repairs; it is validated when that repair runs.
//
// The control: a file that DOES carry the placeholder still refuses an empty
// hook dir, and is left byte-identical — skipping validation must not turn
// into substituting an empty string.
func TestRepair_StatusLineOnlyWithEmptyHookDir(t *testing.T) {
	p := writeSettings(t, `{"statusLine":{"command":"~/.edikt/statusline.sh"}}`)
	rep, err := Repair(Options{SettingsPath: p, HookDir: ""})
	if err != nil {
		t.Fatalf("a statusLine-only repair must not need EDIKT_HOOK_DIR, got: %v", err)
	}
	if !rep.AddedType || rep.Substituted != 0 {
		t.Fatalf("report: %+v", rep)
	}
	out, _ := os.ReadFile(p)
	var m map[string]interface{}
	if err := json.Unmarshal(out, &m); err != nil {
		t.Fatal(err)
	}
	if m["statusLine"].(map[string]interface{})["type"] != "command" {
		t.Fatalf("statusLine.type not added:\n%s", out)
	}

	body := `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"${EDIKT_HOOK_DIR}/pre-tool-use.sh"}]}]}}`
	q := writeSettings(t, body)
	if _, err := Repair(Options{SettingsPath: q, HookDir: ""}); err == nil {
		t.Fatal("a placeholder repair with an empty hook dir must refuse, not substitute an empty string")
	}
	if got, _ := os.ReadFile(q); string(got) != body {
		t.Fatalf("a refused repair rewrote the file:\n%s", got)
	}
}

// TestRepair_PortableHookDir: substituting a portable token must yield a
// settings file that still parses and whose command, once decoded, is the
// token form the host's shell expands. The quoted project token carries '"',
// so a raw text substitution into a JSON string would end the string early;
// the substitution is JSON-escaped.
func TestRepair_PortableHookDir(t *testing.T) {
	for _, dir := range []string{`"$CLAUDE_PROJECT_DIR"/.edikt/hooks`, "$HOME/.edikt/hooks"} {
		t.Run(dir, func(t *testing.T) {
			p := writeSettings(t, `{"hooks":{"PreToolUse":[{"hooks":[{"type":"command","command":"${EDIKT_HOOK_DIR}/pre-tool-use.sh"}]}]}}`)
			rep, err := Repair(Options{SettingsPath: p, HookDir: dir})
			if err != nil {
				t.Fatalf("Repair with portable hook dir %q: %v", dir, err)
			}
			if rep.Substituted != 1 {
				t.Fatalf("report: %+v", rep)
			}
			out, _ := os.ReadFile(p)
			var m struct {
				Hooks map[string][]struct {
					Hooks []struct {
						Command string `json:"command"`
					} `json:"hooks"`
				} `json:"hooks"`
			}
			if err := json.Unmarshal(out, &m); err != nil {
				t.Fatalf("repaired file is not valid JSON: %v\n%s", err, out)
			}
			if got, want := m.Hooks["PreToolUse"][0].Hooks[0].Command, dir+"/pre-tool-use.sh"; got != want {
				t.Fatalf("command = %q, want %q", got, want)
			}
		})
	}
}

// TestRepair_WritesThroughASymlink: a user-scope settings.json is commonly a
// symlink into a dotfiles checkout (commands/upgrade.md resolves it for
// exactly that reason). Repair writes atomically — temp file, rename — and a
// rename onto the LINK replaces it with a regular file, silently detaching
// the user's settings from their dotfiles. /edikt:upgrade now offers this
// repair on the user-scope file, so the write must land on the link's target
// and leave the link in place.
func TestRepair_WritesThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "dotfiles", "claude-settings.json")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte(`{"statusLine":{"command":"x"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "settings.json")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Repair(Options{SettingsPath: link}); err != nil {
		t.Fatal(err)
	}
	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the repair replaced the symlink with a regular file — the user's dotfiles link is gone")
	}
	got, _ := os.ReadFile(target)
	if !strings.Contains(string(got), `"type": "command"`) {
		t.Fatalf("the link's target was not repaired:\n%s", got)
	}
}
