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

## Output

```text
docs/guidelines/
└── guideline-api-response-casing.md
```

Guidelines are compiled into topic rule files by [`/edikt:gov:compile`](/commands/gov/compile) and become part of Claude's active governance.

## Natural language triggers

- "let's add a guideline for X"
- "capture this as a team convention"
- "we should always do X"
- "add a coding standard for X"

## What's next

- [/edikt:guideline:review](/commands/guideline/review) — review guideline language quality
- [/edikt:gov:compile](/commands/gov/compile) — compile guidelines into governance directives
