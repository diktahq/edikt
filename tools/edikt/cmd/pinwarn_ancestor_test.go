package cmd

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindProjectConfig_FindsConfigFromOddDepthSubdirectory is SPEC-024 SR-009's
// acceptance test, written by the coordinator rather than a delivery lane.
//
// WHY IT IS HERE. Phase 7 reported 8/8 with SR-009 entirely undelivered: it
// changed zero Go files. Its criteria are a shell-only harness over
// commands/upgrade.md, so nothing in the phase could observe a Go defect, and a
// lens said so plainly — "no criterion in this phase's shell-only harness can
// observe it". Eight criteria green, one requirement untouched.
//
// THE DEFECT, from SR-009: findProjectConfig climbs TWICE per iteration. The
// loop post-statement is `dir = filepath.Dir(dir)` and the body also ends with
// `dir = parent`, where parent is already filepath.Dir(dir). So from a
// subdirectory one level below the project root, the loop checks that
// subdirectory, then jumps straight past the root to its grandparent — and the
// pin warning silently never fires. One level is the smallest reproduction; the
// bug hits every odd-numbered depth.
func TestFindProjectConfig_FindsConfigFromOddDepthSubdirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, ".edikt", "config.yaml")
	if err := os.WriteFile(want, []byte("edikt_version: \"0.9.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(root, "a")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	prev, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
	if err := os.Chdir(sub); err != nil {
		t.Fatal(err)
	}

	got := findProjectConfig()
	if got == "" {
		t.Fatalf("findProjectConfig() returned empty from %s: the project config at %s was skipped, "+
			"so the pin warning does not fire from an odd-depth subdirectory (SR-009)", sub, want)
	}
	gotResolved, _ := filepath.EvalSymlinks(got)
	wantResolved, _ := filepath.EvalSymlinks(want)
	if gotResolved != wantResolved {
		t.Fatalf("findProjectConfig() = %q, want %q", got, want)
	}
}
