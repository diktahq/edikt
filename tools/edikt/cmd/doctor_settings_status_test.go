package cmd

// doctor_settings_status_test.go — the statusLine.type check, AC-10.1's
// missing half.
//
// Phase 10 moved the two settings.json repairs out of /edikt:upgrade's prose
// into `edikt install settings --repair` and rewrote this check's remedy line
// to name that verb (527d3ce3). AC-10.1's statement claims
// "doctor_settings_status and doctor_hook_paths tests with the new remedy
// text"; doctor_hook_paths_test.go has them, this file did not exist, and the
// criterion's `-run` alternation was satisfied by the hook-paths tests alone.
// So the remedy sentence the phase was FOR shipped unasserted.
//
// The remedy is checked two ways, because a string assertion alone only
// proves the sentence is spelled the same as the day it was written:
//
//  1. the words are there — /edikt:upgrade and `install settings --repair`
//  2. THE REMEDY WORKS — running that repair on the same project makes this
//     check go quiet. A remedy that names a verb which does not fix the
//     finding is worse than no remedy, and no wording check can catch it.
//
// FALSIFIED before trusted, each mutation run against this file:
//   - `type` accepted when absent (drop the !ok branch) -> "must report 1
//     error" fails.
//   - remedy reverted to the pre-Phase-10 wording ("Run /edikt:upgrade to
//     auto-repair, or add \"type\": \"command\" manually") -> the remedy
//     assertion fails while the error-count assertion still passes, which is
//     exactly the gap AC-10.1 left open.
//   - Repair's AddedType branch removed -> the round-trip case fails with the
//     check still firing after the named remedy ran.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// settingsProject writes .claude/settings.json with the given body and
// returns the project root.
func settingsProject(t *testing.T, body string) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runStatusLine(t *testing.T, root string) (string, int, int, bool) {
	t.Helper()
	var buf bytes.Buffer
	errs, warns, ran := runStatusLineTypeCheck(root, &buf)
	return buf.String(), errs, warns, ran
}

// A statusLine block with no `type` invalidates the WHOLE settings file from
// the host's point of view — every hook stops firing too — so it is an ERROR,
// and the error carries the remedy Phase 10 moved it to.
func TestDoctorSettingsStatus_MissingTypeIsAnErrorWithTheRemedy(t *testing.T) {
	root := settingsProject(t, `{"statusLine":{"command":"~/.claude/statusline.sh"}}`)
	out, errs, warns, ran := runStatusLine(t, root)
	if !ran {
		t.Fatal("check did not run on a project with a settings.json")
	}
	if errs != 1 || warns != 0 {
		t.Fatalf("a statusLine with no type must report 1 error, 0 warnings; got %d/%d:\n%s", errs, warns, out)
	}
	if !strings.Contains(out, "Claude Code refuses to load the entire settings file") {
		t.Errorf("the finding does not name the consequence:\n%s", out)
	}
	// The new remedy text: the repair is a verb now, not prose in upgrade.md.
	if !strings.Contains(out, "/edikt:upgrade") {
		t.Errorf("the remedy does not name the command the user runs:\n%s", out)
	}
	if !strings.Contains(out, "install settings --repair") {
		t.Errorf("the remedy does not name the repair verb Phase 10 moved it to:\n%s", out)
	}
	// This check reads the PROJECT's settings file, so its remedy must act on
	// that file: plain --repair resolves the user-scope one (#11, item 7).
	if !strings.Contains(out, "install settings --repair --project") {
		t.Errorf("the remedy names a repair that acts on a different file than the one checked:\n%s", out)
	}
}

// The remedy RESOLVES the finding. doctor stays read-only, so the check
// cannot run the repair itself — but the test can, and must: this is the only
// assertion that would survive the repair verb being renamed, gutted, or
// losing its statusLine branch while the remedy sentence stayed word-perfect.
func TestDoctorSettingsStatus_NamedRemedyClearsTheFinding(t *testing.T) {
	root := settingsProject(t, `{"statusLine":{"command":"~/.claude/statusline.sh"}}`)
	if _, errs, _, _ := runStatusLine(t, root); errs != 1 {
		t.Fatalf("precondition: the check must fire before the repair; got %d error(s)", errs)
	}

	// Run the VERB the remedy names, from inside the project, with the user
	// scope isolated and holding the same defect. Calling installsettings.Repair
	// on the project path directly — as this test once did — proved the
	// library repairs whatever path it is handed and said nothing about which
	// file the named command acts on, which was the defect (#11, item 7).
	// EDIKT_HOOK_DIR is unset: a statusLine-only repair needs no hook dir.
	_, user, _ := isolateSettingsEnv(t)
	t.Setenv("EDIKT_HOOK_DIR", "")
	userBefore, _ := os.ReadFile(user)
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	if err := runInstallSettingsRepair(true); err != nil {
		t.Fatalf("the remedy doctor names failed to run: %v", err)
	}
	if userAfter, _ := os.ReadFile(user); string(userAfter) != string(userBefore) {
		t.Fatal("the remedy for a project finding modified the user-scope settings file")
	}

	out, errs, warns, ran := runStatusLine(t, root)
	if !ran {
		t.Fatal("check stopped running after the repair")
	}
	if errs != 0 || warns != 0 {
		t.Fatalf("after running the remedy the finding must be gone; got %d/%d:\n%s", errs, warns, out)
	}
}

// Negative control. Without it, "missing type is an error" is satisfied by a
// check that errors on every settings.json it can read.
func TestDoctorSettingsStatus_TypePresentIsQuiet(t *testing.T) {
	root := settingsProject(t, `{"statusLine":{"type":"command","command":"~/.claude/statusline.sh"}}`)
	out, errs, warns, ran := runStatusLine(t, root)
	if !ran {
		t.Fatal("check did not run on a project with a settings.json")
	}
	if errs != 0 || warns != 0 {
		t.Errorf("a well-formed statusLine must be quiet; got %d/%d:\n%s", errs, warns, out)
	}
	if out != "" {
		t.Errorf("a well-formed statusLine printed output:\n%s", out)
	}
}

// No statusLine at all is a legitimate shape — the block is optional — and
// must not be reported as a defect.
func TestDoctorSettingsStatus_NoStatusLineBlockIsQuiet(t *testing.T) {
	root := settingsProject(t, `{"hooks":{}}`)
	out, errs, warns, ran := runStatusLine(t, root)
	if !ran || errs != 0 || warns != 0 {
		t.Errorf("a settings.json with no statusLine must run and stay quiet; got ran=%v %d/%d:\n%s", ran, errs, warns, out)
	}
}

// INV-013: an unreadable or unparseable settings.json is UNMEASURED, not
// healthy. ran=false is the only honest answer — the check looked and could
// not tell. A `true` here would let a corrupt settings.json read as a project
// whose statusLine was checked and found fine.
func TestDoctorSettingsStatus_UnmeasuredWhenItCannotLook(t *testing.T) {
	if _, _, _, ran := runStatusLine(t, t.TempDir()); ran {
		t.Error("check claimed to run on a project with no settings.json")
	}
	root := settingsProject(t, `{"statusLine": {`)
	if _, errs, warns, ran := runStatusLine(t, root); ran || errs != 0 || warns != 0 {
		t.Errorf("unparseable settings.json must be unmeasured, not passed; got ran=%v %d/%d", ran, errs, warns)
	}
}
