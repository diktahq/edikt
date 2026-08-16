package reextract

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

func anchored(text string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 7, LineEnd: 7, Quote: "A rule lives here."}},
	}
}

func writeSidecar(t *testing.T, path string, sc *sidecar.Sidecar) {
	t.Helper()
	sc.SchemaVersion = 2
	sc.Topic = "testing"
	sc.Path = "docs/architecture/decisions/ADR-900.md"
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestPreserve_ApprovedPathsSurviveRegeneration is the failure that actually
// happened: the first full-corpus run wiped approved scope on 20 artifacts,
// because the extractor never sees the previous sidecar.
func TestPreserve_ApprovedPathsSurviveRegeneration(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")

	before := &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
		Paths:      []string{"tools/edikt/**/*.go", "commands/**/*.md"},
		PathsApproval: &sidecar.PathsApproval{
			ApprovedAt:  "2026-08-01T00:00:00Z",
			GlobsSHA256: sidecar.HashGlobs([]string{"commands/**/*.md", "tools/edikt/**/*.go"}),
		},
	}
	// The regenerated sidecar has no paths at all — exactly what the extractor
	// writes, since it cannot know what a human approved.
	writeSidecar(t, path, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("A MUST rule. (ref: ADR-900)")},
	})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.PathsRestored {
		t.Fatal("approved paths were not restored")
	}
	after, err := sidecar.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(after.Paths) != 2 {
		t.Fatalf("paths after restore = %v; want both approved globs", after.Paths)
	}
	if after.PathsApproval == nil || after.PathsApproval.ApprovedAt != "2026-08-01T00:00:00Z" {
		t.Fatalf("the approval receipt did not survive: %+v", after.PathsApproval)
	}
	if len(res.Unrestorable) != 0 {
		t.Fatalf("unexpected unrestorable entries: %v", res.Unrestorable)
	}
}

// TestPreserve_VerifyAndApprovalSurviveExactMatch — the common case: the
// directive came back with the same text.
func TestPreserve_VerifyAndApprovalSurviveExactMatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")
	text := "The loader MUST reject unknown keys. (ref: ADR-900)"

	pinned := anchored(text)
	pinned.Verify = "rg -q 'KnownFields\\(true\\)' loader.go"
	pinned.VerifyKind = "structural"
	pinned.HumanApprovedAt = "2026-08-02T10:00:00Z"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored(text)}})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if res.DirectivePins != 1 {
		t.Fatalf("restored %d directive pin(s); want 1", res.DirectivePins)
	}
	after, _ := sidecar.Load(path)
	d := after.Directives[0]
	if d.Verify != pinned.Verify || d.VerifyKind != "structural" || d.HumanApprovedAt != pinned.HumanApprovedAt {
		t.Fatalf("pins did not survive: %+v", d)
	}
}

// TestPreserve_RewordedDirectiveMatchesByNormalizedForm — the v4 contract
// re-words directives by design, so exact-match alone would drop most pins.
func TestPreserve_RewordedDirectiveMatchesByNormalizedForm(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")

	pinned := anchored("The loader MUST reject unknown keys. (ref: ADR-900)")
	pinned.Verify = "rg -q 'KnownFields' loader.go"
	pinned.VerifyKind = "structural"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	writeSidecar(t, path, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("The loader MUST NOT accept unknown keys. (ref: ADR-900)")},
	})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := sidecar.Load(path)
	if res.DirectivePins == 1 {
		if after.Directives[0].Verify == "" {
			t.Fatal("reported a restored pin but the sidecar carries none")
		}
		return
	}
	// Not matching is ACCEPTABLE — but then it must be REPORTED, never
	// silently dropped. That is the property under test.
	if len(res.Unrestorable) == 0 {
		t.Fatal("the pin was neither restored nor reported — a silent drop is the failure this exists to prevent")
	}
}

// TestPreserve_AmbiguousMatchIsReportedNotGuessed — attaching an approved
// verify to the wrong directive claims a human approved a command against a
// rule they never saw it on.
func TestPreserve_AmbiguousMatchIsReportedNotGuessed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")

	pinned := anchored("The gate MUST refuse an unsigned payload. (ref: ADR-900)")
	pinned.Verify = "bash test/fixtures/behavioral/ADR-900/positive.sh"
	pinned.VerifyKind = "behavioral"
	pinned.HumanApprovedAt = "2026-08-03T00:00:00Z"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	// Two directives whose normalized subject is identical.
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{
		anchored("The gate MUST refuse an unsigned payload. (ref: ADR-900)"),
		anchored("The gate MUST NOT refuse an unsigned payload. (ref: ADR-900)"),
	}})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := sidecar.Load(path)
	// The exact-text match is legitimate here and must win.
	if after.Directives[0].HumanApprovedAt != "2026-08-03T00:00:00Z" {
		t.Fatalf("the exact-text match did not receive the pin: %+v", after.Directives[0])
	}
	if after.Directives[1].HumanApprovedAt != "" || after.Directives[1].Verify != "" {
		t.Fatal("a pin was attached to a directive it was never approved against")
	}
	_ = res
}

// TestPreserve_DroppedDirectiveReportsItsPin — the pinned directive has no
// counterpart at all. The approval is lost by construction; what must not
// happen is losing it QUIETLY.
func TestPreserve_DroppedDirectiveReportsItsPin(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")

	pinned := anchored("An entirely distinct obligation about cosign bundles MUST hold. (ref: ADR-900)")
	pinned.Verify = "bash check-cosign.sh"
	pinned.VerifyKind = "behavioral"
	pinned.HumanApprovedAt = "2026-08-04T00:00:00Z"

	before := &sidecar.Sidecar{Directives: []sidecar.Directive{pinned}}
	writeSidecar(t, path, &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("Something completely different about topic registries. (ref: ADR-900)")},
	})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unrestorable) < 2 {
		t.Fatalf("reported %d unrestorable field(s); want the verify AND the approval stamp named", len(res.Unrestorable))
	}
	joined := ""
	for _, u := range res.Unrestorable {
		joined += u.Field + " " + u.Reason + " "
	}
	for _, want := range []string{"verify", "human_approved_at", "no counterpart"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the report omits %q: %s", want, joined)
		}
	}
}

// TestPreserve_StaleApprovalReceiptIsReported — a receipt whose hash no longer
// covers the globs is not re-stamped. Re-stamping would be this code approving
// a scope on a human's behalf.
func TestPreserve_StaleApprovalReceiptIsReported(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")

	before := &sidecar.Sidecar{
		Directives:    []sidecar.Directive{anchored("A rule. (ref: ADR-900)")},
		Paths:         []string{"tools/**/*.go"},
		PathsApproval: &sidecar.PathsApproval{ApprovedAt: "2026-08-01T00:00:00Z", GlobsSHA256: sidecar.HashGlobs([]string{"something/else/**"})},
	}
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A rule. (ref: ADR-900)")}})

	res, err := PreservePinned("ADR-900", before, nil, path)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, u := range res.Unrestorable {
		if u.Field == "paths_approval" {
			found = true
		}
	}
	if !found {
		t.Fatalf("a receipt that does not cover its globs was accepted silently: %+v", res.Unrestorable)
	}
}

// TestPreserve_NoPriorSidecarIsNotAnError — the bootstrap case.
func TestPreserve_NoPriorSidecarIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A rule. (ref: ADR-900)")}})

	res, err := PreservePinned("ADR-900", nil, nil, path)
	if err != nil {
		t.Fatalf("bootstrap treated as an error: %v", err)
	}
	if res.DirectivePins != 0 || len(res.Unrestorable) != 0 {
		t.Fatalf("bootstrap reported work it did not do: %+v", res)
	}
}

// TestPreserve_LoadErrIsNotTreatedAsBootstrap — F-115/A1, the swallow itself,
// not a symptom. A nil `before` means two entirely different things: "no
// prior sidecar" (nothing was ever pinned — TestPreserve_
// NoPriorSidecarIsNotAnError, above) and "a prior sidecar existed and failed
// to load" (unknown pinned state, silently discarded if this function cannot
// tell the two apart). This test asserts the MECHANISM that distinguishes
// them — beforeLoadErr must never be treated as "nothing to preserve" the
// way a genuinely nil before is — not any one field's loss. The prior bug
// wasn't "paths doesn't survive a load error"; it was "nothing does, and
// nothing says so."
func TestPreserve_LoadErrIsNotTreatedAsBootstrap(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A rule. (ref: ADR-900)")}})

	loadErr := fmt.Errorf("yaml: unknown field \"bogus_field\"")
	res, err := PreservePinned("ADR-900", nil, loadErr, path)
	if err != nil {
		t.Fatalf("a load-error report must not itself be an error: %v", err)
	}

	// The swallow, precisely: this must NOT read like the bootstrap case.
	if !res.LoadFailed {
		t.Fatal("LoadFailed is false — a load error was silently treated as \"nothing pinned\", the exact bug this test exists to catch")
	}
	if len(res.Unrestorable) == 0 {
		t.Fatal("zero Unrestorable entries for a load-failed prior sidecar — silently indistinguishable from a clean bootstrap")
	}

	// The symptom, checked as a floor, not the whole claim: F-115 named
	// paths, verify, verify_kind, and human_approved_at as exposed. All four
	// (plus paths_approval and both fixture-path fields — the same code path
	// covers all seven pinnable fields, not a hand-picked subset) must be
	// named, or a future field addition to `pin`/`pinOf` could silently fall
	// outside this report the same way paths originally did.
	byField := map[string]bool{}
	for _, u := range res.Unrestorable {
		byField[u.Field] = true
		if !strings.Contains(u.Reason, "failed to load") {
			t.Errorf("field %q's reason does not explain why: %q", u.Field, u.Reason)
		}
	}
	for _, want := range pinnedFieldNamesAtRisk {
		if !byField[want] {
			t.Errorf("field %q is not named as at-risk when the prior sidecar fails to load", want)
		}
	}
}

// TestPreserve_LoadErrTakesPriorityOverBefore — beforeLoadErr must win even
// if a caller (incorrectly) supplies both a non-nil before AND a non-nil
// beforeLoadErr. discover.go/reextract.go never do this — Sidecar and
// LoadErr are populated as mutually exclusive — but PreservePinned's own
// contract must not depend on trusting that invariant from every future
// caller silently.
func TestPreserve_LoadErrTakesPriorityOverBefore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "ADR-900.edikt.yaml")
	writeSidecar(t, path, &sidecar.Sidecar{Directives: []sidecar.Directive{anchored("A rule. (ref: ADR-900)")}})

	before := &sidecar.Sidecar{
		Directives: []sidecar.Directive{anchored("A rule. (ref: ADR-900)")},
		Paths:      []string{"tools/**/*.go"},
	}
	res, err := PreservePinned("ADR-900", before, fmt.Errorf("parse error"), path)
	if err != nil {
		t.Fatal(err)
	}
	if !res.LoadFailed {
		t.Fatal("a non-nil beforeLoadErr must win regardless of what before itself contains")
	}
}
