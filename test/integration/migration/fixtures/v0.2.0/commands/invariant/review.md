---
name: edikt:invariant:review
description: "Review invariant language quality for enforceability"
effort: high
argument-hint: "[INV-NNN] — omit to review all active invariants"
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# edikt:invariant:review

Review invariants for language quality in the Rule section. Checks whether rule statements are specific, actionable, and phrased correctly to achieve reliable compliance when compiled into non-negotiable governance directives.

Invariants compile into the top and bottom of every governance file — they carry the highest compliance weight. Vague invariants are especially harmful.

CRITICAL: Every finding must cite the specific text that fails the check and provide a concrete rewrite. Never flag a rule without showing how to fix it.

## Arguments

- `$ARGUMENTS` — optional invariant ID (e.g., `INV-002`). If no argument, reviews all active invariants.

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

**With `$ARGUMENTS`** — locate the invariant file matching the given ID. If not found:
```
Invariant not found: {id}
Run: ls {invariants_path}/*.md to see available invariants.
```

**Without `$ARGUMENTS`** — glob all `*.md` files in `{invariants_path}`. Include `status: active` and files with no status field. Skip `status: revoked`.

If no active invariants found:
```
No active invariants found in {invariants_path}.
```

### 3. Review Each Invariant

Display progress: `Step 1/3: Analyzing invariant language quality...`

For each invariant:

1. Read the `## Rule` section (and the opening sentence beneath the title if there is no Rule section).
2. Extract all rule statements — sentences or bullets using MUST, NEVER, or similar constraint language.
3. Score each statement against the Quality Criteria (below) on four dimensions: Specificity, Actionability, Phrasing, Testability. A statement is the weakest rating it receives on any dimension.
4. For each statement rated `weak` or `vague`, provide a concrete rewrite.

Also check the `## Verification` section:
- If absent: flag as missing — invariants without verification criteria cannot be enforced reliably.
- If present but vague ("review the code"): flag and provide a concrete rewrite.

### Quality Criteria

**1. Specificity**

| Rating | Definition |
|---|---|
| Strong | Names specific file types, packages, tools, or patterns |
| Adequate | Describes the constraint clearly without exact syntax |
| Weak | Uses subjective terms without measurable criteria |
| Vague | Could mean anything to different readers |

**2. Actionability**

| Rating | Definition |
|---|---|
| Strong | One clear prohibition or requirement, no ambiguity |
| Adequate | Clear intent, minor interpretation needed |
| Weak | Multiple interpretations possible |
| Vague | No actionable instruction |

**3. Phrasing**

| Rating | Definition |
|---|---|
| Strong | NEVER/MUST (uppercase) with one-clause consequence or reason |
| Adequate | Clear imperative without emphasis marker |
| Weak | Soft language ("should", "try to") for a hard constraint |
| Vague | No imperative, reads as suggestion |

**4. Testability**

| Rating | Definition |
|---|---|
| Strong | Verifiable by grep, CI check, or code review with specific criteria |
| Adequate | Verifiable by reading with clear criteria |
| Weak | Requires subjective judgment to verify |
| Vague | Cannot be verified |

### 4. Check Sentinel Staleness

Display progress: `Step 2/3: Checking sentinel staleness...`

For each invariant reviewed:

1. Look for `[edikt:directives:start]: #` in the file.
2. If present: compute MD5 of content above the sentinel start. Compare with stored `content_hash:`.
   - Match: current
   - Mismatch: stale
3. If absent: missing

Report:
```
⚠ Stale sentinel: {file} — content changed since last compile.
  Run /edikt:invariant:compile INV-{NNN} to regenerate.
```
```
⚠ Missing sentinel: {file}
  Run /edikt:invariant:compile INV-{NNN} to generate.
```

### 5. Output Report

Display progress: `Step 3/3: Generating report...`

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 INVARIANT REVIEW
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

INV-{NNN}: {Title}

  [strong]   "Every command MUST be a plain .md file — NEVER compiled code,
              NEVER a build step." (Rule §1)
  [adequate] "No runtime dependencies" (Rule §2)
  [weak]     "Code should be well-structured"
             → Rewrite: "Domain layer MUST NOT import from infrastructure
               packages — NEVER import Symfony\\*, Doctrine\\*, or HTTP types
               into domain classes." (Rule §3)
  [vague]    "Keep things simple"
             → This is a preference, not an invariant. Remove it or move to
               docs/guidelines/ as a guideline rule.

  Verification: missing
  → Add a ## Verification section with a concrete check:
    "Automated: grep -r 'use Symfony\\\\' src/Domain/ — must return no results"

  Sentinel: current

{next invariant}
  ...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Invariants reviewed: {n}
  Statements analyzed: {n}
  Strong: {n} | Adequate: {n} | Weak: {n} | Vague: {n}

  Sentinels:
    Current:  {n}
    Stale:    {n} — run /edikt:invariant:compile to regenerate
    Missing:  {n} — run /edikt:invariant:compile to generate

  {If weak + vague > 0}:
  Top recommendations:
    1. {most impactful fix}
    2. {second most impactful fix}
    3. {third most impactful fix}

  {If all strong/adequate}:
  All invariant statements are enforceable. Invariant language is production-grade.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 6. Confirm

```
✅ Invariant review complete: {n} invariants reviewed

Next: Run /edikt:invariant:compile to regenerate stale sentinels, then /edikt:gov:compile.
```

---

REMEMBER: Invariants are non-negotiable constraints — they appear at the top and bottom of every governance file and carry the highest compliance weight. A vague invariant degrades the entire governance system. The Verification section is required: without it, there's no way to confirm the invariant is being honored. If a statement uses soft language ("should", "prefer"), it is not an invariant — flag it for removal or migration to guidelines.
