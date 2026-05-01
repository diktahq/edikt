---
name: prd:review
description: "Re-run the PRD evaluator rubric on an existing PRD and report score + gaps + drift + broken refs"
effort: low
argument-hint: "<PRD-NNN>"
allowed-tools:
  - Read
  - Grep
  - Glob
  - Bash
---

# edikt:prd:review

Re-scores an existing PRD against the rubric at `.edikt/rubrics/prd.md` and reports:
- Rubric score (rigor-calibrated threshold)
- Gaps — which rubric items fail
- Sidecar drift — `.md` edited after last sync
- Broken refs — linked INVs or SPECs that don't exist
- Unstarted FRs — proposed FRs never picked up by a SPEC

Closes the audit gap where every other governance artifact type (ADR, INV, guideline) has a review command except PRDs.

## Arguments

- `<PRD-NNN>` — required. Identifier of the PRD to review.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve Paths

Read `.edikt/config.yaml`:
- PRDs: `paths.prds` (default: `docs/product/prds`)
- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)
- Specs: `paths.specs` (default: `docs/product/specs`)
- Rubric: `.edikt/rubrics/prd.md`

### Step 2: Resolve PRD

If `$ARGUMENTS` is empty or doesn't match `PRD-\d+`:
```
Usage: /edikt:prd:review PRD-NNN
```
And stop.

Find the PRD files:
```bash
PRD_ID="$ARGUMENTS"
find {prds_dir}/ -name "${PRD_ID}-*.md" -type f
find {prds_dir}/ -name "${PRD_ID}-*.yaml" -type f
```

If `.md` missing: error "PRD-NNN not found at {prds_dir}/".

If `.yaml` missing (v1 PRD):
```
⚠ PRD-NNN is v1 shape (no .yaml sidecar). Limited review available.
  To upgrade: re-author with /edikt:sdlc:prd PRD-NNN.
```
Continue with a reduced review (rubric score + broken refs only; skip drift and FR coverage checks).

### Step 3: Load Rubric

Check `.edikt/rubrics/prd.md`. If absent, auto-create with the default rubric from `/edikt:sdlc:prd` Step 7a. Proceed.

### Step 4: Run Four Checks

Run all four independently, then compile into a single report.

#### Check 1 — Rubric Score

Read the PRD `.md` + `.yaml`. Apply each rubric item. Score 1 if met, 0 if not.

Rigor threshold (read from sidecar `rigor:`):
- solo: 7/10
- team: 8/10
- platform: 9/10

Record per-item pass/fail for the Gaps section of the report.

#### Check 2 — Sidecar Drift (v2 only)

Compute SHA-256 of the `.md` file:
```bash
CURRENT_MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_MD")
```

Read `_sync.md_hash` from the sidecar. If mismatch (and `_sync.md_hash` is non-empty):
- Record drift. Note the `_sync.synced_at` timestamp.
- This is informational — not an error.

#### Check 3 — Broken Refs

For each `protections[*].ref` that matches `INV-NNN`:
- Verify the file exists in `{invariants_dir}/`.
- Missing → record as broken ref.

For each identifier in `source_specs[*]`:
- Verify a directory matching `SPEC-NNN-*` exists in `{specs_dir}/`.
- Missing → record as broken ref.

For each `solution_references[*].path_or_url`:
- If starts with `/` (absolute local path): verify exists.
- If matches `figma.com` URL: skip (network check is opt-in only).
- Otherwise: skip silently.

#### Check 4 — Unstarted FRs (v2 only)

For each requirement in `requirements[]`:
- If `status: proposed` AND `source_specs:` in parent is empty → flag as "unstarted: proposed FR with no SPEC".
- If `status: proposed` AND any SPEC in `source_specs:` covers this FR (via `source_prd_coverage.covered`) → NOT unstarted (spec exists, just not yet shipped).

### Step 5: Emit Report

```
PRD REVIEW — PRD-NNN

  Title:          {title}
  Status:         {status}
  Rigor:          {rigor}
  Version:        {v1 | v2}

  Rubric score:   {score}/{total} ({rigor} threshold: {threshold})
                  {✓ PASS | ✗ below threshold}

  {if gaps:}
  Gaps:
    • {rubric item that failed}
    • {...}

  {if drift detected:}
  ⚠ Sidecar drift:
    .md edited since last sync ({synced_at}).
    To update hashes without reopening the PRD: /edikt:sdlc:prd:resync PRD-NNN

  {if broken refs:}
  ⚠ Broken references ({count}):
    • INV-999 — not found in {invariants_dir}/
    • SPEC-042 — not found in {specs_dir}/

  {if unstarted FRs:}
  ⚠ Unstarted FRs ({count}):
    • FR-003 — proposed, no SPEC covers it yet
    • FR-005 — proposed, no SPEC covers it yet

  {if all clean:}
  ✓ All checks pass.

Next:
  • Revise PRD:  /edikt:sdlc:prd PRD-NNN
  • Ship FRs:    /edikt:sdlc:prd PRD-NNN ship FR-NNN
  • Write spec:  /edikt:sdlc:spec PRD-NNN
```

### Step 6: Append Revision History

Append to the PRD sidecar `revision_history:` (v2 only):
```yaml
- at: {ISO8601 now}
  author: {git user}
  action: review
  note: "Rubric score: {score}/{total} ({PASS | fail}); {gap_count} gaps; {ref_count} broken refs"
```

Recompute `_sync` hashes (SHA-256 via python3 argv — INV-003 compliant).

## Design Notes

- **Review is read-heavy, mutation-light.** Only mutation is appending a review record to `revision_history:`. This preserves the reviewer intent in the audit trail without touching FRs or ACs.
- **No evaluator loop here.** This command is a scoring pass, not an authoring pass. Users run it to get a current score; iteration happens in `/edikt:sdlc:prd`.
- **v1 PRDs get a limited review.** Drift, back-references, and FR coverage all require sidecar data. The rubric + broken-ref checks still work from the `.md` alone.
