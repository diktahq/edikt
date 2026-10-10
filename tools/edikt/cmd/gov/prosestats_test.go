package gov

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// prose-stats counts, it does not decide (see prosestats.go's header comment).
// These tests pin the two counts and the file-not-found exit code — nothing
// here asserts prose-stats influences a caller's control flow, because it
// must not.

func TestProseStats_CountsBareIDsAndHedges(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900-draft.md")
	body := "---\nreferences:\n  adrs: [ADR-001]\n---\n" +
		"# ADR-900\n\n" +
		"This amends ADR-001, and ADR-002 should probably also apply here.\n" +
		"We try to keep this consistent where possible.\n" +
		"A properly cited rule (ref: ADR-003) is not bare.\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runProseStatsCmd(t, path, 0)

	// bare ids: "ADR-001" and "ADR-002" from the body line (the frontmatter
	// ADR-001 and the (ref: ADR-003) one must NOT count).
	if !strings.Contains(out, `"bare_ids": 2`) {
		t.Errorf("expected bare_ids: 2, got: %s", out)
	}
	// hedges: "should probably" (1), "try to" (1), "where possible" (1).
	if !strings.Contains(out, `"hedges": 3`) {
		t.Errorf("expected hedges: 3, got: %s", out)
	}
	if !strings.Contains(out, `"bare_id_samples"`) || !strings.Contains(out, `"hedge_samples"`) {
		t.Errorf("expected samples to be carried in the output, got: %s", out)
	}
}

func TestProseStats_ZeroIsAMeasurementNotAnAbsence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clean.md")
	if err := os.WriteFile(path, []byte("# Clean\n\nNo ids, no hedges, plain obligation language.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out := runProseStatsCmd(t, path, 0)
	if !strings.Contains(out, `"bare_ids": 0`) || !strings.Contains(out, `"hedges": 0`) {
		t.Errorf("expected explicit zero counts, got: %s", out)
	}
}

func TestProseStats_UnreadableFileExits3(t *testing.T) {
	runProseStatsCmd(t, filepath.Join(t.TempDir(), "missing.md"), 3)
}

func runProseStatsCmd(t *testing.T, path string, wantCode int) string {
	t.Helper()
	bin := buildBinary(t)
	cmd := exec.Command(bin, "gov", "prose-stats", path)
	out, err := cmd.CombinedOutput()
	if wantCode == 0 && err != nil {
		t.Fatalf("expected exit 0, got err=%v\noutput: %s", err, out)
	}
	if wantCode != 0 && !isExitCode(err, wantCode) {
		t.Fatalf("expected exit %d, got err=%v\noutput: %s", wantCode, err, out)
	}
	return string(out)
}
