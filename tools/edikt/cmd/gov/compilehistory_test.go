package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompileHistory_FreshHistory(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001",
		"--history-path", ".edikt/state/compile-history.json")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "Orphan ADR warnings") {
		t.Errorf("missing warning header: %s", out)
	}
	// History file written.
	if _, err := os.Stat(filepath.Join(work, ".edikt/state/compile-history.json")); err != nil {
		t.Errorf("history not written: %v", err)
	}
}

func TestCompileHistory_PersistentOrphan_Blocks(t *testing.T) {
	work := t.TempDir()
	hist := filepath.Join(work, ".edikt/state/compile-history.json")
	if err := os.MkdirAll(filepath.Dir(hist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hist, []byte(`{"schema_version":1,"orphan_adrs":["ADR-001"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001",
		"--history-path", ".edikt/state/compile-history.json")
	if !isExitCode(err, 1) {
		t.Fatalf("expected exit 1 (BLOCK), got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "Orphan ADR BLOCK") {
		t.Errorf("missing BLOCK header: %s", out)
	}
}

func TestCompileHistory_Recovered(t *testing.T) {
	work := t.TempDir()
	hist := filepath.Join(work, ".edikt/state/compile-history.json")
	if err := os.MkdirAll(filepath.Dir(hist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hist, []byte(`{"schema_version":1,"orphan_adrs":["ADR-001","ADR-002"]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001",
		"--history-path", ".edikt/state/compile-history.json")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "subset") {
		t.Errorf("expected subset scenario note: %s", out)
	}
}

func TestCompileHistory_NoOrphans(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "",
		"--history-path", ".edikt/state/compile-history.json")
	if err != nil {
		t.Fatalf("expected exit 0, got err=%v\n%s", err, out)
	}
	// Empty orphan set: silent success.
	if strings.Contains(out, "[WARN]") || strings.Contains(out, "[BLOCK]") {
		t.Errorf("unexpected output: %s", out)
	}
}

func TestCompileHistory_CorruptHistory_TreatedAsAbsent(t *testing.T) {
	work := t.TempDir()
	hist := filepath.Join(work, ".edikt/state/compile-history.json")
	if err := os.MkdirAll(filepath.Dir(hist), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hist, []byte("{garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001",
		"--history-path", ".edikt/state/compile-history.json")
	if err != nil {
		t.Fatalf("expected exit 0 (corrupt → first detection), got err=%v\n%s", err, out)
	}
	if !strings.Contains(out, "unparseable") {
		t.Errorf("expected unparseable warning: %s", out)
	}
}

func TestCompileHistory_INV006_Traversal(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001",
		"--history-path", "../escape.json")
	if !isExitCode(err, 2) {
		t.Fatalf("expected exit 2 on traversal, got err=%v\n%s", err, out)
	}
}

func TestCompileHistory_INV006_BadOrphanID(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "ADR-001;rm -rf /",
		"--history-path", ".edikt/state/compile-history.json")
	if !isExitCode(err, 2) {
		t.Fatalf("expected exit 2 on injection attempt, got err=%v\n%s", err, out)
	}
}

func TestCompileHistory_INV006_AcceptsGuideline(t *testing.T) {
	work := t.TempDir()
	out, err := runGovIn(t, work, "gov", "compile-history",
		"--orphans", "GUIDE-007",
		"--history-path", ".edikt/state/compile-history.json")
	if err != nil {
		t.Fatalf("expected exit 0 with valid GUIDE id, got err=%v\n%s", err, out)
	}
}

// runGovIn runs the binary with cwd pinned to the test work dir, so that
// --history-path resolves under the test root.
func runGovIn(t *testing.T, cwd string, args ...string) (string, error) {
	t.Helper()
	bin := buildBinary(t)
	cmd := exec.Command(bin, args...)
	cmd.Dir = cwd
	out, err := cmd.CombinedOutput()
	return string(out), err
}
