---
name: invariant:compile
description: "Regenerate the sidecar for one Invariant Record (or all active invariants) via the sidecar-extractor agent"
effort: normal
argument-hint: "[INV-NNN | path] — omit to process all active invariants"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
  - Agent
---

# edikt:invariant:compile

Regenerate the directive sidecar (`<INV-NNN>.edikt.yaml`) for one Invariant Record — or for every active invariant if no argument is given.

Per [ADR-027](../../../docs/architecture/decisions/ADR-027-sidecar-architecture-for-governance-metadata.md) (sidecar architecture, supersedes ADR-008), directive metadata for every active Invariant Record lives in a co-located `<INV-NNN>.edikt.yaml` sidecar conforming to `templates/schemas/sidecar.v1.schema.json` (v1). This command never writes to the parent `.md`. It dispatches the locked `sidecar-extractor` agent which reads the parent body and writes the sidecar.

## Arguments

- `$ARGUMENTS` — optional. One of:
  - `INV-NNN` (e.g., `INV-005`) — resolve to `{invariants_dir}/INV-NNN-*.md`
  - An absolute or repo-relative path to an invariant markdown file
  - Empty / omitted — regenerate every active invariant's sidecar

## Instructions

### 1. Config

Read `.edikt/config.yaml` to resolve `invariants_dir` (default `docs/architecture/invariants`).

### 2. Resolve target(s)

- If `$ARGUMENTS` matches `INV-NNN`, find `{invariants_dir}/INV-NNN-*.md`. If multiple match, error out asking the user to disambiguate by full path. If none match, error out with `error: no invariant matches {ARGUMENTS}`.
- If `$ARGUMENTS` is a path, use it directly. Verify the file exists and is under `{invariants_dir}`.
- If `$ARGUMENTS` is empty, list every `INV-*.md` in `{invariants_dir}` whose frontmatter has `status: active` (or no `status:` field — treated as active for backwards compatibility per ADR-009).

### 3. Dispatch the extractor (per ADR-027)

For each target invariant:

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_INV}"`

The agent reads the invariant's body (specifically `## Statement` and `## Enforcement` sections per ADR-009), extracts directives + signals + topic, and writes `<basename>.edikt.yaml` next to it. Its final response is a single line: `SIDECAR WRITTEN: <relative-path>`.

### 4. Detect idempotency

Compare the new sidecar to its prior version using canonical YAML serialization:

```bash
canonicalize() {
  python3 -c 'import yaml,sys; print(yaml.dump(yaml.safe_load(open(sys.argv[1]).read()), sort_keys=True, default_flow_style=False, width=200), end="")' "$1"
}
```

If the canonical form matches, report `unchanged`. Otherwise report `regenerated`.

In v0.6.0-dev the canonical form is approximate (not byte-deterministic across LLM runs). Phase 8 introduces canonical YAML serialization that makes byte-equal regeneration the contract.

### 5. Confirm

For a single-target run:
```
✅ {INV-NNN}.edikt.yaml — {regenerated | unchanged}
   Source: {invariants_dir}/{INV-NNN}-{slug}.md
```

For an all-targets run:
```
Invariant sidecar regeneration:
  ✅ INV-001 — regenerated
  ✅ INV-002 — unchanged
  ...

  {n} regenerated, {m} unchanged, {k} skipped (revoked).
  Next: Run /edikt:gov:compile to update governance.
```

If any extractor invocation fails, list the failures and exit non-zero.

## Why this command exists

Per ADR-027, the `<INV-NNN>.md` is user-authored prose. Sidecar regeneration is the only mechanism by which the invariant's directive metadata changes. This command is invoked:

- By `/edikt:invariant:new` immediately after writing a new Invariant Record (Phase 4)
- By the user when they edit an invariant's prose body
- By `/edikt:gov:compile`'s Phase A (resync) when it detects sidecar staleness — see [ADR-028](../../../docs/architecture/decisions/ADR-028-two-phase-compile-resync-merge.md)

This command does not run the topic-grouping merge step. After regenerating one or more sidecars, run `/edikt:gov:compile` to refresh `.claude/rules/governance/`.
