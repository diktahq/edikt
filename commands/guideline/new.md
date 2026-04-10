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

### 1a. Resolve Template (lookup chain)

edikt v0.3.0+ supports project-level template overrides per [ADR-005](../../docs/architecture/decisions/ADR-005-extensibility-model.md). Follow this precedence when selecting the template for the new guideline:

1. **Project template**: if `.edikt/templates/guideline.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Inline fallback (v0.2.x legacy projects only)**: if no project template exists AND the project's `edikt_version` in `.edikt/config.yaml` is `< 0.3.0` or missing, use the inline template shown in step 5 below. Print a one-time warning:
   ```
   ⚠ No project guideline template found. Using the legacy inline fallback.
     Run /edikt:upgrade followed by /edikt:init to set up project templates.
   ```
3. **Refuse (v0.3.0+ projects with missing templates)**: if no project template exists AND the project's `edikt_version` is `>= 0.3.0`, refuse:
   ```
   ❌ No project guideline template found.

   This project is on edikt v{version}, which requires an explicit
   project template. edikt doesn't assume a style — your project owns this.

   To set up templates, run:
     /edikt:init                     (interactive setup — pick Adapt,
                                      Start fresh, or Write my own)
     /edikt:init --reset-templates   (regenerate templates)

   Or create .edikt/templates/guideline.md manually with at least a
   `## Rules` section using MUST/NEVER language and the
   [edikt:directives:start]: # / [edikt:directives:end]: # sentinel block.
   ```
   Do NOT fall back to inline. Do NOT write the guideline. Exit.

**No global default**: edikt does NOT ship a "default guideline template" that is auto-installed. Projects explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only.

**On terminology**: unlike ADRs and Invariant Records, guidelines use the standard term "guideline" — no formal short form needed.

**Checking the project edikt_version**: extract it from `.edikt/config.yaml`:
```bash
PROJECT_EDIKT_VERSION=$(grep '^edikt_version:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
```
Compare to `0.3.0` using semver ordering. A missing `edikt_version` line means v0.2.x legacy.

### 2. Determine Topic — flexible prose input with reference extraction

The argument is **always prose first**, then mined for embedded references. This is the same pattern `/edikt:sdlc:plan` has used since v0.1.3. Do NOT classify the input into rigid types (path vs identifier vs prose) — treat the whole argument as natural language and scan it for things that resolve to content.

#### 2a. Empty argument → ask

**If `$ARGUMENTS` is empty** — ask:
```
What is this guideline about? (e.g. error-handling, testing, logging, api-design)

Or give me a document or identifier to derive the guideline from:
  /edikt:guideline:new "error-handling using docs/style/errors.md"
  /edikt:guideline:new docs/style/logging-conventions.md
  /edikt:guideline:new ADR-042
```

The user's answer becomes the new `$ARGUMENTS` — re-run this section with the answer.

#### 2b. Non-empty argument → extract embedded references

Treat `$ARGUMENTS` as prose. Scan it for references of three kinds, resolving each to content that feeds the guideline body:

**Reference kind 1: file paths**

Any substring in the prose that looks like a file path AND resolves to an existing file. Detection: a token containing at least one `/` OR ending in a common code/doc extension (`.md`, `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rb`, `.php`, `.rs`, `.java`, `.kt`, `.sql`, `.yaml`, `.yml`, `.toml`, `.json`). Verify existence before accepting.

For guidelines specifically, file references often point at:
- **Existing style docs** — the team's informal notes that need formalizing
- **Code examples** — specific files that exemplify the pattern or anti-pattern
- **Linter configs** — `.golangci.yml`, `.eslintrc`, etc. that document the conventions in machine-readable form
- **Spec documents** — requirements that drive the guideline

For each path that resolves: read the file and add it to the source pool.

**Reference kind 2: identifiers**

Tokens matching edikt artifact ID patterns:
- `ADR-NNN` — often the decision that motivates the guideline
- `INV-NNN` — related hard constraint (guidelines are softer than invariants)
- `SPEC-NNN`, `PRD-NNN` — product requirements
- `PLAN-NNN` — implementation plan

Read `paths:` from `.edikt/config.yaml` to resolve directories. If an identifier doesn't resolve, treat as plain prose — do NOT error.

**Reference kind 3: branch names**

Tokens matching `{prefix}/{name}` where `{prefix}` is `feature`, `feat`, `fix`, `hotfix`, `refactor`, `chore`, `docs`, `release`, `dev`, `spike`, `experiment`. Verify with `git rev-parse --verify`. If the branch exists, read its diff and relevant commits. If git isn't available or the branch doesn't exist, treat as plain prose.

#### 2c. Determine topic slug and build the source pool

After scanning:

- **Topic slug**: derive from the framing prose. Strip references, lowercase, replace non-alphanumeric with hyphens, collapse consecutive hyphens. Examples:
  - `"error-handling using docs/style/errors.md"` → `error-handling`
  - `"logging conventions"` → `logging-conventions`
  - `"api-design from ADR-042"` → `api-design`
  - If the prose is entirely a file path, use the filename (without extension) as the slug candidate.
- **Source pool**: concatenated content of resolved references.
- **Framing prose**: the full `$ARGUMENTS` string, used to set scope and tone.

### 3. Interview

Based on the source pool, identify what's missing:

**If the source pool is empty** (no references resolved) — ask the classic 2-3 questions:
1. What problem does this guideline solve? (one sentence)
2. What are the 3–5 most important rules? (Each will become a MUST or NEVER statement.)
3. Do you have an example of the right way to do it? (optional)

**If the source pool has content** — extract rules directly from it. For each candidate rule found in the source pool, validate it uses MUST or NEVER language. If it doesn't, rewrite it into MUST/NEVER form. Ask the user for confirmation on each extracted rule:
```
Extracted these rules from {source}:
  1. {rule} → MUST/NEVER form: "{rewritten rule}"
  2. {rule} → MUST/NEVER form: "{rewritten rule}"

Accept these as-is, edit, or add more? [accept/edit/add]
```

Interview only for what's missing — if the source pool covers everything, proceed to 4 (Validate Language) with the extracted rules.

After gathering answers (either via interview or via extraction), draft the guideline. If any rule uses soft language ("should", "prefer", "try to"), rewrite it as MUST/NEVER. If a rule cannot be rewritten as MUST/NEVER, it's a suggestion — omit it or ask the user to strengthen it.

**Examples:**

| Input | Behavior |
|---|---|
| `/edikt:guideline:new` (empty) | Ask for the topic interactively |
| `/edikt:guideline:new "error-handling"` | Pure topic name, full interview |
| `/edikt:guideline:new "error-handling using docs/style/errors.md"` | Topic + embedded path, read the file, extract rules, validate MUST/NEVER |
| `/edikt:guideline:new docs/style/logging.md` | Pure path, read the file, derive slug from filename, extract rules |
| `/edikt:guideline:new ADR-042` | Identifier, read the ADR, derive guideline from its Decision section |
| `/edikt:guideline:new "api-design per ADR-055 and docs/openapi.yaml"` | Prose with multiple refs, read all, extract rules |

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

### 6. Auto-chain to /edikt:guideline:compile (ADR-008)

Per ADR-008, this command auto-chains to `/edikt:guideline:compile {slug}` at the end of its workflow so the newly-created guideline has its directive sentinel block populated immediately. Fresh artifacts have nothing to preserve (no manual directives, no suppressed directives, no hand-edits), so the compile runs the slow path cleanly and the user never has to remember to compile after new.

Run `/edikt:guideline:compile {slug}` now. If it produces an error (e.g., headless mode with strategy flags), surface the error but do NOT roll back the guideline creation — the body is already written and the user can run compile manually later.

If the compile succeeds, the directive block is populated with:
- `source_hash` — SHA-256 of the guideline body (excluding the block)
- `directives_hash` — SHA-256 of the auto `directives:` list
- `compiler_version` — current edikt version
- `directives:` — auto-generated from the `## Rules` section (only MUST/NEVER bullets; soft-language bullets are skipped with a warning)
- `manual_directives: []` — empty; user adds rules compile missed
- `suppressed_directives: []` — empty; user adds rules compile got wrong

### 7. Confirm

```
✅ Guideline created: {guidelines_dir}/{slug}.md
✅ Directives compiled: {k} auto directives ({s} soft rules skipped)

  Topic: {Topic Title}
  Rules: {n}

  To refine the directives:
  - Add rules compile missed → edit manual_directives: in the block
  - Reject wrong auto rules → add to suppressed_directives: in the block
  - Re-read ADR-008 for the three-list schema contract

  Next: Run /edikt:gov:compile to include this guideline in governance.
```

---

REMEMBER: Guidelines are enforceable conventions, not suggestions. Every rule in the Rules section must use MUST or NEVER language. Soft language belongs in internal documentation, not in a guideline file that compiles into governance.
