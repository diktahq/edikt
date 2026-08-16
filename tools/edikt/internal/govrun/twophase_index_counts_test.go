package govrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/model"
)

// stageWithRetiredADR writes two ADRs into a never-initialised project: one
// live, one carrying the canonical `**Status:** Superseded by ADR-001` line.
//
// This is PRODUCTION'S input shape and it is the whole point of the test.
// The retired ADR is skip-listed by sidecar.Discover and filtered out by
// RunTwoPhase before Phase B ever sees it, so Phase B is handed a pairs
// slice containing exactly one entry. A test that instead constructs a
// pairs slice with a Skip entry and hands it to phaseb.Merge directly is
// testing a shape the pipeline never produces — the previous attempt at
// this fix passed that way while the defect survived.
func stageWithRetiredADR(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	adrDir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(adrDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	// Line 9 is "Test rule." — the quote the fake runner anchors to.
	live := "---\nstatus: accepted\n---\n\n# ADR-001 — Test\n\n## Decision\n\nTest rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-001-test.md"), []byte(live), 0o644); err != nil {
		t.Fatalf("write live adr: %v", err)
	}

	retired := "---\nstatus: superseded\n---\n\n# ADR-002 — Retired\n\n**Status:** Superseded by ADR-001\n\n## Decision\n\nOld rule.\n"
	if err := os.WriteFile(filepath.Join(adrDir, "ADR-002-retired.md"), []byte(retired), 0o644); err != nil {
		t.Fatalf("write retired adr: %v", err)
	}
	return root
}

// TestRunTwoPhase_IndexReportsRetiredExclusions pins the governance.md
// source header against the counter defect.
//
// The header used to read "N ADRs (N accepted, 0 superseded)": the accepted
// count was a copy of the total and the superseded count was structurally
// zero because nothing ever assigned it. On the dogfood corpus that rendered
// as "41 ADRs (41 accepted, 0 superseded)" against 53 ADR files of which 8
// are retired — a measurement the pipeline never made, reported as a fact.
//
// Phase B cannot derive the retired count on its own; by the time it runs,
// the retired pairs are gone. So the count travels as data from the filter
// site, and the header states what the compiled file actually contains.
func TestRunTwoPhase_IndexReportsRetiredExclusions(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageWithRetiredADR(t)
	runner := &bootstrapRunner{}
	var errBuf, outBuf bytes.Buffer

	if _, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{}); err != nil {
		t.Fatalf("compile failed: %v\nstderr:\n%s", err, errBuf.String())
	}

	// The retired ADR must not have been dispatched to the extractor —
	// if it were, the "excluded" count below would be measuring the wrong
	// thing and the test could pass for the wrong reason.
	if len(runner.dispatched) != 1 || runner.dispatched[0] != "ADR-001" {
		t.Fatalf("expected only ADR-001 dispatched; got %v", runner.dispatched)
	}

	idxBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("governance.md not written: %v", err)
	}
	got := string(idxBody)

	const want = "1 ADRs, 0 invariants, 0 guidelines compiled; 1 retired artifact excluded (adr 1)"
	if !strings.Contains(got, want) {
		t.Fatalf("source header does not report the retired ADR.\nwant substring: %s\ngot header:\n%s", want, headerOf(got))
	}

	// The old wording claimed an accepted/superseded split the pipeline
	// never computed. It must not come back.
	if strings.Contains(got, "accepted, 0 superseded") {
		t.Fatalf("source header still carries the unmeasured accepted/superseded split:\n%s", headerOf(got))
	}
}

// TestRunTwoPhase_IndexReportsZeroExclusionsAsMeasured is the control case.
// A project with nothing retired must say so explicitly rather than going
// quiet — "0 excluded" here is a real measurement, and it has to be
// distinguishable from the unmeasured case that render.excludedNote
// produces for a caller that never counted (INV-013).
func TestRunTwoPhase_IndexReportsZeroExclusionsAsMeasured(t *testing.T) {
	t.Setenv("EDIKT_HEADLESS", "")

	root := stageNeverInitialised(t)
	runner := &bootstrapRunner{}
	var errBuf, outBuf bytes.Buffer

	if _, err := RunTwoPhase(TwoPhaseOptions{
		ProjectRoot: root,
		Runner:      runner,
		Stderr:      &errBuf,
		Stdout:      &outBuf,
		OnLoss:      "accept",
	}, model.RealClock{}); err != nil {
		t.Fatalf("compile failed: %v\nstderr:\n%s", err, errBuf.String())
	}

	idxBody, err := os.ReadFile(filepath.Join(root, ".claude", "rules", "governance.md"))
	if err != nil {
		t.Fatalf("governance.md not written: %v", err)
	}
	got := string(idxBody)

	const want = "1 ADRs, 0 invariants, 0 guidelines compiled; 0 retired artifacts excluded"
	if !strings.Contains(got, want) {
		t.Fatalf("source header does not report a measured zero.\nwant substring: %s\ngot header:\n%s", want, headerOf(got))
	}
	if strings.Contains(got, "UNMEASURED") {
		t.Fatalf("a run that DID count exclusions must not report them as unmeasured:\n%s", headerOf(got))
	}
}

// headerOf returns the HTML-comment metadata block of a compiled index so
// failure output shows the counts rather than the whole governance file.
// The block sits after the YAML frontmatter, so this collects comment lines
// wherever they start rather than assuming line 1.
func headerOf(body string) string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "<!--") {
			out = append(out, line)
			continue
		}
		if len(out) > 0 {
			break
		}
	}
	return strings.Join(out, "\n")
}
