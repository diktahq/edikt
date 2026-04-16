---
name: edikt:invariant:new
description: "Capture a hard architectural constraint that must never be violated"
effort: normal
argument-hint: "[constraint description] — omit to extract from conversation"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---
!`BASE=$(grep "^base:" .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"' || echo "docs"); COUNT=$(ls "${BASE}/invariants/"*.md 2>/dev/null | wc -l | tr -d ' '); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(ls "${BASE}/invariants/"*.md 2>/dev/null | xargs -I{} basename {} .md | sort | tr '\n' ', ' | sed 's/,$//'); printf "<!-- edikt:live -->\nNext INV number: INV-%s\nExisting invariants: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"`

# edikt:invariant:new

Capture an invariant — a hard constraint that must never be violated, regardless of context.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

Invariants are always loaded by `/edikt:context` (all depth levels) because they are non-negotiables.

Two modes:
- **With argument** — `/edikt:invariant:new no floats for money` — define from scratch
- **No argument** — `/edikt:invariant:new` — extract from current conversation

## What Makes a Good Invariant

An invariant is NOT a preference or a guideline. It is a rule where violation causes real harm:
- "All monetary amounts stored as integer cents. Never use float64 for money."
- "Domain package imports only stdlib. No HTTP, no SQL, no framework types."
- "All payment operations require an idempotency key."
- "Never log PII — mask emails, phone numbers, and card data before logging."

If it starts with "prefer" or "try to" — it's a rule, not an invariant. Put it in `.claude/rules/`.

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve paths from the `paths:` section:

- Invariants: `paths.invariants` (default: `docs/architecture/invariants`)

### 1a. Resolve Template (lookup chain)

edikt v0.3.0+ supports project-level template overrides per [ADR-005](../../docs/architecture/decisions/ADR-005-extensibility-model.md) and [ADR-009](../../docs/architecture/decisions/ADR-009-invariant-record-terminology.md). Follow this precedence when selecting the template for the new Invariant Record:

1. **Project template**: if `.edikt/templates/invariant.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Inline fallback (v0.2.x legacy projects only)**: if no project template exists AND the project's `edikt_version` in `.edikt/config.yaml` is `< 0.3.0` or missing, use the inline template shown later in this command. Print a one-time warning:
   ```
   ⚠ No project invariant template found. Using the legacy inline fallback.
     Run /edikt:upgrade followed by /edikt:init to set up project templates
     and formalize your invariants as Invariant Records per ADR-009.
   ```
3. **Refuse (v0.3.0+ projects with missing templates)**: if no project template exists AND the project's `edikt_version` is `>= 0.3.0`, refuse:
   ```
   ❌ No project Invariant Record template found.

   This project is on edikt v{version}, which requires an explicit
   project template. edikt doesn't assume a style — your project owns this.

   To set up templates, run:
     /edikt:init                     (interactive setup — pick Adapt,
                                      Start fresh, or Write my own)
     /edikt:init --reset-templates   (regenerate templates)

   Or create .edikt/templates/invariant.md manually with the required
   sections (Statement, Rationale, Consequences of violation, Enforcement)
   and the [edikt:directives:start]: # / [edikt:directives:end]: # sentinel block.

   See docs/internal/product/prds/PRD-001-spec/invariant-record-template.md
   for the authoritative Invariant Record template and
   docs/internal/product/prds/PRD-001-spec/writing-invariants-guide.md
   for the full writing guide.
   ```
   Do NOT fall back to inline. Do NOT write the invariant. Exit.

**No global default**: edikt does NOT ship a "default invariant template" that is auto-installed. Projects either explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only.

**"Invariant Record" terminology**: as of ADR-009, edikt formalizes "Invariant Record" (short form `INV`) as the governance artifact for hard architectural constraints. See the writing guide at `docs/internal/product/prds/PRD-001-spec/writing-invariants-guide.md` for guidance on writing good Invariant Records.

**Checking the project edikt_version**: extract it from `.edikt/config.yaml`:
```bash
PROJECT_EDIKT_VERSION=$(grep '^edikt_version:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
```
Compare to `0.3.0` using semver ordering. A missing `edikt_version` line means v0.2.x legacy.

### 2. Load Existing Invariants

```bash
ls {BASE}/invariants/*.md 2>/dev/null | sort
```

The correct next INV number is provided at the top of this prompt in the `<!-- edikt:live -->` block. Use it exactly — do not guess or count files yourself.

### 3. Determine Mode — flexible prose input with reference extraction

The argument is **always prose first**, then mined for embedded references. This is the same pattern `/edikt:sdlc:plan` has used since v0.1.3. Do NOT classify the input into rigid types — treat the whole argument as natural language and scan it for things that resolve to content.

#### 3a. Empty argument → infer from conversation

**If `$ARGUMENTS` is empty**, scan the current conversation for statements of the form "we must always / never", "under no circumstances", "this is a hard rule", or explicit non-negotiables that were discussed.

If no clear constraint is found:
```
I couldn't identify a hard constraint in our conversation.

An Invariant Record captures something that must NEVER be violated — not a
preference. Describe it:
  /edikt:invariant:new <constraint>

Examples:
  /edikt:invariant:new "All write operations are idempotent"
  /edikt:invariant:new "Tenant isolation using docs/specs/auth.md and ADR-042"
  /edikt:invariant:new docs/specs/compliance-requirements.md
  /edikt:invariant:new ADR-042
```
And stop.

If a clear constraint IS found in conversation, use it as the framing prose and proceed to 3d (Interview for gaps).

#### 3b. Non-empty argument → extract embedded references

Treat `$ARGUMENTS` as prose. Scan it for references of three kinds, resolving each to content that feeds the invariant body:

**Reference kind 1: file paths**

Any substring in the prose that looks like a file path AND resolves to an existing file. Detection: a token containing at least one `/` OR ending in a common code/doc extension (`.md`, `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rb`, `.php`, `.rs`, `.java`, `.kt`, `.sql`, `.yaml`, `.yml`, `.toml`, `.json`). Verify existence before accepting.

For each path that resolves: read the file and add it to the source pool.

For invariants specifically, file references often point at:
- **Compliance documents** — regulatory requirements that drive the constraint
- **Incident reports** — the post-mortem that led to the invariant
- **Spec documents** — the architectural decision the invariant codifies
- **Existing code** — the pattern being enforced by the invariant

**Reference kind 2: identifiers**

Tokens matching edikt artifact ID patterns:
- `ADR-NNN` — often the decision that established the invariant's constraint
- `INV-NNN` — a related or superseded invariant
- `SPEC-NNN`, `PRD-NNN` — product requirements that mandate the invariant
- `PLAN-NNN` — implementation plan referencing the invariant

Read `paths:` from `.edikt/config.yaml` to resolve directories. For each identifier that resolves: read the corresponding file and add it to the source pool. If an identifier doesn't resolve, treat it as plain prose — do NOT error.

**Reference kind 3: branch names**

Tokens matching `{prefix}/{name}` where `{prefix}` is `feature`, `feat`, `fix`, `hotfix`, `refactor`, `chore`, `docs`, `release`, `dev`, `spike`, `experiment`. Verify with `git rev-parse --verify`. If the branch exists, read its diff against the default branch and relevant commit messages. Add to the source pool. Do NOT error if git isn't available or the branch doesn't exist.

#### 3c. Build the source pool and framing

- **Framing prose** = the full `$ARGUMENTS` string with references inline. Sets the scope of the constraint.
- **Source pool** = concatenated content of every resolved reference, labeled by origin.
- **Primary sources**: resolved references dominate. If a compliance document lists a requirement, the invariant's Rationale cites the specific regulation. If an ADR established the underlying decision, reference it as prose in the Rationale (NOT as a structured frontmatter field per ADR-009).
- **Secondary source**: the framing prose provides tone and scope intent.

**Critical constraint for Invariant Records (per ADR-009):** even when the source pool contains rich context, the Invariant Record itself must describe the **constraint, not the implementation**. Do not let a spec document that says "Use Redis with TTL=24h" produce an invariant that says "Use Redis with TTL=24h". The invariant should lift the constraint to the appropriate level ("Session cache entries expire within 24 hours") — the Redis choice belongs in an ADR. Apply the writing guide's constraint-vs-implementation test before finalizing.

**If only framing prose resolved (no references found)**:
- Treat the prose as the constraint description and drive through the interview in 3d.
- This is the classic `/edikt:invariant:new "All write operations are idempotent"` path.

**If one or more references resolved**:
- Use the source pool to fill the Rationale and Consequences-of-violation sections directly.
- Interview only for gaps: Statement wording (if the source pool isn't declarative enough), Enforcement mechanism (often not in specs), and whether the constraint should be ACTIVE or PROPOSED initially.

#### 3d. Interview for gaps

Ask ONE focused question per missing element:
- **Statement** — "What's the constraint in one declarative sentence?" (Only ask if the source pool didn't give a clear one)
- **Rationale** — "Why is this non-negotiable?" (Usually in the source pool if a compliance doc or incident report was referenced)
- **Consequences of violation** — "What specifically goes wrong if this is violated?" (Concrete failure mode, required)
- **Enforcement** — "How will we catch violations?" (Automated test / linter / edikt directive / review checklist — at least one is mandatory per ADR-009)

Do not ask about anything already covered by the source pool. If the source pool is comprehensive enough that no interview is needed, skip directly to 3e.

**Examples:**

| Input | Behavior |
|---|---|
| `/edikt:invariant:new` (empty) | Scan conversation for hard constraints, interview for gaps |
| `/edikt:invariant:new "All write operations are idempotent"` | Pure prose, no refs, interview for Rationale/Consequences/Enforcement |
| `/edikt:invariant:new docs/compliance/soc2-requirements.md` | Path resolves, read compliance doc, use as primary source for Rationale |
| `/edikt:invariant:new "Tenant isolation per docs/specs/multi-tenant.md"` | Prose with path ref, read spec, use as primary source |
| `/edikt:invariant:new ADR-042` | Identifier resolves, read ADR, lift the constraint from the Decision section |
| `/edikt:invariant:new "We learned from the 2025-11-02 incident that PII must never appear in logs"` | Prose with incident context, interview for Statement wording and Enforcement |

### 3e. Quality check the drafted Statement section

### 4. Draft with Enforcement-Grade Language

Before writing, ensure the invariant's Rule statement and Rationale meet enforcement quality. Invariants compile directly into non-negotiable governance directives — vague language here means vague enforcement.

Rules for writing invariants:

1. **The Rule statement uses MUST or NEVER** (uppercase). Example: "Every command MUST be a plain `.md` file — NEVER compiled code, NEVER a build step."
2. **Name specific things** — file types, namespaces, tools, patterns. "Code should be well-structured" is not an invariant. "Domain layer classes MUST NOT import from infrastructure packages" is.
3. **State the consequence in the Rationale** — not "it's important" but "violations cause X specific harm."
4. **Verification must be concrete** — a command to run, a grep pattern, or explicit review criteria. Not "review the code."

Do NOT write invariants with soft language ("should", "prefer", "try to"). If it's not a hard constraint, it belongs in `docs/guidelines/`.

### 5. Write the Invariant

Create `{BASE}/invariants/INV-{NNN}-{slug}.md`:

```markdown
---
type: invariant
id: INV-{NNN}
title: {Title}
status: active
severity: critical       # critical | high
scope: "**/*"            # path glob — what code this applies to
created_at: {ISO8601 timestamp}
references:
  adrs: []
  specs: []
  established_by: ""     # ADR, PRD, or incident that created this
---

# INV-{NNN}: {Title}

{One sentence. State the constraint as "X must always be true."}

## Rationale

{Why this is non-negotiable — the specific harm that occurs without it, not just "it's important."}

## Scope

{What parts of the system this applies to. Be specific: all code, only Go files, only the domain layer, only API handlers.}

## Violation Consequences

{What breaks if this is violated. Be concrete: data loss, security breach, CI failure, architectural drift.}

## Verification

How to check compliance:
- Automated: {command, test, hook, or CI check that verifies this}
- Manual: {what a reviewer should look for}

## Exceptions

{Can this ever be overridden? If yes, what approval is needed. If no: "No exceptions."}

## Related

{ADRs, specs, or incidents that established this invariant.}

## Directives

LLM-enforceable rules compiled from this invariant. Generated by `/edikt:gov:review`, consumed by `/edikt:gov:compile`.

[edikt:directives:start]: #
paths:
  - {glob patterns — e.g. "**/*" for universal invariants}
scope:
  - {activity scopes — e.g. planning, design, review, implementation}
directives:
  - {The Rule statement rewritten as an enforceable directive using MUST/NEVER}
  - {Include (ref: INV-{NNN}) suffix}
[edikt:directives:end]: #

---

*Captured by edikt:invariant — {date}*
```

An invariant is a HARD CONSTRAINT that can never be violated. If there are exceptions, it might be a guideline (put it in `docs/guidelines/` instead). If it describes a preference, it's not an invariant.

---

REMEMBER: An invariant is a HARD CONSTRAINT where violation causes real harm. If it starts with "prefer" or "try to" — it belongs in docs/guidelines/, not in invariants. Every invariant needs a Verification section describing how to check compliance.

### 6. Auto-chain to /edikt:invariant:compile (ADR-008)

Per ADR-008, this command auto-chains to `/edikt:invariant:compile INV-{NNN}` at the end of its workflow so the newly-created Invariant Record has its directive sentinel block populated immediately. Fresh artifacts have nothing to preserve (no manual directives, no suppressed directives, no hand-edits), so the compile runs the slow path cleanly and the user never has to remember to compile after new.

Run `/edikt:invariant:compile INV-{NNN}` now. If it produces an error (e.g., headless mode with strategy flags), surface the error but do NOT roll back the Invariant Record creation — the body is already written and the user can run compile manually later.

If the compile succeeds, the directive block is populated with:
- `source_hash` — SHA-256 of the invariant body (excluding the block)
- `directives_hash` — SHA-256 of the auto `directives:` list
- `compiler_version` — current edikt version
- `directives:` — auto-generated from the `## Statement` (or legacy `## Rule`) section
- `manual_directives: []` — empty; user adds rules compile missed
- `suppressed_directives: []` — empty; user adds rules compile got wrong

### 7. Confirm

```
✅ Invariant Record captured: {BASE}/invariants/INV-{NNN}-{slug}.md
✅ Directives compiled: {k} auto directives

  INV-{NNN}: {Title}
  Status: Active

  To refine the directives:
  - Add rules compile missed → edit manual_directives: in the block
  - Reject wrong auto rules → add to suppressed_directives: in the block
  - Re-read ADR-008 for the three-list schema contract
  - Re-read ADR-009 for the Invariant Record template contract

  Next: Run /edikt:gov:compile to update governance directives.

  Want architect or security to review this? Say "review this invariant"
```
