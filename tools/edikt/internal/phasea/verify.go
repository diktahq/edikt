// verify.go — `bin/edikt phasea verify`'s READ-ONLY gate (SPEC-013, Phase 3
// of PLAN-v080-reliability-core).
//
// THE D6 SEAM. The original design put pin-restoration inside verify. That
// is structurally impossible: pinpreserve.PreservePinned needs the sidecar as
// it stood BEFORE the extractor overwrote it, and verify runs AFTER tier-1
// has already fanned out. So the work is split across three verbs and this
// file is only the third:
//
//	phasea plan     captures the pre-dispatch pin-set (plan.go, Phase 2)
//	phasea restore  performs the mutation — reinstates pins, quarantines a
//	                bad write (restore.go), called by the skill BETWEEN
//	                fan-out and verify
//	phasea verify   COMPARES the live sidecars against the plan's recorded
//	                pin-set and fails the item, naming the missing field
//
// Nothing in this file JUDGES by writing to a sidecar, an artifact, or a
// pin snapshot. It writes exactly two things, both of them audit state: the
// receipt at .edikt/state/phasea/<plan_id>.receipt.json and journal lines
// at .edikt/state/phasea-dispatch.jsonl. Putting a sidecar write in here to
// repair a defect this file itself found would re-create the exact "did
// the write happen" / "make the write happen" fusion SPEC-013 exists to
// undo — a gate that repairs what it is measuring can no longer report on
// it.
//
// ONE NARROW, DELIBERATE EXCEPTION: after an item is already decided `ok`
// (the verdict, not affected by what follows), this file stamps
// extractor_model/extractor_contract onto that sidecar via
// sidecar.InsertKeyInPlace. This is provenance metadata about the run that
// already happened, not a repair of anything this file found wrong — the
// verdict was `ok` before the stamp and stays `ok` after it. It happens
// here, not in restore or a separate step, because it must land before
// Phase B renders (the compile manifest fingerprint and `gov compile
// --check` freshness are computed over the stamped bytes), and verify is
// the last point in the plan/restore/verify chain before that render.
//
// EXIT CODES (documented and asserted by TestVerifyExitCodes):
//
//	0  ExitVerifyOK           — every item ok. Receipt written with
//	                            consumed:true; `consume` journal line appended.
//	1  ExitVerifyIncomplete   — at least one item not ok AND at least one
//	                            still retryable. Receipt written (this is what
//	                            persists `attempts`), consumed:false.
//	2  ExitVerifyPlanRejected — plan_id unknown, or already consumed. No
//	                            receipt; one `refuse` journal line.
//	3  ExitVerifyMalformed    — malformed invocation or unusable state: bad
//	                            plan_id shape, unparseable plan or prior
//	                            receipt, unreadable config, or a receipt that
//	                            could not be written. No receipt, nothing
//	                            journaled.
//	5  ExitVerifyFailed       — FAILED, terminal. Every remaining non-ok item
//	                            is failed-terminal or pins-unrestorable.
//	                            Receipt written, consumed:false.
//
// WHY 5 AND NOT 4. SPEC-013's own table says 4, written before the collision
// was found: exit 4 is ALREADY the pre-run refusal code — cmd/version_gate.go
// returns it for a version-line refusal, and ADR-041's trust posture exits 4
// as well. A project pinned below the version floor would therefore exit 4
// BEFORE this file evaluated anything, and the skill, reading 4 as
// FAILED-terminal, would report items-failed about items nothing ever looked
// at. That is the INV-013 shape — absence rendered as a measurement — arriving
// through the exit code rather than through the report. Five is unclaimed.
//
// WHY NO INTERNAL ERROR MAY RETURN 1. Exit 1 carries a specific promise to
// the skill: "at least one item is retryable AND a receipt was written."
// `attempts` lives ONLY in the receipt, so an internal failure that returned 1
// without writing one would send the skill around the loop forever — every
// pass re-dispatching the same items with attempts frozen at their prior
// value, never reaching max_attempts_per_item, never becoming failed-terminal.
// An unwritable receipt directory is the concrete case (AC-3.2) and it exits
// 3. Every error path in this file returns 2, 3 or 5; `1` is produced at
// exactly one place, the exit-code computation over an evaluated item set.
package phasea

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/diktahq/edikt/tools/edikt/internal/idvalidate"
	"github.com/diktahq/edikt/tools/edikt/internal/migrate"
	"github.com/diktahq/edikt/tools/edikt/internal/pinpreserve"
	"github.com/diktahq/edikt/tools/edikt/internal/sidecar"
)

// Exit codes — see the file doc above.
const (
	ExitVerifyOK           = 0
	ExitVerifyIncomplete   = 1
	ExitVerifyPlanRejected = 2
	ExitVerifyMalformed    = 3
	ExitVerifyFailed       = 5

	// ExitVerifyDiverged — at least one item STOPPED BECAUSE IT WAS GETTING
	// WORSE, with retry budget still on the table.
	//
	// It is not folded into ExitVerifyFailed because exit 5's contract says
	// "the retry budget is spent", and for a diverged item that is simply
	// untrue: the budget was deliberately left unspent. A caller told "budget
	// exhausted" about an item with three attempts remaining cannot tell that
	// re-planning would not help, and the two states need opposite responses
	// from a human.
	ExitVerifyDiverged = 6

	// ExitVerifyRestoreMissing — the plan has no restore receipt, so the
	// pinned state (approved paths, approval receipts, human-approved
	// verifies, suppressions) was never reinstated after the fan-out. Its own
	// code because it is not an item result: every item could be perfect and
	// the run would still be measuring a corpus missing everything a human
	// pinned. Verify refuses rather than restoring, because a verify that
	// mutates what it judges cannot be re-run to check the mutation.
	ExitVerifyRestoreMissing = 7
)

// planIDPattern is phasea-dispatch-plan.v2 / phasea-receipt.v2 /
// phasea-journal.v1's shared `plan_id` pattern. A plan_id that does not match
// is a malformed invocation (exit 3), never an unknown plan (exit 2): the two
// are different claims, and reporting a typo as "no such plan" hides the typo.
var planIDPattern = regexp.MustCompile(`^pap-[0-9a-f]{12}$`)

// VerifyOptions configures one Verify() call.
type VerifyOptions struct {
	// ProjectRoot is the tree to verify against. Empty means the current
	// working directory (mirrors PlanOptions.ProjectRoot).
	ProjectRoot string

	// PlanID names the plan whose items are being verified.
	PlanID string

	// Stalled lists item_ids the dispatcher reports as having ended without a
	// result (SPEC-013 SR-013). An item in this list whose output is missing
	// — absent, or the prior restore observed was never rewritten — is
	// recorded as `stalled` instead of `output-missing`; an item the extractor
	// did write is evaluated on the output and the report is ignored.
	Stalled []string

	// OnLoss decides what a lifted item's lossless loss (or an unmeasurable
	// baseline) becomes: "" / "abort" keep the non-ok status; "accept"
	// records the finding in the diagnostic and lets the item pass.
	OnLoss string

	// ExtractorModel is a self-report from the dispatching context: under
	// `extractor.model: inherit`, the installed frontmatter carries no
	// model: key, so nothing on disk can answer "which model actually ran
	// this dispatch" — only the tier-1 session that just dispatched it and
	// knows its own model can. Optional: empty means the dispatching
	// context reported none, and the stamp falls back to
	// ResolveExtractorAgentModel (correct for an explicit config pin, where
	// the frontmatter names the model directly) or the literal
	// "unrecorded" — never a guess.
	//
	// Validated against idvalidate.ModelID before use; an invalid value
	// refuses the whole verify call rather than silently degrading to
	// unrecorded, so a caller passing a typo finds out immediately.
	ExtractorModel string

	// RejectedPin maps item_id to the host's raw error when an explicit
	// extractor.model pin was rejected by the dispatching host (D57). An
	// item present here with no output on disk
	// is recorded failed-terminal on FIRST observation, never retried: the
	// pin the host refused is the same input a retry would resubmit, so
	// there is no budget worth spending (the same reasoning that excludes
	// pins-unrestorable from ordinary retry exhaustion). Its diagnostic
	// quotes the host's error, length-capped before doctor or any other
	// reader prints it. An item WITH output on disk is evaluated on the
	// output and the report is ignored — output is the evidence, same rule
	// as Stalled.
	RejectedPin map[string]string
}

// rejectedPinDiagnosticCap bounds how much of a host's raw rejection error
// lands in the receipt diagnostic. The receipt schema caps diagnostic at
// 4000 characters; this leaves room for the prefix sentence naming what the
// quoted text is.
const rejectedPinDiagnosticCap = 2000

func capDiagnostic(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "... (truncated)"
}

// VerifyOutcome is Verify()'s result. Receipt is non-nil exactly when a
// receipt was written (exit 0, 1 and 5).
type VerifyOutcome struct {
	ExitCode int
	Receipt  *Receipt
	Message  string

	// Details carries the per-item diagnostic keyed by item_id — which
	// pinned field went missing, which anchor does not quote its parent,
	// which sha256 was expected. The receipt schema has no field for it
	// (additionalProperties:false), and it is the difference between a
	// report a human can act on and a bare status word.
	Details map[string]string
}

// Verify evaluates every item of the named plan against the live tree and
// writes a receipt. See the file doc for exit codes and the read-only
// guarantee.
func Verify(opts VerifyOptions) (*VerifyOutcome, error) {
	root := opts.ProjectRoot
	planID := opts.PlanID

	// A self-reported --extractor-model that fails shape validation refuses
	// the whole call rather than silently degrading to "unrecorded": the
	// caller passed something, and a typo silently becoming an honest
	// "nothing was reported" would hide the typo (INV-006 — this value
	// crosses a trust boundary from the dispatching context into a
	// sidecar/receipt field).
	if opts.ExtractorModel != "" {
		if err := idvalidate.ModelID(opts.ExtractorModel); err != nil {
			return malformed(fmt.Sprintf("refusing to verify: --extractor-model %q: %v", opts.ExtractorModel, err))
		}
	}

	if !planIDPattern.MatchString(planID) {
		return malformed(fmt.Sprintf(
			"refusing to verify: %q is not a valid plan_id — expected the "+
				"phasea-dispatch-plan.v2 shape `pap-` followed by 12 lowercase hex characters",
			planID))
	}

	cfg, err := LoadConfig(root)
	if err != nil {
		return malformed(fmt.Sprintf("refusing to verify: %v", err))
	}

	// An UNKNOWN prompt_version is NOT a whole-run refusal here the way it is
	// at plan time. Plan time refuses because an unpinned contract would be
	// silently baked into dispatches that have not happened yet; by verify
	// time the dispatches are done, and "the installed extractor's contract no
	// longer matches what this plan was built against" is a real, per-item
	// verdict the receipt has an enum value for. UNKNOWN never equals a
	// pinned "vN", so it lands as prompt-version-mismatch on every item —
	// reported, never substituted (ADR-055 §4).
	promptVersion, _ := ResolveExtractorPromptVersion(root)

	plan, code, msg := loadPlanForVerb(root, planID)
	if plan == nil {
		if code == ExitVerifyPlanRejected {
			msg = withJournal(msg, appendJournal(root, JournalRecord{
				Event:  JournalEventRefuse,
				PlanID: planID,
				Items:  0,
				Cause:  CauseUnknownPlanID,
			}))
		}
		return &VerifyOutcome{ExitCode: code, Message: msg}, fmt.Errorf("%s", msg)
	}

	// THE RESTORE COUPLING. Nothing else enforces the contract's order, and a
	// run that skipped restore reports per-item results that describe a
	// corpus without the state a human pinned.
	restoreReceipt := restoreReceiptFor(root, planID)
	if restoreReceipt == nil {
		msg := fmt.Sprintf(
			"refusing to verify: plan %s has no restore receipt at %s.\n"+
				"  The dispatch contract is plan -> fan out -> restore -> verify, and restore is\n"+
				"  what reinstates the pinned state the fan-out overwrote: approved paths, approval\n"+
				"  receipts, human-approved verifies, suppressions. Verifying without it measures a\n"+
				"  corpus missing everything a human decided.\n"+
				"  Run: edikt phasea restore --plan %s\n"+
				"  This refusal does NOT run restore for you: a verify that mutates what it judges\n"+
				"  cannot be re-run to check the mutation.",
			planID, RestoreReceiptPath(root, planID), planID)
		return &VerifyOutcome{ExitCode: ExitVerifyRestoreMissing, Message: msg}, fmt.Errorf("%s", msg)
	}

	prior, perr := loadReceipt(root, planID)
	if perr != nil {
		return malformed(fmt.Sprintf(
			"refusing to verify: the prior receipt for %s exists but could not be read: %v — "+
				"`attempts` and the consumed flag both live in that file, so proceeding would "+
				"restart the retry budget from zero and could re-consume a terminal plan",
			planID, perr))
	}
	if prior != nil && prior.Consumed {
		msg := fmt.Sprintf(
			"refusing to verify: plan %s is already consumed (verified_at %s) — a consumed plan "+
				"is terminal. To extract again, re-request the artifact (`edikt phasea plan --artifact "+
				"<path>`, or the per-artifact compile command): that derives a successor plan. A corpus "+
				"or config change also gives a new plan_id",
			planID, prior.VerifiedAt)
		msg = withJournal(msg, appendJournal(root, JournalRecord{
			Event:  JournalEventRefuse,
			PlanID: planID,
			Items:  0,
			Cause:  CauseAlreadyConsumed,
		}))
		return &VerifyOutcome{ExitCode: ExitVerifyPlanRejected, Message: msg}, fmt.Errorf("%s", msg)
	}

	priorByID := map[string]ReceiptItem{}
	if prior != nil {
		for _, it := range prior.Items {
			priorByID[it.ItemID] = it
		}
	}

	// What restore OBSERVED: the items whose sidecar the extractor did not
	// write. Read from restore's receipt rather than re-derived here by
	// comparing bytes with the pin — by now restore and the previous verify
	// have stamped the file, and a genuine extraction identical in content to
	// a stamped prior ends up byte-identical to its pin. Comparing here would
	// grade that real extraction output-missing.
	unwrittenSet := map[string]bool{}
	for _, id := range restoreReceipt.Unwritten {
		unwrittenSet[id] = true
	}
	// And the items restore QUARANTINED: the extractor wrote something, it
	// failed the content gate, and restore moved it aside. The expected path
	// is empty because of restore, not because nothing was written.
	quarantinedSet := map[string]bool{}
	for _, id := range restoreReceipt.Quarantined {
		quarantinedSet[id] = true
	}

	items := make([]ReceiptItem, 0, len(plan.Items))
	details := map[string]string{}
	stalledSet := make(map[string]bool, len(opts.Stalled))
	for _, id := range opts.Stalled {
		stalledSet[id] = true
	}
	for _, it := range plan.Items {
		// CARRY-FORWARD. A terminal item is copied from the prior receipt
		// byte-for-byte and NOT re-evaluated: its status is already final and
		// its attempts must not move. This is what makes a FAILED plan stable
		// — every later call re-confirms the same items at the same attempt
		// counts instead of walking them toward some further state — and it is
		// also what keeps an already-ok item from being re-gated against a
		// corpus that has moved on since it passed.
		if p, ok := priorByID[it.ItemID]; ok && IsTerminalStatus(p.Status) {
			items = append(items, p)
			if p.Status != StatusOK {
				details[it.ItemID] = "carried forward from the prior receipt (terminal — not re-evaluated)"
			}
			continue
		}

		attempts := 1
		if p, ok := priorByID[it.ItemID]; ok {
			attempts = p.Attempts + 1
		}

		v := evaluateItem(root, planID, it, promptVersion, opts.OnLoss)
		// An unwritten item has no new output to judge: whatever is on disk
		// is the prior, and grading it would consume a plan nothing
		// extracted. The two provenance verdicts still win — a moved artifact
		// or contract is not fixed by a re-dispatch, and saying output-missing
		// over them would buy a pointless retry (see evaluateItem's order).
		if unwrittenSet[it.ItemID] && v.Status != StatusParentHashMismatch && v.Status != StatusPromptVersionMismatch {
			v = ItemVerdict{Status: StatusOutputMissing, Detail: fmt.Sprintf(
				"restore observed no write since plan %s: %s is unchanged from before the dispatch, so there is "+
					"no extraction to judge. If the extractor has written it since, run `edikt phasea restore --plan %s` "+
					"again before verifying", planID, it.ExpectedOutput.Path, planID)}

		}
		// A QUARANTINED write is judged from its quarantine copy. Before this,
		// it read as output-missing — no axis, no correction_prompt — so the
		// seeded retry re-dispatched blind on every axis, and every real
		// content-gate failure took that route because restore runs first.
		// The reclassification happens before the stalled / rejected-pin
		// overrides below, which key on TRUE output-missing only: the
		// extractor did write, and the output is the evidence.
		classifyPath := filepath.Join(root, filepath.FromSlash(it.ExpectedOutput.Path))
		if v.Status == StatusOutputMissing && quarantinedSet[it.ItemID] && !unwrittenSet[it.ItemID] {
			if qv, qpath, ok := quarantinedVerdict(root, planID, it); ok {
				v, classifyPath = qv, qpath
			}
		}
		status, detail := v.Status, v.Detail
		rejectedPinFinal := false
		if status == StatusOutputMissing {
			switch {
			case stalledSet[it.ItemID]:
				status = StatusStalled
				detail = "the dispatcher reported the subagent ended without a result; " + detail
			case opts.RejectedPin[it.ItemID] != "":
				status = StatusFailedTerminal
				rejectedPinFinal = true
				detail = fmt.Sprintf("the host rejected the explicit extractor.model pin: %s",
					capDiagnostic(opts.RejectedPin[it.ItemID], rejectedPinDiagnosticCap))
			}
		}

		// Classify the defect on every axis. This runs only for outcomes where
		// a sidecar actually exists to measure — an output-missing or
		// hash-mismatch item has nothing on disk to classify, and inventing a
		// zero-defect report for one would read as "clean".
		rec := ReceiptItem{ItemID: it.ItemID, Status: status, Attempts: attempts}
		var rep DefectReport
		haveReport := false
		if status == StatusSchemaInvalid || status == StatusOK {
			rep = ClassifyDefectsIn(root, classifyPath,
				filepath.Join(root, filepath.FromSlash(it.ArtifactPath)))
			haveReport = true

			// SEPARATE LENGTH FROM ANCHOR. Both used to land in
			// schema-invalid, which made a four-sentence split look identical
			// to an extraction that grounded nothing.
			if status == StatusSchemaInvalid {
				switch rep.Axis {
				case DefectAxisLength:
					status = StatusLengthExceeded
				case DefectAxisAnchor:
					status = StatusAnchorInvalid
				}
			}
			rec.Status = status
			rec.DefectAxis = rep.Axis
			rec.DefectCount = rep.Count()
			rec.AnchorsValid = rep.AnchorsValid
			rec.AnchorsBroken = rep.AnchorsBroken
			rec.AnchorsWhitespaceOnly = rep.AnchorsWhitespaceOnly
			rec.NeedsReview = rep.NeedsReview
			if status != StatusOK {
				rec.CorrectionPrompt = rep.CorrectionPrompt()
				if rep.Summary != "" {
					detail = fmt.Sprintf("%s (%s)", detail, rep.Summary)
				}
			}
		}
		// A verdict decided outside the classifier — a lossless or survival
		// loss — brings its own prompt; the classifier's wins when it has one.
		if status != StatusOK && rec.CorrectionPrompt == "" && v.Prompt != "" {
			rec.CorrectionPrompt = v.Prompt
		}

		// CONVERGENCE, then DIVERGENCE. Comparable only against a prior
		// attempt on the SAME axis — a length count and an anchor count are
		// different quantities and their difference means nothing.
		prior, hadPrior := priorByID[it.ItemID]
		switch {
		case !hadPrior || attempts <= 1:
			rec.Convergence = ConvergenceFirstAttempt

		// Reaching ok after a failing attempt IS the improvement, and it is
		// the acceptance signal for the whole retry-seeding fix. Falling
		// through to the axis comparison below would record it as "unknown"
		// (axis "none" against axis "length"), so a correction pass that
		// worked perfectly would not be counted as one.
		case status == StatusOK && prior.Status != StatusOK:
			rec.Convergence = ConvergenceImproved

		case !haveReport || prior.DefectAxis == "" || prior.DefectAxis != rec.DefectAxis:
			rec.Convergence = ConvergenceUnknown
		case rec.DefectCount < prior.DefectCount:
			rec.Convergence = ConvergenceImproved
		case rec.DefectCount == prior.DefectCount:
			rec.Convergence = ConvergenceUnchanged
		default:
			rec.Convergence = ConvergenceDiverged
		}

		// STOP ON DIVERGENCE, before the budget is spent. A retry that is
		// getting worse does not get better by being run again; spending the
		// remainder buys nothing. Checked BEFORE retry exhaustion so the
		// receipt records WHY the item stopped — "diverged" (budget
		// deliberately not spent) rather than "failed-terminal" (budget gone).
		divergedStop := rec.Convergence == ConvergenceDiverged && status != StatusOK

		switch {
		case divergedStop:
			detail = fmt.Sprintf(
				"%s — DIVERGED: attempt %d produced %d defect(s) on the %s axis against "+
					"%d on attempt %d. A retry that is getting worse will not improve by "+
					"being run again; the remaining budget (%d of %d attempts) is "+
					"deliberately NOT spent. This item MUST NOT be re-dispatched against "+
					"plan %s without a human changing the input.",
				detail, attempts, rec.DefectCount, rec.DefectAxis,
				prior.DefectCount, prior.Attempts,
				cfg.MaxAttemptsPerItem-attempts, cfg.MaxAttemptsPerItem, planID)
			status = StatusDiverged
			rec.Status = status

		// Retry exhaustion. pins-unrestorable is excluded because it is
		// already terminal on first observation — folding it into
		// failed-terminal would let a retry budget stand in for a permanent
		// loss of a human's approval (INV-015), and the two are deliberately
		// separate enum values for exactly that reason.
		case status != StatusOK && status != StatusPinsUnrestorable && !rejectedPinFinal && attempts >= cfg.MaxAttemptsPerItem:
			detail = fmt.Sprintf(
				"%s — retry budget exhausted (%d/%d verify calls under phasea.max_attempts_per_item); "+
					"this item MUST NOT be re-dispatched against plan %s",
				detail, attempts, cfg.MaxAttemptsPerItem, planID)
			status = StatusFailedTerminal
			rec.Status = status
		}

		// PERSIST THE DIAGNOSTIC. This one assignment is the root-cause fix:
		// the retry reads the receipt, so anything not written here does not
		// reach it.
		// An ok item can still carry a detail worth keeping: a lossless loss
		// accepted under --on-loss=accept passes, and the record of what was
		// accepted is the only trace it leaves.
		if status != StatusOK || detail != "" {
			rec.Diagnostic = detail
			details[it.ItemID] = detail
		}

		// STAMP extractor_model / extractor_contract on an ok item's
		// sidecar, before Phase B ever renders from it (see the file doc's
		// narrow exception). Precedence: a validated --extractor-model
		// self-report (the only source that can be right under `inherit`,
		// since nothing on disk observes which model a live Agent-tool
		// dispatch ran under) beats ResolveExtractorAgentModel's frontmatter
		// read (correct for an explicit config pin); the literal
		// "unrecorded" is never a guess, only the documented absence value.
		if status == StatusOK {
			model := opts.ExtractorModel
			if model == "" {
				if m, merr := ResolveExtractorAgentModel(root); merr == nil {
					model = m
				} else {
					model = ExtractorContractUnrecorded // same literal, "unrecorded"
				}
			}
			contract, cerr := ResolveExtractorContractDigest(root)
			if cerr != nil {
				contract = ExtractorContractUnrecorded
			}
			sidecarPath := filepath.Join(root, filepath.FromSlash(it.ExpectedOutput.Path))
			if serr := sidecar.InsertKeyInPlace(sidecarPath, []sidecar.KeyValue{
				{Key: "extractor_model", Value: model},
				{Key: "extractor_contract", Value: contract},
			}); serr != nil {
				// A stamp failure does not downgrade an already-decided ok
				// verdict — the extraction succeeded regardless of whether
				// its provenance could be recorded — but it must not be
				// silent (INV-013): surface it in the diagnostic even
				// though the item stays ok.
				rec.Diagnostic = fmt.Sprintf("%s (extractor_model/extractor_contract stamp failed: %v)",
					strings.TrimSpace(rec.Diagnostic+" "+detail), serr)
				details[it.ItemID] = rec.Diagnostic
			} else {
				rec.ExtractorModel = model
				rec.ExtractorContract = contract
			}
		}

		items = append(items, rec)
	}

	var failing []string
	retryable := 0
	diverged := 0
	for _, it := range items {
		if it.Status == StatusOK {
			continue
		}
		failing = append(failing, it.ItemID)
		if it.Status == StatusDiverged {
			diverged++
		}
		if !IsTerminalStatus(it.Status) {
			retryable++
		}
	}
	sort.Strings(failing)

	// The ONLY place exit 1 is produced, and it is reached only with a fully
	// evaluated item set in hand — see the file doc.
	exitCode := ExitVerifyOK
	switch {
	case len(failing) == 0:
		exitCode = ExitVerifyOK
	case retryable > 0:
		exitCode = ExitVerifyIncomplete
	case diverged > 0:
		// Divergence outranks plain exhaustion when both are present: it is
		// the more specific diagnosis and the one that changes what a human
		// does next.
		exitCode = ExitVerifyDiverged
	default:
		exitCode = ExitVerifyFailed
	}

	receipt := &Receipt{
		SchemaVersion: SchemaVersionReceipt,
		PlanID:        planID,
		Items:         items,
		Consumed:      len(failing) == 0,
		VerifiedAt:    time.Now().UTC().Format(time.RFC3339),
	}

	if werr := writeReceipt(root, receipt); werr != nil {
		// Exit 3, never 1. Without a receipt there is nowhere for `attempts`
		// to persist, so a retryable verdict here would loop forever.
		return malformed(fmt.Sprintf(
			"refusing to report a verify outcome: the receipt for %s could not be written: %v — "+
				"`attempts` persists only in the receipt, so reporting a retryable outcome without "+
				"one would put the skill in an unbounded re-dispatch loop",
			planID, werr))
	}

	summary := verifySummary(planID, receipt, details, exitCode)
	if line := overlapReport(restoreReceipt, plan, receipt); line != "" {
		summary += "\n" + line
	}

	jerr := appendJournal(root, JournalRecord{
		Event:   JournalEventVerify,
		PlanID:  planID,
		Items:   len(items),
		Failing: failing,
	})
	if jerr == nil && receipt.Consumed {
		jerr = appendJournal(root, JournalRecord{
			Event:  JournalEventConsume,
			PlanID: planID,
			Items:  len(items),
		})
	}
	// A journal failure does NOT change the exit code. The receipt is already
	// on disk and is the authoritative record of attempts and consumption; a
	// journal-append failure is an audit-trail gap, fully visible in the
	// message, and retracting a written receipt to "fix" it would destroy the
	// one file the retry loop depends on. Same posture plan.go takes for its
	// own emission (INV-013: the gap is reported, not hidden).
	summary = withJournal(summary, jerr)

	// Report the fixture obligation this regeneration created. Appended to the
	// summary rather than gated on exitCode: a verify that PASSED is exactly
	// the case where the debt is invisible, because nothing has refused yet.
	summary += fixtureDebtNotice(collectFixtureDebt(root, plan.Items))

	out := &VerifyOutcome{ExitCode: exitCode, Receipt: receipt, Message: summary, Details: details}
	if exitCode == ExitVerifyOK {
		return out, nil
	}
	return out, fmt.Errorf("%s", summary)
}

// malformed builds the exit-3 outcome: no receipt, nothing journaled. Exit 3
// is the only code an internal failure in this package may produce, which is
// what keeps exit 1's "a receipt was written" promise true.
func malformed(msg string) (*VerifyOutcome, error) {
	return &VerifyOutcome{ExitCode: ExitVerifyMalformed, Message: msg}, fmt.Errorf("%s", msg)
}

// withJournal appends a note when a journal line could not be written, so an
// audit-trail gap is never invisible in the reported message (INV-013).
func withJournal(msg string, err error) string {
	if err == nil {
		return msg
	}
	return msg + fmt.Sprintf(" (additionally, the journal line could not be written: %v)", err)
}

// ItemVerdict is one item's read-only verdict: the receipt-schema status plus
// the human-readable detail that names what was actually observed.
type ItemVerdict struct {
	Status string
	Detail string

	// Prompt is the correction_prompt a verdict decided outside the defect
	// classifier carries — a lossless or survival loss, whose remedy is a
	// re-dispatch that knows which bodies to put back. The classifier's own
	// prompt (length, anchor, parse, overclaim) wins when it has one.
	Prompt string
}

// evaluateItem is the per-item gate. It reads; it never writes.
//
// Order is deliberate and is not the enum's declaration order:
//
//  1. PARENT HASH, recomputed at verify time. The keystone. If the artifact
//     changed between plan and verify, nothing downstream means anything —
//     the extractor may have read prose that no longer exists, so the sidecar
//     cannot be trusted even if it parses, validates and anchors cleanly.
//     Checking it first also stops a changed artifact from surfacing as
//     `schema-invalid` (its anchors would fail against the NEW prose), which
//     would send a human hunting a bad extraction that never happened.
//  2. PROMPT VERSION. The extraction contract itself changed underneath the
//     plan; the output describes a different contract than the one planned.
//  3. OUTPUT EXISTS. Cheapest real observation, but reported after the two
//     provenance checks: an item whose artifact moved will not be fixed by a
//     re-dispatch, and reporting `output-missing` (retryable) over
//     `parent-hash-mismatch` would buy a pointless round of LLM dispatches.
//  4. CONTENT GATE — parse, schema, anchors (ADR-056). Shared verbatim with
//     restore.go and runner.go; see contentGate.
//  5. PINS. The D6 comparison: every field the plan's pre-dispatch snapshot
//     captured must be present on the live sidecar.
func evaluateItem(root, planID string, item PlanItem, resolvedPromptVersion, onLoss string) ItemVerdict {
	artifactPath := underRoot(root, item.ArtifactPath)
	sidecarPath := underRoot(root, item.ExpectedOutput.Path)

	sum, err := sha256Hex(artifactPath)
	if err != nil {
		return ItemVerdict{Status: StatusParentHashMismatch, Detail: fmt.Sprintf(
			"%s could not be read at verify time (%v), so its sha256 cannot be compared with the "+
				"plan's pinned %s", item.ArtifactPath, err, short(item.ArtifactSHA256))}

	}
	if sum != item.ArtifactSHA256 {
		return ItemVerdict{Status: StatusParentHashMismatch, Detail: fmt.Sprintf(
			"%s hashes %s at verify time but the plan pinned %s — the artifact changed between "+
				"planning and dispatch, so whatever the extractor read cannot be proven to be the "+
				"planned version", item.ArtifactPath, short(sum), short(item.ArtifactSHA256))}

	}

	if resolvedPromptVersion != item.PromptVersion {
		return ItemVerdict{Status: StatusPromptVersionMismatch, Detail: fmt.Sprintf(
			"the installed extractor resolves prompt_version %s but the plan pinned %s — the "+
				"extraction contract changed between planning and verify",
			resolvedPromptVersion, item.PromptVersion)}

	}

	st, serr := os.Stat(sidecarPath)
	switch {
	case serr != nil:
		return ItemVerdict{Status: StatusOutputMissing, Detail: fmt.Sprintf(
			"expected_output %s does not exist (%v)", item.ExpectedOutput.Path, serr)}

	case st.IsDir():
		return ItemVerdict{Status: StatusSchemaInvalid, Detail: fmt.Sprintf(
			"expected_output %s is a directory, not a sidecar", item.ExpectedOutput.Path)}

	}

	if gerr := contentGate(root, sidecarPath, artifactPath); gerr != nil {
		return ItemVerdict{Status: StatusSchemaInvalid, Detail: fmt.Sprintf(
			"%s failed the content gate: %v", item.ExpectedOutput.Path, gerr)}

	}

	missing, merr := missingPins(root, planID, item.ItemID, sidecarPath)
	if merr != nil {
		// The pin-set was captured but cannot be compared. Unmeasured must
		// not read as a clean bill of health (INV-013), and a pin whose
		// survival cannot be checked is exactly what pins-unrestorable names.
		return ItemVerdict{Status: StatusPinsUnrestorable, Detail: fmt.Sprintf(
			"the pre-dispatch pin-set for %s could not be compared against the live sidecar: %v",
			item.ItemID, merr)}

	}
	if len(missing) > 0 {
		return ItemVerdict{Status: StatusPinsUnrestorable, Detail: describeMissingPins(item, missing)}
	}
	if item.Reason == ReasonStaleMigrationPreserved {
		lv := losslessVerdict(root, planID, item, sidecarPath, onLoss)
		if lv.Status != StatusOK {
			return lv
		}
		// A lifted item's snapshot carries its baseline under
		// migration_preserved, not as bodies, so the survival check below
		// usually has nothing to compare there. It still runs: the guard is
		// migration-blind, and a lifted prior that does carry bodies loses
		// them the same way any other does.
		sv := survivalVerdict(root, planID, item, artifactPath, sidecarPath, onLoss)
		sv.Detail = joinDetails(lv.Detail, sv.Detail)
		return sv
	}

	return survivalVerdict(root, planID, item, artifactPath, sidecarPath, onLoss)
}

// joinDetails joins two verdict details, dropping an empty one.
func joinDetails(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// quarantinedVerdict judges an item restore quarantined, from the quarantine
// copy: the content gate's verdict on the bytes the extractor actually wrote.
// ok is false when there is no copy to judge (the item stays output-missing).
//
// A copy that now PASSES the gate is still not ok: restore moved it out of
// the corpus, so nothing at the expected path carries it, and grading it ok
// would consume a plan whose sidecar is absent. It stays output-missing, with
// a detail saying the copy passes now.
func quarantinedVerdict(root, planID string, item PlanItem) (ItemVerdict, string, bool) {
	qpath := QuarantinePath(root, planID, item.ItemID)
	if st, err := os.Stat(qpath); err != nil || st.IsDir() {
		return ItemVerdict{}, "", false
	}
	rel := filepath.ToSlash(relPath(root, qpath))
	gerr := contentGate(root, qpath, underRoot(root, item.ArtifactPath))
	if gerr == nil {
		return ItemVerdict{Status: StatusOutputMissing, Detail: fmt.Sprintf(
				"quarantined by restore at %s, and that copy passes the content gate now — but nothing is at %s, "+
					"so there is no sidecar to accept; re-dispatch the item", rel, item.ExpectedOutput.Path)},

			"", true
	}
	return ItemVerdict{Status: StatusSchemaInvalid, Detail: fmt.Sprintf(
			"quarantined by restore at %s: it failed the content gate: %s",
			rel, capDiagnostic(gerr.Error(), quarantineGateErrCap))},

		qpath, true
}

// quarantineGateErrCap bounds the content-gate error quoted in a quarantined
// item's diagnostic. The gate names every faulting anchor with its quote, so
// an extraction with dozens of faults would otherwise overrun the receipt's
// 4000-character diagnostic; the correction_prompt carries the per-anchor
// detail.
const quarantineGateErrCap = 1500

// losslessVerdict is the lossless proof for a lifted sidecar under the
// plan/receipt flow. The baseline the extractor was handed is the pre-dispatch
// snapshot's migration_preserved; the output on disk no longer carries it (the
// extractor is told not to), so the comparison runs here, against the
// snapshot, with the same comparator gov compile used in-process.
func losslessVerdict(root, planID string, item PlanItem, sidecarPath, onLoss string) ItemVerdict {
	accept := onLoss == "accept"
	snap, serr := sidecar.Load(PinSnapshotPath(root, planID, item.ItemID))
	if serr != nil || snap == nil || snap.MigrationPreserved == nil || len(snap.MigrationPreserved.Directives) == 0 {
		reason := "the pre-dispatch snapshot carries no migration_preserved directives to compare against"
		if serr != nil {
			reason = fmt.Sprintf("the pre-dispatch snapshot could not be read: %v", serr)
		}
		if accept {
			return ItemVerdict{Status: StatusOK, Detail: "lossless check UNMEASURED and accepted (--on-loss=accept): " + reason}
		}
		return ItemVerdict{Status: StatusLosslessUnmeasured, Detail: "lossless check could not run for a lifted sidecar: " + reason + " — nothing measured is not a pass"}
	}
	out, lerr := sidecar.Load(sidecarPath)
	if lerr != nil {
		return ItemVerdict{Status: StatusSchemaInvalid, Detail: fmt.Sprintf("%s could not be loaded for the lossless check: %v", item.ExpectedOutput.Path, lerr)}
	}
	losses := migrate.CheckLosslessAgainstDirectives(snap.MigrationPreserved.Directives, out)
	if len(losses) == 0 {
		return ItemVerdict{Status: StatusOK, Detail: ""}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d preserved directive(s) missing from the extractor's output:", len(losses))
	for _, l := range losses {
		fmt.Fprintf(&b, " [%s] %s;", l.Type, headOf(l.LegacyText, 100))
	}
	if accept {
		return ItemVerdict{Status: StatusOK, Detail: "loss accepted (--on-loss=accept): " + b.String()}
	}
	// The retry this text asks for is a correction pass, so it carries the
	// prompt that names what to put back (conflict #6): without one, the
	// INCOMPLETE line reports the item instead of re-dispatching it.
	var p strings.Builder
	fmt.Fprintf(&p, "CORRECTION PASS — the previous attempt dropped %d of the directives this artifact "+
		"already carried. Extract the artifact as before, and make sure the output states each of these, "+
		"with the same modal force and (ref: ...) tail, anchored to the parent passage that states it:", len(losses))
	for _, l := range losses {
		fmt.Fprintf(&p, "\n- %q", headOf(l.LegacyText, 300))
	}
	return ItemVerdict{
		Status: StatusLosslessLoss,
		Detail: b.String() + " re-dispatch this item with the migrate-time suffix, restore the entries by hand, or pass --on-loss=accept to record the loss",
		Prompt: capDiagnostic(p.String(), correctionPromptCap),
	}
}

// headOf is s cut to at most n bytes on a rune boundary, with "..." when cut.
func headOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	cut := n - 3
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "..."
}

// survivalFindingCap and survivalNoteCap bound the two body lists a survival
// verdict quotes in its diagnostic — the lost bodies, and the lexical /
// source-removed notes. Together with the remedy sentence they stay well
// inside the receipt's 4000-character diagnostic, leaving room for the
// retry-exhaustion suffix Verify may append.
const (
	survivalFindingCap = 1800
	survivalNoteCap    = 1200
)

// priorBody is one directive or prohibition of the pre-dispatch snapshot.
type priorBody struct {
	kind    string // "directive" | "prohibition"
	index   int
	text    string
	anchors []sidecar.SourceExcerpt
}

func (pb priorBody) name() string {
	return fmt.Sprintf("%s[%d] %q", pb.kind+"s", pb.index, headOf(pb.text, 100))
}

// survivalVerdict is the body-loss guard for every re-extraction of an
// existing sidecar (#11, c6-b). losslessVerdict guards only a lifted item,
// against its migration_preserved baseline; a stale-quote-drift or
// `requested` re-extraction — `gov reextract`, a per-artifact compile —
// could drop a body whose prose is still in the artifact, and verify
// consumed the plan. INV-015: a body must not leave the corpus silently.
//
// It runs for every item whose plan reason had a prior sidecar and whose
// pre-dispatch snapshot loads. A `missing` item had no prior, so there is
// nothing to have lost, whatever file sits at the pin path.
//
// Each prior directive and prohibition is classified, in this order:
//
//   - covered by anchor: an output anchor quotes the same passage as one of
//     the body's anchors that is still in the parent — equal, containing, or
//     contained, compared in the normalised frame. Rewording and merging are
//     the extractor's job; the passage is the identity.
//   - covered lexically: migrate's tuple comparator finds the body's
//     (modality, ref, noun phrase) in the output. The weaker instrument,
//     and reported as one. It must run before source-removed: a drifted
//     anchor cannot vouch for anything, so without it every drifted body
//     would read source-removed and the guard would be vacuous for exactly
//     the items stale-quote-drift plans.
//   - source-removed: none of its anchors is in the parent and the lexical
//     check failed. Named, not failed — the prose it stated is gone.
//   - otherwise lost: lossless-loss (retryable), or ok with the loss on the
//     record under --on-loss=accept.
//
// Its text is reason-neutral and migration-blind: the remedy is a correction
// pass whose prompt names each lost body and the parent range its anchor
// records, never a migrate-time suffix that does not exist on this path.
func survivalVerdict(root, planID string, item PlanItem, artifactPath, sidecarPath, onLoss string) ItemVerdict {
	accept := onLoss == "accept"
	if item.Reason == ReasonMissing {
		return ItemVerdict{Status: StatusOK}
	}
	snapPath := PinSnapshotPath(root, planID, item.ItemID)
	if _, err := os.Stat(snapPath); err != nil {
		// No snapshot: a bootstrap (nothing was pinned because nothing
		// existed). missingPins reads the same absence the same way.
		return ItemVerdict{Status: StatusOK}
	}
	prior, perr := sidecar.Load(snapPath)
	if perr != nil || prior == nil {
		// missingPins refuses an unreadable snapshot first, so this is a race
		// or a defect. Either way nothing was compared, and that is not a
		// pass (INV-013).
		reason := fmt.Sprintf("the pre-dispatch snapshot could not be read: %v", perr)
		if accept {
			return ItemVerdict{Status: StatusOK, Detail: "survival check UNMEASURED and accepted (--on-loss=accept): " + reason}
		}
		return ItemVerdict{Status: StatusLosslessUnmeasured, Detail: "survival check could not run: " + reason + " — nothing measured is not a pass"}
	}
	raw, rerr := os.ReadFile(artifactPath)
	if rerr != nil {
		return ItemVerdict{Status: StatusParentHashMismatch, Detail: fmt.Sprintf(
			"%s could not be read for the survival check: %v", item.ArtifactPath, rerr)}
	}
	parent := strings.Split(string(raw), "\n")
	out, lerr := sidecar.Load(sidecarPath)
	if lerr != nil {
		return ItemVerdict{Status: StatusSchemaInvalid, Detail: fmt.Sprintf(
			"%s could not be loaded for the survival check: %v", item.ExpectedOutput.Path, lerr)}
	}

	var outQuotes []string
	for _, d := range out.Directives {
		for _, a := range d.Anchors() {
			outQuotes = append(outQuotes, a.Quote)
		}
	}
	for _, p := range out.Prohibitions {
		for _, a := range p.Anchors() {
			outQuotes = append(outQuotes, a.Quote)
		}
	}

	var bodies []priorBody
	for i, d := range prior.Directives {
		bodies = append(bodies, priorBody{"directive", i, d.Text, d.Anchors()})
	}
	for i, p := range prior.Prohibitions {
		bodies = append(bodies, priorBody{"prohibition", i, p.Text, p.Anchors()})
	}

	var lost, lexical, removed []string
	var lostPrompt []string
	for _, pb := range bodies {
		if strings.TrimSpace(pb.text) == "" {
			continue
		}
		var present []sidecar.SourceExcerpt
		for _, a := range pb.anchors {
			if sidecar.QuotePresent(a.Quote, parent) {
				present = append(present, a)
			}
		}
		if anchorCovered(present, outQuotes) {
			continue
		}
		if len(migrate.CheckLosslessAgainstDirectives([]string{pb.text}, out)) == 0 {
			lexical = append(lexical, pb.name())
			continue
		}
		if len(present) == 0 {
			removed = append(removed, pb.name())
			continue
		}
		var ranges, quotes []string
		for _, a := range present {
			ranges = append(ranges, fmt.Sprintf("lines %d-%d", a.LineStart, a.LineEnd))
			quotes = append(quotes, fmt.Sprintf("parent lines %d-%d (%q)", a.LineStart, a.LineEnd, headOf(a.Quote, 120)))
		}
		lost = append(lost, fmt.Sprintf("%s (%s)", pb.name(), strings.Join(ranges, ", ")))
		lostPrompt = append(lostPrompt, fmt.Sprintf("the %s %q — grounded at %s",
			pb.kind, headOf(pb.text, 300), strings.Join(quotes, " and ")))
	}

	var notes []string
	if len(lexical) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d prior bod(y/ies) matched only by the lexical fallback (its anchors are no longer in the parent, "+
				"so the weaker instrument decided): %s", len(lexical), strings.Join(lexical, "; ")))
	}
	if len(removed) > 0 {
		notes = append(notes, fmt.Sprintf(
			"%d prior bod(y/ies) source-removed (no anchor is in the parent any more and the output does not carry "+
				"it; not a loss): %s", len(removed), strings.Join(removed, "; ")))
	}
	note := capDiagnostic(strings.Join(notes, "; "), survivalNoteCap)
	if len(lost) == 0 {
		return ItemVerdict{Status: StatusOK, Detail: note}
	}

	finding := capDiagnostic(fmt.Sprintf(
		"%d prior bod(y/ies) dropped by the new extraction while their source prose remains in %s: %s",
		len(lost), item.ArtifactPath, strings.Join(lost, "; ")), survivalFindingCap)
	if accept {
		return ItemVerdict{Status: StatusOK, Detail: joinDetails("loss accepted (--on-loss=accept): "+finding, note)}
	}
	var p strings.Builder
	fmt.Fprintf(&p, "CORRECTION PASS — the previous attempt dropped %d rule(s) whose source prose is still in "+
		"the parent. Extract the artifact as before, and make sure the output carries an entry for each of "+
		"these, anchored to the passage it was grounded in (it may be reworded, or merged with another entry "+
		"that carries that anchor too):", len(lost))
	for _, l := range lostPrompt {
		p.WriteString("\n- " + l)
	}
	p.WriteString("\nKeep every other entry and anchor as you would extract them.")
	return ItemVerdict{
		Status: StatusLosslessLoss,
		Detail: joinDetails(finding+" — re-dispatch this item as a correction pass carrying its correction_prompt, "+
			"restore the entries by hand, or pass --on-loss=accept to record the loss", note),
		Prompt: capDiagnostic(p.String(), correctionPromptCap),
	}
}

// anchorCovered reports whether any output anchor quotes the same passage as
// any of present — equal, containing, or contained, in QuotePresent's
// normalised frame.
func anchorCovered(present []sidecar.SourceExcerpt, outQuotes []string) bool {
	for _, a := range present {
		for _, o := range outQuotes {
			if sidecar.QuotePresent(a.Quote, []string{o}) || sidecar.QuotePresent(o, []string{a.Quote}) {
				return true
			}
		}
	}
	return false
}

// missingPins compares the plan's pre-dispatch pin snapshot against the LIVE
// sidecar and reports every pinned field the live file does not carry. It is
// pinpreserve.MissingPins — the read-only sibling of the same comparison
// restore.go runs through PreservePinned — so a field can never count as
// "restored" by one and "missing" by the other.
//
// A snapshot that does not exist is not a gap: plan.go writes one only when a
// prior sidecar existed, and the "missing" reason legitimately has nothing to
// preserve.
func missingPins(root, planID, itemID, sidecarPath string) ([]pinpreserve.PinnedField, error) {
	snap := PinSnapshotPath(root, planID, itemID)
	if _, err := os.Stat(snap); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	before, loadErr := sidecar.Load(snap)
	if loadErr != nil {
		before = nil
	}
	return pinpreserve.MissingPins(itemID, before, loadErr, sidecarPath)
}

// describeMissingPins NAMES every field, which is the whole point of the
// status: "pins-unrestorable" alone tells a human that something a colleague
// approved is gone, without saying what.
func describeMissingPins(item PlanItem, missing []pinpreserve.PinnedField) string {
	names := make([]string, 0, len(missing))
	for _, m := range missing {
		// Deliberately not PinnedField.String(), which omits Value: the
		// approval stamp or approved command IS what a human needs in hand to
		// re-approve, and a report that says only "human_approved_at is gone"
		// sends them to git history to find out what it said.
		entry := m.Field
		if m.Value != "" {
			entry += "=" + m.Value
		}
		if m.DirectiveText != "" {
			entry += fmt.Sprintf(" on %q", m.DirectiveText)
		}
		names = append(names, entry+" ("+m.Reason+")")
	}
	return fmt.Sprintf(
		"%d pinned field(s) captured before dispatch are not present on %s: %s — "+
			"`edikt phasea restore --plan` could not reinstate them. Governance "+
			"content, including a human's prior approval, must never leave silently.",
		len(missing), item.ExpectedOutput.Path, strings.Join(names, "; "))
}

// contentGate is the ADR-056 generation boundary, as one function: PARSE
// proves it is YAML, SCHEMA proves it conforms, ANCHORS prove it actually
// quotes the artifact it claims to.
//
// It has three callers — verify.go (to decide a status), restore.go (to
// decide whether to quarantine a bad write) and runner.go's dispatch gate —
// and exactly one definition. Three copies of "what counts as a valid
// extraction" is how a sidecar comes to pass one gate and fail another
// (GL-002: two things that must agree, kept separately).
func contentGate(projectRoot, sidecarPath, parentPath string) error {
	if _, err := sidecar.Load(sidecarPath); err != nil {
		return err
	}
	raw, err := os.ReadFile(sidecarPath)
	if err != nil {
		return err
	}
	// Validate against the schema the DOCUMENT declares, never a hardcoded
	// version — hardcoding is what made a freshly-migrated v2 corpus read as
	// 70 of 82 sidecars failing "the authoritative schema".
	if err := sidecar.ValidateRawAgainstDeclaredSchema(raw); err != nil {
		return err
	}
	if err := verifyAnchorsAgainstParent(sidecarPath, parentPath); err != nil {
		return err
	}
	return verifyNoBehavioralOverclaim(projectRoot, sidecarPath)
}

// ── plan / receipt state ─────────────────────────────────────────────────

// ReceiptFilePath is .edikt/state/phasea/<plan_id>.receipt.json.
func ReceiptFilePath(root, planID string) string {
	return filepath.Join(root, ".edikt", "state", "phasea", planID+".receipt.json")
}

// loadPlanForVerb reads and validates the plan file both `phasea verify` and
// `phasea restore` operate on. On failure it returns a nil plan plus the exit
// code and message the verb should surface: 2 when the plan_id names no
// emitted plan, 3 when a plan file exists but cannot be trusted.
//
// The `..` / absolute-path rejection is not defensive noise. Both item paths
// are read back out of a state file and then joined to the project root;
// restore.go MOVES the file at expected_output.path when the content gate
// fails, so a plan carrying `../../etc/x` would make a state file into a
// filesystem-relocation instruction (INV-006 — the value is externally
// controlled the moment it round-trips through disk).
func loadPlanForVerb(root, planID string) (*DispatchPlan, int, string) {
	path := PlanFilePath(root, planID)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ExitVerifyPlanRejected, fmt.Sprintf(
				"refusing to proceed: plan %s is unknown — no plan file at %s", planID, path)
		}
		return nil, ExitVerifyMalformed, fmt.Sprintf(
			"refusing to proceed: plan file %s exists but could not be read: %v", path, err)
	}

	var plan DispatchPlan
	if err := json.Unmarshal(data, &plan); err != nil {
		return nil, ExitVerifyMalformed, fmt.Sprintf(
			"refusing to proceed: plan file %s is not parseable JSON: %v", path, err)
	}
	if plan.SchemaVersion != SchemaVersionPlan {
		// A plan written by an OLDER binary is the ordinary case (an
		// upgrade landed between plan and verify): name the gap directly
		// and point at its actual remedy — `/edikt:upgrade` removes an
		// unconsumed stale plan (D57 (4)) — rather than the generic
		// "declares X, want Y" phrasing, which reads as a corrupt file
		// rather than a version gap with a one-command fix.
		if plan.SchemaVersion < SchemaVersionPlan {
			return nil, ExitVerifyMalformed, fmt.Sprintf(
				"refusing to proceed: plan file %s was written by schema v%d; this binary writes v%d — run /edikt:upgrade",
				path, plan.SchemaVersion, SchemaVersionPlan)
		}
		return nil, ExitVerifyMalformed, fmt.Sprintf(
			"refusing to proceed: plan file %s declares schema_version %d, want %d",
			path, plan.SchemaVersion, SchemaVersionPlan)
	}
	if plan.PlanID != planID {
		return nil, ExitVerifyMalformed, fmt.Sprintf(
			"refusing to proceed: plan file %s carries plan_id %q, not the requested %q",
			path, plan.PlanID, planID)
	}
	// A plan with no items cannot exist — Plan() returns ExitNothingToPlan
	// rather than emitting one — and a receipt over zero items would be a
	// gate reporting on nothing (INV-013), which phasea-receipt.v2 also
	// forbids with minItems: 1.
	if len(plan.Items) == 0 {
		return nil, ExitVerifyMalformed, fmt.Sprintf(
			"refusing to proceed: plan file %s carries zero items — there is nothing to verify, "+
				"and a receipt over zero items would report a gate that examined nothing", path)
	}
	for _, it := range plan.Items {
		for _, rel := range []string{it.ArtifactPath, it.ExpectedOutput.Path} {
			if !safeRelPath(rel) {
				return nil, ExitVerifyMalformed, fmt.Sprintf(
					"refusing to proceed: plan file %s item %q carries the path %q, which is "+
						"absolute or escapes the project root", path, it.ItemID, rel)
			}
		}
	}
	return &plan, ExitVerifyOK, ""
}

// safeRelPath rejects an absolute path or one whose cleaned form escapes the
// project root.
func safeRelPath(rel string) bool {
	if rel == "" || filepath.IsAbs(rel) || strings.HasPrefix(rel, "/") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel)))
	if clean == ".." || strings.HasPrefix(clean, "../") {
		return false
	}
	return true
}

// underRoot resolves a project-root-relative plan path. root == "" means the
// current working directory, matching PlanOptions.ProjectRoot's convention.
func underRoot(root, rel string) string {
	return filepath.Join(root, filepath.FromSlash(rel))
}

// loadReceipt returns the prior receipt for planID, or (nil, nil) when none
// exists. A receipt that exists but cannot be parsed is an ERROR, never a
// silent nil: treating it as absent would restart the retry budget at zero
// and could re-verify a consumed plan.
func loadReceipt(root, planID string) (*Receipt, error) {
	data, err := os.ReadFile(ReceiptFilePath(root, planID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var r Receipt
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	if r.PlanID != planID {
		return nil, fmt.Errorf("receipt carries plan_id %q, not the requested %q", r.PlanID, planID)
	}
	return &r, nil
}

// writeReceipt serializes the receipt and writes it via temp-file-then-rename,
// so a concurrent reader never observes a partial file and a failed write
// never truncates the prior receipt (which still holds the live `attempts`).
func writeReceipt(root string, r *Receipt) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	dst := ReceiptFilePath(root, r.PlanID)
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".receipt.*.json.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, dst); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}

// ── reporting ────────────────────────────────────────────────────────────

// verifySummary renders what a human (and the skill) needs to act: the
// verdict, the per-status counts, and one line per non-ok item naming the
// item, its status, its attempt count against the ceiling, and WHAT WAS
// OBSERVED. A status word with no observation behind it sends the reader
// hunting.
func verifySummary(planID string, r *Receipt, details map[string]string, exitCode int) string {
	counts := map[string]int{}
	for _, it := range r.Items {
		counts[it.Status]++
	}
	statuses := make([]string, 0, len(counts))
	for s, n := range counts {
		statuses = append(statuses, fmt.Sprintf("%d %s", n, s))
	}
	sort.Strings(statuses)

	// ONE TABLE, EVERY DOCUMENTED CODE — label and next step together.
	//
	// The verdict map used to hold three entries while the verb returns
	// seven, so a rejected run fell through to an EMPTY label and the switch
	// below appended no instruction. What the reader saw was the retry line
	// belonging to exit 1, under a run that exited 2: the message and the
	// exit contract said opposite things, and the contract's own table is
	// clear that 1 is retryable and 2 is not. A caller who believed the prose
	// would re-dispatch against a plan that can never be valid again.
	//
	// Keeping the label and the instruction in one table is the fix, not a
	// bigger switch: two structures keyed by the same code drift the moment
	// one gains an entry.
	v, known := verifyVerdicts[exitCode]
	verdict := v.label
	if !known {
		// Never blank. An unlabelled code is a defect in this table, and it
		// says so rather than rendering as an unexplained gap.
		verdict = fmt.Sprintf("UNLABELLED VERDICT (exit %d) — this exit code has no entry in verifyVerdicts", exitCode)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "verify %s — %s: %d item(s) [%s]",
		planID, verdict, len(r.Items), strings.Join(statuses, ", "))
	// needs_review, counted from the sidecars of the items verify measured
	// (an ok item, or one classified on a defect axis). This is the number a
	// report quotes — never the extractor's hand-back. Absent when nothing
	// was measured, rather than a 0 nothing observed (INV-013).
	measured, needsReview := 0, 0
	for _, it := range r.Items {
		if it.Status == StatusOK || it.DefectAxis != "" {
			measured++
			needsReview += it.NeedsReview
		}
	}
	if measured > 0 {
		fmt.Fprintf(&b, "; needs_review: %d (counted on %d sidecar(s))", needsReview, measured)
	}
	for _, it := range r.Items {
		if it.Status == StatusOK {
			continue
		}
		fmt.Fprintf(&b, "\n  %s: %s (attempt %d) — %s", it.ItemID, it.Status, it.Attempts, details[it.ItemID])
	}
	if known && v.nextStep != "" {
		b.WriteString("\n  " + v.nextStep)
	}
	return b.String()
}

// verifyVerdict is what a caller is told for one exit code: what happened, and
// what to do next. Both, or neither — an exit code with a label and no
// instruction leaves the reader acting on the instruction printed above it,
// which is exactly how a rejected plan came to carry the retry line.
type verifyVerdict struct {
	label    string
	nextStep string
}

// verifyVerdicts covers EVERY exit code Verify can return. The test beside it
// asserts that, and is falsified by deleting an entry: a code with no entry
// must be impossible rather than silently blank.
var verifyVerdicts = map[int]verifyVerdict{
	ExitVerifyOK: {
		label:    "CONSUMED",
		nextStep: "", // nothing to do; the plan is consumed
	},
	ExitVerifyIncomplete: {
		label: "INCOMPLETE",
		nextStep: "re-dispatch each non-terminal item above as a correction pass carrying its receipt " +
			"correction_prompt, then run restore and verify again. An output-missing or stalled item has no output " +
			"to correct: re-dispatch it as a fresh extraction. Any other item with an empty correction_prompt is " +
			"reported, not re-dispatched",
	},
	ExitVerifyPlanRejected: {
		label: "REJECTED (the plan is not valid for this corpus)",
		nextStep: "this plan is invalid — the artifact changed after planning, so what the extractor read " +
			"cannot be proven to be the planned version. Run `edikt phasea plan` for a fresh plan_id; " +
			"do NOT re-dispatch against this one",
	},
	ExitVerifyMalformed: {
		label: "MALFORMED (the plan file cannot be read as a dispatch plan)",
		nextStep: "the plan file is not a valid phasea-dispatch-plan — do not act on any part of it. " +
			"Run `edikt phasea plan` for a fresh plan",
	},
	ExitVerifyFailed: {
		label: "FAILED (terminal)",
		nextStep: "no retryable items remain — this plan_id is terminal and `phasea restore` refuses it. " +
			"Restore the pin (.edikt/state/phasea/<plan_id>.pins/<item>.yaml) over each failed item's sidecar, " +
			"or `git checkout -- <sidecar>`, then re-request the artifact: that derives a successor plan. " +
			"A corpus or config change also gives a new plan_id; reverting an edit does not",
	},
	ExitVerifyDiverged: {
		label: "DIVERGED (an item got worse)",
		nextStep: "an item produced MORE defects than its previous attempt — a retry that is getting worse " +
			"will not improve by being run again. Stop and read the item's diagnostic above",
	},
	ExitVerifyRestoreMissing: {
		label: "REFUSED (restore has not run)",
		nextStep: "run `edikt phasea restore --plan <id>` first — verifying before the pinned state is back " +
			"measures a corpus missing everything a human decided",
	},
}

// short renders a sha256 the way git renders an object id — enough to
// distinguish, short enough to read in a message that names two of them.
func short(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12]
}

// overlapWindow is the width within which two output writes count as
// overlapping dispatches for SR-014's report. It is deliberately coarse: the
// evidence is END times, so this is an approximation and is labelled one.
const overlapWindow = 60 * time.Second

// overlapReport is SPEC-013 SR-014: report when more dispatches than the plan's
// cap overlapped, from the evidence there is — each ok item's extractor write
// time, an END time — and say so in the report itself. Detect, not prevent: it
// never changes the exit code. Empty when nothing crossed the cap or the plan
// carries no cap (a pre-v2 plan).
//
// THE TIMES COME FROM RESTORE'S RECEIPT, NOT FROM os.Stat. Restore stamps every
// sidecar it restores and verify stamps every ok one, so by now each mtime is
// the moment one of those two ran: every multi-item plan read as one burst,
// whatever the extractors did. Restore records each mtime before it writes
// anything. When fewer times are recorded than it takes to exceed the cap (a
// receipt written before they were), the line says the overlap was not
// measured rather than going quiet.
func overlapReport(rr *RestoreReceipt, plan *DispatchPlan, r *Receipt) string {
	capN := plan.Budget.MaxConcurrency
	if capN <= 0 {
		return ""
	}
	okItems := 0
	var times []time.Time
	for _, it := range r.Items {
		if it.Status != StatusOK {
			continue
		}
		okItems++
		if rr == nil {
			continue
		}
		if at, ok := rr.OutputsWrittenAt[it.ItemID]; ok {
			if t, err := time.Parse(time.RFC3339Nano, at); err == nil {
				times = append(times, t)
			}
		}
	}
	if okItems <= capN {
		return ""
	}
	if len(times) <= capN {
		return fmt.Sprintf("concurrency: not measured — %d of %d ok item(s) have an extractor write time recorded by "+
			"restore, and more than the cap of %d (budget.max_concurrency) are needed to compare (a restore receipt "+
			"written before write times were recorded carries none)", len(times), okItems, capN)
	}
	sort.Slice(times, func(i, j int) bool { return times[i].Before(times[j]) })
	maxN := 0
	for i := range times {
		n := 1
		for j := i + 1; j < len(times) && times[j].Sub(times[i]) <= overlapWindow; j++ {
			n++
		}
		if n > maxN {
			maxN = n
		}
	}
	if maxN <= capN {
		return ""
	}
	return fmt.Sprintf("concurrency (approximate, from extractor write times recorded by restore — end times, not dispatch starts): %d outputs written within %s of one another against a cap of %d (budget.max_concurrency); reported, not enforced",
		maxN, overlapWindow, capN)
}
