package phasea

// plan_succession_test.go — PLAN-v090-issue-fixes Phase 2 (#11 body, c1-a–g).
//
// THE DEFECT. plan_id was derived from "<item_id> <reason> <artifact_sha256>"
// and config alone. A re-requested extraction of an UNCHANGED artifact
// therefore recomputed the id of a plan that was already consumed or FAILED,
// and Plan() wrote straight over it: the plan file, and — worse — the
// pre-dispatch pins, replaced by whatever the extractor had last written. The
// next restore then reinstated nothing a human had pinned, and verify either
// refused the closed transaction (consumed) or carried a stale FAILED verdict
// forward over the new output. With a stable extractor the id never moves, so
// SR-011's "the extractor missed a rule, re-run it" was unreachable after the
// first cycle.
//
// THE FIX, asserted here:
//   - the corpus line carries the pre-dispatch sidecar bytes' sha256;
//   - an explicit request over a terminal id derives a deterministic successor
//     (a `supersedes <plan_id>` line in corpus_hash); the corpus path refuses;
//   - the latest unclosed plan per item blocks a re-plan once the sidecar has
//     moved under it;
//   - pins are write-once; restore refuses a FAILED plan.

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

const manualRule = "Never interpolate a query string into SQL. (ref: ADR-900)"

// reqFixture is a project with one accepted ADR and a gate-passing sidecar that
// carries a manual directive — authored prose only the pin can carry forward.
type reqFixture struct {
	root   string
	adr    string
	sc     string
	itemID string
}

func newRequestedFixture(t *testing.T, configBody string) *reqFixture {
	t.Helper()
	root := t.TempDir()
	isolate(t)
	writeExtractorAgent(t, root, 3)
	if configBody != "" {
		writeConfig(t, root, configBody)
	}
	adr := writeADR(t, root, "ADR-900", gateQuote)
	sc := sidecarPathFor(adr)
	writeMarshalledSidecar(t, sc, &sidecar.Sidecar{
		SchemaVersion:    2,
		Topic:            "testing",
		Path:             "docs/architecture/decisions/ADR-900-test.md",
		Directives:       []sidecar.Directive{anchoredAtGateQuote("The original rule MUST hold. (ref: ADR-900)")},
		ManualDirectives: []string{manualRule},
	})
	return &reqFixture{root: root, adr: adr, sc: sc, itemID: "ADR-900"}
}

// extract stands in for one sidecar-extractor dispatch: gate-passing output
// that, like every real extraction, carries no manual_directives.
func (f *reqFixture) extract(t *testing.T, text string) {
	t.Helper()
	writeMarshalledSidecar(t, f.sc, &sidecar.Sidecar{
		SchemaVersion: 2,
		Topic:         "testing",
		Path:          "docs/architecture/decisions/ADR-900-test.md",
		Directives:    []sidecar.Directive{anchoredAtGateQuote(text)},
	})
}

func (f *reqFixture) request(t *testing.T) *PlanOutcome {
	t.Helper()
	out, _ := Plan(PlanOptions{ProjectRoot: f.root, Artifact: f.adr})
	if out == nil {
		t.Fatal("Plan() returned a nil outcome")
	}
	return out
}

func (f *reqFixture) mustRequest(t *testing.T) *DispatchPlan {
	t.Helper()
	out := f.request(t)
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("requested plan was not emitted: exit %d\n%s", out.ExitCode, out.Message)
	}
	return out.Plan
}

func (f *reqFixture) restore(t *testing.T, planID string) *RestoreOutcome {
	t.Helper()
	out, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: planID})
	if out == nil {
		t.Fatalf("Restore(%s) returned a nil outcome: %v", planID, err)
	}
	return out
}

func (f *reqFixture) verify(t *testing.T, planID string) *VerifyOutcome {
	t.Helper()
	out, _ := Verify(VerifyOptions{ProjectRoot: f.root, PlanID: planID})
	if out == nil {
		t.Fatalf("Verify(%s) returned a nil outcome", planID)
	}
	return out
}

// cycle runs one full requested extraction to CONSUMED and returns its id.
func (f *reqFixture) cycle(t *testing.T, text string) string {
	t.Helper()
	p := f.mustRequest(t)
	f.extract(t, text)
	if r := f.restore(t, p.PlanID); r.ExitCode != ExitRestoreOK {
		t.Fatalf("restore %s: exit %d\n%s", p.PlanID, r.ExitCode, r.Message)
	}
	if v := f.verify(t, p.PlanID); v.ExitCode != ExitVerifyOK {
		t.Fatalf("verify %s: exit %d\n%s", p.PlanID, v.ExitCode, v.Message)
	}
	return p.PlanID
}

func readBytes(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// planState is every runtime file a plan owns, by path, so a test can assert a
// closed plan is untouched byte-for-byte.
func planState(t *testing.T, root, planID, itemID string) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	for _, p := range []string{
		PlanFilePath(root, planID),
		PinSnapshotPath(root, planID, itemID),
		ReceiptFilePath(root, planID),
		RestoreReceiptPath(root, planID),
	} {
		if b, err := os.ReadFile(p); err == nil {
			out[p] = b
		}
	}
	return out
}

func assertStateUnchanged(t *testing.T, what string, before, after map[string][]byte) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("%s: plan state files changed in number: %d before, %d after", what, len(before), len(after))
	}
	for p, b := range before {
		if !bytes.Equal(b, after[p]) {
			t.Errorf("%s: %s was rewritten — a closed plan's state must be byte-identical afterwards", what, p)
		}
	}
}

func sha256Bytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// snapshotAll hashes every file under root, .edikt/state/ included (plans,
// pins, receipts), EXCEPT the journal — the one file a refusal must append to.
func snapshotAll(t *testing.T, root string) map[string]string {
	t.Helper()
	out := map[string]string{}
	journal := filepath.Join(root, filepath.FromSlash(JournalRelPath))
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || path == journal {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		out[rel] = sha256Bytes(readBytes(t, path))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func journalPlanLines(t *testing.T, root string) int {
	t.Helper()
	if journalLineCount(t, root) == 0 {
		return 0
	}
	n := 0
	for _, r := range journalRecords(t, root) {
		if r.Event == JournalEventPlan {
			n++
		}
	}
	return n
}

// ── AC-2.1 ───────────────────────────────────────────────────────────────

// TestPlan_RequestedAfterConsume_DerivesFreshPlanID — c1-a. The repro: a
// consumed plan, then the same artifact re-requested unchanged.
func TestPlan_RequestedAfterConsume_DerivesFreshPlanID(t *testing.T) {
	f := newRequestedFixture(t, "")
	first := f.cycle(t, "The first extraction MUST hold. (ref: ADR-900)")
	before := planState(t, f.root, first, f.itemID)

	second := f.mustRequest(t)
	if second.PlanID == first {
		t.Fatalf("a re-request after %s was consumed reused its plan_id — the closed plan's file and pins "+
			"are overwritten and the new dispatch lands in a transaction verify will refuse", first)
	}
	assertStateUnchanged(t, "consumed plan "+first, before, planState(t, f.root, first, f.itemID))

	// The new plan's pin is the CONSUMED output — the pre-dispatch bytes as they
	// stand now — and the consumed plan's pin is still the original sidecar.
	if !bytes.Equal(readBytes(t, PinSnapshotPath(f.root, second.PlanID, f.itemID)), readBytes(t, f.sc)) {
		t.Error("the new plan's pin is not the live pre-dispatch sidecar")
	}
}

// TestPlan_StableExtractor_FixedPointGetsSuccessor — c1-c. A stable extractor
// reproduces byte-identical output, so the sidecar returns to the same bytes
// cycle after cycle and the id derived from them is a fixed point. Cycle 3
// lands on cycle 2's consumed id and must get a successor that names it.
func TestPlan_StableExtractor_FixedPointGetsSuccessor(t *testing.T) {
	f := newRequestedFixture(t, "")
	const same = "The stable extraction MUST hold. (ref: ADR-900)"
	c1 := f.cycle(t, same)
	c2 := f.cycle(t, same)
	if c2 == c1 {
		t.Fatalf("cycle 2 reused cycle 1's consumed id %s", c1)
	}
	pinC2 := readBytes(t, PinSnapshotPath(f.root, c2, f.itemID))
	if !bytes.Equal(pinC2, readBytes(t, f.sc)) {
		t.Fatal("setup: the extractor is not at a fixed point — cycle 2's output differs from its input, " +
			"so this test would not reach the collision it exists for")
	}
	c2State := planState(t, f.root, c2, f.itemID)

	out := f.request(t)
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("cycle 3 at the fixed point was not planned: exit %d\n%s", out.ExitCode, out.Message)
	}
	c3 := out.Plan
	if c3.PlanID == c2 {
		t.Fatalf("cycle 3 reused cycle 2's consumed id %s", c2)
	}
	assertStateUnchanged(t, "consumed plan "+c2, c2State, planState(t, f.root, c2, f.itemID))

	// corpus_hash names its predecessor: the item line, then `supersedes <c2>`.
	it := c3.Items[0]
	line := fmt.Sprintf("%s %s %s %s", it.ItemID, it.Reason, it.ArtifactSHA256, sha256Bytes(pinC2))
	want := sha256Bytes([]byte(line + "\nsupersedes " + c2))
	if c3.CreatedFrom.CorpusHash != want {
		t.Errorf("cycle 3 corpus_hash = %s, want sha256(%q + \"\\nsupersedes %s\") = %s",
			c3.CreatedFrom.CorpusHash, line, c2, want)
	}
	if !strings.Contains(out.Message, c2) {
		t.Errorf("the success message must name the plan it supersedes (%s):\n%s", c2, out.Message)
	}
}

// TestPlan_SuccessorIsDeterministic — SAC-002 holds for succession: no
// timestamp, no randomness. The same on-disk state, receipts included, names
// the same successor — re-planned in place, and replayed in a second tree.
func TestPlan_SuccessorIsDeterministic(t *testing.T) {
	const same = "The stable extraction MUST hold. (ref: ADR-900)"
	run := func(t *testing.T) (*reqFixture, *PlanOutcome) {
		f := newRequestedFixture(t, "")
		f.cycle(t, same)
		f.cycle(t, same)
		out := f.request(t)
		if out.ExitCode != ExitPlanEmitted {
			t.Fatalf("successor not planned: exit %d\n%s", out.ExitCode, out.Message)
		}
		return f, out
	}
	fa, a := run(t)
	planBytes := readBytes(t, PlanFilePath(fa.root, a.Plan.PlanID))

	again := fa.request(t)
	if again.ExitCode != ExitPlanEmitted || again.Plan.PlanID != a.Plan.PlanID {
		t.Fatalf("re-planning the same state named %v (exit %d), want the same successor %s\n%s",
			again.Plan, again.ExitCode, a.Plan.PlanID, again.Message)
	}
	if !bytes.Equal(planBytes, readBytes(t, PlanFilePath(fa.root, a.Plan.PlanID))) {
		t.Error("the successor plan file is not byte-identical when re-planned from the same state")
	}

	_, b := run(t)
	if b.Plan.PlanID != a.Plan.PlanID {
		t.Errorf("two trees with the same history named different successors: %s vs %s", a.Plan.PlanID, b.Plan.PlanID)
	}
}

// ── AC-2.2 ───────────────────────────────────────────────────────────────

// TestPlan_CorpusPath_RefusesTerminalPlanID — the corpus path never derives a
// successor (D-A): an implicit, staleness-driven re-plan onto a closed id is a
// refusal that names what to do, not a silent overwrite.
func TestPlan_CorpusPath_RefusesTerminalPlanID(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) (root, terminal string)
	}{
		{
			name: "consumed — the stale sidecar reverted after its plan was consumed",
			setup: func(t *testing.T) (string, string) {
				f := newStaleFixture(t, anchoredAtStaleQuote("Stale rule. (ref: ADR-900)"))
				stale := readBytes(t, f.scPath)
				f.extractGood(t)
				if r, _ := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID}); r.ExitCode != ExitRestoreOK {
					t.Fatalf("restore: %s", r.Message)
				}
				if v := f.verify(t); v.ExitCode != ExitVerifyOK {
					t.Fatalf("verify: %s", v.Message)
				}
				// `git checkout -- <sidecar>` of the stale bytes: the corpus is
				// exactly what it was when the consumed plan was derived.
				if err := os.WriteFile(f.scPath, stale, 0o644); err != nil {
					t.Fatal(err)
				}
				return f.root, f.planID
			},
		},
		{
			name: "FAILED — retry budget spent on a missing sidecar",
			setup: func(t *testing.T) (string, string) {
				f := newFixture(t, "phasea:\n  max_attempts_per_item: 1\n")
				if v := f.verify(t); v.ExitCode != ExitVerifyFailed {
					t.Fatalf("setup: want FAILED (exit %d), got %d\n%s", ExitVerifyFailed, v.ExitCode, v.Message)
				}
				return f.root, f.planID
			},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root, terminal := c.setup(t)
			treeBefore := snapshotAll(t, root)
			plansBefore := journalPlanLines(t, root)
			linesBefore := journalLineCount(t, root)

			out, err := Plan(PlanOptions{ProjectRoot: root})
			if err == nil || out.ExitCode != ExitRefused {
				t.Fatalf("corpus re-plan onto terminal %s: exit %d, err %v — want exit %d\n%s",
					terminal, out.ExitCode, err, ExitRefused, out.Message)
			}
			if !strings.Contains(out.Message, terminal) {
				t.Errorf("the refusal must name the terminal plan %s:\n%s", terminal, out.Message)
			}
			if got := journalLineCount(t, root) - linesBefore; got != 1 {
				t.Fatalf("refusal appended %d journal line(s), want exactly 1", got)
			}
			if rec := lastJournalRecord(t, root); rec.Event != JournalEventRefuse || rec.Cause != CausePlanIDTerminal || rec.PlanID != terminal {
				t.Errorf("journal line = %+v, want a refuse with cause %q naming %s", rec, CausePlanIDTerminal, terminal)
			}
			if journalPlanLines(t, root) != plansBefore {
				t.Error("a refused plan appended a `plan` journal line")
			}
			treeAfter := snapshotAll(t, root)
			for p, h := range treeBefore {
				if treeAfter[p] != h {
					t.Errorf("refusal mutated %s", p)
				}
			}
			for p := range treeAfter {
				if _, ok := treeBefore[p]; !ok {
					t.Errorf("refusal created %s", p)
				}
			}
		})
	}
}

// ── AC-2.3 ───────────────────────────────────────────────────────────────

// TestPlan_RefusesOverUnrestoredOpenPlan — c1-d. The extractor rewrote the
// sidecar and restore never ran. A re-plan at HEAD reused the id and replaced
// the pin with the extractor's output, so the manual directive was gone for
// good. The guard refuses, names the plan and the remedy, and the open plan's
// own restore still reinstates the directive afterwards.
func TestPlan_RefusesOverUnrestoredOpenPlan(t *testing.T) {
	f := newRequestedFixture(t, "")
	p := f.mustRequest(t)
	pin := readBytes(t, PinSnapshotPath(f.root, p.PlanID, f.itemID))
	f.extract(t, "An extraction nobody restored MUST hold. (ref: ADR-900)")
	linesBefore := journalLineCount(t, f.root)

	out := f.request(t)
	if out.ExitCode != ExitRefused {
		t.Fatalf("re-plan over unrestored output of %s: exit %d, want %d\n%s", p.PlanID, out.ExitCode, ExitRefused, out.Message)
	}
	for _, want := range []string{p.PlanID, "edikt phasea restore --plan " + p.PlanID} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("the refusal must name %q:\n%s", want, out.Message)
		}
	}
	if got := journalLineCount(t, f.root) - linesBefore; got != 1 {
		t.Fatalf("refusal appended %d journal line(s), want 1", got)
	}
	if rec := lastJournalRecord(t, f.root); rec.Cause != CauseUnclosedPlan {
		t.Errorf("journal cause %q, want %q", rec.Cause, CauseUnclosedPlan)
	}
	if !bytes.Equal(pin, readBytes(t, PinSnapshotPath(f.root, p.PlanID, f.itemID))) {
		t.Fatal("the open plan's pin was rewritten by the refused re-plan")
	}

	if r := f.restore(t, p.PlanID); r.ExitCode != ExitRestoreOK {
		t.Fatalf("restore after the refusal: exit %d\n%s", r.ExitCode, r.Message)
	}
	got, err := sidecar.Load(f.sc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.ManualDirectives) != 1 || got.ManualDirectives[0] != manualRule {
		t.Fatalf("restore(%s) did not reinstate the manual directive: %v", p.PlanID, got.ManualDirectives)
	}
}

// TestPlan_RefusesOverFailedPlanOutput — c1-f/c1-g, repro_failed.sh. The
// artifact changed after planning, so the plan went FAILED with the
// extractor's output restored and stamped on disk. Reverting the artifact and
// re-requesting must not adopt that output as the new baseline. Restoring the
// pin and re-requesting then derives a successor that names the FAILED plan.
func TestPlan_RefusesOverFailedPlanOutput(t *testing.T) {
	f := newRequestedFixture(t, "")
	original := readBytes(t, f.adr)
	p := f.mustRequest(t)
	pinPath := PinSnapshotPath(f.root, p.PlanID, f.itemID)
	f.extract(t, "An extraction of a moved parent MUST hold. (ref: ADR-900)")
	if err := os.WriteFile(f.adr, append(append([]byte{}, original...), []byte("\nAn edit after planning.\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	f.restore(t, p.PlanID)
	if v := f.verify(t, p.PlanID); v.ExitCode != ExitVerifyFailed {
		t.Fatalf("setup: want FAILED (exit %d), got %d\n%s", ExitVerifyFailed, v.ExitCode, v.Message)
	}
	if err := os.WriteFile(f.adr, original, 0o644); err != nil { // revert the artifact
		t.Fatal(err)
	}

	out := f.request(t)
	if out.ExitCode != ExitRefused {
		t.Fatalf("re-request over FAILED %s's output: exit %d, want %d\n%s", p.PlanID, out.ExitCode, ExitRefused, out.Message)
	}
	for _, want := range []string{p.PlanID, "FAILED", filepath.ToSlash(relPath(f.root, pinPath))} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("the refusal must name %q:\n%s", want, out.Message)
		}
	}
	if rec := lastJournalRecord(t, f.root); rec.Cause != CauseUnclosedPlan {
		t.Errorf("journal cause %q, want %q", rec.Cause, CauseUnclosedPlan)
	}

	// The documented remedy: copy the pin back, then re-request.
	if err := os.WriteFile(f.sc, readBytes(t, pinPath), 0o644); err != nil {
		t.Fatal(err)
	}
	succ := f.request(t)
	if succ.ExitCode != ExitPlanEmitted || succ.Plan == nil {
		t.Fatalf("re-request after restoring the pin: exit %d\n%s", succ.ExitCode, succ.Message)
	}
	if succ.Plan.PlanID == p.PlanID {
		t.Fatalf("re-request after restoring the pin reused FAILED %s", p.PlanID)
	}
	if !strings.Contains(succ.Message, p.PlanID) {
		t.Errorf("the successor must name the FAILED plan it supersedes (%s):\n%s", p.PlanID, succ.Message)
	}
	if r := f.restore(t, p.PlanID); r.ExitCode != ExitRestorePlanRejected {
		t.Errorf("restore of FAILED %s: exit %d, want %d", p.PlanID, r.ExitCode, ExitRestorePlanRejected)
	}
}

// TestPlan_SupersededAbandonedPlanDoesNotBlock — only the LATEST plan per item
// is consulted. An older plan that was abandoned (never dispatched) and then
// overtaken by a consumed one must not block a later request, even though its
// pin no longer matches the sidecar.
func TestPlan_SupersededAbandonedPlanDoesNotBlock(t *testing.T) {
	f := newRequestedFixture(t, "")
	abandoned := f.mustRequest(t) // never dispatched
	// The artifact moves on, so the next request is a different plan.
	if err := os.WriteFile(f.adr, []byte(acceptedADRBody(gateQuote)+"\nA later paragraph.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	consumed := f.cycle(t, "The later extraction MUST hold. (ref: ADR-900)")
	if consumed == abandoned.PlanID {
		t.Fatal("setup: the artifact edit did not move the plan id")
	}
	consumedState := planState(t, f.root, consumed, f.itemID)

	out := f.request(t)
	if out.ExitCode != ExitPlanEmitted || out.Plan == nil {
		t.Fatalf("a request after %s was consumed was refused over the older abandoned %s: exit %d\n%s",
			consumed, abandoned.PlanID, out.ExitCode, out.Message)
	}
	if out.Plan.PlanID == consumed || out.Plan.PlanID == abandoned.PlanID {
		t.Fatalf("the request reused %s", out.Plan.PlanID)
	}
	assertStateUnchanged(t, "consumed plan "+consumed, consumedState, planState(t, f.root, consumed, f.itemID))
}

// TestPlan_OpenPlanUnchanged_IsIdempotent — re-planning an open plan whose
// sidecar has not moved is the same plan: same id, same bytes, pins untouched.
// The id is derived from the formula that includes the sidecar bytes.
func TestPlan_OpenPlanUnchanged_IsIdempotent(t *testing.T) {
	f := newRequestedFixture(t, "")
	pre := readBytes(t, f.sc)
	first := f.mustRequest(t)
	planBytes := readBytes(t, PlanFilePath(f.root, first.PlanID))
	pin := readBytes(t, PinSnapshotPath(f.root, first.PlanID, f.itemID))

	again := f.request(t)
	if again.ExitCode != ExitPlanEmitted || again.Plan.PlanID != first.PlanID {
		t.Fatalf("re-planning an unchanged open plan: exit %d, %v\n%s", again.ExitCode, again.Plan, again.Message)
	}
	if !bytes.Equal(planBytes, readBytes(t, PlanFilePath(f.root, first.PlanID))) {
		t.Error("plan file not byte-identical on re-plan")
	}
	if !bytes.Equal(pin, readBytes(t, PinSnapshotPath(f.root, first.PlanID, f.itemID))) {
		t.Error("pin rewritten on re-plan")
	}
	it := first.Items[0]
	want := sha256Bytes([]byte(fmt.Sprintf("%s %s %s %s", it.ItemID, it.Reason, it.ArtifactSHA256, sha256Bytes(pre))))
	if first.CreatedFrom.CorpusHash != want {
		t.Errorf("corpus_hash %s does not cover the pre-dispatch sidecar bytes (want %s)", first.CreatedFrom.CorpusHash, want)
	}
}

// ── AC-2.4 ───────────────────────────────────────────────────────────────

// TestCapturePinSnapshot_NeverOverwritesDifferingPin — a pin is the only copy
// of what a human pinned before the extractor ran. Writing over it with
// different bytes is how c1-d lost the manual directives.
func TestCapturePinSnapshot_NeverOverwritesDifferingPin(t *testing.T) {
	root := t.TempDir()
	const planID = "pap-0123456789ab"
	a := workItem{PlanItem: PlanItem{ItemID: "ADR-900"}, beforeSidecar: []byte("schema_version: 2\ntopic: a\n")}
	b := workItem{PlanItem: PlanItem{ItemID: "ADR-900"}, beforeSidecar: []byte("schema_version: 2\ntopic: b\n")}

	if err := capturePinSnapshot(root, planID, a); err != nil {
		t.Fatalf("first capture: %v", err)
	}
	if err := capturePinSnapshot(root, planID, a); err != nil {
		t.Errorf("re-capturing identical bytes must be a no-op, got %v", err)
	}
	if err := capturePinSnapshot(root, planID, b); err == nil {
		t.Error("capturing different bytes over an existing pin succeeded — pins are write-once")
	}
	if got := readBytes(t, PinSnapshotPath(root, planID, "ADR-900")); !bytes.Equal(got, a.beforeSidecar) {
		t.Errorf("pin now holds %q, want the first capture %q", got, a.beforeSidecar)
	}
}

// TestRestore_RefusesFailedTerminalPlan — restoring into a FAILED plan
// stamps output that no gate will ever look at again under that id.
func TestRestore_RefusesFailedTerminalPlan(t *testing.T) {
	f := newFixture(t, "phasea:\n  max_attempts_per_item: 1\n")
	if v := f.verify(t); v.ExitCode != ExitVerifyFailed {
		t.Fatalf("setup: want FAILED, got exit %d\n%s", v.ExitCode, v.Message)
	}
	f.extractGood(t)
	before := readBytes(t, f.scPath)

	out, err := Restore(RestoreOptions{ProjectRoot: f.root, PlanID: f.planID})
	if err == nil || out.ExitCode != ExitRestorePlanRejected {
		t.Fatalf("restore of FAILED %s: exit %d err %v, want %d\n%s", f.planID, out.ExitCode, err, ExitRestorePlanRejected, out.Message)
	}
	for _, want := range []string{"FAILED", "successor"} {
		if !strings.Contains(out.Message, want) {
			t.Errorf("the refusal must say %q:\n%s", want, out.Message)
		}
	}
	if !bytes.Equal(before, readBytes(t, f.scPath)) {
		t.Error("a refused restore rewrote the sidecar")
	}
}
