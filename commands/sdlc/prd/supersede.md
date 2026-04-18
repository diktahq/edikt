---
name: edikt:sdlc:prd:supersede
description: "Supersede a PRD with a new one (≥50% scope change, per ADR-024)"
effort: medium
argument-hint: "<PRD-NNN-to-supersede>"
allowed-tools:
  - Read
  - Write
  - Edit
  - Bash
  - Glob
---

# edikt:sdlc:prd:supersede

Supersede is reserved for ≥50% scope rewrites or problem-framing shifts (ADR-024). For routine FR changes, use `/edikt:sdlc:prd PRD-NNN` continuation or transition commands instead.

This command:
1. Confirms the user understands this is a heavy transition.
2. Runs `/edikt:sdlc:prd` to author a new PRD, pre-populated from the old.
3. Sets `supersedes:` on the new PRD and `superseded_by:` on the old.
4. Sets old PRD `status: superseded`.

## Arguments

- `<PRD-NNN-to-supersede>` — required. The PRD being replaced.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve and Validate

Parse `$ARGUMENTS`. Require `PRD-\d+`.

Read `.edikt/config.yaml` → `paths.prds`.

Find and load the old PRD's `.md` and `.yaml`.

If no `.yaml` sidecar: error "Cannot supersede a v1 PRD. Upgrade with /edikt:sdlc:prd PRD-NNN first, then supersede."

If the old PRD is already `status: superseded` or `status: cancelled`: error "PRD-NNN is already {status}; cannot supersede."

### Step 2: Confirm Heavy Transition

Per ADR-024, supersession is reserved for ≥50% scope rewrites.

```
⚠️  Supersede is a heavy transition (per ADR-024).

It creates a new PRD with:
  • supersedes: PRD-NNN (the old one, marked status: superseded)
  • A fresh forcing-questions interview
  • An independent FR/AC numbering starting at FR-001

For routine FR changes, prefer:
  • /edikt:sdlc:prd PRD-NNN       — continuation (add/revise FRs)
  • /edikt:sdlc:prd:ship FR-NNN   — mark shipped
  • /edikt:sdlc:prd:deprecate     — mark a PRD deprecated without a replacement

Continue with supersession? (y/N)
```

If not `y`, exit 0 without mutations.

### Step 3: Delegate to /edikt:sdlc:prd

Invoke the PRD authoring flow (`/edikt:sdlc:prd`), pre-populating from the old PRD:

Seed data passed to the new authoring:
- `title` — ask user if they want to keep the old title or change it ("PRD-NNN had title '<title>'. Change it?")
- `problem_statement` — seed from old PRD; user can edit
- `user_archetypes` — seed from old
- `solution_references` — seed from old (user prunes / adds)
- `forcing_questions` — ask all five fresh (do NOT seed from old; supersession means the answers may have changed)

The new PRD gets its own `id: PRD-MMM` from the live-injected next number.

Once the new PRD is authored and written, proceed.

### Step 4: Link Supersession

**On the new PRD's sidecar (PRD-MMM):**

```yaml
supersedes: PRD-NNN
revision_history:
  - at: {now}
    author: {git user}
    action: supersede
    note: "Supersedes PRD-NNN"
```

**On the old PRD's sidecar (PRD-NNN):**

```yaml
status: superseded
superseded_by: PRD-MMM
revision_history:
  - at: {now}
    author: {git user}
    action: supersede
    note: "Superseded by PRD-MMM"
```

Recompute `_sync` hashes on both sidecars (python3 + argv, INV-003).

### Step 5: Output

```
✅ Supersession complete

  Old:  PRD-NNN  (status: superseded → PRD-MMM)
  New:  PRD-MMM  (supersedes: PRD-NNN)

Both sidecars updated. Revision history logged on both.

Next:
  • Review new PRD:  /edikt:prd:review PRD-MMM
  • Write spec:      /edikt:sdlc:spec PRD-MMM
```

## Design Notes

- **Why fresh forcing questions.** A supersession implies the problem framing or scope has materially shifted. Re-asking the five questions exposes whether the shift is real or whether this should have been a continuation edit.
- **Old PRD stays on disk.** `status: superseded` hides the old PRD from active views in `/edikt:context` and `/edikt:status`, but the file remains as historical context. Git history is the deeper audit trail.
- **SPECs linked to the old PRD are not auto-migrated.** `source_specs:` on the old PRD still references them. The user decides whether to update each SPEC to `implements: PRD-MMM` manually — this is a judgment call, not a structural mutation edikt should automate.
