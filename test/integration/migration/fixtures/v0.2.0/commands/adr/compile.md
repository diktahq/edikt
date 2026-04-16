---
name: edikt:adr:compile
description: "Generate or regenerate directive sentinel blocks for one ADR or all"
effort: normal
argument-hint: "[ADR-NNN] — omit to process all accepted ADRs missing a sentinel"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
---

# edikt:adr:compile

Generate or regenerate directive sentinel blocks (`[edikt:directives:start/end]: #`) for ADRs. Sentinels are what `/edikt:gov:compile` reads — without them, compile falls back to extraction, which is slower and less accurate.

## Arguments

- `$ARGUMENTS` — optional ADR ID (e.g., `ADR-003`). If no argument, processes all accepted ADRs.

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve:
- Decisions: `paths.decisions` (default: `docs/architecture/decisions`)

### 2. Determine Scope

**With `$ARGUMENTS`** — locate the ADR file matching the given ID (e.g., `ADR-003`). Search for a file whose name contains the ID prefix. If not found:
```
ADR not found: {id}
Run: ls {decisions_path}/*.md to see available ADRs.
```

**Without `$ARGUMENTS`** — glob all `*.md` files in `{decisions_path}`. Filter to those with `status: accepted` (check frontmatter `status:` field, or fall back to `**Status:** accepted` in the body).

If no accepted ADRs found:
```
No accepted ADRs found in {decisions_path}.
```

### 3. Process Each ADR

For each ADR in scope:

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

Read the `## Decision` section. Extract every enforceable statement — statements that prescribe or prohibit a specific behavior, tool, pattern, or structure. Transform each into a directive:

Rules for generating directives:
1. Hard constraints use MUST or NEVER (uppercase) with a one-clause reason.
2. Name specific things — namespaces, tools, patterns, file paths, thresholds.
3. One directive per line.
4. Each directive ends with `(ref: {ADR-ID})`.
5. Drop rationale prose, context, and alternatives — directives only.

Derive `paths:` by scanning the project directory for file types and locations relevant to the decision's topic.

Derive `scope:` from the decision domain:
- Cross-cutting architecture decisions: `[planning, design, review]`
- Implementation patterns: `[implementation, review]`
- Tooling or infrastructure: `[implementation]`

**Validate cross-references:** for every generated directive that references another ADR or INV, confirm that identifier exists in the source document. Strip any fabricated references — keep the directive text if it's otherwise accurate.

Compute `content_hash:` as MD5 of all human-readable content above the sentinel insertion point.

#### Write Sentinel

Insert (or replace) the sentinel block in the ADR file, inside the `## Directives` section if present, or append before the closing `---` line:

```markdown
[edikt:directives:start]: #
content_hash: {md5}
paths:
  - {glob patterns}
scope:
  - {activity scopes}
directives:
  - {directive} (ref: {ADR-ID})
  - {directive} (ref: {ADR-ID})
[edikt:directives:end]: #
```

### 4. Report Results

```
✅ ADR sentinels updated: {n} generated, {m} already current

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

REMEMBER: Sentinel generation reads the ## Decision section only — not the full ADR. Drop rationale and alternatives. Every directive must be verifiable: specific tools, patterns, file paths, or thresholds named explicitly. Fabricated cross-references are worse than no cross-references — always validate before including.
