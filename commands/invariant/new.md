---
name: invariant:new
description: "Capture a hard architectural constraint that must never be violated"
effort: normal
argument-hint: "[constraint description] — omit to extract from conversation"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---
!`${HOME}/.edikt/bin/edikt next-id inv`

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

edikt v0.3.0+ supports project-level template overrides per the relevant decision and the relevant decision. Follow this precedence when selecting the template for the new Invariant Record:

1. **Project template**: if `.edikt/templates/invariant.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Inline fallback (v0.2.x legacy projects only)**: if no project template exists AND the project's `edikt_version` in `.edikt/config.yaml` is `< 0.3.0` or missing, use the inline template shown later in this command. Print a one-time warning:
   ```
   ⚠ No project invariant template found. Using the legacy inline fallback.
     Run /edikt:upgrade followed by /edikt:init to set up project templates
     and formalize your invariants as Invariant Records.
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
   sections (Statement, Rationale, Consequences of violation, Enforcement).
   Under v0.6.0+ the template MUST NOT contain an
   in-body [edikt:directives:start] block — compiled directives live in
   the co-located <name>.edikt.yaml sidecar that /edikt:invariant:compile
   generates next to the .md.

   See the Invariant Record writing guide at
   https://edikt.dev/governance/writing-invariants
   for the template and the full writing guide.
   ```
   Do NOT fall back to inline. Do NOT write the invariant. Exit.

**No global default**: edikt does NOT ship a "default invariant template" that is auto-installed. Projects either explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only.

**"Invariant Record" terminology**: edikt formalizes "Invariant Record" (short form `INV`) as the governance artifact for hard architectural constraints. See https://edikt.dev/governance/writing-invariants for the writing guide.

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
  /edikt:invariant:new "Tenant isolation using docs/specs/auth.md and ADR-942"
  /edikt:invariant:new docs/specs/compliance-requirements.md
  /edikt:invariant:new ADR-942
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
- **Primary sources**: resolved references dominate. If a compliance document lists a requirement, the invariant's Rationale cites the specific regulation. If an ADR established the underlying decision, reference it as prose in the Rationale (NOT as a structured frontmatter field).
- **Secondary source**: the framing prose provides tone and scope intent.

**Critical constraint for Invariant Records:** even when the source pool contains rich context, the Invariant Record itself must describe the **constraint, not the implementation**. Do not let a spec document that says "Use Redis with TTL=24h" produce an invariant that says "Use Redis with TTL=24h". The invariant should lift the constraint to the appropriate level ("Session cache entries expire within 24 hours") — the Redis choice belongs in an ADR. Apply the writing guide's constraint-vs-implementation test before finalizing.

**If only framing prose resolved (no references found)**:
- Treat the prose as the constraint description and drive through the interview in 3d.
- This is the classic `/edikt:invariant:new "All write operations are idempotent"` path.

**If one or more references resolved**:
- Use the source pool to fill the Rationale and Consequences-of-violation sections directly.
- Interview only for gaps: Statement wording (if the source pool isn't declarative enough), Enforcement mechanism (often not in specs), and whether the constraint should be ACTIVE or PROPOSED initially.

#### 3d. Interview for gaps (batched presentation per Opus 4.7 guidance)

**Present ALL gap questions in a single message as a numbered list — do NOT ask one-at-a-time.** Ask ONLY about missing elements not present in the source pool. Every user turn adds reasoning overhead; batching respects the user's attention budget. Each question must be labeled:
- `[required]` — blocking; the invariant cannot be written without this
- `[optional — default: <value>]` — default applied silently if skipped

Gap questions to batch (one per missing element):
- **Statement** — "What's the constraint in one declarative sentence?" [required if source pool unclear]
- **Rationale** — "Why is this non-negotiable?" [optional — default: infer from source pool]
- **Consequences of violation** — "What specifically goes wrong if this is violated?" [required — concrete failure mode]
- **Enforcement** — "How will we catch violations?" [required — at least one of: automated test, linter, edikt directive, review checklist]

Accept a single user reply covering any subset. Skip the interview entirely when the source pool covers all four elements.

**Examples:**

| Input | Behavior |
|---|---|
| `/edikt:invariant:new` (empty) | Scan conversation for hard constraints, interview for gaps |
| `/edikt:invariant:new "All write operations are idempotent"` | Pure prose, no refs, interview for Rationale/Consequences/Enforcement |
| `/edikt:invariant:new docs/compliance/soc2-requirements.md` | Path resolves, read compliance doc, use as primary source for Rationale |
| `/edikt:invariant:new "Tenant isolation per docs/specs/multi-tenant.md"` | Prose with path ref, read spec, use as primary source |
| `/edikt:invariant:new ADR-942` | Identifier resolves, read ADR, lift the constraint from the Decision section |
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

### 4g. Intake gates (GL-001) — REQUIRED before writing

Run the capture gates BEFORE drafting. Burden of proof is on CAPTURE.

1. **Auto-reject screens** — upstream-docs restatement, derivable from code/tests, living state, task-scoped, reviewer-justification: refuse with the screen named.
2. **G0 — already captured**: search invariant statements, ADR titles, sidecar `signals`. On a hit, refuse naming the existing artifact.
3. **G1 — invariant test, ALL THREE required**: (a) a MUST/NEVER property holding at ALL times — a standing property, not a one-time choice; (b) FALSIFIABLE — a nameable observation would prove violation (this fills the required "Falsifiable by" section; cannot name it → not an invariant); (c) ENFORCEABLE — a test, lint, or grep can guard it. Violation of an invariant is a defect.
4. Fails G1 but is a real decision → redirect to `/edikt:adr:new` (G2). A default with named exceptions → `/edikt:guideline:new` (G3). Otherwise refuse (G4).
5. The owner may override explicitly — record the overridden gate in the Rationale.

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

<!--
Compiled directives live in the co-located INV-{NNN}-{slug}.edikt.yaml
sidecar. edikt never writes to this .md — edit prose
only; run /edikt:invariant:compile to regenerate the sidecar.
-->

---

*Captured by edikt:invariant — {date}*
```

An invariant is a HARD CONSTRAINT that can never be violated. If there are exceptions, it might be a guideline (put it in `docs/guidelines/` instead). If it describes a preference, it's not an invariant.

---

REMEMBER: An invariant is a HARD CONSTRAINT where violation causes real harm. If it starts with "prefer" or "try to" — it belongs in docs/guidelines/, not in invariants. Every invariant needs a Verification section describing how to check compliance.

### 6. Generate the sidecar

 (sidecar architecture), the directive metadata for every active Invariant Record lives in a co-located `<name>.edikt.yaml` sidecar — not in an in-body sentinel block. Dispatch the `sidecar-extractor` agent (`templates/agents/sidecar-extractor.md`) with the path of the Invariant Record you just wrote. The agent runs in a forked subagent and writes `<name>.edikt.yaml` next to the `.md`.

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_INV}"`

If the dispatch fails with `Agent type 'sidecar-extractor' not found` but `.claude/agents/sidecar-extractor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

If the agent fails (rare — it has a single locked task), surface the error but do NOT roll back the Invariant Record creation. The body is already written; the user can re-run sidecar generation via `/edikt:invariant:compile INV-{NNN}` once the issue is resolved.

If the sidecar is produced, it conforms to `templates/schemas/gov-sidecar.v2.schema.json` (v2) and contains topic, path, signals, and the directive list extracted from the `## Statement` and `## Enforcement` sections.

The legacy auto-chain to `/edikt:invariant:compile` (which used to write an in-body sentinel block) is removed in v0.6.0. Existing Invariant Records created before this change still have in-body sentinels until `edikt migrate sidecars` lifts them out.

### 7. Run the verify gate

After both files are on disk, run the completion-evidence gate:

```bash
bin/edikt verify gov INV-{NNN}
```

This walks the sidecar's `directives[].verify` and `verification[].verify` and runs each as a shell command. Exit 0 = all pass / all skipped; exit 1 = ≥ 1 failed.

- On exit 0 → proceed to Step 8 (Confirm). No surface.
- On exit 1 → surface a warning with the per-item failures and tell the user to fix the directive prose or the sidecar verify command. **Never auto-delete the artifact** — the file is on disk for inspection.

### 8. Confirm

```
✅ Invariant Record captured: {BASE}/invariants/INV-{NNN}-{slug}.md
✅ Sidecar written: {BASE}/invariants/INV-{NNN}-{slug}.edikt.yaml
✅ Verify: {n} of {m} passed.   (or)   ⚠ Verify: {failing_n} of {m} failed — see above.

  INV-{NNN}: {Title}
  Status: Active

  To refine the directives:
  - Edit the sidecar's directives[] directly to add, edit, or remove rules
  - Re-run /edikt:invariant:compile INV-{NNN} to regenerate the sidecar from prose

  Next: Run /edikt:gov:compile to update governance directives.

  Want architect or security to review this? Say "review this invariant"
```
