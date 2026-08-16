package cmd

// Tests for runCheatRateBenchmarkCheck — Phase 5 of SPEC-009 Plan E.
// Covers AC-5.3 (adversary_model lock) and the latent-defect fix
// (walking the nested <sidecar-id>/<ts>.json layout that
// cheatrate.WriteReport actually produces).

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFakeReport drops a cheat-rate report JSON at the canonical
// layout <root>/.edikt/state/benchmark/<sidecarID>/<timestamp>.json
// and returns the report path.
func writeFakeReport(t *testing.T, root, sidecarID, timestamp, body string) string {
	t.Helper()
	dir := filepath.Join(root, ".edikt", "state", "benchmark", sidecarID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", dir, err)
	}
	path := filepath.Join(dir, timestamp+".json")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return path
}

// reportWithModelAndCheatRate returns minimal-but-valid Report JSON
// for use in doctor tests. AdversaryModel and cheat rate are the two
// dimensions that matter for the section's warnings.
func reportWithModelAndCheatRate(sidecarID, model string, cheatRate float64) string {
	return `{
  "schema_version": 1,
  "sidecar_id": "` + sidecarID + `",
  "ran_at": "2026-05-23T14:30:00Z",
  "adversary_model": "` + model + `",
  "verifies": [],
  "summary": {
    "total": 1,
    "cheated": 0,
    "inconclusive": 0,
    "cheat_rate": ` + floatStr(cheatRate) + `,
    "inconclusive_rate": 0.0
  }
}`
}

func floatStr(f float64) string {
	// Avoid importing strconv in this small helper — keep test
	// deps minimal. fmt.Sprintf would also work but doctor tests
	// already pull bytes / os / strings so we use a tiny constant.
	switch f {
	case 0.0:
		return "0.0"
	case 0.1:
		return "0.1"
	case 0.2:
		return "0.20"
	case 0.25:
		return "0.25"
	case 0.5:
		return "0.5"
	default:
		return "0.0"
	}
}

// TestRunCheatRateBenchmarkCheck_EmptyEmitsHint — when there are no
// reports, doctor must point the operator at the subcommand.
func TestRunCheatRateBenchmarkCheck_EmptyEmitsHint(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	warns, ran := runCheatRateBenchmarkCheck(root, &buf)
	if !ran {
		t.Errorf("expected ran=true even on empty state")
	}
	if warns != 0 {
		t.Errorf("expected 0 warns on empty state, got %d", warns)
	}
	if !strings.Contains(buf.String(), "bin/edikt gov benchmark cheat-rate") {
		t.Errorf("expected bootstrap hint mentioning the subcommand; got:\n%s", buf.String())
	}
}

// TestRunCheatRateBenchmarkCheck_NestedLayout — the doctor must read
// reports written at the canonical <sidecar-id>/<ts>.json layout, not
// the flat layout the pre-Plan-E code scanned for. Regression test
// for the latent Plan C defect.
func TestRunCheatRateBenchmarkCheck_NestedLayout(t *testing.T) {
	root := t.TempDir()
	writeFakeReport(t, root, "ADR-001", "2026-05-23T143000Z",
		reportWithModelAndCheatRate("ADR-001", "claude-opus-4-7", 0.0))

	var buf bytes.Buffer
	warns, ran := runCheatRateBenchmarkCheck(root, &buf)
	if !ran {
		t.Errorf("expected ran=true")
	}
	if warns != 0 {
		t.Errorf("clean report should produce no warnings, got %d (out: %s)", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "1 sidecar(s)") {
		t.Errorf("expected count of 1 sidecar; got:\n%s", buf.String())
	}
}

// TestRunCheatRateBenchmarkCheck_HighCheatRateWarns — a cheat_rate at
// or above the 0.20 ceiling produces a warning that names the
// sidecar and the refresh command.
func TestRunCheatRateBenchmarkCheck_HighCheatRateWarns(t *testing.T) {
	root := t.TempDir()
	writeFakeReport(t, root, "ADR-007", "2026-05-23T143000Z",
		reportWithModelAndCheatRate("ADR-007", "claude-opus-4-7", 0.25))

	var buf bytes.Buffer
	warns, _ := runCheatRateBenchmarkCheck(root, &buf)
	if warns != 1 {
		t.Errorf("expected 1 warn for cheat_rate=0.25, got %d", warns)
	}
	out := buf.String()
	if !strings.Contains(out, "ADR-007") {
		t.Errorf("warn should name the sidecar; got:\n%s", out)
	}
	if !strings.Contains(out, "cheat_rate=0.25") {
		t.Errorf("warn should include the rate value; got:\n%s", out)
	}
}

// TestRunCheatRateBenchmarkCheck_WrongAdversaryModelWarns — AC-5.3.
// A report with adversary_model != "claude-opus-4-7" must surface a
// warning even when the cheat rate itself is low (the rate may be
// under-counted because the adversary was weaker than the ADR-040
// lock).
func TestRunCheatRateBenchmarkCheck_WrongAdversaryModelWarns(t *testing.T) {
	root := t.TempDir()
	writeFakeReport(t, root, "ADR-007", "2026-05-23T143000Z",
		reportWithModelAndCheatRate("ADR-007", "claude-sonnet-4-6", 0.0))

	var buf bytes.Buffer
	warns, _ := runCheatRateBenchmarkCheck(root, &buf)
	if warns != 1 {
		t.Errorf("expected 1 warn for wrong adversary_model, got %d", warns)
	}
	out := buf.String()
	if !strings.Contains(out, "adversary_model=") {
		t.Errorf("warn should mention adversary_model; got:\n%s", out)
	}
	if !strings.Contains(out, "claude-opus-4-7") {
		t.Errorf("warn should reference the locked model; got:\n%s", out)
	}
	if !strings.Contains(out, "--adversary-model=") {
		t.Errorf("warn should give actionable re-run guidance; got:\n%s", out)
	}
}

// TestRunCheatRateBenchmarkCheck_LatestReportWinsPerSidecar — doctor
// must pick the latest report per sidecar (lexicographic sort over
// fs-safe timestamps) so the warning reflects the most recent run,
// not the historic worst.
func TestRunCheatRateBenchmarkCheck_LatestReportWinsPerSidecar(t *testing.T) {
	root := t.TempDir()
	// Two reports for the same sidecar: older has high cheat rate,
	// newer is clean. Doctor should pick up the clean one.
	writeFakeReport(t, root, "ADR-007", "2026-05-22T100000Z",
		reportWithModelAndCheatRate("ADR-007", "claude-opus-4-7", 0.5))
	writeFakeReport(t, root, "ADR-007", "2026-05-23T100000Z",
		reportWithModelAndCheatRate("ADR-007", "claude-opus-4-7", 0.0))

	var buf bytes.Buffer
	warns, _ := runCheatRateBenchmarkCheck(root, &buf)
	if warns != 0 {
		t.Errorf("latest report is clean — expected 0 warns, got %d (out: %s)", warns, buf.String())
	}
}

// TestRunCheatRateBenchmarkCheck_TwoWarnsCompound — a report can
// trigger BOTH warnings simultaneously (high rate AND wrong model).
func TestRunCheatRateBenchmarkCheck_TwoWarnsCompound(t *testing.T) {
	root := t.TempDir()
	writeFakeReport(t, root, "ADR-007", "2026-05-23T143000Z",
		reportWithModelAndCheatRate("ADR-007", "claude-sonnet-4-6", 0.25))

	var buf bytes.Buffer
	warns, _ := runCheatRateBenchmarkCheck(root, &buf)
	if warns != 2 {
		t.Errorf("expected 2 warns (rate + model), got %d (out: %s)", warns, buf.String())
	}
}
