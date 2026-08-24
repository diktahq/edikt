package phasea

// plan_types.go — the on-disk shapes plan.go emits, mirrored field-for-field
// from templates/schemas/phasea-dispatch-plan.v1.schema.json and
// templates/schemas/phasea-journal.v1.schema.json (SPEC-013, Phase 1 of
// PLAN-v080-reliability-core). Those schemas are authoritative; these
// structs exist only to produce bytes that validate against them. Do not
// add a field here without adding it to the schema first — `additionalProperties:
// false` on every object node in both schemas means an extra field here
// would fail the round-trip test that validates emitted output against the
// schema.

// DispatchPlan is templates/schemas/phasea-dispatch-plan.v1.schema.json.
type DispatchPlan struct {
	SchemaVersion int         `json:"schema_version"`
	PlanID        string      `json:"plan_id"`
	CreatedFrom   CreatedFrom `json:"created_from"`
	Budget        Budget      `json:"budget"`
	Items         []PlanItem  `json:"items"`
}

// CreatedFrom is DispatchPlan.created_from.
type CreatedFrom struct {
	CorpusHash string `json:"corpus_hash"`
	ConfigHash string `json:"config_hash"`
}

// Budget is DispatchPlan.budget.
type Budget struct {
	Items          int    `json:"items"`
	Ceiling        int    `json:"ceiling"`
	ForceCeiling   int    `json:"force_ceiling"`
	AppliedCeiling string `json:"applied_ceiling"` // "ceiling" | "force_ceiling"
}

// PlanItem is one entry of DispatchPlan.items.
type PlanItem struct {
	ItemID         string         `json:"item_id"`
	ArtifactPath   string         `json:"artifact_path"`
	ArtifactSHA256 string         `json:"artifact_sha256"`
	Reason         string         `json:"reason"` // "missing" | "stale-quote-drift" | "stale-migration-preserved"
	Agent          string         `json:"agent"`  // always "sidecar-extractor"
	PromptVersion  string         `json:"prompt_version"`
	ExpectedOutput ExpectedOutput `json:"expected_output"`
}

// ExpectedOutput is PlanItem.expected_output.
type ExpectedOutput struct {
	Path               string `json:"path"`
	Schema             string `json:"schema"`               // always "gov-sidecar.v2"
	VerifyParentSHA256 bool   `json:"verify_parent_sha256"` // always true
}

// Reason enum values — mirrored from sidecar.Sidecar.IsStale's own
// categories (drift.go) and twophase.go's bootstrap branch, per SPEC-013's
// explicit instruction not to invent a second detector. There is
// deliberately no "schema-version" reason: a prompt_version bump is handled
// by the separate, explicitly-forced `gov reextract` path, never as an
// implicit Phase A staleness cause.
const (
	ReasonMissing                 = "missing"
	ReasonStaleQuoteDrift         = "stale-quote-drift"
	ReasonStaleMigrationPreserved = "stale-migration-preserved"
)

// AgentSidecarExtractor is the locked subagent type every plan item names.
const AgentSidecarExtractor = "sidecar-extractor"

// SchemaGovSidecarV2 is the schema every expected_output must validate
// against.
const SchemaGovSidecarV2 = "gov-sidecar.v2"

// SchemaVersionPlan is phasea-dispatch-plan.v1's schema_version constant.
const SchemaVersionPlan = 1

// AppliedCeilingCeiling / AppliedCeilingForce are Budget.applied_ceiling's
// two enum values.
const (
	AppliedCeilingCeiling = "ceiling"
	AppliedCeilingForce   = "force_ceiling"
)

// JournalRecord is one line of templates/schemas/phasea-journal.v1.schema.json.
type JournalRecord struct {
	SchemaVersion int      `json:"schema_version"`
	Event         string   `json:"event"` // "plan" | "verify" | "consume" | "refuse"
	PlanID        string   `json:"plan_id"`
	Items         int      `json:"items"`
	Failing       []string `json:"failing"`
	Cause         string   `json:"cause,omitempty"`
	TS            string   `json:"ts"`
}

// Journal event names.
const (
	JournalEventPlan    = "plan"
	JournalEventVerify  = "verify"
	JournalEventConsume = "consume"
	JournalEventRefuse  = "refuse"
)

// Journal refuse causes — phasea-journal.v1's `cause` enum was widened
// 2026-08-20 (SPEC-013 SR-006 / PRD-007 FR-018) from four values to nine, so
// that every plan-time refusal Plan() can hit has a representable cause and
// SR-006's "every refusal MUST append exactly one line" holds without
// exception. Plan-time causes (all seven, produced by plan.go):
//
//   - CauseCeilingExceeded      — stale count exceeds phasea.ceiling.
//   - CauseForceCeilingExceeded — stale count exceeds phasea.force_ceiling
//     on a --force run.
//   - CausePromptVersionUnknown — the extractor prompt_version could not be
//     resolved. Journaled with PlanIDUnresolved: this refusal happens
//     BEFORE any corpus work, so there is no corpus_hash to derive a real
//     plan_id from.
//   - CauseInvalidConfig — .edikt/config.yaml's phasea.* section failed to
//     load or contains a non-positive ceiling/max_attempts_per_item value.
//     Also journaled with PlanIDUnresolved — LoadConfig failed, so there is
//     no config to hash either.
//   - CauseCorpusLoadError — sidecar.Discover itself failed, or one or more
//     pairs failed to load/parse (workItem's loadErrs). Journaled with a
//     real, content-derived plan_id when any items are known (computed from
//     whatever DID load successfully); PlanIDUnresolved when Discover failed
//     outright and no items exist at all.
//   - CauseItemValidationFailed — one or more items failed buildItem's
//     idvalidate dispatch-boundary check (workItem's validationErrs).
//     Journaled with a real, content-derived plan_id from the items that DID
//     validate.
//   - CausePinCaptureFailed — D6's mandatory pre-dispatch pin-set capture
//     failed for one or more items. Journaled with the same real plan_id the
//     (never-written) plan file would have carried.
//
// Verify-time causes (verify.go, Phase 3):
//
//   - CauseUnknownPlanID
//   - CauseAlreadyConsumed
const (
	CauseCeilingExceeded      = "ceiling-exceeded"
	CauseForceCeilingExceeded = "force-ceiling-exceeded"
	CauseUnknownPlanID        = "unknown-plan-id"  // verify.go (Phase 3)
	CauseAlreadyConsumed      = "already-consumed" // verify.go (Phase 3)

	CausePromptVersionUnknown = "prompt-version-unknown"
	CauseCorpusLoadError      = "corpus-load-error"
	CauseItemValidationFailed = "item-validation-failed"
	CausePinCaptureFailed     = "pin-capture-failed"
	CauseInvalidConfig        = "invalid-config"
)

// PlanIDUnresolved is the plan_id journaled for a refusal that occurs BEFORE
// a real plan_id can be derived at all — invalid config (LoadConfig itself
// failed, so there is no Config to hash) or an unresolvable extractor
// prompt_version (Plan() deliberately refuses before touching the corpus in
// that case — see plan.go's package doc — so there is no corpus_hash
// either). Computing a real derivePlanID here would require doing the exact
// corpus/config work the refusal exists to skip.
//
// All-zero hex mirrors git's null-OID convention: unmistakably a sentinel,
// matches phasea-journal.v1's `^pap-[0-9a-f]{12}$` pattern, and can never
// collide with a real derivePlanID output (whose 12 hex characters are a
// sha256 truncation, astronomically unlikely to land on all zeros).
const PlanIDUnresolved = "pap-000000000000"
