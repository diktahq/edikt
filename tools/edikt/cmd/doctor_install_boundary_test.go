package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func boundaryFixture(t *testing.T) (ediktRoot, claudeRoot, outside string) {
	t.Helper()
	base := t.TempDir()
	ediktRoot = filepath.Join(base, "dot-edikt")
	claudeRoot = filepath.Join(base, "dot-claude")
	outside = filepath.Join(base, "checkout")
	for _, d := range []string{
		filepath.Join(ediktRoot, "versions", "v1.0.0", "commands"),
		filepath.Join(claudeRoot, "commands"),
		filepath.Join(outside, "commands"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return
}

func mustSymlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
}

func mustReal(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// An installed layout: current -> versions/<tag>, commands/edikt ->
// $EDIKT_ROOT/current/commands. Both inside; nothing printed.
func TestInstallBoundary_InstalledLayoutIsSilent(t *testing.T) {
	ediktRoot, claudeRoot, _ := boundaryFixture(t)
	mustSymlink(t, filepath.Join(ediktRoot, "versions", "v1.0.0"), filepath.Join(ediktRoot, "current"))
	mustSymlink(t, filepath.Join(ediktRoot, "current", "commands"), filepath.Join(claudeRoot, "commands", "edikt"))

	var out bytes.Buffer
	if n := runInstallBoundaryCheck(ediktRoot, claudeRoot, &out); n != 0 || out.Len() != 0 {
		t.Fatalf("installed layout must be silent; got %d:\n%s", n, out.String())
	}
}

// The live shape: current -> versions/dev (inside, DEV_SOURCE present) while
// commands/edikt points straight at a checkout outside $EDIKT_ROOT. One
// warning, named as a dev link, with the plugin refusal quoted as the reason.
func TestInstallBoundary_DevLinkIsReportedAsTripwire(t *testing.T) {
	ediktRoot, claudeRoot, outside := boundaryFixture(t)
	dev := filepath.Join(ediktRoot, "versions", "dev")
	if err := os.MkdirAll(dev, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dev, "DEV_SOURCE"), []byte(outside+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustSymlink(t, dev, filepath.Join(ediktRoot, "current"))
	mustSymlink(t, filepath.Join(outside, "commands"), filepath.Join(claudeRoot, "commands", "edikt"))

	var out bytes.Buffer
	n := runInstallBoundaryCheck(ediktRoot, claudeRoot, &out)
	s := out.String()
	if n != 1 {
		t.Fatalf("want exactly the commands/edikt link reported, got %d:\n%s", n, s)
	}
	for _, want := range []string{
		"commands/edikt resolves outside the install boundary",
		"-> " + mustReal(t, filepath.Join(outside, "commands")),
		"dev link (DEV_SOURCE present)",
		"refuses this shape for PLUGINS",
		"Plugins only, so far.",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in:\n%s", want, s)
		}
	}
}

// Sensitivity control: the same outside targets WITHOUT a DEV_SOURCE marker
// are still reported — both of them — and not called a dev link.
func TestInstallBoundary_OutsideWithoutMarkerIsNotCalledDev(t *testing.T) {
	ediktRoot, claudeRoot, outside := boundaryFixture(t)
	mustSymlink(t, outside, filepath.Join(ediktRoot, "current"))
	mustSymlink(t, filepath.Join(outside, "commands"), filepath.Join(claudeRoot, "commands", "edikt"))

	var out bytes.Buffer
	n := runInstallBoundaryCheck(ediktRoot, claudeRoot, &out)
	if n != 2 {
		t.Fatalf("both links resolve outside; want 2 warnings, got %d:\n%s", n, out.String())
	}
	if strings.Contains(out.String(), "dev link") {
		t.Fatalf("no DEV_SOURCE, must not be called a dev link:\n%s", out.String())
	}
}

// A boundary spelled through a symlink compares on real paths, so an
// installed layout reached via an aliased $EDIKT_ROOT does not false-positive.
func TestInstallBoundary_BoundaryThroughSymlinkComparesRealPaths(t *testing.T) {
	ediktRoot, claudeRoot, _ := boundaryFixture(t)
	alias := filepath.Join(filepath.Dir(ediktRoot), "alias-root")
	mustSymlink(t, ediktRoot, alias)
	mustSymlink(t, filepath.Join(ediktRoot, "versions", "v1.0.0"), filepath.Join(ediktRoot, "current"))
	mustSymlink(t, filepath.Join(alias, "current", "commands"), filepath.Join(claudeRoot, "commands", "edikt"))

	var out bytes.Buffer
	if n := runInstallBoundaryCheck(alias, claudeRoot, &out); n != 0 {
		t.Fatalf("aliased boundary must resolve inside; got %d:\n%s", n, out.String())
	}
}
