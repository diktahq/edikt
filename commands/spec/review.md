---
name: edikt:spec:review
description: "Re-run the SPEC evaluator rubric and verify PRD coverage + AC pass-through"
effort: low
argument-hint: "<SPEC-NNN>"
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# edikt:spec:review

Re-scores an existing SPEC against the rubric at `.edikt/rubrics/spec.md` and validates:
- Rubric score (rigor inherited from linked PRD)
- FR coverage completeness (every PRD FR-NNN is covered or explicitly deferred)
- AC pass-through integrity (every PRD AC-NNN-M appears in the SPEC unchanged)
- Broken refs + drift (same pattern as `/edikt:prd:review`)

## Arguments

- `<SPEC-NNN>` — required. Identifier of the SPEC to review.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve Paths

Read `.edikt/config.yaml`:
- Specs: `paths.specs` (default: `docs/product/specs`)
- PRDs: `paths.prds` (default: `docs/product/prds`)
- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)
- Rubric: `.edikt/rubrics/spec.md`

### Step 2: Resolve SPEC

If `$ARGUMENTS` is empty or doesn't match `SPEC-\d+`:
```
Usage: /edikt:spec:review SPEC-NNN
```
And stop.

Find SPEC dir:
```bash
SPEC_ID="$ARGUMENTS"
find {specs_dir}/ -maxdepth 1 -type d -name "${SPEC_ID}-*"
```

Read `spec.md` and (if present) `spec.yaml` or the spec's YAML frontmatter.

### Step 3: Resolve Linked PRD

From the SPEC frontmatter, read `implements:` (the linked PRD identifier).

Find the PRD:
```bash
PRD_ID={from implements field}
find {prds_dir}/ -name "${PRD_ID}-*.md" -type f
find {prds_dir}/ -name "${PRD_ID}-*.yaml" -type f
```

If PRD `.md` missing: error "Linked PRD {PRD_ID} not found. SPEC cannot be reviewed without source PRD."

If PRD `.yaml` missing (v1 linked PRD):
```
⚠ SPEC-NNN is linked to a v1 PRD ({PRD_ID}). Coverage and AC pass-through checks
  are not applicable (v1 has no sidecar). Running rubric + broken-ref checks only.
```

### Step 4: Load Rubric

Check `.edikt/rubrics/spec.md`. If absent, auto-create with default rubric:

```markdown
# SPEC Evaluator Rubric

Score each item 0 (missing/weak) or 1 (strong). Threshold inherits from linked PRD's rigor:
  solo: 7/10   team: 8/10   platform: 9/10

## Rubric

- [ ] Summary is one paragraph and names the approach
- [ ] Context explains why this SPEC exists now (not just what)
- [ ] Proposed Design references specific files/modules
- [ ] Components section names file paths (not "somewhere in api/")
- [ ] Non-Goals section is non-empty
- [ ] At least two Alternatives Considered with rejection reasons
- [ ] Risks table has Mitigation + Rollback columns populated
- [ ] Acceptance Criteria use AC-NNN format (inherit from PRD) + optional SAC-NNN for spec-added
- [ ] Testing Strategy names what is hard to test and why
- [ ] No NEEDS CLARIFICATION or TBD blocks remain

_Users can edit this rubric per ADR-005 template overrides._
```

### Step 5: Run Five Checks

Run all five independently.

#### Check 1 — Rubric Score

Walk the rubric. Score per item. Compute threshold from linked PRD's `rigor`.

#### Check 2 — FR Coverage (v2 PRD only)

Read `source_prd_coverage:` from the SPEC (sidecar or frontmatter).

For each FR in the linked PRD's `requirements:`:
- Is it in `source_prd_coverage.covered[]`? → ✓
- Is it in `source_prd_coverage.deferred[]` with a non-empty rationale? → ✓
- Is it in `source_prd_coverage.uncovered[]`? → ✗ flag it
- Not mentioned at all? → ✗ flag as "missing coverage entry"

#### Check 3 — AC Pass-Through (v2 PRD only)

Read `acceptance_criteria:` from the PRD sidecar. For each AC-NNN-M:
- The SPEC's `acceptance_criteria` must contain the same `id`.
- The `given`, `when`, `then` text must match exactly (byte-equal).
- Any modification is a pass-through violation — flag it.

SAC-NNN entries (spec-added) are ignored in this check — they are additions, not modifications.

#### Check 4 — Broken Refs

For each `references.adrs[]`: verify ADR file exists in `{decisions_dir}/`.
For each `references.invariants[]`: verify INV file exists in `{invariants_dir}/`.
For each `implements:`: verify PRD exists (already resolved in Step 3).

#### Check 5 — Drift (v2 SPEC only)

If the spec has a `_sync.md_hash` in its sidecar:
- Compute current SHA-256 of `spec.md`.
- If mismatch → record drift.

### Step 6: Emit Report

```
SPEC REVIEW — SPEC-NNN

  Title:          {title}
  Implements:     {PRD-NNN} ({PRD version: v1 | v2})
  Status:         {status}
  Rigor (from PRD): {rigor}

  Rubric score:   {score}/{total} ({threshold})
                  {✓ PASS | ✗ below threshold}

  {if v2 PRD:}
  FR coverage:    {covered}/{total} covered, {deferred} deferred, {uncovered} uncovered
    {if uncovered:}
    ⚠ Uncovered FRs: FR-003, FR-005
    {if missing_entries:}
    ⚠ Missing coverage entries: FR-007 (not in covered/deferred/uncovered)

  AC pass-through: {passed}/{total} unchanged
    {if violations:}
    ⚠ Modified ACs:
      • AC-001-1: given text changed from "…" to "…"

  {if gaps:}
  Rubric gaps:
    • {rubric item that failed}
    • {...}

  {if broken refs:}
  ⚠ Broken references ({count}):
    • ADR-042 — not found in {decisions_dir}/
    • INV-999 — not found in {invariants_dir}/

  {if drift detected:}
  ⚠ Sidecar drift:
    spec.md edited since last sync ({synced_at}).

  {if all clean:}
  ✓ All checks pass.

Next:
  • Revise SPEC:         /edikt:sdlc:spec PRD-NNN
  • Review linked PRD:   /edikt:prd:review PRD-NNN
  • Check drift:         /edikt:sdlc:drift SPEC-NNN
```

### Step 7: Append Revision History

If the SPEC has a sidecar YAML (v2), append a review record to `revision_history:`:
```yaml
- at: {ISO8601 now}
  author: {git user}
  action: review
  note: "Rubric: {score}/{total}; coverage: {covered}/{total}; AC pass-through: {passed}/{total}"
```

Recompute `_sync` hashes (SHA-256 via python3 argv — INV-003 compliant).

## Design Notes

- **FR coverage is the flagship check.** A SPEC that passes the rubric but has uncovered PRD FRs is silently shipping the wrong scope. Coverage is the only structural link between PRD and SPEC that this command can verify.
- **AC pass-through is strict.** Any rewording of PRD ACs — even to clarify — is a violation. If an AC needs clarification, revise the PRD and re-run the SPEC. This keeps the AC chain intact for the evaluator at phase-end time.
- **v1 PRDs limit what we can check.** Without the sidecar, coverage and AC pass-through are not verifiable. The rubric + broken-ref checks still run.
