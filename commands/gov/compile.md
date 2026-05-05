---
name: gov:compile
description: "Compile ADRs and invariants into governance directives"
effort: high
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
  - Write
---

# edikt:gov:compile

Compile accepted ADRs, active invariants, and team guidelines into topic-grouped governance rule files under `.claude/rules/governance/`, with a routing index at `.claude/rules/governance.md`.

## Compile Schema

**`COMPILE_SCHEMA_VERSION = 2`**

This integer identifies the output format contract of this command. It is independent of edikt's marketing version (see ADR-007).

- `1` — v0.1.x flat `governance.md` (single file, one directive per line, 30-directive cap). Legacy.
- `2` — v0.2.x topic-grouped rule files under `governance/`, routing table index, directive sentinel blocks, no directive cap. Current.

Bump this constant ONLY when the output structure changes in a way that older tooling cannot read. Prose updates, bug fixes, and new directives do NOT bump the schema. When bumped, write a new ADR superseding ADR-007 to document the migration.

Each source document must contain an LLM directive sentinel block (`[edikt:directives:start/end]: #`). If present, compile reads it verbatim — no extraction, no distillation. If absent, compile generates the sentinel block and writes it back to the source document before compiling.

CRITICAL: NEVER write governance files that contain contradictions — detect and report them before writing, and abort or confirm with the user.

## Arguments

- `--check` — validate only, don't write. Exit with errors if contradictions found. For CI.
- `--json` — output only the JSON format (see Reference). No progress indicators, no emoji, no prose.

## Instructions

0. If `.edikt/config.yaml` does not exist, output:
   ```
   No edikt config found. Run /edikt:init to set up this project.
   ```
   And stop.

0a. **Pre-v0.6.0 sentinel gate (ADR-027).** Before any other work, refuse to run when legacy in-body sentinels remain in the project. v0.6.0 reads sidecars only — there is no double-parser fallback (per ADR-027).

    Scan for the marker `[edikt:directives:start]: #` outside fenced regions and outside the documentation skip-list (`ADR-008-*`, `ADR-009-*`, `SPEC-*`). The `edikt` binary handles fence detection and skip-list correctly:

    ```bash
    edikt migrate sidecars --dry-run > /tmp/edikt-sidecar-precheck.out 2>&1
    PRECHECK_EXIT=$?
    ```

    - If `PRECHECK_EXIT == 0` AND output contains `0 sidecars to create` — no migration pending, continue to Step 1.
    - Otherwise — refuse with a single-line actionable error and exit 1:
      ```
      ✗ Migration required. Run /edikt:upgrade to migrate this project to v0.6.0 sidecar architecture (ADR-027).
      ```
      Do NOT print the dry-run plan here — `/edikt:upgrade` shows it. Keep this gate's output to one line so CI logs stay readable.

    NEVER fall back to in-body sentinel parsing. The pre-flight gate is the only path for legacy projects.

0b. If `--json` is in `$ARGUMENTS`, output only the JSON format at the end — no progress indicators, no emoji, no prose.

1. Display progress: `Step 1/5: Reading source documents...`

1b. Read the edikt version from `~/.edikt/VERSION`. If this file doesn't exist, fall back to `edikt_version:` in `.edikt/config.yaml`. If BOTH differ (e.g., VERSION says 0.2.3 but config says 0.3.0), warn:
   ```
   ⚠ ~/.edikt/VERSION (0.2.3) differs from .edikt/config.yaml edikt_version (0.3.0).
     The compiled_by stamp will use ~/.edikt/VERSION. To update, re-run the installer:
     curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash
   ```
   Use `~/.edikt/VERSION` as the authoritative version for the `compiled_by` stamp.

2. Read `.edikt/config.yaml`. Resolve paths from the `paths:` section using the Path Defaults in the Reference section.

3. Read source documents:
   - **ADRs:** include if `status: accepted`. Skip `draft`, `superseded`, `deprecated`. Fall back to checking `**Status:** accepted` in the body for backwards compatibility.
   - **Invariants:** include if `status: active` or no status (backwards compatibility). Skip `status: revoked`.
   - **Guidelines:** include all `.md` files from the guidelines directory. No status filtering. Each filename (without `.md`) becomes the section label.

4. Display progress: `Step 2/5: Checking for contradictions...`

5. Detect contradictions between accepted ADRs: direct contradictions ("use X" vs "never use X"), scope conflicts, approach conflicts. Use the Contradiction Detection examples in the Reference section as a guide for how to report them.

6. Also check: superseded ADRs still referenced by active specs or plans; invariants that conflict with accepted ADRs; guidelines that conflict with ADRs or invariants. Conflicts between guidelines and invariants are errors (invariants always win). Conflicts between guidelines and ADRs are warnings.

7. If `--check` flag: report all contradictions and conflicts, then output the Check Output Format from the Reference section and stop (don't write).

8. If contradictions found and not `--check`: report them and ask user to proceed anyway or abort.

### Extract Directives

9. Display progress: `Step 3/5: Grouping directives by topic...`

10. For each source document (ADR, invariant, guideline), look for its co-located sidecar: `{same_dir}/{basename}.edikt.yaml`. Per ADR-027, sidecars are the authoritative source — `gov:compile` MUST read sidecars only. In-body sentinel blocks are pre-migration artifacts; their presence was already rejected by step 0a.

11. **If sidecar exists:** read the sidecar YAML. The sidecar conforms to `templates/schemas/sidecar.v1.schema.json` (v1):

    - **`directives[]`** — auto-extracted by `/edikt:<artifact>:compile` from the source body. Each entry has `text` and `source_excerpt`.
    - **`manual_directives[]`** — user-authored rules. Optional (defaults to `[]`). Preserved across sidecar regenerations. Always included in the effective rule set.
    - **`suppressed_directives[]`** — auto directives the user rejected. Optional (defaults to `[]`). Subtracted from `directives` at compile time.
    - **`reminders[]`** — pre-action reminders. Optional. Aggregated into `governance.md § Reminders`.
    - **`verification[]`** — verification checklist items. Optional. Aggregated into `governance.md § Verification Checklist`.
    - **`topic`** — the topic key for grouping (e.g., `ai`, `backend`, `privacy`).
    - **`signals[]`** — routing keywords for the routing table.

    Compute the **effective rule set** using the ADR-008 merge formula (still applies, now over sidecar fields):

    ```
    effective_rules = (directives[].text - suppressed_directives) ∪ manual_directives
    ```

    Where `-` is set difference by exact string match, and `∪` is set union preserving document order. Duplicates across both lists are de-duplicated; the first occurrence's source reference is kept.

    **Within-artifact contradiction detection** (same as before): a directive and a manual directive that contradict each other → warn but include both. A suppressed directive that matches nothing in `directives[]` → warn (stale suppression).

12. **If no sidecar exists** (fresh artifact, never compiled): auto-chain to the per-artifact compile commands, then re-run step 11.

    Group missing sidecars by artifact type. Before chaining, print:

    ```
    ℹ No sidecars found for {n} artifacts. Running per-artifact compile:
       → /edikt:adr:compile         ({n_adr} ADRs without sidecar)
       → /edikt:invariant:compile   ({n_inv} invariants without sidecar)
       → /edikt:guideline:compile   ({n_gl} guidelines without sidecar)
    ```

    After each completes, re-run step 11. If sidecars are STILL missing after the chain, abort with the full list.

    **Headless mode** (`EDIKT_HEADLESS=1` or no `/dev/tty`): disable the auto-chain. Print the explicit run-these-three list and exit non-zero.

### Schema Completeness Gate

12a. **After reading all sidecars, validate that required fields are present.**

For each loaded sidecar, verify: `schema_version`, `topic`, `path`, `signals`, `directives` are all present (presence-check only — content validation is the sidecar validator's job). A sidecar missing any required field indicates a partial write or schema version mismatch.

If any sidecar is incomplete, abort with:
```
✗ {n} sidecar(s) are missing required fields:
    {path}: missing [{field1}, {field2}]

Re-run /edikt:adr:compile {ADR-NNN} (or invariant/guideline compile) to regenerate.
Then re-run /edikt:gov:compile.
```

Do NOT partial-write `governance.md` on an incomplete input set.

### Validate Cross-References

12b. For every extracted directive that references a specific invariant ID (INV-NNN), ADR ID (ADR-NNN), or other named artifact — verify the reference exists in the source document. Read the source file and confirm the referenced identifier appears in it. If it does not appear, strip the fabricated reference from the directive (keep the directive text if it's otherwise accurate). Never include a cross-reference that hasn't been confirmed in the actual source file.

### Directive-Quality Pass

12c. **After** the contradiction-detection pass (steps 5–8) and **before** grouping, run the shared directive-quality sub-procedure from `commands/gov/_shared-directive-checks.md` for every accepted ADR and active invariant.

For each source document that has a parsed sentinel block, invoke `bin/edikt gov directive-check` (the tier-2 subcommand documented in `_shared-directive-checks.md §Tier-2 Subcommand`) once per directive in `directives:` and once per directive in `manual_directives:`. Pass:

```json
{
  "adr_id": "<ADR-NNN or INV-NNN>",
  "directive_body": "<directive line text>",
  "canonical_phrases": ["<phrase1>", ...],
  "no_directives_reason": "<reason string or null>"
}
```

Collect all returned warning lines across all documents. If any warnings were produced, output them under a `### Directive-quality warnings` header in the compile output:

```
### Directive-quality warnings

[WARN] ADR-012: directive has 2 sentences but no canonical_phrases — run /edikt:adr:review --backfill
[WARN] ADR-014: canonical_phrase "atomic rename" not found in directive body
```

**AC-021 grace period:** exit 0 even when warnings are present. Do NOT block compilation due to directive-quality warnings in v0.6.0. The header is surfaced so users are aware; it is not an error.

If no warnings were produced, skip the header entirely (do not emit an empty section).

### Orphan Detection Pass

12d. **After** the directive-quality pass (step 12c), run the orphan-detection and history-comparison pass. This implements FR-004 / AC-003 / AC-003b / AC-017 / AC-018 / AC-019.

**Two-layer atomicity model:**
- **Outer layer:** the compile operation as a whole is serialized by the existing `lock.yaml + flock` pattern in `bin/edikt` (SPEC-004 §8). This prevents two concurrent compiles from racing on source files or the governance output. Phase 7 does not change this layer.
- **Inner layer:** the state file `.edikt/state/compile-history.json` is protected specifically from torn writes by using write-to-tempfile + `os.rename()`. If the process crashes between the write and the rename, the previous state file remains intact — safe toward re-warning rather than a silent skip. The `.tmp` file may exist and is safe to remove manually.

#### Pass 1: Orphan collection

Walk all accepted ADRs and active invariants. For each one, check whether:
- The parsed `directives` list AND `manual_directives` list are both empty (i.e., the effective rule set produces zero directives), AND
- The source document's frontmatter does NOT contain a `no-directives:` key with a valid reason.

A "valid reason" is defined by `_shared-directive-checks.md §Check C`: ≥ 10 characters, not in `{tbd, todo, fix later}` (case-insensitive), non-empty after strip.

If both conditions are true, add the ADR/INV ID to the **current orphan set**.

#### Pass 2: History comparison and write

Delegate the five-rule orphan-set state machine to the tier-2 helper. Per ADR-029 + ADR-033 this subcommand is an authorized tier-2 orchestration call; per ADR-020 + ADR-030 the implementation is pure Go (no LLM dispatch).

```bash
bin/edikt gov compile-history \
    --orphans "$EDIKT_ORPHAN_IDS" \
    --history-path ".edikt/state/compile-history.json" \
    --edikt-version "$EDIKT_VERSION"
```

**Inputs (set by the caller):**
- `EDIKT_ORPHAN_IDS` — comma-separated list of orphan IDs (e.g. `ADR-012,INV-003`). Empty = no orphans this run.
- `EDIKT_VERSION` — version string from step 1b, optional (stamps the `edikt_version` field).

**Exit-code contract (ADR-029 Rule 2 — output is informational, exit code is the contract):**
- `0` — first detection, subset/recovered, superset, different-reset, or no-orphans. Compile continues.
- `1` — consecutive scenario (same orphan set as previous run). BLOCK — compile MUST exit non-zero and MUST NOT write governance output.
- `2` — INV-006 refusal (invalid orphan ID, traversal in `--history-path`).

The subcommand handles atomicity (tmp + rename), corrupt-history recovery (warn + treat as absent), and deterministic ordering of the orphan list. Per ADR-029 Rule 2 do not parse stdout — display verbatim.

If exit is non-zero (scenario 2), the overall compile command MUST exit non-zero and MUST NOT proceed to write governance output.

If exit is 0, continue to step 13 (group by topic).

**AC-019 — `.gitignore` management:**

After the orphan script completes (regardless of exit code), run the tier-2 bootstrap subcommand. Per ADR-029 + ADR-033 this is an authorized orchestration call; trailing-slash variants (`.edikt/state/` vs `.edikt/state`) are deduplicated inside the helper.

```bash
bin/edikt gov gitignore-bootstrap --project-root "$EDIKT_PROJECT_ROOT"
```

**AC-019 note:** The helper checks for both `.edikt/state/` (with trailing slash) and `.edikt/state` (without) before appending, to avoid duplicates under trailing-slash normalization variants. If the file is absent, it is created. If the entry is already present in any recognized form, the file is left unchanged. Exit code is the contract — output is informational.

### Group by Topic

13. Analyze all **effective_rules** across all source documents (computed in step 11 via the three-list merge formula) and group them by topic. A topic is a domain area — caching, database, multi-tenancy, authentication, file storage, architecture (cross-cutting), etc.

    Grouping rules:
    - Effective rules from different sources about the same domain go into the same topic file
    - Each rule keeps its source reference (`ref: ADR-008, §Eviction`) whether it originated from `directives:` or `manual_directives:` — readers of governance.md cannot tell which list a rule came from, only which source document it references
    - If a rule doesn't fit an obvious topic, group it under `architecture.md` (cross-cutting)
    - Invariants are special — their effective rules go into `governance.md` (the index), not into topic files
    - Across source documents: if the same rule string appears in multiple effective sets, de-duplicate by exact string match. Keep the first occurrence's source reference.

13a. **CRITICAL — write the resolved `topic:` back to each source document's sentinel block.** Per ADR-020 §c, the LLM grouping is a one-shot fallback; the resolved topic MUST be persisted into the artifact's sentinel so subsequent runs are deterministic and the Go binary helper can group without invoking an LLM.

    For every source document whose sentinel block lacks a `topic:` field, edit the artifact in place to add the assigned topic to its sentinel YAML. The line goes ABOVE `directives:` for readability:

    ```yaml
    [edikt:directives:start]: #
    <!-- edikt:directives — auto-generated, do not edit manually -->
    source_hash: <unchanged>
    directives_hash: <unchanged>
    compiler_version: <unchanged>
    topic: <assigned-topic-slug>          ← NEW LINE
    paths:
      - "**/*"
    scope: [planning, design]
    directives:
      - ...
    manual_directives: []
    suppressed_directives: []
    [edikt:directives:end]: #
    ```

    NEVER overwrite a `topic:` field that the user (or a prior run) has already set — only add when missing. Topic slugs MUST be lowercase kebab-case (`ai-processing`, `database`, `frontend`). Reuse an existing topic name if any other artifact already routes to it; only invent a new slug when no existing topic fits.

    After all writes, log a summary line per artifact:

    ```
    → wrote topic: ai-processing → docs/architecture/decisions/ADR-001-...md
    → wrote topic: frontend → docs/architecture/decisions/ADR-002-...md
    ...
    ```

    This step makes the difference between "first run is LLM-driven, future runs deterministic" (correct) and "every run is LLM-driven" (regression vs ADR-020).

13b. **While writing back `topic:`, also write back `signals:`.** During the LLM grouping pass for an artifact lacking `topic:`, also derive 4–12 routing keywords from the artifact body (concrete domain nouns, tool names, feature terms — same rubric as `<artifact>:compile`). Persist them to the same sentinel block as a `signals:` list. The Go binary aggregates these per topic into the routing-table row, eliminating the need for a hardcoded `topic→signals` map.

    If the artifact already has a non-empty `signals:` list, preserve it verbatim — only emit when missing.

13c. **Re-emit `source_hash` and `directives_hash` after the topic-write and signals-write.** Adding fields to the sentinel changes the file body; without re-hashing, the next `<artifact>:compile` run will see a stale hash and trigger an unnecessary interview. Recompute both hashes and update the sentinel.

### Derive Path Patterns

14. Display progress: `Step 4/5: Scanning codebase for path patterns...`

15. For each topic file, determine the `paths:` frontmatter:
    - **If pinned in sentinel block:** use the author's paths verbatim
    - **If not pinned:** scan the project directory structure to find where code related to this topic lives. Generate glob patterns matching those locations.
    - Use the `paths:` YAML list format (one glob per line)

16. For each topic file, determine the `scope:` metadata for the routing table:
    - **If specified in sentinel block:** use the author's scopes
    - **If not specified:** derive from content — architecture/cross-cutting decisions get `[planning, design, review]`, implementation-specific rules get `[implementation]`

### Write Output

17. Display progress: `Step 5/5: Writing governance files...`

18. Write topic rule files to `.claude/rules/governance/`:

    Each file follows this format:
    ```markdown
    ---
    paths:
      - "**/*.go"
      - "**/adapters/postgres/**"
    compile_schema_version: 2
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- topic: {topic name} -->
    <!-- sources: {list of source documents that contributed} -->
    <!-- compiled_by: edikt v{edikt_version} -->
    <!-- compiled_at: {ISO8601 timestamp} -->

    # {Topic Name}

    - {directive} (ref: {source})
    - {directive} (ref: {source})
    ```

19. Write the governance index to `.claude/rules/governance.md`:

    ```markdown
    ---
    paths: "**/*"
    compile_schema_version: 2
    ---
    <!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
    <!-- compiled_by: edikt v{edikt_version} -->
    <!-- compiled_at: {ISO8601 timestamp} -->

    # Governance Directives

    Follow these directives in every file you write or edit.

    ## Non-Negotiable Constraints

    These are invariants. Violation is never acceptable.

    - {invariant directive} (ref: INV-NNN)

    ## Routing Table

    Before starting a task, scan this table. If your task matches any signals or scopes, read the referenced file.

    | Signals | Scope | File |
    |---|---|---|
    | {keywords} | {scope list} | `governance/{topic}.md` |

    ## Reminders

    Before acting, check the relevant constraint.

    [edikt:reminders:start]: #
    {Aggregate all `reminders:` lists from all source document sentinel blocks.
     De-duplicate by exact string match. Cap at 10 reminders total.
     Format: "- Before {action} → {check} (ref: ID)"}
    [edikt:reminders:end]: #

    ## Verification Checklist

    Before finishing, verify each item. If any fails, fix before submitting.

    {Aggregate all `verification:` lists from all source document sentinel blocks.
     De-duplicate by exact string match. Cap at 15 items total.
     Format: "- [ ] {what to check} (ref: ID)"}

    ## Reminder: Non-Negotiable Constraints

    These constraints were listed above and are restated for emphasis.
    Do not violate them under any circumstances.

    - {repeat invariant directives}
    ```

20. If the compiled output detects an existing flat `governance.md` (old format with `edikt:compiled` marker but no `governance/` directory), this is a migration:
    - Create the `governance/` directory
    - Generate topic files from the old directives
    - Replace the old `governance.md` with the new index format
    - Report the migration:
      ```
      📦 Migrated from flat governance.md to topic-grouped rule files.
         Old format: 1 file, {n} directives
         New format: {m} topic files + index
      ```

21. If any single topic file exceeds 100 directives, warn:
    ```
    ⚠ {topic}.md has {count} directives. Large rule files may dilute compliance.
      Consider splitting into subtopics or running /edikt:gov:review to tighten language.
    ```

22. Log the compilation event:
    ```bash
    source "$HOME/.edikt/hooks/event-log.sh" 2>/dev/null
    edikt_log_event "compile" '{"adrs_compiled":{n},"invariants_compiled":{m},"guidelines_compiled":{g},"topics":{t},"total_directives":{total},"sentinel_coverage":"{pct}%"}'
    ```

23. Output the compilation summary with reverse source map:
    ```
    ✅ Governance compiled

      governance/{topic}.md
        ← {source document} ({sections contributed})
        ← {source document} ({sections contributed})

      governance/{topic}.md
        ← {source document} ({sections contributed})

      ━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
      {n} ADRs + {m} invariants + {g} guidelines
      → {t} topic files + index
      → {total} total directives
      Sentinel coverage: {with_sentinels}/{total_sources} documents ({pct}%)

      {If generated_sentinels > 0}:
      ⚙ {generated_sentinels} sentinel blocks generated and written to source documents.
        Run /edikt:gov:review to review language quality.

      Claude will read these directives automatically in every session.

      Next: Run /edikt:gov:review to review directive language quality.
    ```

24. This command should be suggested (not auto-run) after `/edikt:adr:new` or `/edikt:invariant:new` creates or modifies a document. Add to those commands' output: `Run /edikt:gov:compile to generate directive sentinels and update governance.`

---

REMEMBER: NEVER write governance files with contradictions. Invariants go in the index (governance.md), not topic files — they are always loaded. Topic files contain domain-specific directives grouped from all sources. The ADRs and source documents are the source of truth — compiled output is the enforcement format, never hand-edit it.

## Reference

### Path Defaults

| Key | Default |
|---|---|
| `paths.decisions` | `docs/architecture/decisions` |
| `paths.invariants` | `docs/architecture/invariants` |
| `paths.guidelines` | `docs/guidelines` |

### Fallback Directive Extraction Rules

Used only when a source document has no `[edikt:directives:start/end]` sentinel block.

**From ADRs** — read the `## Decision` section. Extract all enforceable statements. Preserve specifics (namespaces, patterns, thresholds, tool names). Drop rationale, context, alternatives. Each statement becomes one directive.

Example transformation:
```
ADR source (150 lines):
  # ADR-001 — edikt: Context Engine and Guardrail Installer
  ## Decision
  Build edikt as a lean context engine targeting Claude Code exclusively.
  Other tools lack path-conditional rules, hooks, slash commands...
  [... 100 more lines of rationale, alternatives, consequences ...]

Compiled directive (1 line):
  - Claude Code is the only supported platform. Do not write code or
    configuration targeting Cursor, Copilot, or other tools. (ref: ADR-001)
```

**From invariants** — directives are already constraint-shaped; use the Rule section directly:
```
Invariant source:
  # INV-001 — Commands are plain markdown, no compiled code
  ## Rule
  Every edikt command is a .md file. No TypeScript, no compiled binaries...

Compiled directive:
  - Every command and template must be a .md or .yaml file. No TypeScript,
    no compiled binaries, no build step. This constraint is non-negotiable.
    (ref: INV-001)
```

**From guidelines** — each file becomes a set of directives. Guidelines are freeform; extract enforceable bullet points.

### Contradiction Detection Examples

```
⚠️  Contradiction detected:
    ADR-001: "Claude Code only — no multi-tool support"
    ADR-007: "Support Cursor for rule distribution"

    Resolve before compiling. Supersede one or reconcile both.
```

```
⚠️  Conflict between guideline and ADR:
    guidelines/testing.md: "Always mock the database in all tests"
    ADR-003: "Integration tests must hit a real database, no mocks"

    Source: guidelines/testing.md (line 12) vs ADR-003 (Decision section)
    Action: Scope the guideline to unit tests only, or amend ADR-003.
```

```
⚠️  Conflict between guideline and invariant:
    guidelines/dependencies.md: "Use lodash for utility functions"
    INV-001: "No runtime dependencies"

    Source: guidelines/dependencies.md (line 5) vs INV-001 (Rule section)
    Action: Remove the guideline — invariants are non-negotiable.
```

### JSON Output Format

```json
{
  "status": "success",
  "topics": [{"name": "cache", "file": "governance/cache.md", "directives": 12, "sources": ["ADR-008", "guideline-database.md"]}],
  "invariants": [{"id": "INV-001", "directive": "..."}],
  "sentinel_coverage": {"with": 5, "total": 7, "percent": 71},
  "contradictions": [],
  "total_directives": 27
}
```

### Check Output Format

```
/edikt:gov:compile --check

  Sources: {n} ADRs ({m} accepted), {j} invariants ({l} active), {g} guidelines
  Sentinel coverage: {with_sentinels}/{total} documents
  Contradictions: {count}
  Conflicts: {count} (guideline vs ADR/invariant)
  Topics: {count} would be generated
  Directives: {count} would be generated

  {If contradictions: list them}
  {If clean: "All clear — governance compiles cleanly."}
```
