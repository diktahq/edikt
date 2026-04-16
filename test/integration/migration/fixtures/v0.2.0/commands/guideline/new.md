---
name: edikt:guideline:new
description: "Create a new team guideline file"
effort: normal
argument-hint: "[guideline topic] — omit to be prompted"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---

# edikt:guideline:new

Create a team guideline — a set of enforceable conventions for a specific topic. Guidelines are softer than invariants (they allow team discussion) but harder than suggestions (every rule uses MUST or NEVER).

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

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

### 2. Determine Topic

**With `$ARGUMENTS`** — use the provided topic directly.

**Without `$ARGUMENTS`** — ask:
```
What is this guideline about? (e.g. error-handling, testing, logging, api-design)
```

### 3. Interview

Ask 2–3 focused questions to fill out the guideline:
1. What problem does this guideline solve? (one sentence)
2. What are the 3–5 most important rules? (Each will become a MUST or NEVER statement.)
3. Do you have an example of the right way to do it? (optional)

After gathering answers, draft the guideline. If the rules use soft language ("should", "prefer", "try to"), rewrite them as MUST/NEVER statements. If a rule cannot be rewritten as MUST/NEVER, it's a suggestion — omit it or ask the user to strengthen it.

### 4. Validate Language

Before writing, check every rule in the Rules section:
- Hard rules MUST use MUST or NEVER (uppercase)
- Each rule must be specific enough to be verifiable — name exact tools, patterns, or thresholds
- No "prefer", "try to", "consider", "aim to" language

If a rule is too weak to enforce, flag it:
```
⚠ This rule uses soft language: "{rule text}"
  Rewrite as: "{stronger rewrite}"
  Keep as-is, use the rewrite, or omit?
```

### 5. Write the Guideline

Derive a slug from the topic (lowercase, hyphens). Create `{guidelines_dir}/{slug}.md`:

```markdown
# {Topic Title} Guidelines

**Purpose:** {One sentence — what problem this guideline prevents.}

## Rules

- {Rule using MUST or NEVER — specific and verifiable}
- {Rule using MUST or NEVER — specific and verifiable}
- {Rule using MUST or NEVER — specific and verifiable}

## Examples

### Correct

{Code or prose example showing the right approach — omit section if no example provided}

### Incorrect

{Code or prose example showing what to avoid — omit section if no example provided}

---

*Created by edikt:guideline — {date}*
```

### 6. Confirm

```
✅ Guideline created: {guidelines_dir}/{slug}.md

  Topic: {Topic Title}
  Rules: {n}

  Next: Run /edikt:gov:compile to include this guideline in governance.
```

---

REMEMBER: Guidelines are enforceable conventions, not suggestions. Every rule in the Rules section must use MUST or NEVER language. Soft language belongs in internal documentation, not in a guideline file that compiles into governance.
