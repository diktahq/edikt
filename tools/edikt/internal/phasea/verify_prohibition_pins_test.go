package phasea

// verify_prohibition_pins_test.go — PLAN-SPEC-023 phase 4, AC-4.1's consumer
// half.
//
// WHY THIS IS NOT ASSERTED IN pinpreserve ALONE. `internal/pinpreserve` owns
// the comparison; `phasea verify` is what a human or a release gate actually
// looks at. Between them sit missingPins (which decides what to compare),
// verifyItem (which decides the status) and describeMissingPins (which decides
// what the message says) — three places a prohibition loss could be found by
// the package and still never reach anyone. SR-007 is explicit that the gate
// must report the loss, not merely that the package can see it, so the
// red-first evidence runs through the real consumer.
//
// Both tests were RED on the tree before phase 4: verifyItem returned StatusOK
// for a dropped prohibition carrying three pins, because computePins looped
// `before.Directives` and nothing else.

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/pinpreserve"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// pinnedProhibition is the pre-regeneration prohibition these tests lose. Its
// anchor quotes the parent's line 5 byte-exactly, the same way
// anchoredAtGateQuote's does, so the snapshot is a sidecar the loader accepts
// rather than a shape only this file could produce.
//
// verify_kind is STRUCTURAL rather than behavioral on purpose: the content gate
// refuses a behavioral entry with no fixture pair, so a behavioral fixture would
// grade schema-invalid and this file would be measuring the over-claim check
// instead of the pin loss.
func pinnedProhibition(text string) sidecar.Prohibition {
	return sidecar.Prohibition{
		Text:            text,
		SourceExcerpts:  []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: gateQuote}},
		Verify:          "bash check-cosign.sh",
		VerifyKind:      "structural",
		HumanApprovedAt: "2026-08-04T00:00:00Z",
	}
}

const prohibitionText = "The gate MUST NOT accept an unsigned payload. (ref: ADR-900)"

// TestVerifyItem_ProhibitionPinsUnrestorable drives a dropped prohibition pin
// through the real verify path and requires the item to grade
// pins-unrestorable.
//
// The negative control matters as much as the assertion: the same fixture with
// the prohibition CARRIED FORWARD must grade ok. Without it, a verify that
// returned pins-unrestorable for some unrelated reason — a missing receipt, a
// hash mismatch — would satisfy the first half while measuring nothing about
// prohibitions.
func TestVerifyItem_ProhibitionPinsUnrestorable(t *testing.T) {
	const rule = "A prior rule. (ref: ADR-900)"

	snapshot := func(f *verifyFixture) {
		writeMarshalledSidecar(t, PinSnapshotPath(f.root, f.planID, f.itemID), &sidecar.Sidecar{
			SchemaVersion: 2,
			Topic:         "testing",
			Path:          "docs/architecture/decisions/ADR-900-test.md",
			Directives:    []sidecar.Directive{anchoredAtGateQuote(rule)},
			Prohibitions:  []sidecar.Prohibition{pinnedProhibition(prohibitionText)},
		})
	}

	// CONTROL — the regenerated sidecar still carries the prohibition and its
	// pins, so nothing is missing and the item grades ok.
	control := newStaleFixture(t, anchoredAtStaleQuote(rule))
	snapshot(control)
	writeMarshalledSidecar(t, control.scPath, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtGateQuote(rule)},
		Prohibitions:  []sidecar.Prohibition{pinnedProhibition(prohibitionText)},
	})
	if it := onlyItem(t, control.verify(t)); it.Status != StatusOK {
		t.Fatalf("the control graded %q, want %q — a fixture that cannot grade ok says nothing about "+
			"the loss below", it.Status, StatusOK)
	}

	// THE SUBJECT — the extractor emitted the prohibition with none of the
	// human-authored fields, which is what it always does: it never sees the
	// prior sidecar.
	f := newStaleFixture(t, anchoredAtStaleQuote(rule))
	snapshot(f)
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtGateQuote(rule)},
		Prohibitions: []sidecar.Prohibition{{
			Text:           prohibitionText,
			SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: gateQuote}},
		}},
	})

	out := f.verify(t)
	it := onlyItem(t, out)
	if it.Status == StatusOK {
		t.Fatalf("a regeneration that dropped a prohibition's approved verify and its approval stamp "+
			"graded %q. Before SPEC-023 phase 4 this was the tree's behaviour: computePins looped "+
			"before.Directives only, so the gate built to catch a wiped pin reported PASS on the run "+
			"that wiped it (SR-007). Message: %s", it.Status, out.Message)
	}
	if it.Status != StatusPinsUnrestorable {
		t.Fatalf("item graded %q, want %q: %s", it.Status, StatusPinsUnrestorable, out.Message)
	}
	detail := out.Details[it.ItemID]
	for _, want := range []string{"verify", "human_approved_at", "bash check-cosign.sh", prohibitionText} {
		if !strings.Contains(detail, want) {
			t.Errorf("the verdict does not name %q, so a reader cannot re-approve what was lost:\n%s",
				want, detail)
		}
	}
}

// TestDescribeMissingPins_NamesProhibitionField asserts the message half on its
// own, because the status half above is satisfied by a verdict that says only
// "pins-unrestorable".
//
// The `missing` list is produced by a REAL MissingPins call over a real
// before/after pair rather than hand-built, so this cannot pass on a
// PinnedField shape the package never emits.
func TestDescribeMissingPins_NamesProhibitionField(t *testing.T) {
	dir := t.TempDir()
	after := filepath.Join(dir, "ADR-900-test.edikt.yaml")
	writeMarshalledSidecar(t, after, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtGateQuote("A prior rule. (ref: ADR-900)")},
	})

	before := &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtGateQuote("A prior rule. (ref: ADR-900)")},
		Prohibitions:  []sidecar.Prohibition{pinnedProhibition(prohibitionText)},
	}
	missing, err := pinpreserve.MissingPins("ADR-900", before, nil, after)
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) == 0 {
		t.Fatal("MissingPins reported nothing for a dropped prohibition carrying three pins, so the " +
			"message below would be built from an empty list (INV-013)")
	}
	kinds := map[pinpreserve.EntryKind]bool{}
	for _, m := range missing {
		kinds[m.EntryKind] = true
	}
	if !kinds[pinpreserve.KindProhibition] {
		t.Fatalf("no reported pin carries EntryKind %q: %+v", pinpreserve.KindProhibition, missing)
	}

	msg := describeMissingPins(PlanItem{
		ItemID:         "ADR-900",
		ExpectedOutput: ExpectedOutput{Path: "docs/architecture/decisions/ADR-900-test.edikt.yaml"},
	}, missing)
	for _, want := range []string{
		"verify=bash check-cosign.sh",
		"verify_kind=structural",
		"human_approved_at=2026-08-04T00:00:00Z",
		prohibitionText,
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("describeMissingPins does not name %q — \"pins-unrestorable\" alone tells a human "+
				"something a colleague approved is gone without saying what:\n%s", want, msg)
		}
	}
}
