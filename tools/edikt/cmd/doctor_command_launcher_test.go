package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeCommand(t *testing.T, dir, name, body string) {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// An installed command body naming the dev-only `bin/edikt` is reported:
// a user's project has no bin/, so following it produces "no such file"
// and sends the agent hunting for the binary (D81 a).
func TestDoctorCommandLauncher_DevPathIsReported(t *testing.T) {
	dir := t.TempDir()
	writeCommand(t, dir, "gov/compile.md", "Run:\n\n```bash\nbin/edikt gov compile\n```\n")
	writeCommand(t, dir, "doctor.md", "Run `edikt doctor`.\n")
	var out bytes.Buffer
	if got := scanCommandLauncherPaths(dir, &out); got != 1 {
		t.Fatalf("want 1 warning, got %d\n%s", got, out.String())
	}
	s := out.String()
	for _, want := range []string{"bin/edikt", "gov/compile.md", "/edikt:upgrade", "on PATH"} {
		if !strings.Contains(s, want) {
			t.Errorf("report does not mention %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "doctor.md") {
		t.Errorf("a clean body must not be named:\n%s", s)
	}
}

// The project-mode install path is correct and must not be reported, or the
// warning would push authors off the one path a project install uses.
func TestDoctorCommandLauncher_ProjectModePathIsNotReported(t *testing.T) {
	dir := t.TempDir()
	writeCommand(t, dir, "upgrade.md", "if [ -x \".edikt/bin/edikt\" ]; then EDIKT_BIN=\".edikt/bin/edikt\"; fi\n")
	var out bytes.Buffer
	if got := scanCommandLauncherPaths(dir, &out); got != 0 {
		t.Fatalf("project-mode path must not warn; got %d\n%s", got, out.String())
	}
}

// Nothing to report is silence, and a missing directory is not a finding.
func TestDoctorCommandLauncher_CleanTreeAndMissingDirAreSilent(t *testing.T) {
	dir := t.TempDir()
	writeCommand(t, dir, "status.md", "Run `edikt doctor` and `edikt verify`.\n")
	var out bytes.Buffer
	if got := scanCommandLauncherPaths(dir, &out); got != 0 || out.Len() != 0 {
		t.Fatalf("clean tree must be silent; got %d\n%s", got, out.String())
	}
	out.Reset()
	if got := scanCommandLauncherPaths(filepath.Join(dir, "nope"), &out); got != 0 || out.Len() != 0 {
		t.Fatalf("missing dir must be silent; got %d\n%s", got, out.String())
	}
}

// The real shipped commands are clean — the same claim check-commands-invoke-launcher.sh
// makes from the shell, asserted here against the tree this binary is built from.
func TestDoctorCommandLauncher_ShippedCommandsAreClean(t *testing.T) {
	var out bytes.Buffer
	if got := scanCommandLauncherPaths(filepath.Join("..", "..", "commands"), &out); got != 0 {
		t.Fatalf("shipped commands name the dev binary path:\n%s", out.String())
	}
}
