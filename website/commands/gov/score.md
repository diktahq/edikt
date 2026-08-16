# /edikt:gov:score

Score the overall quality of compiled governance output. Measures context budget, LLM compliance across all directives, manual directive health, and surfaces the weakest links.

## Usage

```bash
/edikt:gov:score
/edikt:gov:score --json
```

## Arguments

| Argument | Description |
|----------|-------------|
| (none) | Human-readable governance quality report |
| `--json` | JSON output for CI integration |

## What it measures

Scoring reads the render manifest (`.claude/rules/governance/manifest.yaml`) and scores exactly the surfaces it lists — the ambient core, topic files, skill packages, and the directive index — rather than walking a directory, so it can't silently score a set nobody rendered. If the manifest is absent, `gov:score` says so and scores nothing rather than falling back to a walk.

### Context budget

Total tokens across the **ambient surfaces only** — the ambient core (`.claude/rules/governance.md`) plus anything else loaded on every edit. Topic files load conditionally (only when a touched path matches their `paths:` frontmatter), and skill packages and the directive index are trigger- or write-time-loaded, not ambient cost — none of those count against this budget, since counting them would report a cost nobody actually pays on a typical edit.

| Budget | Rating |
|--------|--------|
| <1000 tokens | Lean |
| 1000-2000 | OK |
| 2000-4000 | Heavy |
| >4000 | Warning |

### LLM compliance metrics

Each directive is scored on:

- **Token specificity** — literal code tokens (backtick-wrapped identifiers, function names). High = 3+, Medium = 1-2, Low = 0.
- **MUST/NEVER** — hard constraint language present.
- **Grep-ability** — can compliance be checked with a shell command.
- **"No exceptions."** — present on invariant directives with absolute language.

### Manual directive health

Manual directives bypass compile quality checks. This command scores them to the same standard — flags soft language, missing references, and conflicts with auto-generated directives.

### Reminders and checklist

Counts items in each topic file's `## Reminders` and `## Verification Checklist` sections — these sections live per topic file, not in the ambient core, which carries neither. Flags when empty (missing) or when exceeding caps (10 reminders, 15 checklist items).

## Example output

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 GOVERNANCE QUALITY REPORT
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Context budget: 847 tokens — OK
Sources: 7 ADR, 2 INV, 3 guidelines
Directives: 18 auto + 3 manual = 21 total

LLM Compliance:
  Token specificity:  18/21 high
  MUST/NEVER:         19/21 (90%)
  Grep-able:          15/21 (71%)
  "No exceptions.":   4/5 invariant directives
  Average score:      7.8/10

Reminders: 6 items
Checklist: 8 items

Manual Directive Health:
  Passing quality:    1/3
  Needs rewrite:      2/3

Overall: 7.8/10
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

## When to run

- After `/edikt:gov:compile` to verify output quality
- In CI to enforce a minimum governance score
- Periodically to catch governance drift as the project grows

## Related commands

- [`/edikt:gov:compile`](compile) — generate the governance files this command scores
- [`/edikt:gov:review`](review) — review governance for contradictions and language quality
- [`/edikt:invariant:review`](/commands/invariant/review) — per-invariant quality scoring (human-side + directive-side)
- [`/edikt:adr:review`](/commands/adr/review) — per-ADR quality scoring
