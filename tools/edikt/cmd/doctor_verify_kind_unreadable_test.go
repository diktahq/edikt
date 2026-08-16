package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stageCoverageCorpus writes a decisions dir holding `good` valid sidecars
// and `rotten` unparseable ones, using the default artifact layout so
// resolveArtifactDirs finds them without a config file.
//
// This is the shape a real corpus takes as it rots: a sidecar is
// hand-edited, a merge conflict lands in one, an extractor writes a
// truncated file. The bad ones sit next to the good ones under the same
// directory the check already walks.
func stageCoverageCorpus(t *testing.T, good, rotten int) string {
	t.Helper()
	root := t.TempDir()
	dir := filepath.Join(root, "docs", "architecture", "decisions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	for i := 0; i < good; i++ {
		body := "schema_version: 1\ntopic: \"testing\"\n" +
			"path: \"docs/architecture/decisions/ADR-10" + string(rune('0'+i)) + "-x.md\"\n" +
			"signals:\n  - \"sig\"\n" +
			"directives:\n  - text: \"A rule MUST hold. (ref: ADR-10" + string(rune('0'+i)) + ")\"\n" +
			"    verify: \"true\"\n    verify_kind: \"structural\"\n" +
			"    source_excerpt:\n      line_start: 1\n      line_end: 1\n      quote: \"q\"\n"
		p := filepath.Join(dir, "ADR-10"+string(rune('0'+i))+"-x.edikt.yaml")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatalf("write good sidecar: %v", err)
		}
	}
	for i := 0; i < rotten; i++ {
		p := filepath.Join(dir, "ADR-90"+string(rune('0'+i))+"-rot.edikt.yaml")
		// Unbalanced YAML — Load fails at parse, before any validation.
		if err := os.WriteFile(p, []byte("schema_version: 1\ntopic: \"x\ndirectives: [\n"), 0o644); err != nil {
			t.Fatalf("write rotten sidecar: %v", err)
		}
	}
	return root
}

// TestVerifyKindCoverage_NamesUnreadableSidecars pins the drop.
//
// The walk did `sc, err := sidecar.Load(path); if err != nil { continue }`.
// An unparseable sidecar left the corpus silently: it never reached
// sidecarsSeen and never appeared in the output, so the coverage line
// described only the files that still parsed. Coverage therefore rose as
// data rotted — every sidecar that broke made the remaining tally look
// more complete, and nothing said how many had dropped out.
//
// INV-013: a control that HAD a subject and could not observe it must say
// so. The counts are worthless without knowing what they are counts OF.
func TestVerifyKindCoverage_NamesUnreadableSidecars(t *testing.T) {
	root := stageCoverageCorpus(t, 2, 3)

	var buf bytes.Buffer
	_, ran := runVerifyKindCoverageCheck(root, &buf)
	out := buf.String()

	if !ran {
		t.Fatal("check reported not-run against a corpus of 5 sidecars")
	}
	// Every dropped sidecar must be attributable to a file, not folded
	// into an anonymous shortfall the reader has to subtract out.
	got := strings.Count(out, "is unreadable")
	if got != 3 {
		t.Errorf("expected 3 unreadable sidecars named, got %d:\n%s", got, out)
	}
	for _, id := range []string{"ADR-900-rot", "ADR-901-rot", "ADR-902-rot"} {
		if !strings.Contains(out, id) {
			t.Errorf("unreadable sidecar %s is not named in the output:\n%s", id, out)
		}
	}
	// The denominator has to be visible, otherwise the tally is a bare
	// count with nothing to anchor it.
	if !strings.Contains(out, "2 of 5") && !strings.Contains(out, "2/5") {
		t.Errorf("coverage output carries no denominator naming how many sidecars were read:\n%s", out)
	}
}

// TestVerifyKindCoverage_AllUnreadableIsNotSilence is the boundary case
// the silent-drop hid.
//
// `if sidecarsSeen == 0 { return 0, false }` is correct for a project with
// no sidecars — announcing non-coverage of a subject that does not exist
// is noise. But with every sidecar unreadable, sidecarsSeen is also 0, and
// the check went quiet on a corpus it entirely failed to read. Doctor then
// looked identical on a non-edikt project and on an edikt project whose
// governance had been corrupted wholesale.
func TestVerifyKindCoverage_AllUnreadableIsNotSilence(t *testing.T) {
	root := stageCoverageCorpus(t, 0, 4)

	var buf bytes.Buffer
	warns, ran := runVerifyKindCoverageCheck(root, &buf)
	out := buf.String()

	if !ran {
		t.Fatalf("check stayed silent on 4 unreadable sidecars — indistinguishable "+
			"from a project with no sidecars at all:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "unreadable") {
		t.Errorf("output does not name the unreadable sidecars:\n%s", out)
	}
	if warns == 0 {
		t.Error("a wholly unreadable corpus produced no warning")
	}
}

// TestVerifyKindCoverage_NoSidecarsStaysSilent guards the other direction,
// so the fix above does not turn every non-edikt project into noise.
// INV-013 again: a control that had NO subject must stay silent.
func TestVerifyKindCoverage_NoSidecarsStaysSilent(t *testing.T) {
	root := stageCoverageCorpus(t, 0, 0)

	var buf bytes.Buffer
	_, ran := runVerifyKindCoverageCheck(root, &buf)

	if ran {
		t.Errorf("check spoke up on a project with no sidecars:\n%s", buf.String())
	}
}
