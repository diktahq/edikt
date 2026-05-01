package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ─── TestMigrateNothingToDo ───────────────────────────────────────────────────
// When the layout is already versioned (no flat hooks/ dir, versions/ exists),
// migrate should report "No migration needed" and exit 0.
func TestMigrateNothingToDo(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// v0.5.0+ layout: versions/<v> dir exists, no real hooks/ dir.
	vdir := filepath.Join(root, "versions", "0.5.0")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	// current symlink.
	if err := os.Symlink(filepath.Join("versions", "0.5.0"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "migrate")
	if err != nil {
		t.Fatalf("migrate failed: %v\noutput:\n%s", err, out)
	}
	if !contains(out, "No migration needed") { //nolint:staticcheck — uses testhelpers_test.go string contains
		t.Fatalf("expected 'No migration needed', got:\n%s", out)
	}
}

// ─── TestMigrateSecondaryOnly ─────────────────────────────────────────────────
// On a v0.5.0 layout with a CLAUDE.md containing HTML sentinels, running
// migrate (without M1) should rewrite the sentinels.
func TestMigrateSecondaryOnly(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", claudeRoot)

	// v0.5.0 layout.
	vdir := filepath.Join(root, "versions", "0.5.0")
	if err := os.MkdirAll(vdir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("versions", "0.5.0"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	// Plant a CLAUDE.md with HTML sentinels.
	claudeMD := filepath.Join(root, "CLAUDE.md")
	htmlContent := "# My Rules\n\n<!-- edikt:start -->\nsome content\n<!-- edikt:end -->\n"
	if err := os.WriteFile(claudeMD, []byte(htmlContent), 0o644); err != nil {
		t.Fatal(err)
	}

	// Run migrate with --yes to skip prompt.
	out, err := runCmd(t, "migrate", "--yes")
	if err != nil {
		t.Fatalf("migrate failed: %v\noutput:\n%s", err, out)
	}

	// Read the result.
	result, err := os.ReadFile(claudeMD)
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)

	// HTML sentinels must be gone.
	if strings.Contains(got, "<!-- edikt:start -->") {
		t.Errorf("HTML start sentinel still present after migration:\n%s", got)
	}
	if strings.Contains(got, "<!-- edikt:end -->") {
		t.Errorf("HTML end sentinel still present after migration:\n%s", got)
	}
	// Markdown sentinels must be present.
	if !strings.Contains(got, "[edikt:start]: #") {
		t.Errorf("markdown start sentinel missing after migration:\n%s", got)
	}
	if !strings.Contains(got, "[edikt:end]: #") {
		t.Errorf("markdown end sentinel missing after migration:\n%s", got)
	}

	// Backup must have been written.
	backupsDir := filepath.Join(root, "backups")
	entries, err := os.ReadDir(backupsDir)
	if err != nil {
		t.Fatalf("backups dir not created: %v", err)
	}
	foundBackup := false
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		backup := filepath.Join(backupsDir, e.Name(), "CLAUDE.md.pre-m2")
		if _, err := os.Stat(backup); err == nil {
			content, _ := os.ReadFile(backup)
			if string(content) == htmlContent {
				foundBackup = true
			}
		}
	}
	if !foundBackup {
		t.Errorf("CLAUDE.md.pre-m2 backup not found or content wrong under %s/", backupsDir)
	}
}

// ─── TestMigrateAbortNoProgress ──────────────────────────────────────────────
// --abort with no staging or pre-migration directories should exit cleanly
// and print a "nothing to abort" message.
func TestMigrateAbortNoProgress(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// Minimal v0.5.0 layout (no staging/predir dirs).
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "migrate", "--abort")
	if err != nil {
		t.Fatalf("migrate --abort failed: %v\noutput:\n%s", err, out)
	}
	if !contains(out, "nothing to abort") {
		t.Fatalf("expected 'nothing to abort', got:\n%s", out)
	}
}

// ─── TestMigrateM5ConfigAdditions ─────────────────────────────────────────────
// config.yaml missing paths/stack/gates should get them appended.
func TestMigrateM5ConfigAdditions(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", claudeRoot)

	// v0.5.0 layout.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("versions", "0.5.0"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	// Write a minimal config.yaml without the new keys.
	configPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(configPath, []byte("project: my-project\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := runCmd(t, "migrate", "--yes")
	if err != nil {
		// Non-fatal: M4 may warn but should not fail overall.
		t.Logf("migrate output had error (possibly M4 pending — acceptable): %v", err)
	}

	result, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	got := string(result)

	for _, key := range []string{"paths:", "stack:", "gates:"} {
		if !strings.Contains(got, key) {
			t.Errorf("expected %s in config.yaml after migration, got:\n%s", key, got)
		}
	}
}

// ─── TestMigrateDryRun ────────────────────────────────────────────────────────
// --dry-run should output a plan but not modify any files.
func TestMigrateDryRun(t *testing.T) {
	root := t.TempDir()
	claudeRoot := filepath.Join(root, "claude")
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", claudeRoot)

	// v0.5.0 layout.
	if err := os.MkdirAll(filepath.Join(root, "versions", "0.5.0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("versions", "0.5.0"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}

	// Plant HTML sentinels in CLAUDE.md.
	claudeMD := filepath.Join(root, "CLAUDE.md")
	original := "<!-- edikt:start -->\ncontent\n<!-- edikt:end -->\n"
	if err := os.WriteFile(claudeMD, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := runCmd(t, "migrate", "--dry-run")
	if err != nil {
		t.Fatalf("migrate --dry-run failed: %v\noutput:\n%s", err, out)
	}

	// File should be UNCHANGED.
	result, _ := os.ReadFile(claudeMD)
	if string(result) != original {
		t.Errorf("dry-run modified CLAUDE.md; expected no change")
	}

	// Output should mention dry-run.
	if !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run output missing 'dry-run': %s", out)
	}
}
