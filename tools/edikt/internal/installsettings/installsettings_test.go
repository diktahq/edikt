package installsettings

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testTemplate = `{
  "permissions": {
    "defaultMode": "askBeforeAllow",
    "deny": [
      "Bash(sudo **)"
    ],
    "allow": [
      "Read(**)"
    ]
  },
  "hooks": {
    "SessionStart": [
      {
        "hooks": [
          {
            "type": "command",
            "command": "${EDIKT_HOOK_DIR}/session-start.sh"
          }
        ]
      }
    ]
  }
}
`

func writeTestTemplate(t *testing.T, dir string) string {
	t.Helper()
	p := filepath.Join(dir, "settings.json.tmpl")
	if err := os.WriteFile(p, []byte(testTemplate), 0o644); err != nil {
		t.Fatalf("writing test template: %v", err)
	}
	return p
}

// TestSettingsWriteDeterminism pins AC-10.1: two independent runs over the
// same inputs must produce byte-identical settings.json.
func TestSettingsWriteDeterminism(t *testing.T) {
	tmplPath := writeTestTemplate(t, t.TempDir())

	run := func() []byte {
		destDir := t.TempDir()
		dest := filepath.Join(destDir, "settings.json")
		sidecar := filepath.Join(destDir, "state", "settings-managed.json")
		if _, err := Write(Options{
			TemplatePath: tmplPath,
			SettingsPath: dest,
			SidecarPath:  sidecar,
			HookDir:      "/opt/edikt/hooks",
			BackupHome:   t.TempDir(), // isolate from real $HOME
		}); err != nil {
			t.Fatalf("Write: %v", err)
		}
		data, err := os.ReadFile(dest)
		if err != nil {
			t.Fatalf("reading dest: %v", err)
		}
		return data
	}

	first := run()
	second := run()
	if string(first) != string(second) {
		t.Fatalf("two runs produced different settings.json:\n--- first ---\n%s\n--- second ---\n%s", first, second)
	}
}

// TestBackupOneShot pins AC-10.2: an existing settings.json is backed up
// exactly once, guarded by the marker file, and both the marker and the
// backup copy are mode 0600.
func TestBackupOneShot(t *testing.T) {
	tmplPath := writeTestTemplate(t, t.TempDir())
	claudeDir := t.TempDir()
	homeDir := t.TempDir()
	dest := filepath.Join(claudeDir, "settings.json")
	sidecar := filepath.Join(claudeDir, "state", "settings-managed.json")

	if err := os.WriteFile(dest, []byte(`{"existing": true}`), 0o644); err != nil {
		t.Fatal(err)
	}

	opts := Options{TemplatePath: tmplPath, SettingsPath: dest, SidecarPath: sidecar, HookDir: "/opt/edikt/hooks", BackupHome: homeDir}
	if _, err := Write(opts); err != nil {
		t.Fatalf("first write: %v", err)
	}

	backupRoot := filepath.Join(homeDir, ".edikt", "backup")
	marker := filepath.Join(backupRoot, "pre-v0.6.0-marker")
	markerInfo, err := os.Stat(marker)
	if err != nil {
		t.Fatalf("expected marker file to exist: %v", err)
	}
	if markerInfo.Mode().Perm() != 0o600 {
		t.Errorf("marker mode = %v, want 0600", markerInfo.Mode().Perm())
	}

	listBackupDirs := func() []string {
		entries, err := os.ReadDir(backupRoot)
		if err != nil {
			t.Fatal(err)
		}
		var dirs []string
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), "pre-v0.6.0-") {
				dirs = append(dirs, e.Name())
			}
		}
		return dirs
	}

	dirs := listBackupDirs()
	if len(dirs) != 1 {
		t.Fatalf("expected exactly 1 backup dir after first write, got %d: %v", len(dirs), dirs)
	}
	backupFile := filepath.Join(backupRoot, dirs[0], "settings.json")
	bInfo, err := os.Stat(backupFile)
	if err != nil {
		t.Fatalf("expected backup file: %v", err)
	}
	if bInfo.Mode().Perm() != 0o600 {
		t.Errorf("backup file mode = %v, want 0600", bInfo.Mode().Perm())
	}
	backedUp, err := os.ReadFile(backupFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(backedUp), `"existing"`) {
		t.Errorf("backup did not preserve pre-existing content: %s", backedUp)
	}

	// A second write over an again-pre-existing dest must NOT create a
	// second backup dir — the marker makes this one-shot.
	if _, err := Write(opts); err != nil {
		t.Fatalf("second write: %v", err)
	}
	dirs2 := listBackupDirs()
	if len(dirs2) != 1 {
		t.Fatalf("expected still exactly 1 backup dir after second write (one-shot), got %d: %v", len(dirs2), dirs2)
	}
}

// TestHookDirValidation pins AC-10.3: the allowlist (^/[A-Za-z0-9._/-]+$,
// no "..") must reject $, backticks, parentheses, and semicolons — every
// one of which install.sh's old denylist (install.sh:844-849) permitted
// through into a shell-interpreted command string.
func TestHookDirValidation(t *testing.T) {
	valid := []string{
		"/home/user/.edikt/hooks",
		"/opt/edikt-hooks/v0.8.0",
		"/Users/me/My.Project/.edikt/hooks",
	}
	for _, v := range valid {
		if err := ValidateHookDir(v); err != nil {
			t.Errorf("ValidateHookDir(%q) = %v, want nil", v, err)
		}
	}

	invalid := []string{
		"/home/$(whoami)/hooks",
		"/home/`whoami`/hooks",
		"/home/user;rm -rf /",
		"/home/(evil)/hooks",
		"/home/user/hooks && rm -rf /",
		"/home/../etc/passwd",
		"relative/not/absolute",
		"",
	}
	for _, v := range invalid {
		if err := ValidateHookDir(v); err == nil {
			t.Errorf("ValidateHookDir(%q) = nil, want an error (a shell-denylist-permitted character must be rejected by the allowlist)", v)
		} else {
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Errorf("ValidateHookDir(%q) returned %T, want *ValidationError", v, err)
			}
		}
	}
}

// TestAtomicWrite pins AC-10.6: settings.json is written via a
// same-directory temp file, fsync, and rename — no leftover temp file, and
// the destination is always valid, fully-substituted JSON.
func TestAtomicWrite(t *testing.T) {
	tmplPath := writeTestTemplate(t, t.TempDir())
	claudeDir := t.TempDir()
	dest := filepath.Join(claudeDir, "settings.json")
	sidecar := filepath.Join(claudeDir, "state", "settings-managed.json")

	if _, err := Write(Options{TemplatePath: tmplPath, SettingsPath: dest, SidecarPath: sidecar, HookDir: "/opt/edikt/hooks", BackupHome: t.TempDir()}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	entries, err := os.ReadDir(claudeDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if e.Name() != "settings.json" && e.Name() != "state" {
			t.Errorf("unexpected leftover entry in dest dir: %s (an atomic write should leave no tmp files)", e.Name())
		}
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("dest is not valid JSON: %v", err)
	}
	if parsed["permissions"] == nil {
		t.Fatal("permissions key missing from written settings.json")
	}

	hooks, _ := parsed["hooks"].(map[string]interface{})
	sessionStart, _ := hooks["SessionStart"].([]interface{})
	if len(sessionStart) == 0 {
		t.Fatal("SessionStart hook missing")
	}
	entry := sessionStart[0].(map[string]interface{})
	hooksArr := entry["hooks"].([]interface{})
	cmdVal := hooksArr[0].(map[string]interface{})["command"].(string)
	want := "/opt/edikt/hooks/session-start.sh"
	if cmdVal != want {
		t.Errorf("hook command = %q, want %q (EDIKT_HOOK_DIR substitution)", cmdVal, want)
	}
}

// TestManagedSidecarPermissionsAndHash pins AC-10.7: the sidecar is 0600
// and its managed_hash matches sha256 over the canonicalized `permissions`
// key of the ACTUAL written settings.json.
func TestManagedSidecarPermissionsAndHash(t *testing.T) {
	tmplPath := writeTestTemplate(t, t.TempDir())
	claudeDir := t.TempDir()
	dest := filepath.Join(claudeDir, "settings.json")
	sidecar := filepath.Join(claudeDir, "state", "settings-managed.json")

	if _, err := Write(Options{TemplatePath: tmplPath, SettingsPath: dest, SidecarPath: sidecar, HookDir: "/opt/edikt/hooks", BackupHome: t.TempDir()}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	info, err := os.Stat(sidecar)
	if err != nil {
		t.Fatalf("sidecar not written: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("sidecar mode = %v, want 0600", info.Mode().Perm())
	}

	sidecarData, err := os.ReadFile(sidecar)
	if err != nil {
		t.Fatal(err)
	}
	var got managedSidecar
	if err := json.Unmarshal(sidecarData, &got); err != nil {
		t.Fatalf("sidecar is not valid JSON: %v", err)
	}
	if got.SentinelVersion != 1 {
		t.Errorf("sentinel_version = %d, want 1", got.SentinelVersion)
	}
	if len(got.ManagedKeys) != 1 || got.ManagedKeys[0] != "permissions" {
		t.Errorf("managed_keys = %v, want [permissions]", got.ManagedKeys)
	}

	settingsData, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	var full map[string]interface{}
	if err := json.Unmarshal(settingsData, &full); err != nil {
		t.Fatal(err)
	}
	managed := map[string]interface{}{"permissions": full["permissions"]}
	canonical, err := json.Marshal(managed)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(canonical)
	want := hex.EncodeToString(sum[:])
	if got.ManagedHash != want {
		t.Errorf("managed_hash = %s, want %s (sha256 of the written settings.json's own permissions key)", got.ManagedHash, want)
	}
}

// TestManagedSidecarWriteFailureIsFatal pins the constraint that the
// sidecar write failing must be FATAL, not a swallowed `|| true` the way
// install.sh:911 used to. It points the sidecar at a path whose parent is
// a file (not a directory), so the write structurally cannot succeed.
func TestManagedSidecarWriteFailureIsFatal(t *testing.T) {
	tmplPath := writeTestTemplate(t, t.TempDir())
	claudeDir := t.TempDir()
	dest := filepath.Join(claudeDir, "settings.json")

	blocker := filepath.Join(claudeDir, "blocker-file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	sidecar := filepath.Join(blocker, "settings-managed.json")

	_, err := Write(Options{TemplatePath: tmplPath, SettingsPath: dest, SidecarPath: sidecar, HookDir: "/opt/edikt/hooks", BackupHome: t.TempDir()})
	if err == nil {
		t.Fatal("expected an error when the sidecar cannot be written, got nil")
	}
	var fatal *FatalSidecarError
	if !errors.As(err, &fatal) {
		t.Fatalf("expected *FatalSidecarError, got %T: %v", err, err)
	}

	if _, statErr := os.Stat(dest); statErr != nil {
		t.Errorf("settings.json should still have been written even though the sidecar failed: %v", statErr)
	}
}

// TestCapabilityPreflightFailsOnVersionMismatch pins AC-10.5: when the
// injected probe reports that a staged binary does NOT support
// `install settings`, CheckCapability returns an error naming the version
// it actually reported and the version the verb requires.
func TestCapabilityPreflightFailsOnVersionMismatch(t *testing.T) {
	probe := func(path string) (bool, string, error) {
		return false, "0.7.2", nil
	}
	err := CheckCapability("/staged/bin/edikt", probe)
	if err == nil {
		t.Fatal("expected an error for a binary that does not support install settings")
	}
	if !strings.Contains(err.Error(), "0.7.2") {
		t.Errorf("error should name the reported version: %v", err)
	}
	if !strings.Contains(err.Error(), "0.8.0") {
		t.Errorf("error should name the required version: %v", err)
	}
}

// TestCapabilityPreflightPassesOnSupportedVersion is the positive
// complement — a probe reporting support must not produce an error.
func TestCapabilityPreflightPassesOnSupportedVersion(t *testing.T) {
	probe := func(path string) (bool, string, error) {
		return true, "0.8.0", nil
	}
	if err := CheckCapability("/staged/bin/edikt", probe); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

// TestCapabilityPreflightSurfacesProbeError proves a probe failure (binary
// missing, not executable, crashed) is reported as an error too, rather
// than silently treated as "supported".
func TestCapabilityPreflightSurfacesProbeError(t *testing.T) {
	probe := func(path string) (bool, string, error) {
		return false, "", errors.New("exec: permission denied")
	}
	if err := CheckCapability("/staged/bin/edikt", probe); err == nil {
		t.Fatal("expected an error when the probe itself fails")
	}
}
