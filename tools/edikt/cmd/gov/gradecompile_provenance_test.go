package gov

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/gradecompile"
	"github.com/diktahq/edikt/tools/edikt/internal/phasea"
)

// gradecompile_provenance_test.go — `--model` is PROVENANCE, not a selector.
//
// THE DEFECT, measured against the shipped binary before the change: the flag
// defaulted to the literal "claude-opus-4-7" and its value is written verbatim
// into report.grader_model, which records WHICH MODEL GRADED. Nothing in this
// package dispatches a grader (ADR-044, INV-012), so a grader report that
// omitted its model produced a persisted report asserting it had been graded
// by claude-opus-4-7 — a model that may never have run. A grade report naming
// a grader that did not grade it is worse than one naming none. INV-013's
// shape at the provenance layer.

const gradeReportNoModel = `{"schema_version":1,"graded_at":"2026-09-06T00:00:00Z",` +
	`"target_dir":".claude/rules/governance",` +
	`"scores":{"coherence":7,"conciseness":6,"signal_to_noise":6,` +
	`"description_quality":7,"tier_assignment":7,"no_double_loading":7},` +
	`"overall":6,"findings":[],"summary":"ok"}`

const gradeReportWithModel = `{"schema_version":1,"graded_at":"2026-09-06T00:00:01Z",` +
	`"grader_model":"claude-sonnet-4-5","target_dir":".claude/rules/governance",` +
	`"scores":{"coherence":7,"conciseness":6,"signal_to_noise":6,` +
	`"description_quality":7,"tier_assignment":7,"no_double_loading":7},` +
	`"overall":6,"findings":[],"summary":"ok"}`

// recordGrade runs `gov grade-compile --record <file>` in a scratch project and
// returns the grader_model the persisted report ended up carrying.
func recordGrade(t *testing.T, body string, extraArgs ...string) (string, int) {
	t.Helper()
	bin := buildBinary(t)
	root := t.TempDir()
	reportPath := filepath.Join(root, "grade.json")
	if err := os.WriteFile(reportPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write grader report: %v", err)
	}

	args := append([]string{"gov", "grade-compile", "--record", reportPath}, extraArgs...)
	cmd := exec.Command(bin, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "EDIKT_INVOKER=ci", "EDIKT_SKIP_VERSION_GATE=1")
	out, err := cmd.CombinedOutput()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatalf("exec: %v\n%s", err, out)
	}
	if code != 0 {
		return "", code
	}

	written, gerr := filepath.Glob(filepath.Join(root, ".edikt", "state", "compile-quality", "*.json"))
	if gerr != nil || len(written) != 1 {
		t.Fatalf("expected exactly one persisted report, got %v (err=%v)\n%s", written, gerr, out)
	}
	raw, rerr := os.ReadFile(written[0])
	if rerr != nil {
		t.Fatalf("read persisted report: %v", rerr)
	}
	var got struct {
		GraderModel string `json:"grader_model"`
	}
	if uerr := json.Unmarshal(raw, &got); uerr != nil {
		t.Fatalf("parse persisted report: %v\n%s", uerr, raw)
	}
	return got.GraderModel, code
}

// TestGradeCompile_OmittedModelRecordsUnrecorded is the defect, end to end. A
// grader report with no grader_model must persist as `unrecorded`, never as a
// model name the binary supplied.
func TestGradeCompile_OmittedModelRecordsUnrecorded(t *testing.T) {
	got, code := recordGrade(t, gradeReportNoModel)
	if code != 0 {
		t.Fatalf("exit %d recording a report that omits grader_model", code)
	}
	if got != gradecompile.GraderModelUnrecorded {
		t.Errorf("grader_model: got %q, want %q — the binary named a grader that may never have run",
			got, gradecompile.GraderModelUnrecorded)
	}
	// The specific literal that was there. Naming it keeps the test pointed at
	// the defect rather than at any value that happens to differ.
	if got == "claude-opus-4-7" {
		t.Error("grader_model still defaults to claude-opus-4-7 — fabricated provenance")
	}
}

// TestGradeCompile_ReportedModelSurvives is the must-NOT-break direction: the
// fix must not blank real provenance. A report that DOES name its model keeps
// that value untouched.
func TestGradeCompile_ReportedModelSurvives(t *testing.T) {
	got, code := recordGrade(t, gradeReportWithModel)
	if code != 0 {
		t.Fatalf("exit %d recording a report that names its model", code)
	}
	if got != "claude-sonnet-4-5" {
		t.Errorf("grader_model: got %q, want %q — the agent's own provenance was overwritten",
			got, "claude-sonnet-4-5")
	}
}

// TestGradeCompile_ExplicitModelIsStillHonoured — an operator who KNOWS which
// model graded can still record it. The default is the absent case, not a lock.
func TestGradeCompile_ExplicitModelIsStillHonoured(t *testing.T) {
	got, code := recordGrade(t, gradeReportNoModel, "--model", "claude-haiku-4-5")
	if code != 0 {
		t.Fatalf("exit %d with an explicit --model", code)
	}
	if got != "claude-haiku-4-5" {
		t.Errorf("grader_model: got %q, want the explicitly passed %q", got, "claude-haiku-4-5")
	}
}

// TestGradeCompile_EmptyModelIsStillRefused — absence needs a NAME. The schema
// requires grader_model minLength 1, so an emptied flag must refuse rather than
// persist a blank that reads as unreadable instead of as stated-absent.
func TestGradeCompile_EmptyModelIsStillRefused(t *testing.T) {
	_, code := recordGrade(t, gradeReportNoModel, "--model", "")
	if code != 3 {
		t.Errorf("exit code: got %d, want 3 on an empty --model", code)
	}
}

// TestGraderModelUnrecorded_MatchesTheExtractorProvenanceLiteral pins the two
// spellings together. edikt already had a reserved provenance-absent literal
// (v0.9.0 AC-7.4: "the host-reported model, or `unrecorded` when the host
// reports none"); a second, differently-spelled one would be two names for one
// concept, and a reader grepping for the absent case would find half of them.
func TestGraderModelUnrecorded_MatchesTheExtractorProvenanceLiteral(t *testing.T) {
	if gradecompile.GraderModelUnrecorded != phasea.ExtractorContractUnrecorded {
		t.Errorf("provenance-absent literal drifted: gradecompile %q vs phasea %q",
			gradecompile.GraderModelUnrecorded, phasea.ExtractorContractUnrecorded)
	}
	// And it must NOT be the SELECTION literal. "inherit" means "take the
	// host's model" — recording it here would claim a selection was made.
	if gradecompile.GraderModelUnrecorded == phasea.ExtractorModelInherit {
		t.Error("provenance-absent literal is the selection literal 'inherit' — " +
			"the report would claim a model was chosen")
	}
}
