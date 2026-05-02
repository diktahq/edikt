package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// scaffoldPlanWithCriteria writes PLAN-<id>.md with the given progress
// table body and a sibling -criteria.yaml. Returns the project root.
func scaffoldPlanWithCriteria(t *testing.T, planID, planBody, criteriaBody string) string {
	t.Helper()
	root := t.TempDir()
	plansDir := filepath.Join(root, "docs", "internal", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "PLAN-"+planID+".md"), []byte(planBody), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "PLAN-"+planID+"-criteria.yaml"), []byte(criteriaBody), 0o644); err != nil {
		t.Fatalf("write criteria: %v", err)
	}
	return root
}

func writeReport(t *testing.T, root, planID, phase, ts string, failed int) string {
	t.Helper()
	dir := filepath.Join(root, ".edikt", "state", "verify")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body := `{"plan_id":"` + planID + `","phase":"` + phase +
		`","summary":{"passed":1,"failed":` + intStr(failed) + `,"skipped":0,"timeout":0,"total":1},"criteria":[]}`
	p := filepath.Join(dir, planID+"-phase-"+phase+"-"+ts+".json")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatalf("write report: %v", err)
	}
	return p
}

func intStr(n int) string {
	if n == 0 {
		return "0"
	}
	if n == 1 {
		return "1"
	}
	if n == 2 {
		return "2"
	}
	// Tests only use small values; punt on larger integers.
	return "9"
}

const minimalCriteria = `plan: demo
schema_version: 1
phases:
  - id: "1"
    name: phase one
    classification: testable
    criteria:
      - id: 1.1
        statement: ok
        verify: "exit 0"
`

const planWithDoneRow = `# PLAN-demo

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-05-02 |
| 2     | pending | 0/5    | -          |
`

func TestDoctorVerify_warnsWhenNoReport(t *testing.T) {
	root := scaffoldPlanWithCriteria(t, "demo", planWithDoneRow, minimalCriteria)
	var buf bytes.Buffer
	warns, ran := runVerifyChecks(root, &buf)
	if !ran {
		t.Fatal("expected ran=true since plan + criteria sidecar exist")
	}
	if warns != 1 {
		t.Errorf("warns: got %d, want 1\nout=%s", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "phase 1") {
		t.Errorf("expected mention of phase 1: %s", buf.String())
	}
}

func TestDoctorVerify_silentWhenNoPlans(t *testing.T) {
	root := t.TempDir()
	var buf bytes.Buffer
	warns, ran := runVerifyChecks(root, &buf)
	if ran || warns != 0 {
		t.Errorf("expected silent (ran=false, warns=0), got ran=%v warns=%d", ran, warns)
	}
	if buf.Len() != 0 {
		t.Errorf("expected no output: %s", buf.String())
	}
}

func TestDoctorVerify_quietWhenReportRecent(t *testing.T) {
	root := scaffoldPlanWithCriteria(t, "demo", planWithDoneRow, minimalCriteria)
	// Write a passing report timestamped well into the future so it beats
	// any criteria-file mtime threshold.
	future := time.Now().UTC().Add(24 * time.Hour).Format("20060102T150405Z")
	writeReport(t, root, "demo", "1", future, 0)
	var buf bytes.Buffer
	warns, ran := runVerifyChecks(root, &buf)
	if !ran {
		t.Fatal("expected ran=true")
	}
	if warns != 0 {
		t.Errorf("warns: got %d, want 0\n%s", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "All marked-done phases") {
		t.Errorf("expected success line: %s", buf.String())
	}
}

func TestDoctorVerify_warnsOnFailingReport(t *testing.T) {
	root := scaffoldPlanWithCriteria(t, "demo", planWithDoneRow, minimalCriteria)
	future := time.Now().UTC().Add(24 * time.Hour).Format("20060102T150405Z")
	writeReport(t, root, "demo", "1", future, 1) // 1 failure recorded
	var buf bytes.Buffer
	warns, _ := runVerifyChecks(root, &buf)
	if warns != 1 {
		t.Errorf("warns: got %d, want 1\n%s", warns, buf.String())
	}
}

func TestDoctorVerify_overrideMarkerWarns(t *testing.T) {
	const overridePlan = `# PLAN-demo

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | done (overrides: 2) |
`
	root := scaffoldPlanWithCriteria(t, "demo", overridePlan, minimalCriteria)
	var buf bytes.Buffer
	warns, _ := runVerifyChecks(root, &buf)
	if warns != 1 {
		t.Errorf("warns: got %d, want 1\n%s", warns, buf.String())
	}
	if !strings.Contains(buf.String(), "override") {
		t.Errorf("expected override mention: %s", buf.String())
	}
}

func TestParseProgressTable(t *testing.T) {
	const body = `# Plan

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | done   | 1/5     | 2026-05-02 |
| 4b    | pending | 0/5    | -          |
| 12    | done   | 2/5     | done (overrides: 1) |
`
	dir := t.TempDir()
	p := filepath.Join(dir, "x.md")
	os.WriteFile(p, []byte(body), 0o644)
	rows := parseProgressTable(p)
	if len(rows) != 3 {
		t.Fatalf("rows: got %d, want 3 — %v", len(rows), rows)
	}
	if rows[0].phase != "1" || rows[0].status != "done" {
		t.Errorf("row 0: %+v", rows[0])
	}
	if rows[1].phase != "4b" || rows[1].status != "pending" {
		t.Errorf("row 1: %+v", rows[1])
	}
	if rows[2].phase != "12" || !hasOverrideMarker(rows[2].updated) {
		t.Errorf("row 2: %+v", rows[2])
	}
}
