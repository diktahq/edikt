package phasea

// verify_types.go — the on-disk shape verify.go emits, mirrored
// field-for-field from templates/schemas/phasea-receipt.v2.schema.json
// (SPEC-013, Phase 3 of PLAN-v080-reliability-core). That schema is
// authoritative; this struct exists only to produce bytes that validate
// against it. `additionalProperties: false` on every object node means an
// extra field here would fail the round-trip validation, so nothing is added
// here that is not in the schema first — which is also why the per-item
// diagnostic ("WHICH pinned field went missing", "WHICH anchor is wrong")
// travels in VerifyOutcome.Details, in memory, rather than in the receipt.

// SchemaVersionReceipt is phasea-receipt.v3's schema_version constant.
// Bumped 2 -> 3 alongside the plan schema to add extractor_model /
// extractor_contract per item.
// Bumped 1 → 2 by SPEC-013's Amendment of 2026-09-02: the `stalled` status
// is a contract change under additionalProperties:false.
const SchemaVersionReceipt = 3

// Receipt is templates/schemas/phasea-receipt.v2.schema.json.
type Receipt struct {
	SchemaVersion int           `json:"schema_version"`
	PlanID        string        `json:"plan_id"`
	Items         []ReceiptItem `json:"items"`
	Consumed      bool          `json:"consumed"`
	VerifiedAt    string        `json:"verified_at"`
}

// ReceiptItem is one entry of Receipt.items.
//
// Diagnostic, DefectAxis, DefectCount and Convergence are THE FIX. Before them
// the per-item diagnostic lived only in VerifyOutcome.Details, in memory, and
// died when the process exited — while the retry in commands/gov/compile.md
// reads the RECEIPT. A re-dispatch therefore resent the identical prompt with
// no knowledge that it had failed or why, which is an unseeded retry, not a
// flaky model. See defects.go for the measured cost of that.
type ReceiptItem struct {
	ItemID   string `json:"item_id"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`

	// Diagnostic is the human- and machine-actionable reason this item is not
	// ok. Empty exactly when Status is ok.
	Diagnostic string `json:"diagnostic,omitempty"`

	// DefectAxis names WHICH KIND of failure this is, so a retry can be
	// steered: "length" wants sentences split, "anchor" wants spans re-quoted,
	// "schema"/"parse" admit no mechanical correction. Collapsing length and
	// anchor into one schema-invalid bucket is what made "split four
	// sentences" indistinguishable from "re-extract everything".
	DefectAxis string `json:"defect_axis,omitempty"`

	// DefectCount is the number of defects on DefectAxis. It is the quantity
	// convergence is measured on, and is only comparable between attempts that
	// share an axis.
	DefectCount int `json:"defect_count,omitempty"`

	// AnchorsValid / AnchorsBroken are recorded on EVERY outcome, including a
	// pure length failure. The event-transport artifact had 83 valid anchors
	// and failed only on length; a receipt that omits that cannot tell a
	// four-sentence fix from a total re-extraction.
	AnchorsValid  int `json:"anchors_valid,omitempty"`
	AnchorsBroken int `json:"anchors_broken,omitempty"`

	// AnchorsWhitespaceOnly is the part of AnchorsBroken whose quote carries
	// the parent's words with reconstructed whitespace (#7). A copy fixes
	// those; the rest are spliced, misnumbered or invented, and do not.
	AnchorsWhitespaceOnly int `json:"anchors_whitespace_only,omitempty"`

	// NeedsReview counts the sidecar's needs_review: true entries, measured
	// on the file for every item verify classified (ok and content-gate
	// failures). Reports quote this, never the extractor's hand-back (#11).
	NeedsReview int `json:"needs_review,omitempty"`

	// Convergence compares this attempt against the previous one on the same
	// axis: first-attempt | improved | unchanged | diverged. A retry policy
	// that cannot see divergence keeps paying for it — the measured case went
	// 1 over-cap directive to 4 while spending its entire budget.
	Convergence string `json:"convergence,omitempty"`

	// CorrectionPrompt is the instruction the next dispatch MUST carry. It is
	// persisted rather than recomputed so that the retry reads one artifact —
	// the receipt — instead of re-deriving the diagnostic from a tree that may
	// have changed underneath it.
	CorrectionPrompt string `json:"correction_prompt,omitempty"`

	// ExtractorModel / ExtractorContract are set only when Status is ok —
	// the same values stamped onto the sidecar itself via
	// sidecar.InsertKeyInPlace, recorded here too so the receipt is a
	// complete, self-describing record of what this run produced, without
	// requiring a reader to also open the sidecar.
	ExtractorModel    string `json:"extractor_model,omitempty"`
	ExtractorContract string `json:"extractor_contract,omitempty"`
}

// Per-item status values — phasea-receipt.v2's `status` enum, verbatim.
//
// Three of these are TERMINAL and are never re-evaluated on a later verify
// call (see IsTerminalStatus): StatusFailedTerminal, StatusPinsUnrestorable,
// and StatusOK. The other four are retryable while the item still has
// attempts left under phasea.max_attempts_per_item.
const (
	// StatusOK — the expected output exists, parses, validates against
	// gov-sidecar.v2, its anchors quote the parent byte-exactly, the
	// artifact's sha256 recomputed AT VERIFY TIME still equals the plan's
	// pinned value, and every pinned field the plan captured pre-dispatch
	// is present on the live sidecar.
	StatusOK = "ok"

	// StatusLengthExceeded — the sidecar parses and its anchors may be
	// perfectly valid, but one or more directive/prohibition texts exceed
	// gov-sidecar.v2's 500-character cap. Split out of schema-invalid because
	// it has a KNOWN mechanical remedy the extractor already carries
	// (sidecar-extractor.md:742), so a correction pass can fix it without
	// re-extracting. Retryable.
	StatusLengthExceeded = "length-exceeded"

	// StatusAnchorInvalid — the sidecar parses and validates, but one or more
	// anchors do not quote the parent byte-exactly. Also split out of
	// schema-invalid: the remedy is to re-quote those spans, not to re-extract.
	// Retryable.
	StatusAnchorInvalid = "anchor-invalid"

	// StatusDiverged — attempt N produced MORE defects on the same axis than
	// attempt N-1. Terminal, and deliberately reached BEFORE the retry budget
	// is exhausted: a retry that is getting worse will not get better by being
	// run again, and the remaining budget buys nothing. On the measured
	// artifact this state alone saves ~140k tokens and ~1.7 hours.
	//
	// Distinct from failed-terminal, which means "budget spent". Divergence
	// means "budget deliberately NOT spent, because it was being wasted".
	StatusDiverged = "diverged"

	// StatusOutputMissing — no extraction output to judge: expected_output.path
	// does not exist on disk, or restore observed that the extractor wrote
	// nothing since the plan (the file on disk is the prior, byte-identical to
	// the pin or to what the previous restore left). Retryable: a re-dispatch
	// can produce it.
	StatusOutputMissing = "output-missing"

	// StatusStalled — the dispatcher reported the subagent ended without a
	// result (stream watchdog, kill, crash) AND there is no output: nothing on
	// disk, or only the prior, which restore observed the extractor never
	// rewrote. Retryable like output-missing, and distinct from it on purpose:
	// two failures with different remedies MUST NOT share a name (SPEC-013
	// SR-013). It is the classified case, assigned only from the caller's
	// report; output-missing stays the default. Never assigned when the
	// extractor wrote output — the output is the evidence, the report is not
	// (SR-013's falsifying observation).
	StatusStalled = "stalled"

	// StatusSchemaInvalid — the file exists but fails the content gate:
	// unparseable YAML, schema-invalid, or an anchor that does not quote the
	// parent at the range it records (ADR-056). Retryable.
	StatusSchemaInvalid = "schema-invalid"

	// StatusParentHashMismatch — the keystone check. artifact_path's on-disk
	// sha256 no longer equals the plan's pinned artifact_sha256, so whatever
	// the extractor read cannot be proven to be the planned version.
	StatusParentHashMismatch = "parent-hash-mismatch"

	// StatusPromptVersionMismatch — the installed extractor's resolved
	// prompt_version (ADR-055) no longer matches the plan item's pinned one.
	StatusPromptVersionMismatch = "prompt-version-mismatch"

	// StatusFailedTerminal — retry exhaustion. The item has been evaluated
	// attempts >= phasea.max_attempts_per_item times while still not ok.
	StatusFailedTerminal = "failed-terminal"

	// StatusPinsUnrestorable — a mandatory pinned field the plan's pre-dispatch
	// pin-set captured is not present on the live sidecar, and `phasea restore`
	// could not put it back.
	//
	// TERMINAL ON FIRST OBSERVATION, unlike StatusFailedTerminal's
	// retry-exhaustion. Re-dispatching identical fields (which the skill
	// contract requires — no reword, no annotation) yields an identical
	// extraction, so a retry would spend two more LLM dispatches to reach a
	// conclusion already known on attempt one. INV-015 is the other half of
	// the reason: a human's prior approval leaving the corpus is a permanent,
	// human-visible loss, and letting a retry budget stand in for it would
	// make the loss look like a transient failure that simply ran out of road.
	StatusPinsUnrestorable = "pins-unrestorable"

	// StatusLosslessLoss — the extractor's output dropped a body the item's
	// prior sidecar carried. Two comparisons reach it, both against the
	// plan's pre-dispatch pin snapshot, because the extractor overwrites the
	// sidecar and the snapshot is the only copy of what it replaced:
	//
	//   - a lifted item (plan reason stale-migration-preserved) whose output
	//     no longer states a directive the snapshot carried under
	//     migration_preserved (losslessVerdict);
	//   - any re-extraction of an existing sidecar (stale-quote-drift,
	//     requested, and a lifted item too) whose output dropped a prior
	//     directive or prohibition while the parent prose it was anchored to
	//     remains (survivalVerdict). A body whose prose is gone is
	//     source-removed and named, not lost.
	//
	// Retryable: the receipt's correction_prompt names each dropped body and
	// the parent range it was grounded in. --on-loss=accept records the loss
	// in the diagnostic and lets the item pass.
	StatusLosslessLoss = "lossless-loss"

	// StatusLosslessUnmeasured — a lifted item whose snapshot carries no
	// usable baseline (no migration_preserved directives, or the snapshot is
	// unreadable): the lossless check ran and had nothing to compare. Not a
	// pass (INV-013); --on-loss=accept records it and lets the item pass. // edikt-guard:allow
	StatusLosslessUnmeasured = "lossless-unmeasured"
)

// IsTerminalStatus reports whether a status is never re-evaluated on a later
// verify call for the same plan_id. Terminal items are carried forward from
// the prior receipt VERBATIM — same status, same attempts — which is what
// makes a FAILED plan stop consuming retry budget while still being reported
// in full on every call.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusOK, StatusFailedTerminal, StatusPinsUnrestorable, StatusDiverged, StatusParentHashMismatch:
		// StatusDiverged is terminal for the same reason the others are: the
		// item must not be re-dispatched against this plan. It differs only in
		// WHY — the budget was deliberately left unspent rather than consumed.
		// Omitting it here would make the divergence stop decorative: the
		// receipt would say "diverged" while the retry loop dispatched it
		// again, which is the exact shape of a detector that never fires.
		//
		// StatusParentHashMismatch (D119a): the plan pins ArtifactSHA256 at
		// plan time and evaluateItem re-hashes the LIVE artifact on every
		// verify call — the pin never moves and the artifact does not
		// revert, so no re-dispatch against THIS plan can ever clear a
		// mismatch. Classifying it retryable made the INCOMPLETE retry
		// instruction a dead end; it is terminal in fact, and the remedy is
		// a fresh plan; not another attempt against this one.
		return true
	}
	return false
}

// Convergence values — how attempt N compares to attempt N-1 on the SAME
// defect axis.
//
// This exists because the retry policy assumed attempt N+1 is more likely to
// succeed than attempt N. For an UNSEEDED retry that assumption is simply
// false: the same prompt against the same artifact draws from the same
// distribution, so the next sample is as likely to be worse. Measured, it was —
// 1 over-cap directive became 4. Nothing was watching, so the budget kept
// paying.
const (
	// ConvergenceFirstAttempt — no prior attempt to compare against.
	ConvergenceFirstAttempt = "first-attempt"

	// ConvergenceImproved — fewer defects than the previous attempt. This is
	// what a seeded correction pass is expected to produce, and it is the
	// acceptance signal for the fix.
	ConvergenceImproved = "improved"

	// ConvergenceUnchanged — same defect count. Not divergence, but not
	// progress either; the budget still drains.
	ConvergenceUnchanged = "unchanged"

	// ConvergenceDiverged — MORE defects than the previous attempt. Triggers
	// StatusDiverged and stops the run.
	ConvergenceDiverged = "diverged"

	// ConvergenceUnknown — the two attempts failed on DIFFERENT axes, or this
	// outcome had no sidecar to measure. A length count and an anchor count
	// are different quantities and their difference is meaningless, so this
	// says so rather than manufacturing a comparison. Not treated as
	// divergence: an unmeasurable comparison is not evidence of getting worse —
	// absence is not a verdict in either direction.
	ConvergenceUnknown = "unknown"
)
