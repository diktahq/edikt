---
name: edikt:invariant:compile
description: "Generate or regenerate directive sentinel blocks for one invariant or all"
effort: normal
argument-hint: "[INV-NNN] — omit to process all active invariants missing a sentinel"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
---

# edikt:invariant:compile

Generate or regenerate directive sentinel blocks (`[edikt:directives:start/end]: #`) for invariants. Sentinels are what `/edikt:gov:compile` reads — without them, compile falls back to extraction, which is slower and less accurate.

## Arguments

- `$ARGUMENTS` — optional invariant ID (e.g., `INV-002`). If no argument, processes all active invariants.

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve:
- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)

### 2. Determine Scope

**With `$ARGUMENTS`** — locate the invariant file matching the given ID (e.g., `INV-002`). Search for a file whose name contains the ID prefix. If not found:
```
Invariant not found: {id}
Run: ls {invariants_path}/*.md to see available invariants.
```

**Without `$ARGUMENTS`** — glob all `*.md` files in `{invariants_path}`. Filter to those with `status: active` (check frontmatter `status:` field). Include files with no `status:` field for backwards compatibility.

Skip any file with `status: revoked`.

If no active invariants found:
```
No active invariants found in {invariants_path}.
```

### 3. Process Each Invariant

For each invariant in scope:

#### Check for Existing Sentinel

Look for `[edikt:directives:start]: #` in the file.

**If sentinel exists — check for staleness:**

1. Compute the MD5 of the human-readable content above `[edikt:directives:start]: #`:
   ```bash
   content=$(sed '/\[edikt:directives:start\]: #/q' {file} | head -n -1)
   echo -n "$content" | md5sum | awk '{print $1}'
   ```
2. Read the `content_hash:` field stored inside the sentinel block.
3. If hashes match: mark as **current** — skip.
4. If hashes differ: mark as **stale** — offer to regenerate:
   ```
   ⚠ Stale sentinel: {file}
     Content has changed since the sentinel was generated.
     Regenerate? (y/n)
   ```
   If yes: proceed to sentinel generation. If no: skip.

**If no sentinel exists:** proceed directly to sentinel generation.

#### Generate Sentinel

Read the `## Rule` section. Invariants are already constraint-shaped — translate the Rule statement into directive format directly:

Rules for generating directives:
1. The Rule section already uses MUST or NEVER language — preserve it exactly.
2. Keep the one-clause reason (after the dash). Do not strip rationale from the Rule statement itself.
3. One directive per rule statement.
4. Each directive ends with `(ref: {INV-ID})`.
5. If the Rule section contains prose rather than directive-format statements, rewrite into MUST/NEVER format.

Derive `paths:` from the `scope:` frontmatter field, or from the `## Scope` section if present. Default to `"**/*"` for universal invariants.

Derive `scope:` from the invariant's domain — invariants always apply to `[planning, design, review, implementation]` because they are non-negotiable across all activities.

**Validate cross-references:** for every generated directive that references an ADR or other INV, confirm that identifier exists in the source document. Strip fabricated references.

Compute `content_hash:` as MD5 of all human-readable content above the sentinel insertion point.

#### Write Sentinel

Insert (or replace) the sentinel block in the invariant file, inside the `## Directives` section if present, or append before the closing `---` line:

```markdown
[edikt:directives:start]: #
content_hash: {md5}
paths:
  - {glob patterns}
scope:
  - planning
  - design
  - review
  - implementation
directives:
  - {directive} (ref: {INV-ID})
  - {directive} (ref: {INV-ID})
[edikt:directives:end]: #
```

### 4. Report Results

```
✅ Invariant sentinels updated: {n} generated, {m} already current

  Generated:
    {file} — {k} directives
    ...

  Current (skipped):
    {file}
    ...

  {If stale rejections}:
  Skipped (user declined regeneration):
    {file}

Next: Run /edikt:gov:compile to compile updated sentinels into governance files.
```

---

REMEMBER: Invariants compile into the Non-Negotiable Constraints section of the governance index — they appear at the top and bottom of every governance file to exploit primacy and recency bias. The Rule section is the source of truth — preserve MUST/NEVER language exactly. Invariants scope to all activities by default because they cannot be violated under any circumstances.
