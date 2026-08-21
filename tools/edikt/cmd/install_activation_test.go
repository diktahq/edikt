package cmd

// install_activation_test.go — F11,
// docs/internal/issues/install-does-not-report-or-perform-activation.md.
//
// `edikt install <tag>` stages a version under versions/<tag>/ but never
// calls writeLock — `edikt version` keeps reporting the prior tag, and
// before this fix install's success path printed nothing explaining why.
// It's easy to believe the install itself failed to take effect. install
// and use are deliberately separate verbs (`edikt use <tag>` already
// existed and does the activation); the fix is just telling the operator
// that split exists, right when they'd otherwise be confused by it.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReportInstallNotActivated_PrintsPointerToUse(t *testing.T) {
	root := t.TempDir()
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	reportInstallNotActivated(root, "0.7.2")
	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if !strings.Contains(out, "not activated") {
		t.Errorf("expected a not-activated notice, got: %q", out)
	}
	if !strings.Contains(out, "edikt use 0.7.2") {
		t.Errorf("expected the message to name the activation command, got: %q", out)
	}
}

func TestReportInstallNotActivated_SilentWhenAlreadyActive(t *testing.T) {
	root := t.TempDir()
	if err := writeLock(root, "0.7.2", "launcher"); err != nil {
		t.Fatal(err)
	}

	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = w
	reportInstallNotActivated(root, "0.7.2")
	w.Close()
	os.Stderr = old

	var buf bytes.Buffer
	buf.ReadFrom(r)
	out := buf.String()

	if out != "" {
		t.Errorf("expected no message when the installed tag is already active, got: %q", out)
	}
}

// End-to-end: a real `edikt install <tag>` from a local directory source
// must print the not-activated notice, and `edikt version` must still
// report the PRIOR active tag afterward — proving install genuinely did
// not activate, matching what it now says about itself.
func TestInstall_LocalSource_ReportsNotActivated(t *testing.T) {
	root := t.TempDir()
	t.Setenv("EDIKT_ROOT", root)
	t.Setenv("CLAUDE_HOME", filepath.Join(root, "claude"))

	// A prior active version, so `edikt version` has something to keep
	// reporting after the new install.
	priorDir := filepath.Join(root, "versions", "0.7.1")
	if err := os.MkdirAll(priorDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(priorDir, "VERSION"), []byte("0.7.1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("versions", "0.7.1"), filepath.Join(root, "current")); err != nil {
		t.Fatal(err)
	}
	if err := writeLock(root, "0.7.1", "launcher"); err != nil {
		t.Fatal(err)
	}

	// A local source directory for the new version to install.
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "VERSION"), []byte("0.7.2\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("EDIKT_INSTALL_SOURCE", source)

	out, err := runCmd(t, "install", "0.7.2")
	if err != nil {
		t.Fatalf("install failed: %v\noutput:\n%s", err, out)
	}
	if !contains(out, "not activated") || !contains(out, "edikt use 0.7.2") {
		t.Fatalf("expected install to report non-activation and name `edikt use 0.7.2`, got:\n%s", out)
	}

	lf, err := readLock(root)
	if err != nil {
		t.Fatal(err)
	}
	if lf.Active != "0.7.1" {
		t.Fatalf("install must not activate the version it staged — lock.yaml active = %q, want %q", lf.Active, "0.7.1")
	}
}
