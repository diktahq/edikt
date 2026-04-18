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

### Step 2: Gate on ≥50% Threshold (ADR-024)

Per ADR-024, supersession is reserved for changes that rewrite ≥50% of the PRD's scope or shift the problem framing. Routine FR refinements MUST use edit-in-place transition commands instead. This step enforces that gate via four binary questions — every "no" is a signal the user should NOT supersede.

First, show the user what would be lost and what the alternatives are:

```
⚠️  Supersede is a heavy transition (ADR-024).

You are about to:
  • Mark PRD-NNN as status: superseded (hidden from active views)
  • Create a new PRD-MMM from scratch with its own FR/AC numbering
  • Break the stable ID chain — existing SPECs linked to PRD-NNN FR-001
    will no longer trace to the new PRD-MMM FR-001

ADR-024 reserves this for rewrites ≥50% scope OR problem-framing shifts.
For routine changes, prefer:
  • /edikt:sdlc:prd PRD-NNN       — add/revise/refine FRs in place
  • /edikt:sdlc:prd:ship FR-NNN   — mark individual FRs shipped
  • /edikt:sdlc:prd:deprecate     — retire the PRD without replacement
```

Then ask four gating questions, one at a time. The user must answer all four; any three "no" answers aborts the supersession.

```
Gate 1/4 — Has the PROBLEM framing changed since PRD-NNN was authored?
(Not the solution — the problem statement itself.)
(y/n)
```

```
Gate 2/4 — Would ≥50% of the requirements (FR-NNN) be rewritten or removed?
(If you're adding new FRs but keeping the old ones, answer n.)
(y/n)
```

```
Gate 3/4 — Are the Protections or Goals so different that ACs from the
old PRD would no longer apply?
(y/n)
```

```
Gate 4/4 — Have you already tried continuation via /edikt:sdlc:prd PRD-NNN
and concluded it is insufficient?
(y/n)
```

Decision table:

| Yes count | Action |
|-----------|--------|
| 4 | Supersede clearly warranted — proceed to Step 3 |
| 3 | Supersede likely warranted — confirm one more time: "Three of four gates passed. Proceed? (y/N)" |
| 2 | Ambiguous — show message below, offer an alternative path, require explicit override |
| 0-1 | Supersede not warranted — abort with guidance |

For ambiguous (2 yes) or insufficient (0-1 yes) cases, output:

```
⛔ Supersede gate not cleared ({n}/4 — threshold: 3/4 minimum)

Based on your answers, the right next step is:
  {if gate 2 was no:}  /edikt:sdlc:prd PRD-NNN   — continuation adds new FRs without supersession
  {if gate 3 was no:}  /edikt:sdlc:prd PRD-NNN   — revise ACs in place; stable IDs preserved
  {if gate 1 was no:}  /edikt:sdlc:prd:deprecate — problem no longer relevant, no replacement needed

To override (you are certain this is a ≥50% rewrite):
  /edikt:sdlc:prd:supersede PRD-NNN --force
```

If the user invoked with `--force`, skip the gate but log the override in the new PRD's revision_history: `note: "Supersede gate overridden with --force"`.

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

Mutate both sidecars in a single python3 heredoc (INV-003 compliant, argv for all untrusted values):

```bash
OLD_YAML="{path}/PRD-NNN-slug.yaml"
NEW_YAML="{path}/PRD-MMM-slug.yaml"
OLD_ID="PRD-NNN"
NEW_ID="PRD-MMM"
AUTHOR=$(git config user.name 2>/dev/null || echo "unknown")
NOW=$(date -u +%Y-%m-%dT%H:%M:%SZ)
FORCED="${FORCED:-0}"  # 1 if --force was passed

python3 <<'PYEOF' "$OLD_YAML" "$NEW_YAML" "$OLD_ID" "$NEW_ID" "$AUTHOR" "$NOW" "$FORCED"
import sys, yaml
old_path, new_path, old_id, new_id, author, now, forced = sys.argv[1:]

# Old sidecar — mark superseded
with open(old_path) as f:
    old = yaml.safe_load(f) or {}
old["status"] = "superseded"
old["superseded_by"] = new_id
old.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "supersede",
    "note": f"Superseded by {new_id}",
})
old.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(old_path, "w") as f:
    yaml.safe_dump(old, f, sort_keys=False)

# New sidecar — record supersedes link (and --force override if applicable)
with open(new_path) as f:
    new = yaml.safe_load(f) or {}
new["supersedes"] = old_id
note = f"Supersedes {old_id}"
if forced == "1":
    note += " (supersede gate overridden with --force)"
new.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "supersede",
    "note": note,
})
new.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(new_path, "w") as f:
    yaml.safe_dump(new, f, sort_keys=False)
PYEOF
```

After mutation, recompute `_sync` hashes on both sidecars:

```bash
for Y in "$OLD_YAML" "$NEW_YAML"; do
  M="${Y%.yaml}.md"
  MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$M")
  YAML_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$Y")
  # Edit $Y's _sync block with MD_HASH, YAML_HASH, and current ISO8601 timestamp
done
```

Use the Edit tool on each sidecar's `_sync:` block to insert the computed hashes.

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
