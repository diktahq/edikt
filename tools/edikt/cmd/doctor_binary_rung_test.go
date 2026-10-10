package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// The live instance, reproduced as a fixture: two checkouts of one repository,
// one trusted with a local build and one not, resolving different binaries.
func setupRoots(t *testing.T) (trusted, untrusted, ediktRoot string) {
	t.Helper()
	base := t.TempDir()
	trusted = filepath.Join(base, "edikt-dev")
	untrusted = filepath.Join(base, "edikt-worktree")
	ediktRoot = filepath.Join(base, "dot-edikt")
	for _, d := range []string{
		filepath.Join(trusted, "bin"), filepath.Join(untrusted, "bin"),
		filepath.Join(ediktRoot, "bin"), filepath.Join(ediktRoot, "state"),
	} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Installed launcher on rung 3 always exists.
	if err := os.WriteFile(filepath.Join(ediktRoot, "bin", "edikt"), []byte("installed"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Only the trusted root is registered.
	resolved, _ := filepath.EvalSymlinks(trusted)
	if err := os.WriteFile(filepath.Join(ediktRoot, "state", "trusted-roots.json"),
		[]byte(`{"`+resolved+`":{"since":"x"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return trusted, untrusted, ediktRoot
}

func TestTrustedRootWithLocalBuildAnswersRung2(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	if err := os.WriteFile(filepath.Join(trusted, "bin", "edikt"), []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := resolveBinaryRung(trusted, ediktRoot)
	if r.Rung != 2 {
		t.Fatalf("rung = %d, want 2 (trusted root with a local build)", r.Rung)
	}
	if !r.Trusted {
		t.Error("root should be reported trusted")
	}
}

// The pair must disagree, or the check proves nothing: an untrusted worktree of
// the SAME repo, with an identical local build, must still fall to rung 3.
func TestUntrustedWorktreeFallsToRung3(t *testing.T) {
	_, untrusted, ediktRoot := setupRoots(t)
	if err := os.WriteFile(filepath.Join(untrusted, "bin", "edikt"), []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := resolveBinaryRung(untrusted, ediktRoot)
	if r.Rung != 3 {
		t.Fatalf("rung = %d, want 3 (untrusted root ignores its own build)", r.Rung)
	}
	var buf bytes.Buffer
	warns := runBinaryRungCheck(untrusted, ediktRoot, false, &buf)
	if warns == 0 {
		t.Error("an untrusted root ignoring its own build must warn")
	}
	if !strings.Contains(buf.String(), "your own build is being ignored") {
		t.Errorf("the silent-ignore case must be named; got: %s", buf.String())
	}
}

func TestRungOneWinsOverRungTwo(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	if err := os.MkdirAll(filepath.Join(trusted, ".edikt", "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{
		filepath.Join(trusted, ".edikt", "bin", "edikt"),
		filepath.Join(trusted, "bin", "edikt"),
	} {
		if err := os.WriteFile(p, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if r := resolveBinaryRung(trusted, ediktRoot); r.Rung != 1 {
		t.Fatalf("rung = %d, want 1 — the ladder's order must be reproduced", r.Rung)
	}
}

// An unreadable trust store must not grant trust (INV-011).
func TestUnreadableTrustStoreDoesNotGrantTrust(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	if err := os.WriteFile(filepath.Join(ediktRoot, "state", "trusted-roots.json"),
		[]byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(trusted, "bin", "edikt"), []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	r := resolveBinaryRung(trusted, ediktRoot)
	if r.Trusted {
		t.Error("an unparseable trust store must not grant trust")
	}
	if r.Rung != 3 {
		t.Fatalf("rung = %d, want 3", r.Rung)
	}
}

// SR-008: the check declares what it swept, not only what it found.
func TestCheckDeclaresItsDenominator(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	if err := os.WriteFile(filepath.Join(trusted, "bin", "edikt"), []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	runBinaryRungCheck(trusted, ediktRoot, false, &buf)
	if !strings.Contains(buf.String(), "swept 3 candidate(s)") {
		t.Errorf("denominator must be declared; got: %s", buf.String())
	}
}

// SSP-001: report the measurement, never the implication.
func TestReportsMeasurementNotImplication(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	if err := os.WriteFile(filepath.Join(trusted, "bin", "edikt"), []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	runBinaryRungCheck(trusted, ediktRoot, true, &buf)
	for _, forbidden := range []string{"exemption", "ADR-074 is active", "governance is current"} {
		if strings.Contains(buf.String(), forbidden) {
			t.Errorf("check must not claim %q — it measures a path, not a decision", forbidden)
		}
	}
}

func TestStalenessProxyFlagsAnOlderBinaryAndSaysItIsAProxy(t *testing.T) {
	trusted, _, ediktRoot := setupRoots(t)
	bin := filepath.Join(trusted, "bin", "edikt")
	if err := os.WriteFile(bin, []byte("local"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(trusted, "tools", "edikt")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "main.go"), []byte("package main"), 0o644); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(bin, old, old); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	warns := runBinaryRungCheck(trusted, ediktRoot, true, &buf)
	if warns == 0 {
		t.Error("a binary older than its source must warn")
	}
	if !strings.Contains(buf.String(), "STALENESS PROXY") {
		t.Errorf("the proxy must be labelled as one; got: %s", buf.String())
	}
}
