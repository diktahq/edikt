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
// Nothing in this file writes to a sidecar, an artifact, or a pin snapshot.
// It writes exactly two things, both of them audit state: the receipt at
// .edikt/state/phasea/<plan_id>.receipt.json and journal lines at
// .edikt/state/phasea-dispatch.jsonl. Putting a sidecar write in here would
// re-create the exact "did the write happen" / "make the write happen" fusion
// SPEC-013 exists to undo — a gate that repairs what it is measuring can no
// longer report on it.
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
)

// planIDPattern is phasea-dispatch-plan.v1 / phasea-receipt.v1 /
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

	if !planIDPattern.MatchString(planID) {
		return malformed(fmt.Sprintf(
			"refusing to verify: %q is not a valid plan_id — expected the "+
				"phasea-dispatch-plan.v1 shape `pap-` followed by 12 lowercase hex characters",
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
				"is terminal; recovery requires a fresh plan, which a corpus or config change produces",
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

	items := make([]ReceiptItem, 0, len(plan.Items))
	details := map[string]string{}
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

		v := evaluateItem(root, planID, it, promptVersion)
		status, detail := v.Status, v.Detail

		// Retry exhaustion. pins-unrestorable is excluded because it is
		// already terminal on first observation — folding it into
		// failed-terminal would let a retry budget stand in for a permanent
		// loss of a human's approval (INV-015), and the two are deliberately
		// separate enum values for exactly that reason.
		if status != StatusOK && status != StatusPinsUnrestorable && attempts >= cfg.MaxAttemptsPerItem {
			detail = fmt.Sprintf(
				"%s — retry budget exhausted (%d/%d verify calls under phasea.max_attempts_per_item); "+
					"this item MUST NOT be re-dispatched against plan %s",
				detail, attempts, cfg.MaxAttemptsPerItem, planID)
			status = StatusFailedTerminal
		}

		items = append(items, ReceiptItem{ItemID: it.ItemID, Status: status, Attempts: attempts})
		if status != StatusOK {
			details[it.ItemID] = detail
		}
	}

	var failing []string
	retryable := 0
	for _, it := range items {
		if it.Status == StatusOK {
			continue
		}
		failing = append(failing, it.ItemID)
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
func evaluateItem(root, planID string, item PlanItem, resolvedPromptVersion string) ItemVerdict {
	artifactPath := underRoot(root, item.ArtifactPath)
	sidecarPath := underRoot(root, item.ExpectedOutput.Path)

	sum, err := sha256Hex(artifactPath)
	if err != nil {
		return ItemVerdict{StatusParentHashMismatch, fmt.Sprintf(
			"%s could not be read at verify time (%v), so its sha256 cannot be compared with the "+
				"plan's pinned %s", item.ArtifactPath, err, short(item.ArtifactSHA256))}
	}
	if sum != item.ArtifactSHA256 {
		return ItemVerdict{StatusParentHashMismatch, fmt.Sprintf(
			"%s hashes %s at verify time but the plan pinned %s — the artifact changed between "+
				"planning and dispatch, so whatever the extractor read cannot be proven to be the "+
				"planned version", item.ArtifactPath, short(sum), short(item.ArtifactSHA256))}
	}

	if resolvedPromptVersion != item.PromptVersion {
		return ItemVerdict{StatusPromptVersionMismatch, fmt.Sprintf(
			"the installed extractor resolves prompt_version %s but the plan pinned %s — the "+
				"extraction contract changed between planning and verify",
			resolvedPromptVersion, item.PromptVersion)}
	}

	st, serr := os.Stat(sidecarPath)
	switch {
	case serr != nil:
		return ItemVerdict{StatusOutputMissing, fmt.Sprintf(
			"expected_output %s does not exist (%v)", item.ExpectedOutput.Path, serr)}
	case st.IsDir():
		return ItemVerdict{StatusSchemaInvalid, fmt.Sprintf(
			"expected_output %s is a directory, not a sidecar", item.ExpectedOutput.Path)}
	}

	if gerr := contentGate(sidecarPath, artifactPath); gerr != nil {
		return ItemVerdict{StatusSchemaInvalid, fmt.Sprintf(
			"%s failed the content gate: %v", item.ExpectedOutput.Path, gerr)}
	}

	missing, merr := missingPins(root, planID, item.ItemID, sidecarPath)
	if merr != nil {
		// The pin-set was captured but cannot be compared. Unmeasured must
		// not read as a clean bill of health (INV-013), and a pin whose
		// survival cannot be checked is exactly what pins-unrestorable names.
		return ItemVerdict{StatusPinsUnrestorable, fmt.Sprintf(
			"the pre-dispatch pin-set for %s could not be compared against the live sidecar: %v",
			item.ItemID, merr)}
	}
	if len(missing) > 0 {
		return ItemVerdict{StatusPinsUnrestorable, describeMissingPins(item, missing)}
	}

	return ItemVerdict{StatusOK, ""}
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
			"`bin/edikt phasea restore --plan` could not reinstate them. Governance "+
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
func contentGate(sidecarPath, parentPath string) error {
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
	return verifyAnchorsAgainstParent(sidecarPath, parentPath)
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
	// gate reporting on nothing (INV-013), which phasea-receipt.v1 also
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

	verdict := map[int]string{
		ExitVerifyOK:         "CONSUMED",
		ExitVerifyIncomplete: "INCOMPLETE",
		ExitVerifyFailed:     "FAILED (terminal)",
	}[exitCode]

	var b strings.Builder
	fmt.Fprintf(&b, "verify %s — %s: %d item(s) [%s]",
		planID, verdict, len(r.Items), strings.Join(statuses, ", "))
	for _, it := range r.Items {
		if it.Status == StatusOK {
			continue
		}
		fmt.Fprintf(&b, "\n  %s: %s (attempt %d) — %s", it.ItemID, it.Status, it.Attempts, details[it.ItemID])
	}
	switch exitCode {
	case ExitVerifyIncomplete:
		b.WriteString("\n  re-dispatch only the non-terminal items above, verbatim, then verify again")
	case ExitVerifyFailed:
		b.WriteString("\n  no retryable items remain — this plan_id is terminal; recovery requires a fresh plan " +
			"(a corpus or config change produces a new plan_id)")
	}
	return b.String()
}

// short renders a sha256 the way git renders an object id — enough to
// distinguish, short enough to read in a message that names two of them.
func short(sum string) string {
	if len(sum) <= 12 {
		return sum
	}
	return sum[:12]
}
