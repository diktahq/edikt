package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestVersionLineBelowFloor(t *testing.T) {
	// Floor is 0.7 (SPEC-011: gov-sidecar.v2). The 0.6 rows moved from
	// not-below to below when the floor moved — that is the property under
	// test, not an incidental update: every line beneath the floor is refused,
	// and the previous line is no longer special.
	cases := map[string]bool{
		"0.5.0":     true,
		"0.5.1":     true,
		"0.4.0":     true,
		"v0.5.0":    true,
		"0.2.0":     true,
		"0.6.0":     true,
		"0.6.0-rc4": true,
		"0.6.1":     true,
		"0.7.0":     false,
		"0.7.0-rc1": false, // prerelease of the line is still on the line
		"0.7.1":     false,
		"0.8.0":     false,
		"1.0.0":     false,
		"garbage":   false, // unparseable → not gated
		"0.7":       false,
	}
	for v, want := range cases {
		if got := versionLineBelowFloor(v); got != want {
			t.Errorf("versionLineBelowFloor(%q) = %v, want %v", v, got, want)
		}
	}
}

func TestVersionGateApplies(t *testing.T) {
	root := &cobra.Command{Use: "edikt"}
	gov := &cobra.Command{Use: "gov"}
	compile := &cobra.Command{Use: "compile"}
	gov.AddCommand(compile)
	migrate := &cobra.Command{Use: "migrate"}
	sidecars := &cobra.Command{Use: "sidecars"}
	migrate.AddCommand(sidecars)
	verify := &cobra.Command{Use: "verify"}
	doctor := &cobra.Command{Use: "doctor"}
	upgrade := &cobra.Command{Use: "upgrade"}
	root.AddCommand(gov, migrate, verify, doctor, upgrade)

	cases := map[*cobra.Command]bool{
		compile:  true,  // gov subcommand → gated
		verify:   true,  // gated
		sidecars: false, // under migrate → exempt
		migrate:  false, // version-management → exempt
		doctor:   false, // read-only → exempt
		upgrade:  false, // fixes the mismatch → exempt
		root:     false, // bare invocation → exempt
	}
	for cmd, want := range cases {
		if got := versionGateApplies(cmd); got != want {
			t.Errorf("versionGateApplies(%q) = %v, want %v", cmd.Name(), got, want)
		}
	}
}

func TestEnsureVersionLine(t *testing.T) {
	writeConfig := func(t *testing.T, version string) {
		proj := t.TempDir()
		home := t.TempDir() // unrelated HOME so the walk finds proj first
		if err := os.MkdirAll(filepath.Join(proj, ".edikt"), 0o755); err != nil {
			t.Fatal(err)
		}
		if version != "" {
			body := "edikt_version: \"" + version + "\"\n"
			if err := os.WriteFile(filepath.Join(proj, ".edikt", "config.yaml"), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		t.Setenv("HOME", home)
		t.Chdir(proj)
	}

	t.Run("pre-floor project is refused", func(t *testing.T) {
		t.Setenv("EDIKT_SKIP_VERSION_GATE", "")
		writeConfig(t, "0.5.1")
		err := ensureVersionLine()
		if err == nil {
			t.Fatal("expected refusal for 0.5.1 project, got nil")
		}
		if !strings.Contains(err.Error(), "version lines") {
			t.Errorf("unexpected message: %v", err)
		}
	})

	// The previous line is refused too. This case exists because it is the one
	// a floor bump is most likely to get wrong: the line that used to pass.
	t.Run("previous line is refused after the floor moves", func(t *testing.T) {
		t.Setenv("EDIKT_SKIP_VERSION_GATE", "")
		writeConfig(t, "0.6.0-rc4")
		err := ensureVersionLine()
		if err == nil {
			t.Fatal("expected refusal for a 0.6-line project at floor 0.7, got nil")
		}
		// The refusal must name the CURRENT floor. A message hardcoding the
		// previous one would send the operator after the wrong release.
		if !strings.Contains(err.Error(), "v"+versionLineFloor+" line") {
			t.Errorf("refusal does not name the current floor (%s): %v", versionLineFloor, err)
		}
	})

	t.Run("current line passes", func(t *testing.T) {
		t.Setenv("EDIKT_SKIP_VERSION_GATE", "")
		writeConfig(t, "0.7.0-rc1")
		if err := ensureVersionLine(); err != nil {
			t.Errorf("0.7.0-rc1 should pass, got %v", err)
		}
	})

	t.Run("bypass overrides", func(t *testing.T) {
		t.Setenv("EDIKT_SKIP_VERSION_GATE", "1")
		writeConfig(t, "0.4.0")
		if err := ensureVersionLine(); err != nil {
			t.Errorf("bypass should pass, got %v", err)
		}
	})

	t.Run("unpinned project passes", func(t *testing.T) {
		t.Setenv("EDIKT_SKIP_VERSION_GATE", "")
		writeConfig(t, "") // no edikt_version line
		if err := ensureVersionLine(); err != nil {
			t.Errorf("unpinned project should pass, got %v", err)
		}
	})
}
