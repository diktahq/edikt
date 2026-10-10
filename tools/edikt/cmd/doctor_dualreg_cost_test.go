package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/hookmatch"
)

// A hook registered at BOTH scopes fires twice per event. That ran for weeks
// on the reference machine as a WARN printed on every run and acted on by
// nobody, so FR-034 makes it RED and requires the measured cost from the hook
// journal beside it — the count of doubled hooks is the property; the pairs
// per event come from what actually fired.

func writeSettingsWithHooks(t *testing.T, dir string, byEvent map[string][]string) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	hooks := map[string]any{}
	for event, cmds := range byEvent {
		var entries []map[string]any
		for _, c := range cmds {
			entries = append(entries, map[string]any{"type": "command", "command": c})
		}
		hooks[event] = []map[string]any{{"hooks": entries}}
	}
	b, err := json.MarshalIndent(map[string]any{"hooks": hooks}, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, append(b, '\n'), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestDualReg_RedWithCost(t *testing.T) {
	claudeRoot := t.TempDir()
	projectRoot := t.TempDir()
	both := map[string][]string{
		"SessionStart": {"/h/session-start.sh"},
		"Stop":         {"/h/stop-hook.sh"},
	}
	writeSettingsWithHooks(t, claudeRoot, both)
	writeSettingsWithHooks(t, filepath.Join(projectRoot, ".claude"), both)

	var out bytes.Buffer
	errN, warnN := runDualRegistrationCheckWithCost(claudeRoot, projectRoot, strings.NewReader(""), false,
		hookmatch.Summary{JournalFound: true, Total: 40, ByHook: map[string]int{"session-start": 20, "stop-hook": 20}}, &out)

	if errN == 0 {
		t.Fatalf("a doubled registration must be RED, not a warning: errN=%d warnN=%d\n%s", errN, warnN, out.String())
	}
	s := out.String()
	for _, want := range []string{"🔴", "2 hook(s)", "twice per event"} {
		if !strings.Contains(s, want) {
			t.Errorf("the finding must carry %q:\n%s", want, s)
		}
	}
	// The measured cost, from the journal — not an estimate.
	if !strings.Contains(s, "journal") {
		t.Errorf("the finding must name the journal as the source of the cost:\n%s", s)
	}
	if !strings.Contains(s, "40") {
		t.Errorf("the finding must carry the measured invocation count:\n%s", s)
	}
}

// A journal that has never been written is not a zero cost. The finding still
// fires — the doubling is visible in the settings — and the cost says
// UNMEASURED rather than implying none (INV-013).
func TestDualReg_RedWithNoJournalSaysUnmeasured(t *testing.T) {
	claudeRoot := t.TempDir()
	projectRoot := t.TempDir()
	both := map[string][]string{"SessionStart": {"/h/session-start.sh"}}
	writeSettingsWithHooks(t, claudeRoot, both)
	writeSettingsWithHooks(t, filepath.Join(projectRoot, ".claude"), both)

	var out bytes.Buffer
	errN, _ := runDualRegistrationCheckWithCost(claudeRoot, projectRoot, strings.NewReader(""), false,
		hookmatch.Summary{JournalFound: false}, &out)
	if errN == 0 {
		t.Fatal("the doubling is visible in settings.json regardless of the journal")
	}
	if !strings.Contains(out.String(), "UNMEASURED") {
		t.Errorf("a journal that never ran must read as UNMEASURED, not as zero cost:\n%s", out.String())
	}
}

// The user-scope settings file may be a symlink into a dotfiles checkout.
// The remedy must edit the RESOLVED file and say so, or it writes a regular
// file over the link and the user's dotfiles silently stop being the source.
func TestRemoveUserHooks_ResolvedPathOnly(t *testing.T) {
	dotfiles := t.TempDir()
	real := writeSettingsWithHooks(t, dotfiles, map[string][]string{
		"SessionStart": {"/h/session-start.sh", "/h/keep-mine.sh"},
	})
	claudeRoot := t.TempDir()
	link := filepath.Join(claudeRoot, "settings.json")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	if err := removeUserHookEntries(claudeRoot, []string{"SessionStart/session-start.sh"}); err != nil {
		t.Fatal(err)
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("the remedy replaced the symlink with a regular file — the user's dotfiles are no longer the source")
	}
	body, err := os.ReadFile(real)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "session-start.sh") {
		t.Errorf("the duplicated entry was not removed from the resolved file:\n%s", body)
	}
	if !strings.Contains(string(body), "keep-mine.sh") {
		t.Errorf("a user's own hook was removed:\n%s", body)
	}
	// The backup sits beside the resolved file, not beside the link.
	if _, err := os.Stat(real + ".bak"); err != nil {
		t.Errorf("no backup beside the resolved file: %v", err)
	}
}
