package gov

import (
	"errors"
	"strings"
	"testing"
)

func d(id string) Directive { return Directive{ID: id, Grade: "advisory", Text: "rule " + id} }

// Files resolving to the SAME directive set collapse into one review unit. This
// is the cost driver: reviewing per file re-sends identical directives once per
// file, and on a whole-tree pass that is the difference between affordable and
// not.
func TestBuildReviewUnits_CollapsesIdenticalSets(t *testing.T) {
	resolve := func(p string) ([]Directive, error) {
		if strings.HasPrefix(p, "go/") {
			return []Directive{d("A:1"), d("B:2")}, nil
		}
		return []Directive{d("C:3")}, nil
	}
	rep := BuildReviewUnits([]string{"go/a.go", "go/b.go", "go/c.go", "sh/x.sh"}, resolve)

	if rep.Unmeasured {
		t.Fatalf("a resolvable set was reported unmeasured: %s", rep.Reason)
	}
	if rep.ReviewUnits != 2 {
		t.Errorf("want 2 units (3 go files collapse to 1), got %d", rep.ReviewUnits)
	}
	if rep.FilesScanned != 4 || rep.FilesMatched != 4 {
		t.Errorf("scanned=%d matched=%d, want 4/4", rep.FilesScanned, rep.FilesMatched)
	}
	if rep.DirectivesTot != 3 {
		t.Errorf("distinct directives = %d, want 3", rep.DirectivesTot)
	}
}

// Set identity must not depend on the order the matcher returns directives in,
// or two files governed identically would be reviewed twice.
func TestBuildReviewUnits_OrderIndependentSetIdentity(t *testing.T) {
	resolve := func(p string) ([]Directive, error) {
		if p == "a" {
			return []Directive{d("X:1"), d("Y:2")}, nil
		}
		return []Directive{d("Y:2"), d("X:1")}, nil
	}
	rep := BuildReviewUnits([]string{"a", "b"}, resolve)
	if rep.ReviewUnits != 1 {
		t.Errorf("directive order changed the set identity: got %d units, want 1", rep.ReviewUnits)
	}
}

// EMPTY SCOPE IS UNMEASURED. This is the defect being removed: an empty
// directive list produced verdicts: [], which consumers read as "verified, no
// violations".
func TestBuildReviewUnits_NoFilesIsUnmeasuredNotClean(t *testing.T) {
	rep := BuildReviewUnits(nil, func(string) ([]Directive, error) { return nil, nil })
	if !rep.Unmeasured {
		t.Fatal("an empty file set was not reported as unmeasured — it would render as a clean pass")
	}
	if rep.Reason == "" {
		t.Error("unmeasured with no reason: the report cannot be acted on")
	}
}

// Files exist but none is governed. True, sayable, and NOT a verified-clean
// result — the distinction the old code collapsed.
func TestBuildReviewUnits_NoGovernedFileIsUnmeasured(t *testing.T) {
	rep := BuildReviewUnits([]string{"a", "b"}, func(string) ([]Directive, error) { return nil, nil })
	if !rep.Unmeasured {
		t.Fatal("a scan where nothing is governed reported as measured — that is verdicts:[] again")
	}
	if rep.FilesUnmapped != 2 {
		t.Errorf("ungoverned files not counted: %d, want 2", rep.FilesUnmapped)
	}
	if !strings.Contains(rep.Reason, "nothing was reviewed") {
		t.Errorf("reason does not say nothing was reviewed: %q", rep.Reason)
	}
}

// A broken matcher must be UNMEASURED, never a clean scan. If the index is
// missing every file fails, and reporting zero findings would be the loudest
// possible version of absence-as-pass.
func TestBuildReviewUnits_TotalResolveFailureIsUnmeasured(t *testing.T) {
	rep := BuildReviewUnits([]string{"a", "b"}, func(string) ([]Directive, error) {
		return nil, errors.New("index missing")
	})
	if !rep.Unmeasured {
		t.Fatal("total matcher failure reported as a clean scan")
	}
	if !strings.Contains(rep.Reason, "index") {
		t.Errorf("reason does not name the cause: %q", rep.Reason)
	}
}

// A PARTIAL failure is not unmeasured — some files really were reviewed. The
// distinction matters: degrading a partial result to unmeasured would hide real
// findings, and promoting it to clean would hide the gap.
func TestBuildReviewUnits_PartialFailureStillReports(t *testing.T) {
	resolve := func(p string) ([]Directive, error) {
		if p == "bad" {
			return nil, errors.New("boom")
		}
		return []Directive{d("A:1")}, nil
	}
	rep := BuildReviewUnits([]string{"bad", "good"}, resolve)
	if rep.Unmeasured {
		t.Fatalf("a partial failure was reported unmeasured, hiding the file that WAS reviewed: %s", rep.Reason)
	}
	if rep.ReviewUnits != 1 || rep.FilesMatched != 1 {
		t.Errorf("units=%d matched=%d, want 1/1", rep.ReviewUnits, rep.FilesMatched)
	}
}

// Output must be deterministic: same input, same unit order and same file order
// inside each unit. A report that reshuffles between runs cannot be diffed.
func TestBuildReviewUnits_DeterministicOrdering(t *testing.T) {
	resolve := func(p string) ([]Directive, error) {
		switch p {
		case "z", "a":
			return []Directive{d("M:1")}, nil
		default:
			return []Directive{d("N:2")}, nil
		}
	}
	first := BuildReviewUnits([]string{"z", "a", "q"}, resolve)
	second := BuildReviewUnits([]string{"q", "a", "z"}, resolve)

	if len(first.Units) != len(second.Units) {
		t.Fatalf("unit counts differ: %d vs %d", len(first.Units), len(second.Units))
	}
	for i := range first.Units {
		if first.Units[i].SetHash != second.Units[i].SetHash {
			t.Errorf("unit %d hash differs across input orderings", i)
		}
		if strings.Join(first.Units[i].Files, ",") != strings.Join(second.Units[i].Files, ",") {
			t.Errorf("unit %d file order differs: %v vs %v", i, first.Units[i].Files, second.Units[i].Files)
		}
	}
}
