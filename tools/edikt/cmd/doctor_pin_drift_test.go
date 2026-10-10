package cmd

// doctor_pin_drift_test.go — `edikt doctor` reports pin drift (#11 comment
// item 8). The per-invocation pin warning is now silent on Incomparable pins
// and inside hooks; the drift itself must still surface somewhere a human
// looks, or silencing the warning would have hidden it. Doctor is that place,
// and it reports EVERY pin != active, Incomparable included — the case the
// warning no longer prints.

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func pinDriftFixture(t *testing.T, pinned, active string) (proj, ediktRoot string) {
	t.Helper()
	ediktRoot = t.TempDir()
	if active != "" {
		if err := os.WriteFile(filepath.Join(ediktRoot, "lock.yaml"), []byte("active: \""+active+"\"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	proj = t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".edikt", "config.yaml"), []byte("edikt_version: \""+pinned+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	return proj, ediktRoot
}

func TestDoctorPinDrift_ReportsIncomparableAndBehind(t *testing.T) {
	cases := []struct {
		name, pinned, active string
		want                 []string // substrings of the single WARN line + remedy
		notWant              string
	}{
		{"incomparable: a dev install against a release pin", "0.9.0", "dev",
			[]string{"WARN:", "0.9.0", "dev", "not comparable"}, "upgrade-pin` inside"},
		{"behind: the pin trails the install", "0.8.0", "0.9.0",
			[]string{"WARN:", "0.8.0", "0.9.0", "behind", "edikt upgrade-pin"}, "do NOT"},
		{"ahead: the pin leads the install", "0.9.1", "0.9.0",
			[]string{"WARN:", "ahead", "do NOT run `edikt upgrade-pin`"}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			proj, root := pinDriftFixture(t, c.pinned, c.active)
			var out bytes.Buffer
			if n := runPinDriftCheck(proj, root, &out); n != 1 {
				t.Fatalf("want 1 warning, got %d:\n%s", n, out.String())
			}
			if got := strings.Count(out.String(), "WARN:"); got != 1 {
				t.Errorf("one finding, one WARN line (FR-035); got %d:\n%s", got, out.String())
			}
			for _, w := range c.want {
				if !strings.Contains(out.String(), w) {
					t.Errorf("output lacks %q:\n%s", w, out.String())
				}
			}
			if c.notWant != "" && strings.Contains(out.String(), c.notWant) {
				t.Errorf("output carries the wrong-direction remedy %q:\n%s", c.notWant, out.String())
			}
		})
	}

	// Controls: a matching pin (in either spelling) is not drift, and an
	// active version that cannot be read is reported as UNMEASURED, never as
	// "no drift" (INV-013) and never counted as a warning.
	t.Run("matching pin is quiet", func(t *testing.T) {
		proj, root := pinDriftFixture(t, "v0.9.0", "0.9.0")
		var out bytes.Buffer
		if n := runPinDriftCheck(proj, root, &out); n != 0 || out.Len() != 0 {
			t.Fatalf("a matching pin must be silent; got %d:\n%s", n, out.String())
		}
	})
	t.Run("no active version is unmeasured, not clean", func(t *testing.T) {
		proj, root := pinDriftFixture(t, "0.9.0", "")
		var out bytes.Buffer
		if n := runPinDriftCheck(proj, root, &out); n != 0 {
			t.Fatalf("an unreadable active version is not a warning; got %d", n)
		}
		if !strings.Contains(out.String(), "UNMEASURED") {
			t.Fatalf("an unreadable active version must say nothing was measured:\n%s", out.String())
		}
	})
	t.Run("the global config in $HOME is not a project pin", func(t *testing.T) {
		proj, root := pinDriftFixture(t, "0.8.0", "0.9.0")
		t.Setenv("HOME", proj)
		var out bytes.Buffer
		if n := runPinDriftCheck(proj, root, &out); n != 0 || out.Len() != 0 {
			t.Fatalf("$HOME/.edikt/config.yaml is edikt's global config, not a project pin; got %d:\n%s", n, out.String())
		}
	})
}
