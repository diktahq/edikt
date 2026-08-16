package verify

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestCurrentDepth_DefaultsToTopLevel(t *testing.T) {
	t.Setenv(EnvDepth, "")
	if got := CurrentDepth(); got != 0 {
		t.Fatalf("unset depth = %d, want 0", got)
	}
}

func TestCurrentDepth_UnparseableAndNegativeClampToZero(t *testing.T) {
	// A corrupt or hostile value must not be able to raise the cap.
	for _, raw := range []string{"not-a-number", "-3", "1e9x", " "} {
		t.Setenv(EnvDepth, raw)
		if got := CurrentDepth(); got != 0 {
			t.Errorf("depth(%q) = %d, want 0", raw, got)
		}
	}
}

func TestDepthExceeded_AllowsOneNestingRefusesTwo(t *testing.T) {
	cases := []struct {
		depth string
		want  bool
	}{
		{"", false},  // user-invoked run
		{"0", false}, // user-invoked run
		{"1", false}, // spawned from inside a verify — legitimate (ADR-038)
		{"2", true},  // second nesting — the cycle signature
		{"7", true},
	}
	for _, c := range cases {
		t.Setenv(EnvDepth, c.depth)
		if got := DepthExceeded(); got != c.want {
			t.Errorf("DepthExceeded(depth=%q) = %v, want %v", c.depth, got, c.want)
		}
	}
}

func TestRunOne_PropagatesIncrementedDepthToChild(t *testing.T) {
	t.Setenv(EnvDepth, "1")
	res := RunOne("d", "s", "printf '%s' \"$EDIKT_VERIFY_DEPTH\"", RunOptions{})
	if res.Status != StatusPassed {
		t.Fatalf("status = %s, want passed (stderr: %s)", res.Status, res.StderrExcerpt)
	}
	if res.StdoutExcerpt != "2" {
		t.Fatalf("child saw depth %q, want %q", res.StdoutExcerpt, "2")
	}
}

// The fanout half of the containment. A verify that forks a background
// grandchild and then hangs must not leave that grandchild running once the
// timeout fires: killing the direct child is not enough, which is precisely
// how the INV-009 self-referential verify exhausted the process table.
func TestRunOne_TimeoutReapsForkedDescendants(t *testing.T) {
	marker, err := os.CreateTemp(t.TempDir(), "sentinel-*")
	if err != nil {
		t.Fatal(err)
	}
	markerPath := marker.Name()
	marker.Close()
	os.Remove(markerPath)

	// Grandchild sleeps, then creates the marker. If the process group is
	// reaped on timeout it never gets there; if only the direct child is
	// killed, the marker appears.
	script := "( sleep 3; touch " + markerPath + " ) & sleep 30"

	start := time.Now()
	res := RunOne("t", "timeout case", script, RunOptions{Timeout: 500 * time.Millisecond})
	if res.Status != StatusTimeout {
		t.Fatalf("status = %s, want timeout", res.Status)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("runner blocked %s — timeout did not fire promptly", elapsed)
	}

	// Wait past the grandchild's own delay before checking.
	time.Sleep(4 * time.Second)
	if _, err := os.Stat(markerPath); err == nil {
		t.Fatal("forked grandchild survived the timeout — process group was not reaped")
	}
}

// End-to-end refusal: the built binary must refuse to start a verify run when
// it is already nested at the cap, rather than spawning another level.
func TestVerifyBinary_RefusesReentryAtMaxDepth(t *testing.T) {
	bin := os.Getenv("EDIKT_TEST_BIN")
	if bin == "" {
		t.Skip("EDIKT_TEST_BIN not set — binary-level re-entry check skipped")
	}
	cmd := exec.Command(bin, "verify", "all")
	cmd.Env = append(os.Environ(), EnvDepth+"=2", "EDIKT_VERIFY_TRUST=1")
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatal("nested verify at max depth exited 0, want refusal")
	}
	if !strings.Contains(string(out), "refusing to re-enter the verify runner") {
		t.Fatalf("refusal message missing; got: %s", out)
	}
}
