---
name: adr:review
description: "Review ADR language quality for enforceability and directive strength"
effort: high
argument-hint: "[ADR-NNN] — omit to review all accepted ADRs"
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# edikt:adr:review

Review ADRs for language quality in the Decision section. Checks whether decision statements are specific, actionable, and phrased correctly to achieve reliable compliance when compiled into governance directives.

This is not a structural check (that's `/edikt:doctor`), a contradiction check (that's `/edikt:gov:compile --check`), or sentinel generation (that's `/edikt:adr:compile`). This is a language quality review.

CRITICAL: Every finding must cite the specific text that fails the check and provide a concrete rewrite. Never flag a directive without showing how to fix it.

## Arguments

- `$ARGUMENTS` — optional ADR ID (e.g., `ADR-NNN`). If no argument, reviews all accepted ADRs.
- `--backfill` — DEPRECATED under v0.6.0. Was an in-body-sentinel feature (pre-v0.6); short-circuits with a deprecation message under the current sidecar contract. See section 7.

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

**With `$ARGUMENTS`** — locate the ADR file matching the given ID. If not found:
```
ADR not found: {id}
Run: ls {decisions_path}/*.md to see available ADRs.
```

**Without `$ARGUMENTS`** — glob all `*.md` files in `{decisions_path}`. Filter to `status: accepted`.

If no accepted ADRs found:
```
No accepted ADRs found in {decisions_path}.
```

### 2b. Routing: --backfill Mode

If `--backfill` is present in `$ARGUMENTS`, skip sections 3–6 and run the **Backfill Flow** (section 7) instead.

### 3. Review Each ADR

Display progress: `Step 1/3: Analyzing ADR language quality...`

For each ADR:

1. Read the `## Decision` section.
2. Extract all enforceable statements — any sentence or bullet that prescribes or prohibits a behavior, tool, pattern, or structure.
3. Score each statement against the Quality Criteria (below) on four dimensions: Specificity, Actionability, Phrasing, Testability. A statement is the weakest rating it receives on any dimension.
4. For each statement rated `weak` or `vague`, provide a concrete rewrite.

### Quality Criteria

**1. Specificity**

| Rating | Definition |
|---|---|
| Strong | Names specific patterns, tools, namespaces, or file paths |
| Adequate | Describes the behavior clearly without exact syntax |
| Weak | Uses subjective terms without measurable criteria |
| Vague | Could mean anything to different readers |

**2. Actionability**

| Rating | Definition |
|---|---|
| Strong | One clear action, no ambiguity about what to produce |
| Adequate | Clear intent, minor interpretation needed |
| Weak | Multiple interpretations possible |
| Vague | No actionable instruction |

**3. Phrasing**

| Rating | Definition |
|---|---|
| Strong | NEVER/MUST (uppercase) for hard constraints with one-clause reason |
| Adequate | Clear imperative without emphasis marker |
| Weak | Soft language ("should", "prefer") for a hard constraint |
| Vague | No imperative, reads as suggestion |

**4. Testability**

| Rating | Definition |
|---|---|
| Strong | Verifiable by grep, test, or code review with specific criteria |
| Adequate | Verifiable by reading the code with clear criteria |
| Weak | Requires subjective judgment to verify |
| Vague | Cannot be verified |

### 3b. Soft-Language Scanner

For each ADR reviewed, scan every compiled directive body (from both `directives:` and `manual_directives:` in the sentinel block) for soft-language markers. The six markers are:

| Marker | Case-insensitive | Suggested replacement |
|---|---|---|
| `should` | yes | `MUST` (positive) |
| `ideally` | yes | `MUST` (positive) |
| `prefer` | yes | `MUST` (positive) |
| `try to` | yes | `NEVER` / `MUST NOT` (negative) |
| `might` | yes | `MUST` (positive) |
| `consider` | yes | `MUST` (positive) |

For each match found, emit:
```
[WARN] ADR-{ID}: directive body contains "{marker}" — suggest {replacement}
  Directive: "{first 120 chars of directive body}..."
```

Replacement mapping:
- `should` / `might` / `consider` / `ideally` → `MUST` (positive)
- `try to` / `prefer to avoid` → `NEVER` / `MUST NOT` (negative)
- `prefer` → `MUST` (positive, unless the context is clearly "prefer to avoid" → `NEVER`)

If no soft-language markers are found in any directive, emit:
```
✓ No soft-language markers found in directive bodies.
```

Include soft-language warnings inline in the ADR's review section, directly after the decision-quality findings.

### 3c. Review Compiled Directives (LLM Compliance)

Read the ADR's co-located `<name>.edikt.yaml` sidecar. If the sidecar exists, score each compiled directive for LLM compliance. For each directive in `directives[]` AND `manual_directives[]`, score on:

- **Token specificity** — 0 backtick tokens = Low, 1-2 = Medium, 3+ = High
- **Length** — <10w flag, 10-30w good, 30-50w check splittable, >50w split
- **MUST/NEVER** — present = pass; absent = flag
- **Grep-ability** — can compliance be checked with a shell command? Propose it if yes.
- **Ambiguity** — could two engineers disagree? Flag if yes.

Each directive gets a 1-10 score. Score <5 gets a rewrite suggestion.

Score manual directives to the same standard. Flag soft language, missing `(ref:)`, and conflicts with auto-generated directives.

**Friction risk:** flag directives contradicting common language/framework patterns with a suggested alternative.

### 4. Check Sidecar Staleness

Display progress: `Step 2/3: Checking sidecar staleness...`

v0.6.0+ governance metadata lives in co-located `<artifact>.edikt.yaml` sidecars. Staleness is detected by `bin/edikt gov compile --check`, which compares each of a directive's `source_excerpts[]` entries' recorded `quote` against the current parent `.md` body line range. The legacy v0.2-v0.4 `content_hash:` field is NOT used — its writer/reader hash mismatch made every freshly-compiled file born-stale.

For each ADR reviewed, run the corpus-wide check once and grep for the artifact:

```bash
check_output=$(bin/edikt gov compile --check 2>&1)
adr_stale=$(echo "$check_output" | grep -E "^\s*stale:\s+ADR-${NNN}\b" | head -1)
adr_missing=$(echo "$check_output" | grep -E "ADR-${NNN}.*sidecar missing" | head -1)
```

Report:
```
⚠ Stale sidecar: {file} — directive quote no longer matches parent .md line range.
  Run /edikt:adr:compile ADR-{NNN} to resync.
```
```
⚠ Missing sidecar: {file}
  Run /edikt:adr:compile ADR-{NNN} to generate.
```

If the parent `.md` has a legacy v0.2-v0.4 sentinel block (with `content_hash:`), don't try to re-hash it — `/edikt:doctor` flags such files with a migration prompt; review trusts that migration is the correct remedy and treats the file as missing-sidecar rather than stale-sentinel.

### 5. Output Report

Display progress: `Step 3/3: Generating report...`

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 ADR REVIEW
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ADR-{NNN}: {Title}

  [strong]   "NEVER import infrastructure packages from domain layer — coupling
              the domain to Symfony breaks portability" (Decision §1)
  [adequate] "Use repository interfaces in the domain layer" (Decision §2)
  [weak]     "Try to keep services small"
             → Rewrite: "Application services MUST have a single responsibility.
               If a service method exceeds 30 lines, extract a domain service."
             (Decision §3)
  [vague]    "Follow the established architecture patterns"
             → Rewrite: "NEVER bypass the application layer to access domain
               objects directly from controllers — all state changes go through
               application services." (Decision §4)

  Sentinel: stale — run /edikt:adr:compile ADR-{NNN}

{next ADR}
  ...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ADRs reviewed: {n}
  Statements analyzed: {n}
  Strong: {n} | Adequate: {n} | Weak: {n} | Vague: {n}

  Sentinels:
    Current:  {n}
    Stale:    {n} — run /edikt:adr:compile to regenerate
    Missing:  {n} — run /edikt:adr:compile to generate

  Soft-language warnings: {n}
    {If n > 0}: Run /edikt:adr:review and apply suggested rewrites.
    {Note: --backfill mode is deprecated under v0.6.0 — canonical_phrases is not part of the current sidecar schema.}

  {If weak + vague > 0}:
  Top recommendations:
    1. {most impactful fix}
    2. {second most impactful fix}
    3. {third most impactful fix}

  {If all strong/adequate}:
  All decision statements are enforceable. ADR language is production-grade.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 6. Confirm

```
✅ ADR review complete: {n} ADRs reviewed

Next: Run /edikt:adr:compile to regenerate stale sentinels, then /edikt:gov:compile.
```

---

### 7. Backfill Flow (`--backfill`) — DEPRECATED under v0.6.0

This flow was designed for the pre-v0.6 in-body sentinel architecture: it wrote `canonical_phrases:` into the `[edikt:directives:start]` block. Under v0.6.0, governance metadata lives in the co-located `<name>.edikt.yaml` sidecar — there is no in-body sentinel block to backfill into, and the current sidecar schema (`templates/schemas/gov-sidecar.v2.schema.json`) does NOT currently include a `canonical_phrases` field.

**If `--backfill` is present in `$ARGUMENTS`, short-circuit with this message and exit:**

```
/edikt:adr:review --backfill is deprecated under v0.6.0+.

  Why: canonical_phrases was an in-body-sentinel feature (pre-v0.6).
       v0.6.0 removed in-body sentinels and the current
       sidecar schema doesn't include canonical_phrases.

  Recommended: drop --backfill. Run /edikt:adr:review without flags
               for the v0.6.0 review flow.

  If you need canonical_phrases-style governance, file an issue —
  the right answer is probably a schema extension to add the field
  to sidecars and a new /edikt:adr:enrich-like flow.
```

**Historical reference (do not execute under v0.6.0+):** the original backfill rewrote the in-body sentinel's `canonical_phrases:` field and recomputed `source_hash` + `directives_hash`. That mechanism is incompatible with the v0.6.0 sidecar contract; do not resurrect without a new ADR. The full legacy implementation has been removed from this command — recover from git history if needed (`git log --all -- commands/adr/review.md`).

---

REMEMBER: This command reviews language quality in the ## Decision section only. Rationale, context, and alternatives sections are not reviewed — they are not compiled into governance. The question for every statement is: "If Claude reads this directive, will it know exactly what to do and be able to verify compliance?"

---

## Sidecar Cross-Check

After the language-quality review completes, run a sidecar cross-check on each ADR. The check is read-only and advisory — it surfaces drift between the prose body and the co-located sidecar but does NOT regenerate or modify either file. The user resolves drift by running `/edikt:adr:compile` (sidecar regeneration) or by editing the prose.

For each reviewed ADR at `{decisions_dir}/ADR-NNN-{slug}.md`:

1. **Sidecar presence.** Look for the co-located sidecar at `{decisions_dir}/ADR-NNN-{slug}.edikt.yaml`. If absent, surface:
   ```
   ⚠️  ADR-NNN: no sidecar found — run /edikt:adr:compile ADR-NNN to generate.
   ```
   Skip the remaining checks for that ADR; an absent sidecar is a single resolvable issue.

2. **Quote drift (sidecar → prose).** For every entry in the sidecar's `directives[]`, locate each of its `source_excerpts[]` entries' recorded `quote` verbatim in the prose body between that entry's recorded `line_start` and `line_end`. If any quote is not found verbatim:
   ```
   ⚠️  ADR-NNN: sidecar directive #{i} no longer matches body
       Quote (recorded): "{first 80 chars of quote}…"
       Recorded location: lines {line_start}–{line_end}
       Hint: prose body has been edited; run /edikt:adr:compile ADR-NNN to resync.
   ```

3. **Missing directives (prose → sidecar).** Scan the ADR's `## Decision` section for imperative sentences (containing MUST, MUST NOT, SHOULD, NEVER, ALWAYS) that are not represented in the sidecar's `directives[].text`. For any not represented:
   ```
   ⚠️  ADR-NNN: prose body contains imperative directive not in sidecar
       Sentence: "{first 80 chars}…" (line {n})
       Hint: run /edikt:adr:compile ADR-NNN to refresh the sidecar.
   ```
   Use string containment as a coarse match (a sidecar directive whose `text` shares ≥60% of the prose sentence's tokens, or whose recorded `source_excerpts[]` entries overlap the sentence's line range, is "represented"). Avoid exact-string equality — sidecar `text` fields include the parenthetical `(ref: ADR-NNN)` tail that the prose body does not.

4. **In-sync confirmation.** If steps 1–3 surface no findings for an ADR:
   ```
   ✅ ADR-NNN: sidecar in sync
   ```

The cross-check runs after the language-quality review's existing output. It does NOT modify any file. The user resolves drift via `/edikt:adr:compile` or by editing the prose.
