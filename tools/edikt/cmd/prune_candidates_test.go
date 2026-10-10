package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// prunePlan writes an incomplete PLAN-<slug>.md so activeplan.Resolve
// picks it up as the active plan.
func prunePlan(t *testing.T, plansDir, slug string) string {
	t.Helper()
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir plansDir: %v", err)
	}
	body := "# PLAN-" + slug + "\n\n## Progress\n\n| Phase | Status |\n|---|---|\n| 1 | in-progress |\n"
	path := filepath.Join(plansDir, "PLAN-"+slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return path
}

// pruneConfig writes .edikt/config.yaml with paths.plans set.
func pruneConfig(t *testing.T, root string) {
	t.Helper()
	cfgDir := filepath.Join(root, ".edikt")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir .edikt: %v", err)
	}
	cfg := "paths:\n  plans: docs/internal/plans\n"
	if err := os.WriteFile(filepath.Join(cfgDir, "config.yaml"), []byte(cfg), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// pruneDeliveryMap writes delivery-map.yaml at its conventional
// fallback location with the given directive ids (already in
// delivery-map form, e.g. "ADR-900:d01"), each under one glob.
func pruneDeliveryMap(t *testing.T, root string, directiveIDs []string, prohibitionIDs []string) {
	t.Helper()
	dir := filepath.Join(root, ".claude", "rules", "governance")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir governance dir: %v", err)
	}
	body := "schema_version: 1\nglobs:\n  \"**/*.go\":\n"
	for _, id := range directiveIDs {
		body += "    - id: " + id + "\n      grade: must\n      surface: topic-file\n"
	}
	for _, id := range prohibitionIDs {
		body += "    - id: " + id + "\n      grade: must\n      surface: topic-file\n"
	}
	if err := os.WriteFile(filepath.Join(dir, "delivery-map.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write delivery-map.yaml: %v", err)
	}
}

// prunePostFlightReport writes a post-flight report file with the
// exact <slug>-<phase>-<ts>.json naming shape. citedDirectives == nil
// means the key is entirely absent (unmeasured report).
func prunePostFlightReport(t *testing.T, root, slug string, phase int, ts time.Time, citedDirectives *[]string) {
	t.Helper()
	dir := filepath.Join(root, ".edikt", "state", "post-flight")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir post-flight dir: %v", err)
	}
	type reportShape struct {
		Meta struct {
			Plan  string `json:"plan"`
			Phase int    `json:"phase"`
		} `json:"meta"`
		CitedDirectives *[]string `json:"cited_directives,omitempty"`
	}
	var r reportShape
	r.Meta.Plan = slug
	r.Meta.Phase = phase
	r.CitedDirectives = citedDirectives
	raw, err := json.Marshal(r)
	if err != nil {
		t.Fatalf("marshal report: %v", err)
	}
	name := slug + "-" + strconv.Itoa(phase) + "-" + ts.UTC().Format("20060102T150405Z") + ".json"
	if err := os.WriteFile(filepath.Join(dir, name), raw, 0o644); err != nil {
		t.Fatalf("write report %s: %v", name, err)
	}
}

func TestPruneCandidates_ReturnsZeroCitationCandidates(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	prunePlan(t, plansDir, "x")
	pruneDeliveryMap(t, root, []string{"ADR-900:d01", "ADR-900:d02"}, []string{"ADR-900:p01"})
	cited := []string{"ADR-900.directive[0]"} // -> ADR-900:d01
	prunePostFlightReport(t, root, "x", 1, time.Now(), &cited)

	out, r := runPruneJSON(t, root)
	if !r.Measured {
		t.Fatalf("expected measured=true, got false; out=%q", out)
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "ADR-900:d02" {
		t.Errorf("expected candidates [ADR-900:d02], got %v", r.Candidates)
	}
	// ADR-900:p01 (prohibition) must never appear as a candidate.
	for _, c := range r.Candidates {
		if c == "ADR-900:p01" {
			t.Errorf("prohibition id leaked into candidates: %v", r.Candidates)
		}
	}
}

func TestPruneCandidates_UnmeasuredWhenNoReportCarriesField(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	prunePlan(t, plansDir, "x")
	pruneDeliveryMap(t, root, []string{"ADR-900:d01"}, nil)
	prunePostFlightReport(t, root, "x", 1, time.Now(), nil) // no cited_directives key at all

	_, r := runPruneJSON(t, root)
	if r.Measured {
		t.Errorf("expected measured=false (report predates the field), got true: %+v", r)
	}
	if r.Candidates != nil {
		t.Errorf("expected no candidates when unmeasured, got %v", r.Candidates)
	}
}

func TestPruneCandidates_WindowLimitsToRecentReports(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	prunePlan(t, plansDir, "x")
	pruneDeliveryMap(t, root, []string{"ADR-900:d01", "ADR-900:d02"}, nil)

	now := time.Now()
	// Old report (outside the window) cites d02; recent one cites d01 only.
	oldCited := []string{"ADR-900.directive[1]"} // -> d02
	prunePostFlightReport(t, root, "x", 1, now.Add(-10*time.Hour), &oldCited)
	newCited := []string{"ADR-900.directive[0]"} // -> d01
	prunePostFlightReport(t, root, "x", 2, now, &newCited)

	pruneCandidatesWindow = 1 // window=1 -> only the newest report counts
	defer func() { pruneCandidatesWindow = 5 }()

	_, r := runPruneJSON(t, root)
	if r.ReportsScanned != 1 {
		t.Errorf("expected exactly 1 report scanned under window=1, got %d", r.ReportsScanned)
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "ADR-900:d02" {
		t.Errorf("expected d02 as the only candidate (d01 cited by the in-window report), got %v", r.Candidates)
	}
}

func TestPruneCandidates_InvalidDirectiveIDSkippedNotCrashed(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	prunePlan(t, plansDir, "x")
	pruneDeliveryMap(t, root, []string{"ADR-900:d01"}, nil)
	cited := []string{"; rm -rf / #.directive[0]", "not-shaped-like-a-directive-id"}
	prunePostFlightReport(t, root, "x", 1, time.Now(), &cited)

	_, r := runPruneJSON(t, root)
	if !r.Measured {
		t.Fatalf("expected measured=true (the report DID carry the field)")
	}
	if len(r.Candidates) != 1 || r.Candidates[0] != "ADR-900:d01" {
		t.Errorf("malformed ids should be skipped, not crash or silently validate; got candidates %v", r.Candidates)
	}
}

func TestPruneCandidates_RemovesNothing(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	prunePlan(t, plansDir, "x")
	pruneDeliveryMap(t, root, []string{"ADR-900:d01", "ADR-900:d02"}, nil)
	cited := []string{}
	reportPath := filepath.Join(root, ".edikt", "state", "post-flight")
	prunePostFlightReport(t, root, "x", 1, time.Now(), &cited)

	beforeMap, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml"))
	if err != nil {
		t.Fatalf("read delivery-map before: %v", err)
	}
	entriesBefore, err := os.ReadDir(reportPath)
	if err != nil {
		t.Fatalf("read post-flight dir before: %v", err)
	}

	_, r := runPruneJSON(t, root)
	if len(r.Candidates) != 2 {
		t.Fatalf("sanity: expected both directives as candidates, got %v", r.Candidates)
	}

	afterMap, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance", "delivery-map.yaml"))
	if err != nil {
		t.Fatalf("read delivery-map after: %v", err)
	}
	if string(beforeMap) != string(afterMap) {
		t.Error("delivery-map.yaml was mutated — prune-candidates must remove nothing")
	}
	entriesAfter, err := os.ReadDir(reportPath)
	if err != nil {
		t.Fatalf("read post-flight dir after: %v", err)
	}
	if len(entriesBefore) != len(entriesAfter) {
		t.Errorf("post-flight report count changed (%d -> %d) — prune-candidates must remove nothing",
			len(entriesBefore), len(entriesAfter))
	}
}

func TestPruneCandidates_NoActivePlanIsUnmeasured(t *testing.T) {
	root := t.TempDir()
	root, _ = filepath.EvalSymlinks(root)
	t.Chdir(root)
	pruneConfig(t, root)
	if err := os.MkdirAll(filepath.Join(root, "docs", "internal", "plans"), 0o755); err != nil {
		t.Fatalf("mkdir plans dir: %v", err)
	}
	// No PLAN-*.md at all.

	_, r := runPruneJSON(t, root)
	if r.Measured {
		t.Errorf("expected measured=false with no active plan, got true")
	}
}

// runPruneJSON invokes runPruneCandidates in --json mode against a
// buffer and decodes the resulting pruneCandidatesReport.
func runPruneJSON(t *testing.T, root string) (string, pruneCandidatesReport) {
	t.Helper()
	pruneCandidatesJSON = true
	t.Cleanup(func() { pruneCandidatesJSON = false })

	cmd := &cobra.Command{Use: "prune-candidates"}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)

	if err := runPruneCandidates(cmd, nil); err != nil {
		t.Fatalf("runPruneCandidates (json): %v", err)
	}
	var r pruneCandidatesReport
	if err := json.Unmarshal(buf.Bytes(), &r); err != nil {
		t.Fatalf("unmarshal report: %v (raw=%q)", err, buf.String())
	}
	return buf.String(), r
}
