package cmd

// pinwarn_quiet_test.go — the pin warning is quiet where it has no remedy
// (#11 comment item 8, PLAN-v090-issue-fixes phase 9 step 7).
//
// The warning printed on EVERY invocation whose project pin differed from the
// active version, including:
//
//   - a `dev` install against any release pin. versioncmp calls the two
//     Incomparable, and pinRemedy's own text for that case says neither
//     moving the pin nor upgrading the binary follows from it — a warning
//     with no action, on every command, for every developer of edikt itself.
//   - every hook. Hooks run `edikt hook <name>` many times per turn; each one
//     printed the warning to a stderr the host surfaces as hook noise, where
//     no one can act on it.
//
// Pin drift is still reported — by `edikt doctor`, which is where a standing
// condition belongs (doctor_pin_drift_test.go).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// pinWarnFixture builds an edikt root whose lock says active, and a project
// pinning pinned, and chdirs into the project. HOME is a separate temp dir so
// findProjectConfig's stop-at-HOME rule cannot end the walk early.
func pinWarnFixture(t *testing.T, pinned, active string) string {
	t.Helper()
	ediktRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(ediktRoot, "lock.yaml"), []byte("active: \""+active+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	proj := t.TempDir()
	if err := os.MkdirAll(filepath.Join(proj, ".edikt"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proj, ".edikt", "config.yaml"), []byte("edikt_version: \""+pinned+"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", t.TempDir())
	t.Chdir(proj)
	return ediktRoot
}

func TestEmitPinWarn_SilentOnIncomparable(t *testing.T) {
	for _, c := range []struct{ pinned, active string }{
		{"0.9.0", "dev"},
		{"dev", "0.9.0"},
	} {
		root := pinWarnFixture(t, c.pinned, c.active)
		var buf bytes.Buffer
		emitPinWarnTo(&buf, root)
		if buf.Len() != 0 {
			t.Errorf("pin %s vs active %s is Incomparable — no remedy follows — yet the warning printed:\n%s", c.pinned, c.active, buf.String())
		}
	}
}

// The control: silence must be specific to the no-remedy case. A pin behind
// the install has a remedy, and still says so.
func TestEmitPinWarn_EmitsWhenPinBehind(t *testing.T) {
	root := pinWarnFixture(t, "0.8.0", "0.9.0")
	var buf bytes.Buffer
	emitPinWarnTo(&buf, root)
	out := buf.String()
	if !strings.Contains(out, "pins edikt 0.8.0") || !strings.Contains(out, "0.9.0") {
		t.Fatalf("a pin behind the install must still warn, naming both versions:\n%s", out)
	}
	if !strings.Contains(out, "upgrade-pin") {
		t.Errorf("the behind-case warning lost its remedy:\n%s", out)
	}
}

// Every `edikt hook <name>` invocation is exempt, through the same
// parent-chain walk root.go uses, so a hook added tomorrow is covered without
// anyone listing it.
func TestPinWarnExempt_HookSubtree(t *testing.T) {
	if !pinWarnExempt("hook") {
		t.Fatal(`pinWarnExempt("hook") = false — every hook invocation prints the pin warning into hook output`)
	}
	parent := &cobra.Command{Use: "hook"}
	leaf := &cobra.Command{Use: "session-start"}
	parent.AddCommand(leaf)
	if !pinWarnExemptChain(leaf) {
		t.Error("`edikt hook session-start` is not exempt through its parent")
	}
	if pinWarnExemptChain(&cobra.Command{Use: "install"}) {
		t.Error("control: `install` must stay non-exempt — the launcher pin tests rely on it warning")
	}
}
