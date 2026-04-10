# /edikt:guideline:new

Capture a team guideline — a coding standard or best practice that should be consistently followed but doesn't rise to the level of an invariant.

## Usage

```bash
/edikt:guideline:new all API responses use camelCase keys
/edikt:guideline:new                         ← extracts from current conversation
```

## Guidelines vs invariants

| | Invariant | Guideline |
|--|-----------|-----------|
| **Violation** | Causes real harm (data loss, security breach, domain corruption) | Breaks consistency, creates tech debt |
| **Enforcement** | Non-negotiable | Strong preference |
| **When to capture** | Hard rules with consequences | Standards and team conventions |

If your rule uses "NEVER" and violation would cause real harm, use [`/edikt:invariant:new`](/commands/invariant/new) instead.

## Two modes

### With argument — define from scratch

```bash
/edikt:guideline:new all API responses use camelCase keys
```

edikt creates the guideline with clear language about what it applies to, when to follow it, and any exceptions.

Creates: `docs/guidelines/guideline-{slug}.md`

### No argument — extract from conversation

```bash
/edikt:guideline:new
```

Extracts the last team standard or coding convention discussed in the current conversation.

## Template

edikt uses a template to structure the guideline. The template lookup chain:

1. **Project override** — `.edikt/templates/guideline.md` (if present)
2. **edikt default** — built-in template

The default template produces:

```markdown
# {topic-name}

## Purpose
## Rules            ← compile reads this (MUST/NEVER language)
## Examples
## When NOT to apply

[edikt:directives:start]: #
[edikt:directives:end]: #
```

The `## Rules` section is what compile reads. Each bullet must use MUST or NEVER — soft language ("should", "prefer") is rejected by compile with a warning. See [Guidelines](/governance/guidelines) for details.

## Output

```text
docs/guidelines/
└── guideline-api-response-casing.md
```

After creating the guideline, edikt automatically runs `/edikt:guideline:compile` to generate the directive sentinel block. Your new guideline is immediately ready for `/edikt:gov:compile`.

## Natural language triggers

- "let's add a guideline for X"
- "capture this as a team convention"
- "we should always do X"
- "add a coding standard for X"

## What's next

- [/edikt:guideline:compile](/commands/guideline/compile) — compile into governance directives
- [/edikt:guideline:review](/commands/guideline/review) — review language quality + directive LLM compliance
- [Guidelines](/governance/guidelines) — what they are, when to use, vs ADRs vs invariants
- [Extensibility](/governance/extensibility) — manual directives, suppressed directives, template overrides
- [/edikt:gov:compile](/commands/gov/compile) — compile all governance into enforcement files
