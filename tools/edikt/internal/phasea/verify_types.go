package phasea

// verify_types.go — the on-disk shape verify.go emits, mirrored
// field-for-field from templates/schemas/phasea-receipt.v1.schema.json
// (SPEC-013, Phase 3 of PLAN-v080-reliability-core). That schema is
// authoritative; this struct exists only to produce bytes that validate
// against it. `additionalProperties: false` on every object node means an
// extra field here would fail the round-trip validation, so nothing is added
// here that is not in the schema first — which is also why the per-item
// diagnostic ("WHICH pinned field went missing", "WHICH anchor is wrong")
// travels in VerifyOutcome.Details, in memory, rather than in the receipt.

// SchemaVersionReceipt is phasea-receipt.v1's schema_version constant.
const SchemaVersionReceipt = 1

// Receipt is templates/schemas/phasea-receipt.v1.schema.json.
type Receipt struct {
	SchemaVersion int           `json:"schema_version"`
	PlanID        string        `json:"plan_id"`
	Items         []ReceiptItem `json:"items"`
	Consumed      bool          `json:"consumed"`
	VerifiedAt    string        `json:"verified_at"`
}

// ReceiptItem is one entry of Receipt.items.
type ReceiptItem struct {
	ItemID   string `json:"item_id"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts"`
}

// Per-item status values — phasea-receipt.v1's `status` enum, verbatim.
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

	// StatusOutputMissing — expected_output.path does not exist on disk.
	// Retryable: a re-dispatch can produce it.
	StatusOutputMissing = "output-missing"

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
)

// IsTerminalStatus reports whether a status is never re-evaluated on a later
// verify call for the same plan_id. Terminal items are carried forward from
// the prior receipt VERBATIM — same status, same attempts — which is what
// makes a FAILED plan stop consuming retry budget while still being reported
// in full on every call.
func IsTerminalStatus(status string) bool {
	switch status {
	case StatusOK, StatusFailedTerminal, StatusPinsUnrestorable:
		return true
	}
	return false
}
