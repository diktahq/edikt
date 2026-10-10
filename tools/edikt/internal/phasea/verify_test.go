package phasea

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// ── fixture helpers ──────────────────────────────────────────────────────

// gateQuote sits at line 5 of every ADR these tests write (see
// acceptedADRBody), so a sidecar anchored at 5..5 quoting it clears the
// ADR-056 rung of the content gate.
const gateQuote = "The extractor MUST quote the artifact byte-exactly."

// staleQuote is prose that is NOT in the parent, which is what makes a
// sidecar carrying it stale by quote drift — the condition plan.go plans on.
const staleQuote = "an older line of prose that no longer exists"

// verifyFixture is a planned project: an extractor agent with a pinned
// prompt_version, one accepted ADR, and an emitted plan whose single item
// covers it.
type verifyFixture struct {
	root   string
	planID string
	plan   *DispatchPlan
	mdPath string
	scPath string
	itemID string
}

// newFixture plans a project whose single item's reason is "missing" — no
// prior sidecar, therefore no pin snapshot. The simplest shape for the
// exit-code and retry behaviour, which none of the pin machinery affects.
func newFixture(t *testing.T, configBody string) *verifyFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	if configBody != "" {
		writeConfig(t, root, configBody)
	}
	md := writeADR(t, root, "ADR-900", gateQuote)

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("fixture did not produce a one-item plan: exit %d, %s", out.ExitCode, out.Message)
	}
	// EVERY fixture that drives Verify plants a restore receipt, because
	// verify now refuses without one — the coupling that makes the contract's
	// order enforceable rather than conventional. The tests below are about
	// item verdicts, not about the coupling; the coupling has its own tests in
	// restore_receipt_test.go, including the refusal.
	if err := writeRestoreReceipt(root, out.Plan.PlanID, &RestoreOutcome{ExitCode: ExitRestoreOK}); err != nil {
		t.Fatalf("planting the restore receipt: %v", err)
	}

	return &verifyFixture{
		root:   root,
		planID: out.Plan.PlanID,
		plan:   out.Plan,
		mdPath: md,
		scPath: sidecarPathFor(md),
		itemID: out.Plan.Items[0].ItemID,
	}
}

// newStaleFixture plans a project whose single item's reason is quote drift:
// a prior sidecar exists, so plan.go captures a pin snapshot of it. before is
// the prior sidecar's directive, carrying whatever pins the test needs.
func newStaleFixture(t *testing.T, before sidecar.Directive) *verifyFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	md := writeADR(t, root, "ADR-900", gateQuote)
	sc := filepath.Join(filepath.Dir(md), "ADR-900-test.edikt.yaml")
	writeMarshalledSidecar(t, sc, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{before},
	})

	out, err := Plan(PlanOptions{ProjectRoot: root})
	if err != nil {
		t.Fatalf("Plan(): %v", err)
	}
	if out.ExitCode != ExitPlanEmitted || len(out.Plan.Items) != 1 {
		t.Fatalf("fixture did not produce a one-item plan: exit %d, %s", out.ExitCode, out.Message)
	}
	if out.Plan.Items[0].Reason != ReasonStaleQuoteDrift {
		t.Fatalf("fixture item reason is %q, want %q", out.Plan.Items[0].Reason, ReasonStaleQuoteDrift)
	}
	if _, err := os.Stat(PinSnapshotPath(root, out.Plan.PlanID, out.Plan.Items[0].ItemID)); err != nil {
		t.Fatalf("fixture did not capture a pin snapshot: %v", err)
	}
	// Same reason as in newFixture: verify refuses without a restore receipt,
	// and these fixtures are about item verdicts. The refusal has its own
	// tests.
	if err := writeRestoreReceipt(root, out.Plan.PlanID, &RestoreOutcome{ExitCode: ExitRestoreOK}); err != nil {
		t.Fatalf("planting the restore receipt: %v", err)
	}

	return &verifyFixture{
		root:   root,
		planID: out.Plan.PlanID,
		plan:   out.Plan,
		mdPath: md,
		scPath: sc,
		itemID: out.Plan.Items[0].ItemID,
	}
}

// anchoredAtGateQuote builds a directive whose single anchor quotes the
// parent's line 5 byte-exactly.
func anchoredAtGateQuote(text string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: gateQuote}},
	}
}

// anchoredAtStaleQuote builds a directive whose anchor quotes prose that is
// NOT in the parent — the drift that makes a sidecar stale.
func anchoredAtStaleQuote(text string) sidecar.Directive {
	return sidecar.Directive{
		Text:           text,
		SourceExcerpts: []sidecar.SourceExcerpt{{LineStart: 5, LineEnd: 5, Quote: staleQuote}},
	}
}

func writeMarshalledSidecar(t *testing.T, path string, sc *sidecar.Sidecar) {
	t.Helper()
	body, err := sidecar.Marshal(sc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
}

// extractGood stands in for a sidecar-extractor dispatch that did its job:
// schema-valid YAML whose anchors quote the parent.
func (f *verifyFixture) extractGood(t *testing.T, ds ...sidecar.Directive) {
	t.Helper()
	if len(ds) == 0 {
		ds = []sidecar.Directive{anchoredAtGateQuote("The extractor MUST quote the artifact byte-exactly. (ref: ADR-900)")}
	}
	writeMarshalledSidecar(t, f.scPath, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    ds,
	})
}

func (f *verifyFixture) verify(t *testing.T) *VerifyOutcome {
	t.Helper()
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if out == nil {
		t.Fatal("Verify() returned a nil outcome")
	}
	return out
}

func (f *verifyFixture) receiptExists() bool {
	_, err := os.Stat(ReceiptFilePath(f.root, f.planID))
	return err == nil
}

// onlyItem is the single item's receipt entry, which every fixture has
// exactly one of.
func onlyItem(t *testing.T, out *VerifyOutcome) ReceiptItem {
	t.Helper()
	if out.Receipt == nil {
		t.Fatalf("no receipt was written; message: %s", out.Message)
	}
	if len(out.Receipt.Items) != 1 {
		t.Fatalf("receipt covers %d items, want 1", len(out.Receipt.Items))
	}
	return out.Receipt.Items[0]
}

func journalLineCount(t *testing.T, root string) int {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(JournalRelPath)))
	if err != nil {
		if os.IsNotExist(err) {
			return 0
		}
		t.Fatal(err)
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" {
		return 0
	}
	return len(strings.Split(trimmed, "\n"))
}

func journalRecords(t *testing.T, root string) []JournalRecord {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(JournalRelPath)))
	if err != nil {
		t.Fatal(err)
	}
	var out []JournalRecord
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var rec JournalRecord
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			t.Fatalf("journal line %q is not parseable: %v", line, err)
		}
		out = append(out, rec)
	}
	return out
}

// ── AC-3.1: TestVerifyExitCodes ──────────────────────────────────────────

// TestVerifyExitCodes pins all five outcomes, including the one that is NOT
// what SPEC-013's table says. The spec wrote 4 for FAILED-terminal before the
// collision was found: exit 4 is already taken by the pre-run refusal path
// (cmd/version_gate.go's version-line refusal, ADR-041's trust posture), so a
// project pinned below the version floor would exit 4 before verify evaluated
// anything and the skill would report items-failed about items nothing looked
// at. Five is unclaimed, and this test is where that decision is enforced.
func TestVerifyExitCodes(t *testing.T) {
	t.Run("0 — every item ok, receipt consumed, consume journal line", func(t *testing.T) {
		f := newFixture(t, "")
		f.extractGood(t)

		out := f.verify(t)
		if out.ExitCode != ExitVerifyOK {
			t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyOK, out.Message)
		}
		if it := onlyItem(t, out); it.Status != StatusOK {
			t.Fatalf("item status %q, want %q: %s", it.Status, StatusOK, out.Message)
		}
		if !out.Receipt.Consumed {
			t.Error("an all-ok receipt must set consumed:true")
		}
		var sawConsume bool
		for _, rec := range journalRecords(t, f.root) {
			if rec.Event == JournalEventConsume && rec.PlanID == f.planID {
				sawConsume = true
			}
		}
		if !sawConsume {
			t.Error("no consume journal line — Phase B running at all requires one for this plan_id")
		}
	})

	t.Run("1 — a non-ok item that is still retryable, receipt written", func(t *testing.T) {
		f := newFixture(t, "")
		// No extractor dispatch happened at all.

		out := f.verify(t)
		if out.ExitCode != ExitVerifyIncomplete {
			t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyIncomplete, out.Message)
		}
		it := onlyItem(t, out)
		if it.Status != StatusOutputMissing || it.Attempts != 1 {
			t.Fatalf("got %+v, want output-missing at attempt 1", it)
		}
		if out.Receipt.Consumed {
			t.Error("consumed:true with a failing item")
		}
		// The whole promise of exit 1 is that `attempts` persisted.
		if !f.receiptExists() {
			t.Fatal("exit 1 without a receipt on disk — attempts would reset every call and the " +
				"skill would re-dispatch forever")
		}
	})

	t.Run("2 — an unknown plan_id, no receipt, refuse journal line", func(t *testing.T) {
		f := newFixture(t, "")
		unknown := "pap-abcdef012345"

		out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: unknown})
		if err == nil {
			t.Fatal("expected an error for an unknown plan_id")
		}
		if out.ExitCode != ExitVerifyPlanRejected {
			t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyPlanRejected, out.Message)
		}
		if out.Receipt != nil {
			t.Error("a rejected plan_id must not produce a receipt")
		}
		if _, statErr := os.Stat(ReceiptFilePath(f.root, unknown)); !os.IsNotExist(statErr) {
			t.Errorf("a receipt file was written for an unknown plan_id: %v", statErr)
		}
		last := lastJournalRecord(t, f.root)
		if last.Event != JournalEventRefuse || last.Cause != CauseUnknownPlanID {
			t.Errorf("journal line is %+v, want a refuse/%s", last, CauseUnknownPlanID)
		}
	})

	t.Run("3 — a malformed plan_id, no receipt, nothing journaled", func(t *testing.T) {
		f := newFixture(t, "")
		before := journalLineCount(t, f.root)

		out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: "not-a-plan-id"})
		if err == nil {
			t.Fatal("expected an error for a malformed plan_id")
		}
		if out.ExitCode != ExitVerifyMalformed {
			t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyMalformed, out.Message)
		}
		if out.Receipt != nil {
			t.Error("a malformed invocation must not produce a receipt")
		}
		if got := journalLineCount(t, f.root); got != before {
			t.Errorf("journal grew from %d to %d lines — a malformed invocation is not a dispatch "+
				"outcome and has nothing to journal", before, got)
		}
	})

	t.Run("5 — FAILED terminal when no retryable item remains", func(t *testing.T) {
		// max_attempts_per_item: 1 makes the first verify call the last one.
		f := newFixture(t, "phasea:\n  max_attempts_per_item: 1\n")

		out := f.verify(t)
		if out.ExitCode != ExitVerifyFailed {
			t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyFailed, out.Message)
		}
		if out.ExitCode == 4 {
			t.Fatal("exit 4 is the pre-run refusal code and must never mean FAILED-terminal")
		}
		it := onlyItem(t, out)
		if it.Status != StatusFailedTerminal {
			t.Fatalf("item status %q, want %q", it.Status, StatusFailedTerminal)
		}
		if out.Receipt.Consumed {
			t.Error("a FAILED plan must never be consumed")
		}
		if !f.receiptExists() {
			t.Error("exit 5 must still write a receipt — it is the terminal record")
		}
	})
}

// ── AC-3.2: TestVerifyUnwritableReceiptDirExits3 ─────────────────────────

// TestVerifyUnwritableReceiptDirExits3 is the concrete case behind "no
// internal-error path returns exit 1".
//
// Exit 1 promises the skill two things at once: at least one item is
// retryable, AND a receipt was written. `attempts` lives ONLY in the receipt.
// An internal failure that returned 1 without one would leave the skill
// re-dispatching the same items forever, every pass reading the same frozen
// attempts value, never reaching max_attempts_per_item, never becoming
// failed-terminal. The loop has no exit because the counter that ends it is
// in the file that could not be written.
func TestVerifyUnwritableReceiptDirExits3(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-bit simulation is POSIX-specific")
	}
	if os.Geteuid() == 0 {
		t.Skip("running as root bypasses permission bits")
	}
	f := newFixture(t, "")
	f.extractGood(t)

	// r-x: the plan file inside stays readable, but nothing new can be
	// created — which is exactly what writeReceipt needs to do.
	phaseaDir := filepath.Join(f.root, ".edikt", "state", "phasea")
	if err := os.Chmod(phaseaDir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(phaseaDir, 0o755) })

	out, err := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err == nil {
		t.Fatal("expected an error when the receipt cannot be written")
	}
	if out.ExitCode == ExitVerifyIncomplete {
		t.Fatal("an unwritable receipt directory returned exit 1 — attempts persists only in the " +
			"receipt, so this is an unbounded re-dispatch loop")
	}
	if out.ExitCode != ExitVerifyMalformed {
		t.Fatalf("got exit %d, want %d: %s", out.ExitCode, ExitVerifyMalformed, out.Message)
	}
	if out.Receipt != nil {
		t.Error("a failed receipt write must not report a receipt")
	}
	os.Chmod(phaseaDir, 0o755)
	if _, statErr := os.Stat(ReceiptFilePath(f.root, f.planID)); !os.IsNotExist(statErr) {
		t.Errorf("a receipt exists despite the write failing: %v", statErr)
	}
}

// ── AC-3.3: TestBoundedRetryFailedTerminal ───────────────────────────────

func TestBoundedRetryFailedTerminal(t *testing.T) {
	// Default max_attempts_per_item is 2.
	f := newFixture(t, "")

	first := f.verify(t)
	if first.ExitCode != ExitVerifyIncomplete {
		t.Fatalf("call 1: got exit %d, want %d: %s", first.ExitCode, ExitVerifyIncomplete, first.Message)
	}
	if it := onlyItem(t, first); it.Attempts != 1 || it.Status != StatusOutputMissing {
		t.Fatalf("call 1: got %+v, want output-missing at attempt 1", it)
	}

	second := f.verify(t)
	if second.ExitCode != ExitVerifyFailed {
		t.Fatalf("call 2: got exit %d, want %d: %s", second.ExitCode, ExitVerifyFailed, second.Message)
	}
	if it := onlyItem(t, second); it.Attempts != 2 || it.Status != StatusFailedTerminal {
		t.Fatalf("call 2: got %+v, want failed-terminal at attempt 2", it)
	}

	// A failed-terminal item stops incrementing: further calls re-confirm the
	// same state rather than walking the counter upward.
	third := f.verify(t)
	if it := onlyItem(t, third); it.Attempts != 2 || it.Status != StatusFailedTerminal {
		t.Fatalf("call 3: got %+v — a failed-terminal item must not increment further", it)
	}
	if third.ExitCode != ExitVerifyFailed {
		t.Fatalf("call 3: got exit %d, want %d", third.ExitCode, ExitVerifyFailed)
	}

	// And the plan NEVER reaches consumed — not even once the underlying
	// failure is fixed. Recovery requires a fresh plan_id, which a corpus or
	// config change produces; re-opening a closed plan by fixing the tree
	// would make the terminal state meaningless.
	f.extractGood(t)
	fourth := f.verify(t)
	if fourth.Receipt.Consumed {
		t.Fatal("a FAILED plan became consumed after the tree was fixed — the terminal state must " +
			"survive, or `failed-terminal` is only a suggestion")
	}
	if it := onlyItem(t, fourth); it.Status != StatusFailedTerminal || it.Attempts != 2 {
		t.Fatalf("call 4: got %+v, want the carried-forward failed-terminal item at attempt 2", it)
	}
}

// ── AC-3.4: TestParentHashMismatch ───────────────────────────────────────

// TestParentHashMismatch is the keystone check: the sha256 is RECOMPUTED at
// verify time, so an artifact edited between plan and verify can never grade
// ok — no matter how clean the sidecar sitting next to it looks.
func TestParentHashMismatch(t *testing.T) {
	f := newFixture(t, "")
	f.extractGood(t)

	// Sanity: without the edit this item grades ok, so the assertion below is
	// about the hash and not about some other defect in the fixture.
	if it := onlyItem(t, f.verify(t)); it.Status != StatusOK {
		t.Fatalf("fixture does not grade ok before the edit: %+v", it)
	}

	// A fresh plan over the same corpus, then the artifact moves underneath it.
	f2 := newFixture(t, "")
	f2.extractGood(t)
	body, err := os.ReadFile(f2.mdPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(f2.mdPath, append(body, []byte("\nA sentence added after planning.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}

	out := f2.verify(t)
	it := onlyItem(t, out)
	if it.Status == StatusOK {
		t.Fatal("an artifact modified after plan time graded ok — the extractor may have read prose " +
			"that no longer exists, so the sidecar cannot be trusted regardless of whether it parses")
	}
	if it.Status != StatusParentHashMismatch {
		t.Fatalf("item status %q, want %q: %s", it.Status, StatusParentHashMismatch, out.Message)
	}
	// The message must name what was observed AND what was expected.
	sum := sha256.Sum256(append(body, []byte("\nA sentence added after planning.\n")...))
	detail := out.Details[it.ItemID]
	if !strings.Contains(detail, short(hex.EncodeToString(sum[:]))) ||
		!strings.Contains(detail, short(f2.plan.Items[0].ArtifactSHA256)) {
		t.Errorf("detail %q names neither the observed nor the pinned hash", detail)
	}
}

// ── AC-3.7: TestVerifyNoWrites ───────────────────────────────────────────

// TestVerifyNoWrites is the executable form of the D6 seam. The original
// design put pin-restoration inside verify; this test is what would fail if
// anyone put it back.
func TestVerifyNoWrites(t *testing.T) {
	f := newStaleFixture(t, anchoredAtStaleQuote("A prior rule. (ref: ADR-900)"))
	// A regenerated sidecar that is MISSING a pin the snapshot carries, so
	// verify has every incentive to "helpfully" write it back.
	pinned := anchoredAtGateQuote("A prior rule. (ref: ADR-900)")
	pinned.HumanApprovedAt = "2026-08-03T00:00:00Z"
	writeMarshalledSidecar(t, PinSnapshotPath(f.root, f.planID, f.itemID), &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{pinned},
	})
	f.extractGood(t, anchoredAtGateQuote("A prior rule. (ref: ADR-900)"))

	before := snapshotTree(t, f.root)
	out := f.verify(t)
	after := snapshotTree(t, f.root)

	// INV-013: a test that asserted "nothing changed" while verify silently
	// did nothing at all would pass for the wrong reason.
	if out.Receipt == nil {
		t.Fatalf("verify wrote no receipt, so this test proves nothing: %s", out.Message)
	}
	if it := onlyItem(t, out); it.Status != StatusPinsUnrestorable {
		t.Fatalf("expected verify to REPORT the missing pin rather than fix it; got %+v", it)
	}

	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("verify modified %s", path)
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("verify created %s", path)
		}
	}
	for path := range before {
		if _, ok := after[path]; !ok {
			t.Errorf("verify removed %s", path)
		}
	}
}

// snapshotTree hashes every file under root EXCEPT .edikt/state/, which is
// where the receipt and journal legitimately live. Everything else — the
// corpus, every sidecar, every pin snapshot — must come out byte-identical.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	stateDir := filepath.Join(root, ".edikt", "state")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if path == stateDir {
				return filepath.SkipDir
			}
			return nil
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		sum := sha256.Sum256(data)
		rel, _ := filepath.Rel(root, path)
		out[rel] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out) == 0 {
		t.Fatal("snapshotTree walked an empty tree — the comparison would be vacuous")
	}
	return out
}

// ── AC-3.10: TestConsumedPlanRejected ────────────────────────────────────

func TestConsumedPlanRejected(t *testing.T) {
	f := newFixture(t, "")
	f.extractGood(t)

	first := f.verify(t)
	if first.ExitCode != ExitVerifyOK || !first.Receipt.Consumed {
		t.Fatalf("setup: expected a consumed plan, got exit %d: %s", first.ExitCode, first.Message)
	}
	receiptBefore, err := os.ReadFile(ReceiptFilePath(f.root, f.planID))
	if err != nil {
		t.Fatal(err)
	}

	second, serr := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if serr == nil {
		t.Fatal("re-verifying a consumed plan returned no error")
	}
	if second.ExitCode != ExitVerifyPlanRejected {
		t.Fatalf("got exit %d, want %d: %s", second.ExitCode, ExitVerifyPlanRejected, second.Message)
	}
	if !strings.Contains(second.Message, "already consumed") {
		t.Errorf("message %q does not say the plan is already consumed", second.Message)
	}
	last := lastJournalRecord(t, f.root)
	if last.Event != JournalEventRefuse || last.Cause != CauseAlreadyConsumed {
		t.Errorf("journal line is %+v, want a refuse/%s", last, CauseAlreadyConsumed)
	}

	receiptAfter, err := os.ReadFile(ReceiptFilePath(f.root, f.planID))
	if err != nil {
		t.Fatal(err)
	}
	if string(receiptBefore) != string(receiptAfter) {
		t.Error("the terminal receipt was rewritten by a rejected verify call")
	}

	// The rejection survives the corpus changing underneath it: consumed is
	// the one irreversible transition in the phasea contract.
	if err := os.Remove(f.scPath); err != nil {
		t.Fatal(err)
	}
	third, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: f.planID})
	if third.ExitCode != ExitVerifyPlanRejected {
		t.Fatalf("after the corpus changed: got exit %d, want %d", third.ExitCode, ExitVerifyPlanRejected)
	}
}

// ── the receipt schema is authoritative ──────────────────────────────────

// TestReceiptMatchesSchemaShape holds the emitted receipt to
// templates/schemas/phasea-receipt.v3.schema.json, which is the authority —
// verify_types.go's structs exist only to produce bytes that validate against
// it. Every object node in that schema declares `additionalProperties: false`,
// so a field added to the Go struct without being added to the schema first
// makes every receipt this package writes invalid. That is not a failure a
// unit test on the struct would ever see, because the struct is self-
// consistent by construction; only the comparison catches it.
//
// It reads the schema off disk rather than embedding a mirror of it: an
// embedded copy is a second thing that has to agree with the first (GL-002),
// and the byte-identity apparatus that keeps gov-sidecar's mirror honest is
// not worth standing up for a check this test performs directly.
func TestReceiptMatchesSchemaShape(t *testing.T) {
	schema := readReceiptSchema(t)

	f := newFixture(t, "")
	f.extractGood(t)
	out := f.verify(t)
	if out.Receipt == nil {
		t.Fatalf("no receipt to compare: %s", out.Message)
	}
	raw, err := os.ReadFile(ReceiptFilePath(f.root, f.planID))
	if err != nil {
		t.Fatal(err)
	}
	var emitted map[string]any
	if err := json.Unmarshal(raw, &emitted); err != nil {
		t.Fatalf("the emitted receipt is not parseable JSON: %v", err)
	}

	props := schemaProperties(t, schema)
	assertSameKeys(t, "receipt", emitted, props, schemaRequired(t, schema))

	itemSchema := schema["properties"].(map[string]any)["items"].(map[string]any)["items"].(map[string]any)
	items, _ := emitted["items"].([]any)
	if len(items) == 0 {
		t.Fatal("the emitted receipt carries no items — the comparison below would be vacuous")
	}
	for _, it := range items {
		assertSameKeys(t, "receipt item", it.(map[string]any),
			schemaProperties(t, itemSchema), schemaRequired(t, itemSchema))
	}

	// Every status constant must be a member of the schema's enum. A status
	// this package can emit but the schema does not admit would produce a
	// receipt nothing downstream could validate.
	enum := map[string]bool{}
	for _, v := range itemSchema["properties"].(map[string]any)["status"].(map[string]any)["enum"].([]any) {
		enum[v.(string)] = true
	}
	for _, s := range []string{
		StatusOK, StatusOutputMissing, StatusSchemaInvalid, StatusParentHashMismatch,
		StatusPromptVersionMismatch, StatusFailedTerminal, StatusPinsUnrestorable,
		StatusLengthExceeded, StatusAnchorInvalid, StatusDiverged, StatusStalled,
		StatusLosslessLoss, StatusLosslessUnmeasured,
	} {
		if !enum[s] {
			t.Errorf("status %q is not in phasea-receipt.v3's status enum", s)
		}
		delete(enum, s)
	}
	for s := range enum {
		t.Errorf("phasea-receipt.v3 declares status %q which this package has no constant for", s)
	}
}

func readReceiptSchema(t *testing.T) map[string]any {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 8; i++ {
		candidate := filepath.Join(dir, "templates", "schemas", "phasea-receipt.v3.schema.json")
		if data, rerr := os.ReadFile(candidate); rerr == nil {
			var doc map[string]any
			if err := json.Unmarshal(data, &doc); err != nil {
				t.Fatalf("%s is not parseable JSON: %v", candidate, err)
			}
			return doc
		}
		dir = filepath.Dir(dir)
	}
	t.Fatal("could not locate templates/schemas/phasea-receipt.v3.schema.json by walking up from the " +
		"package directory — the comparison cannot run, and a check that could not run must not report a pass")
	return nil
}

func schemaProperties(t *testing.T, node map[string]any) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	props, ok := node["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema node declares no properties")
	}
	for k := range props {
		out[k] = true
	}
	if additional, ok := node["additionalProperties"].(bool); !ok || additional {
		t.Fatal("schema node does not declare additionalProperties:false, so this comparison would " +
			"not catch an extra field")
	}
	return out
}

func schemaRequired(t *testing.T, node map[string]any) []string {
	t.Helper()
	var out []string
	for _, r := range node["required"].([]any) {
		out = append(out, r.(string))
	}
	return out
}

func assertSameKeys(t *testing.T, what string, emitted map[string]any, allowed map[string]bool, required []string) {
	t.Helper()
	for k := range emitted {
		if !allowed[k] {
			t.Errorf("%s emits %q, which phasea-receipt.v3 does not declare (additionalProperties:false)", what, k)
		}
	}
	for _, r := range required {
		if _, ok := emitted[r]; !ok {
			t.Errorf("%s omits the required field %q", what, r)
		}
	}
}
