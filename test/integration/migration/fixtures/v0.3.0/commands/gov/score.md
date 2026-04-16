---
name: edikt:gov:score
description: "Score overall governance quality — context budget, directive compliance metrics, manual directive health"
effort: normal
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

# edikt:gov:score

Score the overall quality of the compiled governance output. Measures context budget, LLM compliance across all directives, manual directive health, and surfaces the weakest links.

Designed for CI integration and periodic governance health checks.

CRITICAL: This command reads the compiled output — it does NOT score source documents. Run `/edikt:invariant:review` or `/edikt:adr:review` for per-artifact scoring. This command gives the aggregate view.

## Arguments

- `--json` — output JSON only (for CI)
- No arguments — human-readable report

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist:
```
No edikt config found. Run /edikt:init to set up this project.
```

### 1. Read Compiled Governance

Read `.claude/rules/governance.md` (the index) and all files in `.claude/rules/governance/` (topic files).

If no governance files exist:
```
No compiled governance found. Run /edikt:gov:compile first.
```

### 2. Extract All Directives

From governance.md and each topic file, extract every directive line (lines starting with `- ` that end with `(ref: ...)`).

Classify each as:
- **Invariant directive** — appears in `## Non-Negotiable Constraints` section
- **ADR directive** — appears in topic files with `(ref: ADR-NNN)`
- **Guideline directive** — appears in topic files with other refs
- **Manual directive** — identified by `(manual)` tag or presence in a `manual_directives:` block in the source

### 3. Score Each Directive

For each directive, compute:

| Metric | How |
|---|---|
| Token specificity | Count backtick-wrapped tokens: 0=Low, 1-2=Medium, 3+=High |
| Length | Word count: <10=short, 10-30=good, 30-50=check, >50=split |
| MUST/NEVER | Uppercase MUST or NEVER present? |
| Grep-ability | Could a grep/shell command verify this? |
| "No exceptions." | Present on invariant directives with absolute language? |

Aggregate into:
- Per-metric pass rates
- Average score (1-10) across all directives
- List of directives scoring <5

### 4. Compute Context Budget

Count total tokens in governance.md + all topic files. Use approximate word-to-token ratio of 1.3.

| Budget | Rating |
|---|---|
| <1000 tokens | Lean |
| 1000-2000 | OK |
| 2000-4000 | Heavy — consider tightening |
| >4000 | Warning — governance competing with task context |

### 5. Check Reminders and Checklist

Count items in `## Reminders` and `## Verification Checklist` sections.
- 0 reminders → flag: "No reminders. Run /edikt:invariant:compile and /edikt:adr:compile to generate."
- 0 checklist items → flag: "No verification checklist."
- >10 reminders → flag: "Too many reminders. Cap at 10 for focus."
- >15 checklist items → flag: "Too many checklist items. Cap at 15."

### 6. Output Report

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 GOVERNANCE QUALITY REPORT
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Context budget: {n} tokens — {rating}
Sources: {n} ADR, {m} INV, {g} guidelines
Directives: {a} auto + {m} manual = {total} total

LLM Compliance:
  Token specificity:  {h}/{total} high, {m}/{total} medium, {l}/{total} low
  MUST/NEVER:         {n}/{total} ({pct}%)
  Grep-able:          {n}/{total} ({pct}%)
  "No exceptions.":   {n}/{invariant_total} invariant directives
  Average score:      {x}/10

Reminders: {n} items
Checklist: {n} items

Manual Directive Health:
  Total:              {n}
  Passing quality:    {p}/{n}
  Needs rewrite:      {r}/{n}

{If any directives score <5}:
Weakest directives (score <5):
  1. "{directive text}" — score {x}/10 — {reason}
     ⚠ Rewrite: "{suggested}"
  2. ...

Overall: {x}/10

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

### 7. JSON Output (if `--json`)

```json
{
  "context_budget": {"tokens": 847, "rating": "OK"},
  "sources": {"adrs": 7, "invariants": 2, "guidelines": 3},
  "directives": {"auto": 18, "manual": 3, "total": 21},
  "compliance": {
    "token_specificity": {"high": 15, "medium": 4, "low": 2},
    "must_never": {"count": 19, "total": 21, "pct": 90},
    "grepable": {"count": 15, "total": 21, "pct": 71},
    "no_exceptions": {"count": 4, "invariant_total": 5},
    "average_score": 7.8
  },
  "reminders": 6,
  "checklist": 8,
  "manual_health": {"total": 3, "passing": 1, "needs_rewrite": 2},
  "weakest": [
    {"directive": "...", "score": 3, "reason": "no code tokens, soft language"}
  ],
  "overall_score": 7.8
}
```

---

REMEMBER: This command scores the COMPILED output, not source documents. It answers: "How well will Claude follow our governance?" Run per-artifact reviews for source quality. Run this for the aggregate picture. Designed for CI — the `--json` output can be parsed by any CI tool.
