package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Two numbers doctor did not report, so both read as healthy by being
// invisible: how much of the corpus carries any check at all, and how many
// claimed checks have never produced a result. A consumer's compile ran 30 of
// 976 verify items and skipped 946 with nothing saying so.

func plantSidecarWithVerifies(t *testing.T, root, id string, entries string) {
	t.Helper()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The corpus resolver needs the artifact dirs to exist for every kind,
	// the way scaffoldProject makes them.
	for _, d := range []string{"docs/architecture/invariants", "docs/guidelines"} {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	md := filepath.Join(dir, id+"-x.md")
	if err := os.WriteFile(md, []byte("---\nstatus: accepted\n---\n# "+id+"\n\nBody.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body := "schema_version: 2\ntopic: testing\npath: docs/architecture/decisions/" + id + "-x.md\n" + entries
	if err := os.WriteFile(filepath.Join(dir, id+"-x.edikt.yaml"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestEnforcementCoverage_ReportsTheFractionWithAnyCheck(t *testing.T) {
	root := t.TempDir()
	plantSidecarWithVerifies(t, root, "ADR-800", `directives:
  - text: "A MUST hold. (ref: ADR-800)"
    verify: "true"
    verify_kind: structural
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
  - text: "B MUST hold. (ref: ADR-800)"
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
`)
	plantSidecarWithVerifies(t, root, "ADR-801", `directives:
  - text: "C MUST hold. (ref: ADR-801)"
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
prohibitions:
  - text: "MUST NOT do D. (ref: ADR-801)"
    verify: "true"
    verify_kind: behavioral
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
`)
	var out bytes.Buffer
	runEnforcementCoverageCheck(root, &out)
	s := out.String()
	// 2 of 4 entries carry a verify.
	if !strings.Contains(s, "2 of 4") {
		t.Fatalf("the coverage line must carry the fraction:\n%s", s)
	}
	if !strings.Contains(s, "structural") || !strings.Contains(s, "behavioral") {
		t.Errorf("the breakdown by kind is the actionable half:\n%s", s)
	}
	// A directive with no check is UNENFORCED — a count, not a finding.
	if !strings.Contains(strings.ToLower(s), "unenforced") {
		t.Errorf("the line must name what the remainder is:\n%s", s)
	}
}

// A corpus with nothing to measure says so rather than reporting 0 of 0 as
// if it were a state worth reporting.
func TestEnforcementCoverage_EmptyCorpusIsSilent(t *testing.T) {
	var out bytes.Buffer
	runEnforcementCoverageCheck(t.TempDir(), &out)
	if out.Len() != 0 {
		t.Fatalf("no corpus, nothing to report:\n%s", out.String())
	}
}

// A claimed check with no recorded result is distinct from a failing one:
// "never ran" and "ran and failed" are different problems with different
// remedies, and collapsing them is how 946 skipped items read as fine.
func TestEnforcementCoverage_NeverRunIsCountedApartFromFailing(t *testing.T) {
	root := t.TempDir()
	plantSidecarWithVerifies(t, root, "ADR-802", `directives:
  - text: "E MUST hold. (ref: ADR-802)"
    verify: "true"
    verify_kind: structural
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
`)
	var out bytes.Buffer
	runEnforcementCoverageCheck(root, &out)
	s := out.String()
	if !strings.Contains(s, "never run") {
		t.Fatalf("a verify with no recorded result must be counted as never run:\n%s", s)
	}
	if strings.Contains(s, "failing") && !strings.Contains(s, "0 failing") {
		t.Errorf("never-run must not be reported as failing:\n%s", s)
	}
}

// D160: a suppressed directive is subtracted from every delivered surface
// at gov:compile time (INV-015's recorded removal), but this count used to
// iterate sc.Directives raw with no idea suppression existed -- a retired
// rule still inflated the denominator. Falsified before trusting the fix:
// this fixture plants a directive whose exact text is ALSO listed in
// suppressed_directives, and the coverage line must not count it.
func TestEnforcementCoverage_SuppressedDirectiveIsNotCounted(t *testing.T) {
	root := t.TempDir()
	plantSidecarWithVerifies(t, root, "ADR-803", `directives:
  - text: "F MUST hold. (ref: ADR-803)"
    verify: "true"
    verify_kind: structural
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
  - text: "G is retired and MUST NOT be enforced. (ref: ADR-803)"
    source_excerpts:
      - line_start: 5
        line_end: 5
        quote: "Body."
suppressed_directives:
  - "G is retired and MUST NOT be enforced. (ref: ADR-803)"
`)
	var out bytes.Buffer
	runEnforcementCoverageCheck(root, &out)
	s := out.String()
	// 1 of 1: the suppressed entry (G) must not count toward the total,
	// leaving only the live entry (F, which carries a verify).
	if !strings.Contains(s, "1 of 1") {
		t.Fatalf("a suppressed directive must not be counted -- want '1 of 1', got:\n%s", s)
	}
}
