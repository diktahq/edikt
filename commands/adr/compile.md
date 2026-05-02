---
name: adr:compile
description: "Regenerate the sidecar for one ADR (or all accepted ADRs) via the sidecar-extractor agent"
effort: normal
argument-hint: "[ADR-NNN | path] — omit to process all accepted ADRs"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:adr:compile

Regenerate the directive sidecar (`<ADR-NNN>.edikt.yaml`) for one ADR — or for every accepted ADR if no argument is given.

Per [ADR-027](../../../docs/architecture/decisions/ADR-027-sidecar-architecture-for-governance-metadata.md) (sidecar architecture, supersedes ADR-008), directive metadata for every ADR lives in a co-located `<ADR-NNN>.edikt.yaml` sidecar conforming to `templates/schemas/sidecar.schema.json` (v1). This command never writes to the parent `.md`. It dispatches the locked `sidecar-extractor` agent which reads the parent body and writes the sidecar.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `ADR-NNN` (e.g., `ADR-018`) — resolve to `{decisions_dir}/ADR-NNN-*.md`
  - An absolute or repo-relative path to an ADR markdown file
  - Empty / omitted — regenerate every accepted ADR's sidecar

## Instructions

### 1. Config

Read `.edikt/config.yaml` to resolve `decisions_dir` (default `docs/architecture/decisions`).

### 2. Resolve target(s)

- If `$ARGUMENTS` matches `ADR-NNN`, find `{decisions_dir}/ADR-NNN-*.md`. If multiple match, error out asking the user to disambiguate by full path. If none match, error out with `error: no ADR matches {ARGUMENTS}`.
- If `$ARGUMENTS` is a path, use it directly. Verify the file exists and is under `{decisions_dir}` or another configured artifacts directory.
- If `$ARGUMENTS` is empty, list every `ADR-*.md` in `{decisions_dir}` whose frontmatter has `status: accepted`. Drafts and superseded ADRs are skipped.

### 3. Dispatch the extractor (per ADR-027)

For each target ADR:

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_ADR}"`

The agent reads the ADR body, extracts directives + signals + topic, and writes `<basename>.edikt.yaml` next to it. Its final response is a single line: `SIDECAR WRITTEN: <relative-path>`.

### 4. Detect idempotency

After the agent returns, compare the new sidecar to its prior version (if one existed before this run). Use canonical YAML serialization for the comparison:

```bash
canonicalize() {
  python3 -c 'import yaml,sys; print(yaml.dump(yaml.safe_load(open(sys.argv[1]).read()), sort_keys=True, default_flow_style=False, width=200), end="")' "$1"
}
```

If the canonical form of the new sidecar matches the canonical form of the prior sidecar, this regeneration is a no-op — report `unchanged`. Otherwise report `regenerated`.

In v0.6.0-dev the canonical form is approximate: the sidecar-extractor's output is not byte-deterministic across runs. Phase 8 introduces a canonical YAML serializer that makes byte-equal regeneration the contract. Until then, idempotency is reported best-effort and the user MAY see false `regenerated` reports when the agent produces semantically-equivalent but textually-different output.

### 5. Confirm

For a single-target run:
```
✅ {ADR-NNN}.edikt.yaml — {regenerated | unchanged}
   Source: {decisions_dir}/{ADR-NNN}-{slug}.md
```

For an all-targets run:
```
ADR sidecar regeneration:
  ✅ ADR-001 — regenerated
  ✅ ADR-002 — unchanged
  ✅ ADR-003 — unchanged
  ...

  {n} regenerated, {m} unchanged, {k} skipped (superseded/draft).
  Next: Run /edikt:gov:compile to update the topic-grouped governance files.
```

If any extractor invocation fails, list the failures and exit non-zero. The successful regenerations stay on disk; the user can re-run for just the failures.

## Why this command exists

Per ADR-027, the `<artifact>.md` is user-authored prose that edikt never edits. Sidecar regeneration is the only mechanism by which directive metadata changes. This command is invoked:

- By `/edikt:adr:new` immediately after writing a new ADR (Phase 4)
- By the user when they edit an ADR's prose body and want the sidecar to catch up
- By `/edikt:gov:compile`'s Phase A (resync) when it detects sidecar staleness — see [ADR-028](../../../docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md)

This command does not run the topic-grouping merge step. That is `/edikt:gov:compile`'s Phase B (deterministic merge over all sidecars). After regenerating one or more sidecars here, run `/edikt:gov:compile` to refresh `.claude/rules/governance/`.
