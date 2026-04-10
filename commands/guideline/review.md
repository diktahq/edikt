---
name: edikt:guideline:review
description: "Review guideline language quality for enforceability"
effort: normal
argument-hint: "[filename] — omit to review all guidelines"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
---

# edikt:guideline:review

Review guideline files for language quality. Checks whether rules are specific, actionable, and phrased correctly to achieve reliable compliance when Claude reads them.

This is the same quality check as `/edikt:gov:review` applied specifically to guideline files. Every weak or vague rule gets a concrete rewrite.

## Arguments

- `$ARGUMENTS` — optional filename (with or without `.md`). If no argument, reviews all guidelines.

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve:
- Guidelines directory: `paths.guidelines` (default: `docs/guidelines`)

### 2. Determine Scope

**With `$ARGUMENTS`** — locate `{guidelines_dir}/{filename}` (try with and without `.md`). If not found:
```
Guideline not found: {filename}
Available guidelines: {list files in guidelines_dir}
```

**Without `$ARGUMENTS`** — glob all `.md` files in `{guidelines_dir}`.

If no guideline files found:
```
No guidelines found in {guidelines_dir}.
Run /edikt:guideline:new to create your first guideline.
```

### 3. Review Each File

Display progress: `Reviewing {n} guideline(s)...`

For each guideline file:

1. Read all rules from the Rules section (bullet points starting with `-`).
2. Score each rule against the Quality Criteria (same as `/edikt:gov:review`):
   - **Specificity** — does it name the exact thing to do or avoid?
   - **Actionability** — can Claude follow this without interpretation?
   - **Phrasing** — does it use MUST/NEVER for hard constraints?
   - **Testability** — can compliance be verified?
3. A rule is the weakest rating it receives on any dimension.
4. For each `weak` or `vague` rule, provide a concrete rewrite.

Also check document-level issues:
- Rules using soft language ("should", "prefer", "try to", "consider") — flag each one
- Rules that are too broad to be verifiable — flag each one
- Missing purpose statement — flag if absent

### 3b. Review Compiled Directives (LLM Compliance)

If the guideline has a `[edikt:directives:start]: #` sentinel block, score each compiled directive for LLM compliance. For each directive in `directives:` AND `manual_directives:`, score on:

- **Token specificity** — 0 backtick tokens = Low, 1-2 = Medium, 3+ = High
- **Length** — <10w flag, 10-30w good, 30-50w check splittable, >50w split
- **MUST/NEVER** — present = pass; absent = flag (guidelines that survived soft-language rejection should all have MUST/NEVER)
- **Grep-ability** — can compliance be checked with a shell command? Propose it if yes.
- **Ambiguity** — could two engineers disagree? Flag if yes.

Each directive gets a 1-10 score. Score <5 gets a rewrite suggestion. Score manual directives to the same standard.

### 4. Output Report

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 GUIDELINE REVIEW
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

{guideline filename} ({n} rules)

  [strong]   "NEVER log PII — mask emails and phone numbers before writing to logs"
  [adequate] "Use structured logging in all services"
  [weak]     "Try to keep functions small"
             → Rewrite: "Functions MUST be under 50 lines. Extract helpers when
               a function does more than one thing."
  [vague]    "Write good error messages"
             → Rewrite: "Error messages MUST include: what failed, why it failed,
               and what the user can do. Never expose internal stack traces."

  Document-level:
  [!!] Rule 3 uses soft language ("should avoid") — rewrite as MUST/NEVER
  [ok] Purpose statement present

{next guideline}
  ...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 Summary
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Guidelines reviewed: {n}
  Rules analyzed: {n}
  Strong: {n} | Adequate: {n} | Weak: {n} | Vague: {n}

  {If weak + vague > 0}:
  Top recommendations:
    1. {most impactful fix}
    2. {second most impactful fix}
    3. {third most impactful fix}

  {If all strong/adequate}:
  All rules are enforceable. Guideline language is production-grade.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 5. Confirm

```
✅ Guideline review complete

Next: Run /edikt:gov:compile to recompile after updates.
```

---

REMEMBER: Every finding must cite the specific rule text that fails the check and provide a concrete rewrite. Never flag a rule without showing how to fix it. The question is: "If Claude reads this rule, will it know exactly what to do?"
