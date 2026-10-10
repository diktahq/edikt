package pinpreserve

// preserve_characterisation_test.go — PLAN-SPEC-023 phase 1 (AC-1.1, AC-1.3).
//
// WHAT THIS FILE IS. Fifteen characterisation cells: the five fields
// `internal/pinpreserve` preserves TODAY (`paths`, `paths_approval`, `verify`,
// `verify_kind`, `human_approved_at`) crossed with the three observable
// outcomes the package can produce for each of them —
//
//	restored      the pin is carried into the regenerated sidecar on disk
//	unrestorable  the pin is NAMED in PreserveResult.Unrestorable
//	at-risk       the pin is NAMED in the load-error report (earlyResult)
//
// — asserted against UNCHANGED production source. Nothing here is a proposal
// for how the package should behave; every cell records what it does behave
// like on the tree that carries this file, so that phases 2 and 3 can move the
// implementation and be told, mechanically, whether they moved the behaviour
// with it. AC-2.5 requires this file to stay BYTE-IDENTICAL through phase 2,
// which is why it depends on no helper defined outside it: a shared helper
// edited by a later phase would silently re-point these assertions.
//
// EVIDENCE CLASS. Every cell here is green on unmodified source, so none of
// them is RED-on-tree evidence. Their falsification is the recorded mutation
// kill in `test/fixtures/pinpreserve-mutation-kills.yaml`, run by
// `test/test-pinpreserve-mutation-kills.sh`: for each cell, a named source
// edit is applied to a scratch copy of the module, the cell is required to go
// red, and the edit is reverted. That harness proves the recorded edit turns
// the cell red; it CANNOT prove the edit is the behaviour the cell claims to
// cover. That mapping is hand-authored in the manifest, and is the phase's
// declared residual attestation rather than something it claims away.
//
// ONE CELL DIVERGES FROM AC-1.1's PROSE, DELIBERATELY.
// `TestCharacterisation_Paths_DroppedEntryProducesNoUnrestorable` is the
// (paths x unrestorable) cell, and it asserts the OPPOSITE of the outcome
// AC-1.1 predicts: artifact-level `paths` is never named in `Unrestorable` on
// a dropped entry, because the restore at preserve.go:230 is unconditional and
// there is no per-entry text-matching step that could fail for it. This is the
// same structural fact PLAN-SPEC-023's AC-5.2 states for `manual_directives`
// and `suppressed_directives` — "whole-list artifact-level fields with no
// text-matching step, so there is no per-entry unrestorable path" — which
// applies equally to `paths` and was not carried into AC-1.1. The cell is
// written two-sided: it fails if the loss ever STARTS being reported, so it
// cannot quietly become a test of an unreachable state. `paths_approval`, by
// contrast, DOES have a reachable unrestorable path — the stale-hash branch —
// and that is cell 5, pinned explicitly by AC-1.3.

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ---------------------------------------------------------------------------
// Fixtures. Self-contained by design (see AC-2.5 note above).
// ---------------------------------------------------------------------------

// charAnchored builds a minimally valid v2 directive: text plus one source
// anchor, which Validate requires of every entry.
func charAnchored(text string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 7, LineEnd: 7, Quote: "A rule lives here."}},
	}
}

// charWriteAfter marshals `sc` as the REGENERATED sidecar and returns its
// path. This is the file `PreservePinned` loads, mutates and rewrites.
func charWriteAfter(t *testing.T, sc *sidecar.Sidecar) string {
	t.Helper()
	sc.SchemaVersion = 2
	sc.Topic = "testing"
	sc.Path = "docs/architecture/decisions/ADR-923.md"
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatalf("marshalling the regenerated fixture: %v", err)
	}
	path := filepath.Join(t.TempDir(), "ADR-923.edikt.yaml")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatalf("writing the regenerated fixture: %v", err)
	}
	return path
}

// charReload reads the sidecar back FROM DISK. Every "restored" cell asserts
// against the reloaded file rather than against the returned result: a
// PreserveResult reporting a restore that never reached the filesystem is the
// shape this whole package exists to make impossible, so the result is never
// the sole witness for its own claim.
func charReload(t *testing.T, path string) *sidecar.Sidecar {
	t.Helper()
	sc, err := sidecar.Load(path)
	if err != nil {
		t.Fatalf("reloading the restored sidecar: %v", err)
	}
	return sc
}

// charUnrestorable returns the Unrestorable entries naming `field`.
func charUnrestorable(res *PreserveResult, field string) []PinnedField {
	var out []PinnedField
	for _, u := range res.Unrestorable {
		if u.Field == field {
			out = append(out, u)
		}
	}
	return out
}

// charLoadErrReport drives the load-error arm and returns the report. The
// regenerated sidecar is written but never read — earlyResult returns before
// PreservePinned reaches sidecar.Load — and that is itself part of the
// characterisation: a load failure on the PRIOR sidecar is answered without
// consulting the new one at all.
func charLoadErrReport(t *testing.T) *PreserveResult {
	t.Helper()
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("A MUST rule. (ref: ADR-923)")},
	})
	res, err := PreservePinned("ADR-923", nil, errors.New("yaml: line 4: mapping values are not allowed"), path)
	if err != nil {
		t.Fatalf("a load-error report must not itself be an error: %v", err)
	}
	if !res.LoadFailed {
		t.Fatal("LoadFailed is false — the load error was answered as if it were a clean bootstrap")
	}
	return res
}

// charAtRiskNamed asserts `field` is named in a load-error report, with a
// reason that says why. The field name is written out LITERALLY here, never
// read from pinnedFieldNamesAtRisk: a check that derives its expectation from
// the list it is checking shrinks in lockstep with the defect (INV-014), which
// is the exact state preserve_test.go:297 has been in since ADR-070.
func charAtRiskNamed(t *testing.T, res *PreserveResult, field string) {
	t.Helper()
	hits := charUnrestorable(res, field)
	if len(hits) == 0 {
		t.Fatalf("%q is not named as at-risk when the prior sidecar fails to load; the report named %d field(s): %v",
			field, len(res.Unrestorable), charFieldNames(res))
	}
	if hits[0].Reason == "" {
		t.Fatalf("%q is named as at-risk with no reason — the report says a pin may be lost without saying why", field)
	}
}

func charFieldNames(res *PreserveResult) []string {
	out := make([]string, 0, len(res.Unrestorable))
	for _, u := range res.Unrestorable {
		out = append(out, u.Field)
	}
	return out
}

// ---------------------------------------------------------------------------
// paths — artifact-level approved scope.
// ---------------------------------------------------------------------------

// CELL 1/15 — paths x restored.
//
// The directive text is IDENTICAL before and after, which is the exact-text
// match AC-1.1 names. Note what the cell shows about that: artifact-level
// `paths` restores VERBATIM and is not matched to any entry, so the identical
// text is the surrounding condition rather than the mechanism. The mechanism
// is preserve.go:230, and it is unconditional on entry matching.
func TestCharacterisation_Paths_RestoredOnExactTextMatch(t *testing.T) {
	text := "The loader MUST reject unknown keys. (ref: ADR-923)"
	approved := []string{"tools/edikt/**/*.go", "commands/**/*.md"}

	before := &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored(text)},
		Paths:      approved,
	}
	// What the extractor writes: it cannot know what a human approved.
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored(text)},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.PathsRestored {
		t.Fatal("PathsRestored is false — the approved scope was not carried forward")
	}
	after := charReload(t, path)
	if len(after.Paths) != 2 || after.Paths[0] != "commands/**/*.md" || after.Paths[1] != "tools/edikt/**/*.go" {
		t.Fatalf("paths on disk = %v; want both approved globs, sorted", after.Paths)
	}
}

// CELL 2/15 — paths x unrestorable. THE DIVERGENT CELL (see the file header).
//
// The pinned directive is dropped outright and replaced by an unrelated one,
// which is the shape that makes an ENTRY pin unrestorable. For artifact-level
// `paths` it produces no such report, because the restore never had to find
// anything: the approved scope is carried forward regardless of what happened
// to the entries. This cell asserts that observed outcome and is TWO-SIDED —
// it fails if a report ever appears — so a later phase that gives `paths` an
// unrestorable path turns this red instead of leaving a dead assertion behind.
func TestCharacterisation_Paths_DroppedEntryProducesNoUnrestorable(t *testing.T) {
	before := &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("An obligation about cosign bundles MUST hold. (ref: ADR-923)")},
		Paths:      []string{"tools/edikt/**/*.go"},
	}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("Something else entirely about topic registries. (ref: ADR-923)")},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if hits := charUnrestorable(res, "paths"); len(hits) != 0 {
		t.Fatalf("`paths` is now reported unrestorable on a dropped entry (%d entr(ies): %+v). "+
			"That is a BEHAVIOUR CHANGE this cell exists to catch: on unmodified source the "+
			"artifact-level restore is unconditional and produces no such report. If the change is "+
			"intended, this cell is the record that must be updated deliberately", len(hits), hits)
	}
	// The other half of the observation, so the cell cannot pass by the paths
	// machinery having stopped running altogether.
	if !res.PathsRestored {
		t.Fatal("PathsRestored is false — the approved scope was not carried forward either, so the " +
			"absence of an unrestorable report above measured nothing")
	}
}

// CELL 3/15 — paths x at-risk on a prior-sidecar load failure.
func TestCharacterisation_Paths_NamedAtRiskOnLoadError(t *testing.T) {
	charAtRiskNamed(t, charLoadErrReport(t), "paths")
}

// ---------------------------------------------------------------------------
// paths_approval — the receipt that says a human approved that scope.
// ---------------------------------------------------------------------------

// CELL 4/15 — paths_approval x restored.
//
// The regenerated sidecar already carries the approved globs, so the paths
// branch is not entered and this cell observes the RECEIPT restore alone.
func TestCharacterisation_PathsApproval_RestoredOnExactTextMatch(t *testing.T) {
	text := "The loader MUST reject unknown keys. (ref: ADR-923)"
	approved := []string{"tools/edikt/**/*.go"}

	before := &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored(text)},
		Paths:      approved,
		PathsApproval: &sidecar.PathsApproval{
			ApprovedAt:  "2026-08-01T00:00:00Z",
			GlobsSHA256: sidecar.HashGlobs(approved),
		},
	}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored(text)},
		Paths:      approved,
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.ApprovalRestored {
		t.Fatal("ApprovalRestored is false — the approval receipt was not carried forward")
	}
	after := charReload(t, path)
	if after.PathsApproval == nil {
		t.Fatal("the approval receipt is absent from the sidecar on disk")
	}
	if after.PathsApproval.ApprovedAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("approved_at on disk = %q; want the pinned stamp", after.PathsApproval.ApprovedAt)
	}
	if after.PathsApproval.GlobsSHA256 != sidecar.HashGlobs(approved) {
		t.Fatalf("globs_sha256 on disk = %q; want the pinned hash", after.PathsApproval.GlobsSHA256)
	}
}

// CELL 5/15 — paths_approval x unrestorable. AC-1.3, pinned explicitly.
//
// preserve.go:259. When HashGlobs(after.Paths) does not equal the receipt's
// recorded GlobsSHA256, TWO things must hold and are asserted SEPARATELY:
// the receipt is NAMED in Unrestorable, and it is NOT RE-STAMPED. Asserting
// only the first passes an implementation that reports the receipt and then
// quietly recomputes its hash — which would be this code approving a scope on
// a human's behalf, the worse of the two outcomes.
//
// The fixture makes a WRITE happen, because "not re-stamped" is only measured
// if something was written at all: an assertion against a file nobody rewrote
// would pass on any implementation. The write is driven by the approved scope
// differing between before and after — NOT by a directive pin, which would make
// this cell red whenever the `verify` write site is mutated and so make it
// impossible to tell the two cells apart.
//
// The receipt is already present on the regenerated sidecar, so the restore
// branch at preserve.go:242 is not entered either. What remains is the
// stale-hash arm alone.
//
// This is the cell a table-driven rewrite is most likely to flatten into the
// scalar case — `paths_approval` fits neither the entry-level nor the
// artifact-level table, and its rule is conditional on a hash it must not
// recompute.
func TestCharacterisation_PathsApprovalStaleHash(t *testing.T) {
	text := "The gate MUST refuse an unsigned payload. (ref: ADR-923)"
	approved := []string{"tools/edikt/**/*.go"}
	staleHash := sidecar.HashGlobs([]string{"some/other/scope/**"})

	before := &sidecar.Sidecar{
		Directives:    []sidecar.Directive{charAnchored(text)},
		Paths:         approved,
		PathsApproval: &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: staleHash},
	}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives:    []sidecar.Directive{charAnchored(text)},
		Paths:         []string{"docs/**/*.md"},
		PathsApproval: &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: staleHash},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.SidecarRewritten {
		t.Fatal("the fixture did not cause a write, so \"not re-stamped\" below would measure nothing")
	}

	// Half one: NAMED.
	hits := charUnrestorable(res, "paths_approval")
	if len(hits) == 0 {
		t.Fatalf("a receipt whose hash does not cover the restored globs was accepted silently; "+
			"the report named %v", charFieldNames(res))
	}
	if hits[0].Reason == "" {
		t.Fatal("the stale receipt is named with no reason — nothing tells the human to re-approve")
	}

	// Half two: NOT RE-STAMPED.
	after := charReload(t, path)
	if after.PathsApproval == nil {
		t.Fatal("the receipt vanished from disk; the stale-hash arm must report, never delete")
	}
	if after.PathsApproval.GlobsSHA256 != staleHash {
		t.Fatalf("globs_sha256 on disk = %q; want the ORIGINAL stale hash %q. Re-stamping the receipt "+
			"is this code approving a scope on a human's behalf", after.PathsApproval.GlobsSHA256, staleHash)
	}
}

// CELL 6/15 — paths_approval x at-risk on a prior-sidecar load failure.
func TestCharacterisation_PathsApproval_NamedAtRiskOnLoadError(t *testing.T) {
	charAtRiskNamed(t, charLoadErrReport(t), "paths_approval")
}

// ---------------------------------------------------------------------------
// verify — the approved command for one directive.
// ---------------------------------------------------------------------------

// CELL 7/15 — verify x restored on an exact-text match.
func TestCharacterisation_Verify_RestoredOnExactTextMatch(t *testing.T) {
	text := "The loader MUST reject unknown keys. (ref: ADR-923)"
	want := "rg -q 'KnownFields\\(true\\)' loader.go"

	pinned := charAnchored(text)
	pinned.Verify = want

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{charAnchored(text)}})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.DirectivePins != 1 {
		t.Fatalf("DirectivePins = %d; want 1", res.DirectivePins)
	}
	if res.MatchedByNormForm != 0 {
		t.Fatalf("MatchedByNormForm = %d; the texts are byte-identical, so this must be the EXACT-text arm",
			res.MatchedByNormForm)
	}
	if got := charReload(t, path).Directives[0].Verify; got != want {
		t.Fatalf("verify on disk = %q; want %q", got, want)
	}
}

// CELL 8/15 — verify x unrestorable when the entry it was pinned to is dropped.
//
// The approval is lost by construction — the directive it was approved against
// no longer exists. What must not happen is losing it QUIETLY, and the report
// must carry the VALUE, not merely the field name, or nobody can re-approve
// without going to git history.
func TestCharacterisation_Verify_NamedUnrestorableOnDroppedEntry(t *testing.T) {
	want := "bash check-cosign.sh"

	pinned := charAnchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-923)")
	pinned.Verify = want

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("Something completely different about topic registries. (ref: ADR-923)")},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	hits := charUnrestorable(res, "verify")
	if len(hits) == 0 {
		t.Fatalf("the pinned verify was dropped without being named; the report named %v", charFieldNames(res))
	}
	if hits[0].Value != want {
		t.Fatalf("the report names the field but carries value %q; want %q — a name with no value cannot be re-approved",
			hits[0].Value, want)
	}
	if hits[0].DirectiveText != pinned.Text {
		t.Fatalf("the report does not say WHICH directive lost the pin: %q", hits[0].DirectiveText)
	}
	if got := charReload(t, path).Directives[0].Verify; got != "" {
		t.Fatalf("the pin was attached to a directive it was never approved against: %q", got)
	}
}

// CELL 9/15 — verify x at-risk on a prior-sidecar load failure.
func TestCharacterisation_Verify_NamedAtRiskOnLoadError(t *testing.T) {
	charAtRiskNamed(t, charLoadErrReport(t), "verify")
}

// ---------------------------------------------------------------------------
// verify_kind — the classification of that command's enforcement medium.
// ---------------------------------------------------------------------------

// CELL 10/15 — verify_kind x restored on an exact-text match.
//
// Pinned ALONE, with no verify beside it. That is deliberate: it is the shape
// that proves verify_kind has its own capture-and-write path rather than
// riding along on verify's, and it is what makes this cell's mutation kill
// isolable from cell 7's.
func TestCharacterisation_VerifyKind_RestoredOnExactTextMatch(t *testing.T) {
	text := "The loader MUST reject unknown keys. (ref: ADR-923)"

	pinned := charAnchored(text)
	pinned.VerifyKind = "structural"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{charAnchored(text)}})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.DirectivePins != 1 {
		t.Fatalf("DirectivePins = %d; want 1", res.DirectivePins)
	}
	if got := charReload(t, path).Directives[0].VerifyKind; got != "structural" {
		t.Fatalf("verify_kind on disk = %q; want %q", got, "structural")
	}
}

// CELL 11/15 — verify_kind x unrestorable when its entry is dropped.
func TestCharacterisation_VerifyKind_NamedUnrestorableOnDroppedEntry(t *testing.T) {
	pinned := charAnchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-923)")
	pinned.VerifyKind = "behavioral"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("Something completely different about topic registries. (ref: ADR-923)")},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	hits := charUnrestorable(res, "verify_kind")
	if len(hits) == 0 {
		t.Fatalf("the pinned verify_kind was dropped without being named; the report named %v", charFieldNames(res))
	}
	if hits[0].Value != "behavioral" {
		t.Fatalf("the report names the field but carries value %q; want %q", hits[0].Value, "behavioral")
	}
	if hits[0].DirectiveText != pinned.Text {
		t.Fatalf("the report does not say WHICH directive lost the pin: %q", hits[0].DirectiveText)
	}
}

// CELL 12/15 — verify_kind x at-risk on a prior-sidecar load failure.
func TestCharacterisation_VerifyKind_NamedAtRiskOnLoadError(t *testing.T) {
	charAtRiskNamed(t, charLoadErrReport(t), "verify_kind")
}

// ---------------------------------------------------------------------------
// human_approved_at — the record that the approval ceremony happened.
// ---------------------------------------------------------------------------

// CELL 13/15 — human_approved_at x restored on an exact-text match.
func TestCharacterisation_HumanApprovedAt_RestoredOnExactTextMatch(t *testing.T) {
	text := "The loader MUST reject unknown keys. (ref: ADR-923)"
	want := "2026-08-02T10:00:00Z"

	pinned := charAnchored(text)
	pinned.HumanApprovedAt = want

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{Directives: []sidecar.Directive{charAnchored(text)}})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.DirectivePins != 1 {
		t.Fatalf("DirectivePins = %d; want 1", res.DirectivePins)
	}
	if got := charReload(t, path).Directives[0].HumanApprovedAt; got != want {
		t.Fatalf("human_approved_at on disk = %q; want %q", got, want)
	}
}

// CELL 14/15 — human_approved_at x unrestorable when its entry is dropped.
//
// Of the five, this is the one whose silent loss is least visible downstream:
// a missing verify shows up the moment anything tries to run it, while a
// missing approval stamp simply reads as "never approved."
func TestCharacterisation_HumanApprovedAt_NamedUnrestorableOnDroppedEntry(t *testing.T) {
	want := "2026-08-04T00:00:00Z"

	pinned := charAnchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-923)")
	pinned.HumanApprovedAt = want

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	path := charWriteAfter(t, &sidecar.Sidecar{
		Directives: []sidecar.Directive{charAnchored("Something completely different about topic registries. (ref: ADR-923)")},
	})

	res, err := PreservePinned("ADR-923", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	hits := charUnrestorable(res, "human_approved_at")
	if len(hits) == 0 {
		t.Fatalf("the pinned approval stamp was dropped without being named; the report named %v", charFieldNames(res))
	}
	if hits[0].Value != want {
		t.Fatalf("the report names the field but carries value %q; want %q", hits[0].Value, want)
	}
	if hits[0].DirectiveText != pinned.Text {
		t.Fatalf("the report does not say WHICH directive lost the pin: %q", hits[0].DirectiveText)
	}
	if got := charReload(t, path).Directives[0].HumanApprovedAt; got != "" {
		t.Fatalf("an approval stamp was welded to a directive no human approved: %q", got)
	}
}

// CELL 15/15 — human_approved_at x at-risk on a prior-sidecar load failure.
func TestCharacterisation_HumanApprovedAt_NamedAtRiskOnLoadError(t *testing.T) {
	charAtRiskNamed(t, charLoadErrReport(t), "human_approved_at")
}
