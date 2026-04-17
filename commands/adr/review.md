---
name: edikt:adr:review
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

- `$ARGUMENTS` — optional ADR ID (e.g., `ADR-003`). If no argument, reviews all accepted ADRs.
- `--backfill` — interactive mode: propose `canonical_phrases` for existing multi-sentence directives that have none, writing only with per-ADR approval.

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

If the ADR has a `[edikt:directives:start]: #` sentinel block, score each compiled directive for LLM compliance. For each directive in `directives:` AND `manual_directives:`, score on:

- **Token specificity** — 0 backtick tokens = Low, 1-2 = Medium, 3+ = High
- **Length** — <10w flag, 10-30w good, 30-50w check splittable, >50w split
- **MUST/NEVER** — present = pass; absent = flag
- **Grep-ability** — can compliance be checked with a shell command? Propose it if yes.
- **Ambiguity** — could two engineers disagree? Flag if yes.

Each directive gets a 1-10 score. Score <5 gets a rewrite suggestion.

Score manual directives to the same standard. Flag soft language, missing `(ref:)`, and conflicts with auto-generated directives.

**Friction risk:** flag directives contradicting common language/framework patterns with a suggested alternative.

### 4. Check Sentinel Staleness

Display progress: `Step 2/3: Checking sentinel staleness...`

For each ADR reviewed:

1. Look for `[edikt:directives:start]: #` in the file.
2. If present: compute MD5 of content above the sentinel start. Compare with stored `content_hash:`.
   - Match: current
   - Mismatch: stale
3. If absent: missing

Report:
```
⚠ Stale sentinel: {file} — content changed since last compile.
  Run /edikt:adr:compile ADR-{NNN} to regenerate.
```
```
⚠ Missing sentinel: {file}
  Run /edikt:adr:compile ADR-{NNN} to generate.
```

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
    {If n > 0 and any directive has canonical_phrases missing}:
      Consider: /edikt:adr:review --backfill to add canonical_phrases in bulk.

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

### 7. Backfill Flow (`--backfill`)

This section runs **instead of** sections 3–6 when `$ARGUMENTS` contains `--backfill`.

**Purpose:** Retrofit `canonical_phrases` onto existing accepted ADRs that have multi-sentence directives but no canonical phrases. Writes only with explicit per-ADR user approval.

**Eligibility criteria.** An ADR directive is eligible for backfill when ALL of the following are true:
1. The ADR has `status: accepted`.
2. The sentinel block exists and `canonical_phrases:` is empty (`[]` or absent).
3. The directive body (after stripping the `(ref: …)` tail) contains **more than one declarative sentence** — split on `. ` or `; `.

Single-sentence directives are **not eligible**. Directives already having `canonical_phrases` populated are **not eligible**. Skip them silently.

**Phrase proposal heuristic.** For each eligible directive, extract 2–3 candidate phrases using this heuristic (applied to the directive body with the `(ref: …)` tail stripped):
1. **Pre-MUST/NEVER words:** any word or short phrase (1–3 words) immediately preceding `MUST` or `NEVER` in the directive body.
2. **Quoted terms:** any text inside double or backtick quotes in the directive body.
3. **Key nouns:** the 2 most specific nouns in the directive (prefer technical nouns: tool names, file types, layer names, identifiers). Avoid generic words like "code", "rule", "directive".

Limit proposed phrases to 2–3 items. If the heuristic yields more, prefer shorter phrases (≤ 3 words each).

**Interactive flow per eligible ADR:**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 BACKFILL: ADR-{NNN} — {Title}
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Directive:
  "{first 200 chars of directive body}..."

Proposed canonical_phrases (heuristic: noun/verb extraction):
  1. "{phrase 1}" — {why: e.g., "pre-MUST word"}
  2. "{phrase 2}" — {why: e.g., "quoted term"}
  3. "{phrase 3}" — {why: e.g., "key noun"}

[y]es apply / [n]o skip / [e]dit phrases >
```

On `y`: write the proposed phrases into the sentinel block's `canonical_phrases:` field and recompute `source_hash` + `directives_hash`.
On `n`: skip this ADR, move to next eligible.
On `e`: prompt `Enter phrases (one per line; blank line to finish):`. Accept user-typed phrases. Then re-show the directive with the edited phrases and prompt `Apply? [y/n] >`. Apply on `y`, skip on `n`.

**Writing the sentinel block.** When applying canonical_phrases:
1. Locate the `[edikt:directives:start]: #` block in the ADR file.
2. Append `canonical_phrases:` after `suppressed_directives:` (or at the end of the list fields, before `source_hash`).
3. Recompute `source_hash` as SHA-256 of the file content with the sentinel block excluded (normalized: CRLF→LF, trailing whitespace stripped per line, trailing blank lines stripped). Use the same normalization as `test/integration/governance/test_adr_sentinel_integrity.py::_body_without_block`.
4. Recompute `directives_hash` as SHA-256 of the joined `directives:` list items (unchanged — directives were not modified).
5. Write the updated file.

**Post-write integrity check.** After writing, confirm the updated sentinel block would pass the existing integrity tests:
- `source_hash` matches freshly computed hash of the updated body.
- `directives_hash` matches the directives list (unchanged).

If the integrity check fails, report the mismatch and **do not write** (or roll back if already written).

**Backfill Summary:**

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 BACKFILL COMPLETE
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Eligible ADRs found:  {n}
  Applied:  {n_applied}
  Skipped:  {n_skipped}
  Edited:   {n_edited}
  Not eligible (single-sentence or already has phrases): {n_ineligible}

  {If n_applied > 0}:
  Updated files:
    {list of ADR filenames}

  Run /edikt:gov:compile to propagate the new canonical_phrases.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

---

REMEMBER: This command reviews language quality in the ## Decision section only. Rationale, context, and alternatives sections are not reviewed — they are not compiled into governance. The question for every statement is: "If Claude reads this directive, will it know exactly what to do and be able to verify compliance?"
