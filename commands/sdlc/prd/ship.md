---
name: edikt:sdlc:prd:ship
description: "Mark PRD requirements as shipped (edit-in-place, ADR-024)"
effort: low
argument-hint: "<PRD-NNN> [FR-001 FR-002 ...]"
allowed-tools:
  - Read
  - Edit
  - Bash
---

# edikt:sdlc:prd:ship

Marks one or more PRD requirements as `status: shipped` in the sidecar. If all FRs are shipped, the top-level PRD `status:` flips to `shipped` too.

Per ADR-024, PRDs evolve via edit-in-place — shipping is a structural mutation, not a supersession. Git diff + the `revision_history:` log is the audit trail.

## Arguments

- `<PRD-NNN>` — required. PRD identifier.
- `[FR-001 FR-002 ...]` — optional. Specific FRs to ship. If omitted, ask interactively.

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve PRD

Read `.edikt/config.yaml` → `paths.prds`.

Parse `$ARGUMENTS`:
- First token matching `PRD-\d+` → the PRD identifier.
- Subsequent tokens matching `FR-\d+` → FR identifiers to ship.

Find the sidecar:
```bash
find {prds_dir}/ -name "${PRD_ID}-*.yaml" -type f
```

If not found: error "PRD-NNN not found (or is v1 — no sidecar to mutate)."

### Step 2: Determine FRs to Ship

If specific FR-NNN args provided: use them.

Otherwise, read the sidecar and list all FRs not yet `status: shipped`:

```
PRD-NNN has {n} non-shipped requirements:

  [1] FR-001 (proposed)    — "{text}"
  [2] FR-002 (accepted)    — "{text}"
  [3] FR-004 (proposed)    — "{text}"

Which to ship? (comma-separated numbers, "all", or "cancel")
>
```

### Step 3: Validate

For each FR:
- If it doesn't exist in the sidecar: error, skip.
- If it's already `shipped` or `deprecated`: warn, skip.
- Otherwise: add to the "to ship" list.

### Step 4: Mutate Sidecar

Use `python3` for YAML mutation (avoids shell-interpolation errors on odd field values):

```bash
python3 <<'PYEOF' "$PRD_YAML" "$FR_LIST" "$AUTHOR" "$NOW"
import sys, yaml
path, frs_raw, author, now = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
frs = frs_raw.split(",")
with open(path) as f:
    data = yaml.safe_load(f)
affected = []
for req in data.get("requirements", []):
    if req["id"] in frs and req.get("status") != "shipped":
        req["status"] = "shipped"
        req["shipped_at"] = now
        affected.append(req["id"])
# Revision history
data.setdefault("revision_history", []).append({
    "at": now, "author": author, "action": "ship",
    "note": f"Marked shipped: {', '.join(affected)}",
    "affected": affected,
})
# Top-level status flip if all shipped
all_shipped = all(r.get("status") == "shipped" for r in data.get("requirements", []))
if all_shipped and data.get("requirements"):
    data["status"] = "shipped"
# Clear _sync hashes — they'll be recomputed at Step 5
data.setdefault("_sync", {}).update({"md_hash": "", "yaml_hash": "", "synced_at": ""})
with open(path, "w") as f:
    yaml.safe_dump(data, f, sort_keys=False)
print(",".join(affected))
PYEOF
```

### Step 5: Recompute _sync Hashes

After mutation, recompute the `.md` hash (unchanged in this command, but recorded for consistency) and the `.yaml` hash (changed). Use python3 with argv per INV-003:

```bash
MD_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_MD")
YAML_HASH=$(python3 -c 'import hashlib,sys; print(hashlib.sha256(open(sys.argv[1],"rb").read()).hexdigest())' "$PRD_YAML")
```

Edit the sidecar `_sync:` block with these values and the current ISO8601 timestamp.

### Step 6: Output

```
✅ PRD-NNN: shipped FR-001, FR-002

  Top-level status: shipped  (all requirements now shipped)
  Revision history updated: ship @ {timestamp}
  Sync hashes recomputed.

Next:
  • Review the PRD:   /edikt:prd:review PRD-NNN
  • Drift check:      /edikt:sdlc:drift PRD-NNN
```

If not all FRs shipped, the top-level status stays `accepted` and the summary says:

```
  Top-level status: accepted  ({n}/{total} requirements shipped)
```

## Design Notes

- **No markdown mirroring here.** The `.md` "Requirements" section is a view of the sidecar. This command mutates the sidecar; the `.md` becomes drifted until the next `/edikt:sdlc:prd PRD-NNN` regenerates the view. `/edikt:doctor` surfaces the drift via `_sync.md_hash` comparison.
- **INV-003 compliance.** All JSON/hash emission goes through `python3` with argv. No shell JSON concatenation.
- **Idempotent.** Re-running ship on already-shipped FRs is a no-op (warned, not errored).
