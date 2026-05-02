---
name: guideline:compile
description: "Regenerate the sidecar for one guideline (or all guidelines) via the sidecar-extractor agent"
effort: normal
argument-hint: "[slug | path] — omit to process all guidelines"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:guideline:compile

Regenerate the directive sidecar (`<slug>.edikt.yaml`) for one guideline — or for every guideline if no argument is given.

Per [ADR-027](../../../docs/architecture/decisions/ADR-027-sidecar-architecture-for-governance-metadata.md) (sidecar architecture, supersedes ADR-008), directive metadata for every guideline lives in a co-located `<slug>.edikt.yaml` sidecar conforming to `templates/schemas/sidecar.schema.json` (v1). This command never writes to the parent `.md`. It dispatches the locked `sidecar-extractor` agent which reads the parent body and writes the sidecar.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - A guideline slug (e.g., `error-handling`) — resolve to `{guidelines_dir}/{slug}.md`
  - An absolute or repo-relative path to a guideline markdown file
  - Empty / omitted — regenerate every guideline's sidecar

## Instructions

### 1. Config

Read `.edikt/config.yaml` to resolve `guidelines_dir` (default `docs/guidelines`).

### 2. Resolve target(s)

- If `$ARGUMENTS` is a slug, find `{guidelines_dir}/{slug}.md`. If not found, error out with `error: no guideline matches {ARGUMENTS}`.
- If `$ARGUMENTS` is a path, use it directly. Verify the file exists and is under `{guidelines_dir}`.
- If `$ARGUMENTS` is empty, list every `*.md` in `{guidelines_dir}`. Guidelines have no `status:` filter (per ADR-009 they remain a single category).

### 3. Dispatch the extractor (per ADR-027)

For each target guideline:

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_GUIDELINE}"`

The agent walks the guideline body and extracts every imperative sentence (MUST / MUST NOT / SHOULD / NEVER / ALWAYS) into directives. Soft-language bullets (e.g., bullets without normative verbs) are skipped — guidelines compile only enforceable rules. The agent writes `<slug>.edikt.yaml` next to the source `.md`. Its final response is a single line: `SIDECAR WRITTEN: <relative-path>`.

### 4. Detect idempotency

Compare the new sidecar to its prior version using canonical YAML serialization:

```bash
canonicalize() {
  python3 -c 'import yaml,sys; print(yaml.dump(yaml.safe_load(open(sys.argv[1]).read()), sort_keys=True, default_flow_style=False, width=200), end="")' "$1"
}
```

If the canonical form matches, report `unchanged`. Otherwise report `regenerated`.

In v0.6.0-dev the canonical form is approximate. Phase 8 introduces canonical YAML serialization that makes byte-equal regeneration the contract.

### 5. Confirm

For a single-target run:
```
✅ {slug}.edikt.yaml — {regenerated | unchanged}
   Source: {guidelines_dir}/{slug}.md
```

For an all-targets run:
```
Guideline sidecar regeneration:
  ✅ error-handling — regenerated
  ✅ http-handlers — unchanged
  ...

  {n} regenerated, {m} unchanged.
  Next: Run /edikt:gov:compile to update governance.
```

If any extractor invocation fails, list the failures and exit non-zero.

## Why this command exists

Per ADR-027, the `<slug>.md` is user-authored prose. Sidecar regeneration is the only mechanism by which the guideline's directive metadata changes. This command is invoked:

- By `/edikt:guideline:new` immediately after writing a new guideline (Phase 4)
- By the user when they edit a guideline's prose body
- By `/edikt:gov:compile`'s Phase A (resync) when it detects sidecar staleness — see [ADR-028](../../../docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md)

This command does not run the topic-grouping merge step. After regenerating one or more sidecars, run `/edikt:gov:compile` to refresh `.claude/rules/governance/`.
