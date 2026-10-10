package phasea

// receipt_counts_test.go — PLAN-v090-issue-fixes Phase 4 (#7 c3 Part 2,
// #11 c2-c).
//
// Two counts the receipt did not carry:
//
//   - Whitespace-only anchor faults were reported in one bucket with spliced
//     and misnumbered quotes. On the field shape (34 re-wrapped quotes, 1
//     splice) "35 anchors do not quote the parent" sends a reader looking for
//     35 extraction errors when 34 are a copy away from fixed.
//   - The needs_review count reached the user from the extractor's own
//     hand-back narration, which is not a measurement. The receipt now counts
//     the entries on the sidecar, and reports read it from there.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func TestClassifyDefects_WhitespaceOnlySeparated(t *testing.T) {
	dir := t.TempDir()
	var parent strings.Builder
	parent.WriteString("# ADR-900\n\n") // lines 1-2
	var ds []sidecar.Directive
	line := 3
	for i := 0; i < 34; i++ {
		fmt.Fprintf(&parent, "Rule %d MUST hold across\nthe wrapped line %d.\n", i, i)
		ds = append(ds, sidecar.Directive{
			Text: fmt.Sprintf("Rule %d MUST hold. (ref: ADR-900)", i),
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: line, LineEnd: line + 1,
				Quote: fmt.Sprintf("Rule %d MUST hold across the wrapped line %d.", i, i)}},
		})
		line += 2
	}
	// One splice and two valid anchors.
	parent.WriteString("First clause holds. Middle clause. Last clause holds.\n")
	ds = append(ds,
		sidecar.Directive{Text: "Spliced MUST fail. (ref: ADR-900)", SourceExcerpts: []sidecar.SourceExcerpt{{
			LineStart: line, LineEnd: line, Quote: "First clause holds. Last clause holds."}}},
		sidecar.Directive{Text: "Valid one MUST pass. (ref: ADR-900)", SourceExcerpts: []sidecar.SourceExcerpt{{
			LineStart: line, LineEnd: line, Quote: "First clause holds."}}},
		sidecar.Directive{Text: "Valid two MUST pass. (ref: ADR-900)", SourceExcerpts: []sidecar.SourceExcerpt{{
			LineStart: line, LineEnd: line, Quote: "Last clause holds."}}},
	)
	md := filepath.Join(dir, "ADR-900-test.md")
	if err := os.WriteFile(md, []byte(parent.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	sc := filepath.Join(dir, "ADR-900-test.edikt.yaml")
	writeMarshalledSidecar(t, sc, &sidecar.Sidecar{SchemaVersion: 2, Topic: "testing",
		Path: "docs/architecture/decisions/ADR-900-test.md", Directives: ds})

	rep := ClassifyDefects(sc, md)
	if rep.Axis != DefectAxisAnchor || rep.AnchorsBroken != 35 || rep.AnchorsValid != 2 {
		t.Fatalf("axis/broken/valid = %s/%d/%d, want anchor/35/2", rep.Axis, rep.AnchorsBroken, rep.AnchorsValid)
	}
	if rep.AnchorsWhitespaceOnly != 34 {
		t.Fatalf("whitespace-only = %d, want 34", rep.AnchorsWhitespaceOnly)
	}
	if want := "34 whitespace-only, 1 not verbatim/misnumbered; 2 valid"; !strings.Contains(rep.Summary, want) {
		t.Errorf("summary %q does not split the faults as %q", rep.Summary, want)
	}
	p := rep.CorrectionPrompt()
	if len(p) > 4000 {
		t.Fatalf("correction prompt is %d chars, over the receipt's 4000 cap", len(p))
	}
	// The one splice must survive the cap even with 34 whitespace faults
	// around it: it is the fault a copy does not fix.
	if !strings.Contains(p, "never splice") || !strings.Contains(p, fmt.Sprintf("lines %d-%d", line, line)) {
		t.Errorf("the splice is not named in the capped prompt:\n%s", p)
	}
	if !strings.Contains(p, `"Rule 0 MUST hold across\nthe wrapped line 0."`) {
		t.Errorf("the first whitespace-only fault's canonical bytes are missing:\n%s", p)
	}
}

func needsReviewDirectives(n, flagged int) []sidecar.Directive {
	var ds []sidecar.Directive
	for i := 0; i < n; i++ {
		d := anchoredAtGateQuote(fmt.Sprintf("Entry %d MUST be reviewed. (ref: ADR-900)", i))
		d.NeedsReview = i < flagged
		ds = append(ds, d)
	}
	return ds
}

func TestVerify_ReceiptCountsNeedsReview(t *testing.T) {
	for _, tc := range []struct {
		name    string
		flagged int
	}{{"five flagged", 5}, {"one flipped", 4}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, "")
			f.extractGood(t, needsReviewDirectives(5, tc.flagged)...)
			v := f.verify(t)
			it := onlyItem(t, v)
			if it.Status != StatusOK {
				t.Fatalf("fixture: status %q: %s", it.Status, v.Message)
			}
			if it.NeedsReview != tc.flagged {
				t.Fatalf("receipt needs_review = %d, want %d — counted from the sidecar, not narrated", it.NeedsReview, tc.flagged)
			}
			if want := fmt.Sprintf("needs_review: %d", tc.flagged); !strings.Contains(v.Message, want) {
				t.Errorf("the verify summary must carry %q:\n%s", want, v.Message)
			}
		})
	}

	t.Run("counted on a quarantined anchor-invalid item too", func(t *testing.T) {
		f := withoutReceipt(t, newFixture(t, ""))
		ds := needsReviewDirectives(3, 2)
		ds[2].SourceExcerpts[0].Quote = "a quote the parent does not contain"
		f.extractGood(t, ds...)
		f.restore(t)
		it := onlyItem(t, f.verify(t))
		if it.Status != StatusAnchorInvalid || it.NeedsReview != 2 {
			t.Fatalf("status/needs_review = %s/%d, want anchor-invalid/2", it.Status, it.NeedsReview)
		}
	})
}

func TestVerify_ReceiptCarriesWhitespaceOnlyCount(t *testing.T) {
	f := newWrappedFixture(t, "")
	f.extractGood(t, anchoredAt("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)", 5, 6,
		"The extractor MUST quote the artifact byte-exactly at the recorded range."))
	f.restore(t)
	it := onlyItem(t, f.verify(t))
	if it.AnchorsBroken != 1 || it.AnchorsWhitespaceOnly != 1 {
		t.Fatalf("anchors_broken/anchors_whitespace_only = %d/%d, want 1/1", it.AnchorsBroken, it.AnchorsWhitespaceOnly)
	}
}

// The two new counts are declared by phasea-receipt.v3 (additionalProperties
// is false, so an undeclared key would make every such receipt invalid).
func TestReceiptWithCountsMatchesSchemaShape(t *testing.T) {
	schema := readReceiptSchema(t)
	itemSchema := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	allowed := schemaProperties(t, itemSchema)

	f := newWrappedFixture(t, "")
	d := anchoredAt("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)", 5, 6,
		"The extractor MUST quote the artifact byte-exactly at the recorded range.")
	d.NeedsReview = true
	f.extractGood(t, d)
	f.restore(t)
	f.verify(t)
	raw, err := os.ReadFile(ReceiptFilePath(f.root, f.planID))
	if err != nil {
		t.Fatal(err)
	}
	var emitted struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(raw, &emitted); err != nil || len(emitted.Items) != 1 {
		t.Fatalf("receipt: %v %s", err, raw)
	}
	for _, k := range []string{"anchors_whitespace_only", "needs_review"} {
		if _, ok := emitted.Items[0][k]; !ok {
			t.Fatalf("fixture: the receipt item does not carry %q, so nothing is compared: %s", k, raw)
		}
	}
	assertSameKeys(t, "receipt item", emitted.Items[0], allowed, schemaRequired(t, itemSchema))
}
