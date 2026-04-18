---
name: edikt:sdlc:prd:deprecate
description: "Mark a PRD as deprecated (feature abandoned or no longer relevant)"
effort: low
argument-hint: "<PRD-NNN> [reason]"
allowed-tools:
  - Read
  - Edit
  - Bash
---

# edikt:sdlc:prd:deprecate

Marks a PRD as `status: deprecated`. Use when a feature is abandoned, no longer strategically relevant, or replaced by a different approach without a direct supersession (otherwise use `/edikt:sdlc:prd:supersede`).

Deprecated PRDs remain on disk as historical record. They are hidden from active views in `/edikt:context` and `/edikt:status`, and the evaluator skips them.

Difference from cancel:
- **Deprecate** — was shipped or accepted, now obsolete (e.g., replaced by a different feature direction).
- **Cancel** — never shipped; work stopped before completion.

## Arguments

- `<PRD-NNN>` — required. The PRD to deprecate.
- `[reason]` — optional. Free-form reason text. If omitted, asked interactively.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve PRD

Parse `$ARGUMENTS`. Extract `PRD-\d+` as identifier. Remainder (if any) is the reason.

Read `.edikt/config.yaml` → `paths.prds`. Find the sidecar.

If no sidecar (v1 PRD): error "Cannot mutate v1 PRD status. Upgrade first with /edikt:sdlc:prd PRD-NNN."

If already `status: deprecated` or `status: cancelled`: error "PRD-NNN is already {status}."

### Step 2: Get Reason

If no reason given:

```
Why is PRD-NNN being deprecated? (short description — this is recorded
in revision_history and shown in /edikt:status)

>
```

Minimum 10 characters. Reject empty strings.

### Step 3: Mutate Sidecar

```bash
python3 <<'PYEOF' "$PRD_YAML" "$REASON" "$AUTHOR" "$NOW"
import sys, yaml
path, reason, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
with open(path) as f:
    data = yaml.safe_load(f)
data["status"] = "deprecated"
data["deprecated_at"] = now
data["deprecated_reason"] = reason
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "deprecate",
    "note": f"Deprecated: {reason}",
})
# Clear _sync — recomputed by caller
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
PYEOF
```

### Step 4: Recompute _sync

INV-003-compliant hash computation:

```bash
MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_MD")
YAML_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_YAML")
```

Edit the sidecar `_sync:` block.

### Step 5: Output

```
✅ PRD-NNN deprecated

  Reason:  {reason}
  At:      {timestamp}

Revision history updated. File kept as historical record.
Linked SPECs ({count}) are not automatically updated — review each:
  • SPEC-042  — implements PRD-NNN
  • SPEC-043  — implements PRD-NNN
```

## Design Notes

- **INV-003 compliance.** `python3` with argv for hashing and YAML mutation. No shell interpolation of untrusted values (reason text can contain any characters).
- **No cascade.** SPECs that `implements: PRD-NNN` are not touched. The user reviews each and decides (deprecate the SPEC, leave as-is, or pivot).
- **Reversible via edit.** A deprecated PRD can be reopened by manually editing the sidecar's `status:` back to `accepted` — but this is discouraged. Prefer a new PRD that supersedes.
