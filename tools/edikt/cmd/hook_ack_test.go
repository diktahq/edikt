package cmd

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ADR-050 §2 — the ack surface for the surfaced-ledger. Contract under test:
//   - `hook ack` REQUIRES --why (an ack without a reason is the silent
//     suppression the ledger exists to prevent) and --until.
//   - fingerprints are allowlist-validated (INV-006) before touching state.
//   - `hook held` lists held items with why + until; `hook unack` clears.
//   - --until commit-touching:<path> records the current HEAD for the path.

func hackProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".edikt", "state"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".edikt", "config.yaml"),
		[]byte("edikt_version: \"0.7.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	return root
}

func TestHookAck_RequiresWhy(t *testing.T) {
	hackProject(t)
	_, err := runCmd(t, "hook", "ack", "abc123def456", "--until", "2027-01-01")
	if err == nil {
		t.Fatal("ack without --why must fail")
	}
}

func TestHookAck_RejectsUnsafeFingerprint(t *testing.T) {
	hackProject(t)
	_, err := runCmd(t, "hook", "ack", "../../etc/passwd", "--until", "2027-01-01", "--why", "x")
	if err == nil {
		t.Fatal("non-hex fingerprint must be rejected (INV-006)")
	}
}

func TestHookAck_HeldAndUnackRoundTrip(t *testing.T) {
	root := hackProject(t)
	if _, err := runCmd(t, "hook", "ack", "abc123def456", "--until", "2027-01-01",
		"--why", "propose-only approval gate holds these hashes deliberately"); err != nil {
		t.Fatalf("ack: %v", err)
	}

	// State file shape.
	raw, err := os.ReadFile(filepath.Join(root, ".edikt", "state", "hook-acks.json"))
	if err != nil {
		t.Fatalf("acks file not written: %v", err)
	}
	var acks map[string]map[string]string
	if err := json.Unmarshal(raw, &acks); err != nil {
		t.Fatalf("acks file not JSON: %v", err)
	}
	if acks["abc123def456"]["why"] == "" || acks["abc123def456"]["until"] == "" {
		t.Fatalf("ack entry incomplete: %v", acks)
	}

	out, err := runCmd(t, "hook", "held")
	if err != nil {
		t.Fatalf("held: %v", err)
	}
	if !strings.Contains(out, "abc123def456") || !strings.Contains(out, "propose-only") {
		t.Fatalf("held must list fingerprint and why, got:\n%s", out)
	}

	if _, err := runCmd(t, "hook", "unack", "abc123def456"); err != nil {
		t.Fatalf("unack: %v", err)
	}
	out, err = runCmd(t, "hook", "held")
	if err != nil {
		t.Fatalf("held after unack: %v", err)
	}
	if strings.Contains(out, "abc123def456") {
		t.Fatalf("unack did not clear the entry:\n%s", out)
	}
}

func TestHookAck_EventUntilRecordsHead(t *testing.T) {
	root := hackProject(t)
	// A git repo so commit-touching can record a HEAD.
	run := func(args ...string) {
		t.Helper()
		if out, err := runGit(root, args...); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-q", "-b", "main", ".")
	if err := os.WriteFile(filepath.Join(root, "watched.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("-c", "user.email=t@t", "-c", "user.name=t", "commit", "-qm", "init")

	if _, err := runCmd(t, "hook", "ack", "cafe01", "--until", "commit-touching:watched.md",
		"--why", "waiting on the doc rewrite"); err != nil {
		t.Fatalf("ack: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, ".edikt", "state", "hook-acks.json"))
	var acks map[string]map[string]string
	_ = json.Unmarshal(raw, &acks)
	if len(acks["cafe01"]["head"]) < 7 {
		t.Fatalf("commit-touching ack must record the current HEAD for the path, got: %v", acks["cafe01"])
	}
}

// runGit is a minimal git exec helper local to these tests.
func runGit(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
