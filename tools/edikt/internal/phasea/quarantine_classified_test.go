package phasea

// quarantine_classified_test.go — PLAN-v090-issue-fixes Phase 4 (#7, c3-c).
//
// THE DEFECT. Restore moves a sidecar that fails the content gate into
// .edikt/state/phasea/<plan_id>.quarantine/, so verify found nothing at the
// expected path and graded the item output-missing. output-missing carries no
// defect axis and no correction_prompt, so the "seeded retry" re-dispatched
// the original prompt blind — on every axis — while the retry text promised a
// correction pass. Every real content-gate failure takes that route, because
// restore runs before verify.
//
// THE FIX, asserted here: verify reads restore's receipt; a quarantined item
// is classified from the quarantine copy (anchor-invalid, length-exceeded, or
// schema-invalid with its axis) and carries a self-contained correction_prompt
// naming each faulting PARENT line range (D-I: the extractor never sees the
// quarantined sidecar, so the prompt cannot point into it).

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// wrappedParagraph sits at lines 5-7 of the ADR acceptedADRBody writes.
const wrappedParagraph = "The extractor MUST quote the artifact\n" + // 5
	"byte-exactly at the recorded range.\n" + // 6
	"It MUST NOT splice two sentences together." // 7

// newWrappedFixture plans a bootstrap item over an ADR whose rule is wrapped
// across lines 5-6, with no restore receipt planted: these tests run the
// real restore, because the quarantine is restore's.
func newWrappedFixture(t *testing.T, configBody string) *verifyFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	if configBody != "" {
		writeConfig(t, root, configBody)
	}
	md := writeADR(t, root, "ADR-900", wrappedParagraph)
	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil || out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("fixture did not produce a one-item plan: %v %+v", err, out)
	}
	return &verifyFixture{
		root: root, planID: out.Plan.PlanID, plan: out.Plan,
		mdPath: md, scPath: sidecarPathFor(md), itemID: out.Plan.Items[0].ItemID,
	}
}

func anchoredAt(text string, start, end int, quote string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: start, LineEnd: end, Quote: quote}},
	}
}

func (f *verifyFixture) verifyWith(t *testing.T, opts VerifyOptions) *VerifyOutcome {
	t.Helper()
	opts.ProjectRoot, opts.PlanID = f.root, f.planID
	out, _ := Verify(opts)
	if out == nil {
		t.Fatal("Verify() returned a nil outcome")
	}
	return out
}

func TestVerify_QuarantinedItemCarriesCorrectionPrompt(t *testing.T) {
	f := newWrappedFixture(t, "phasea:\n  max_attempts_per_item: 3\n")
	f.extractGood(t,
		// Re-wrapped: the words are the parent's, the line break is not.
		anchoredAt("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)", 5, 6,
			"The extractor MUST quote the artifact byte-exactly at the recorded range."),
		// Spliced: two pieces of the range joined into one quote.
		anchoredAt("Quotes MUST be contiguous. (ref: ADR-900)", 5, 7,
			"The extractor MUST quote the artifact. It MUST NOT splice two sentences together."),
		// Valid.
		anchoredAt("Splicing is forbidden. (ref: ADR-900)", 7, 7, "It MUST NOT splice two sentences together."),
	)

	r := f.restore(t)
	if !containsID(r.Quarantined, f.itemID) {
		t.Fatalf("fixture: restore did not quarantine the item: %s", r.Message)
	}

	// --stalled must not override a quarantined item: the extractor DID
	// write, and the output is the evidence (conflict #5).
	v := f.verifyWith(t, VerifyOptions{Stalled: []string{f.itemID}})
	it := onlyItem(t, v)
	if it.Status != StatusAnchorInvalid {
		t.Fatalf("a quarantined anchor failure must arrive classified, got %q: %s", it.Status, v.Message)
	}
	if v.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("exit %d, want %d (retryable): %s", v.ExitCode, ExitVerifyIncomplete, v.Message)
	}
	if it.DefectAxis != DefectAxisAnchor || it.AnchorsBroken != 2 || it.AnchorsValid != 1 {
		t.Errorf("axis/broken/valid = %s/%d/%d, want anchor/2/1", it.DefectAxis, it.AnchorsBroken, it.AnchorsValid)
	}
	qrel := filepath.ToSlash(relPath(f.root, QuarantinePath(f.root, f.planID, f.itemID)))
	if !strings.Contains(it.Diagnostic, "quarantined by restore at "+qrel) {
		t.Errorf("the diagnostic must name the quarantine copy %s:\n%s", qrel, it.Diagnostic)
	}

	p := it.CorrectionPrompt
	if p == "" {
		t.Fatal("a quarantined anchor item carries no correction_prompt — the retry is unseeded")
	}
	for _, want := range []string{
		"CORRECTION PASS",
		"lines 5-6",
		"lines 5-7",
		// The parent's exact bytes for the re-wrapped span, YAML double-quoted.
		`"The extractor MUST quote the artifact\nbyte-exactly at the recorded range."`,
		"never splice",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("correction_prompt omits %q:\n%s", want, p)
		}
	}
	if len(p) > 4000 {
		t.Errorf("correction_prompt is %d chars, over the receipt's 4000 cap", len(p))
	}
}

func TestVerify_QuarantinedLengthItemCarriesCorrectionPrompt(t *testing.T) {
	f := withoutReceipt(t, newFixture(t, "phasea:\n  max_attempts_per_item: 3\n"))
	f.extractGood(t, overCap(MaxItemTextLen+72, "The service MUST publish every domain event."))

	r := f.restore(t)
	if !containsID(r.Quarantined, f.itemID) {
		t.Fatalf("fixture: an over-hard-cap sidecar must fail the gate and be quarantined: %s", r.Message)
	}
	v := f.verify(t)
	it := onlyItem(t, v)
	if it.Status != StatusLengthExceeded {
		t.Fatalf("a quarantined length failure must arrive classified, got %q: %s", it.Status, v.Message)
	}
	if v.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("exit %d, want %d: %s", v.ExitCode, ExitVerifyIncomplete, v.Message)
	}
	p := it.CorrectionPrompt
	for _, want := range []string{
		"CORRECTION PASS", "lines 5-5", fmt.Sprintf("%d", MaxItemTextLen+72), fmt.Sprintf("%d", MaxItemTextLen),
		"The service MUST publish every domain event.",
	} {
		if !strings.Contains(p, want) {
			t.Errorf("correction_prompt omits %q:\n%s", want, p)
		}
	}
	// The extractor never sees the quarantined sidecar: a prompt that says to
	// reuse its excerpts points at nothing.
	if strings.Contains(p, "reusing the same source_excerpts") || strings.Contains(p, "reuse the same source_excerpts") {
		t.Errorf("the length prompt assumes a sidecar the extractor cannot see:\n%s", p)
	}
}

// A restore re-run in the same round, with the sidecar still quarantined,
// keeps the item listed: otherwise the second run's receipt would forget the
// quarantine and verify would fall back to output-missing.
func TestRestore_RerunKeepsQuarantine(t *testing.T) {
	f := withoutReceipt(t, newFixture(t, ""))
	f.extractGood(t, overCap(MaxItemTextLen+72, "The service MUST publish every domain event."))
	f.restore(t)
	r := f.restore(t)
	if !containsID(r.Quarantined, f.itemID) {
		t.Fatalf("a same-round re-run dropped the quarantine: %s", r.Message)
	}
	if it := onlyItem(t, f.verify(t)); it.Status != StatusLengthExceeded {
		t.Fatalf("after a re-run, status %q, want %q", it.Status, StatusLengthExceeded)
	}
}
