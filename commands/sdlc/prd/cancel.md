---
name: edikt:sdlc:prd:cancel
description: "Cancel a PRD (work stopped before shipping)"
effort: low
argument-hint: "<PRD-NNN> [reason]"
allowed-tools:
  - Read
  - Edit
  - Bash
---

# edikt:sdlc:prd:cancel

Marks a PRD as `status: cancelled`. Use when work on a feature stopped before it shipped — e.g., the discovery showed the hypothesis was wrong, the team priorities shifted, or the feature was merged into a different PRD without formal supersession.

Difference from deprecate:
- **Cancel** — never shipped; work stopped before completion.
- **Deprecate** — was shipped or accepted, now obsolete.

Cancelled PRDs remain on disk as historical record.

## Arguments

- `<PRD-NNN>` — required. PRD identifier.
- `[reason]` — optional. Free-form reason text. Asked interactively if omitted.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve PRD

Parse `$ARGUMENTS`. Extract `PRD-\d+`. Remainder is the reason.

Read `.edikt/config.yaml` → `paths.prds`. Find the sidecar.

If no sidecar (v1 PRD): error "Cannot mutate v1 PRD status. Upgrade first with /edikt:sdlc:prd PRD-NNN."

If already `status: cancelled` or `status: deprecated`: error "PRD-NNN is already {status}."

If `status: shipped`: confirm with user — "PRD-NNN is shipped. Cancel a shipped PRD? Usually you want /edikt:sdlc:prd:deprecate. Continue? (y/N)".

### Step 2: Get Reason

If not provided:

```
Why is PRD-NNN being cancelled? (short description — recorded in revision_history)

>
```

Minimum 10 characters. Reject empty.

### Step 3: Mutate Sidecar

```bash
python3 <<'PYEOF' "$PRD_YAML" "$REASON" "$AUTHOR" "$NOW"
import sys, yaml
path, reason, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
with open(path) as f:
    data = yaml.safe_load(f)
data["status"] = "cancelled"
data["cancelled_at"] = now
data["cancelled_reason"] = reason
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "cancel",
    "note": f"Cancelled: {reason}",
})
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
PYEOF
```

### Step 4: Recompute _sync

```bash
MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_MD")
YAML_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_YAML")
```

Edit the sidecar `_sync:` block with the hashes + ISO8601 timestamp.

### Step 5: Output

```
✅ PRD-NNN cancelled

  Reason:  {reason}
  At:      {timestamp}

File kept as historical record.
Hidden from active views (/edikt:context, /edikt:status).

Linked SPECs ({count}) are not automatically touched:
  • SPEC-042  — implements PRD-NNN
  {if count > 0:}
  Consider: /edikt:sdlc:spec:cancel SPEC-042 (not yet implemented; spec-level
  cancel may be added in a future release — for now edit SPEC status manually).
```

## Design Notes

- **Cancel is a terminal state for incomplete work.** No `proposed → cancelled → accepted` path. To revive, start a new PRD (optionally with `supersedes: PRD-NNN` if the framing is close).
- **INV-003 compliance.** `python3 -c` with argv for all hashing and YAML mutation. Reason text is user-controlled and can contain any character — never interpolate into shell strings.
- **Parity with deprecate.** This command and `prd:deprecate` share structure. The difference is semantic — `cancelled_at` vs `deprecated_at` — so auditors can tell whether the work stopped mid-flight or was retired after shipping.
