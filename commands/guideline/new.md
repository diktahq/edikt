---
name: guideline:new
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

Project templates always take precedence over edikt's built-in defaults — your project owns its template shape. Follow this precedence when selecting the template:

1. **Project template**: if `.edikt/templates/guideline.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Refuse**: if no project template exists, refuse:
   ```
   ❌ No project guideline template found.

   edikt requires an explicit
   project template. edikt doesn't assume a style — your project owns this.

   To set up templates, run:
     /edikt:init                     (interactive setup — pick Adapt,
                                      Start fresh, or Write my own)
     /edikt:init --reset-templates   (regenerate templates)

   Or create .edikt/templates/guideline.md manually with at least a
   `## Rules` section using MUST/NEVER language. The
   template MUST NOT contain an in-body
   [edikt:directives:start] block — compiled directives live in the
   co-located <name>.edikt.yaml sidecar that /edikt:guideline:compile
   generates next to the .md.
   ```
   Do NOT fall back to inline. Do NOT write the guideline. Exit.

**No global default**: edikt does NOT ship a "default guideline template" that is auto-installed. Projects explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only.

**On terminology**: unlike ADRs and Invariant Records, guidelines use the standard term "guideline" — no formal short form needed.

### 2. Determine Topic — flexible prose input with reference extraction

The argument is **always prose first**, then mined for embedded references. This is the same pattern `/edikt:sdlc:plan` has used since v0.1.3. Do NOT classify the input into rigid types (path vs identifier vs prose) — treat the whole argument as natural language and scan it for things that resolve to content.

#### 2a. Empty argument → ask

**If `$ARGUMENTS` is empty** — ask:
```
What is this guideline about? (e.g. error-handling, testing, logging, api-design)

Or give me a document or identifier to derive the guideline from:
  /edikt:guideline:new "error-handling using docs/style/errors.md"
  /edikt:guideline:new docs/style/logging-conventions.md
  /edikt:guideline:new ADR-942
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
  - `"api-design from ADR-942"` → `api-design`
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
| `/edikt:guideline:new ADR-942` | Identifier, read the ADR, derive guideline from its Decision section |
| `/edikt:guideline:new "api-design per ADR-955 and docs/openapi.yaml"` | Prose with multiple refs, read all, extract rules |

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

### 4g. Intake gates (GL-001) — REQUIRED before writing

<!-- edikt:artifact-kinds-contract:start — one text, five surfaces; edited in GL-001's amendment and carried here verbatim -->
**The subject axis, before the questions.** A governance rule binds exactly two things: a standing property of the project, or a choice the project made. It never binds a code site, a third-party tool's behaviour, an SDLC artifact, or a test by name. Those are the draft's sources: they belong in provenance, never in the prose, and never as what the rule binds.

**The one-line division of labour:** an ADR decides an approach and owns a vocabulary; an invariant states the condition; a guideline holds the procedure.

### Q1 — Is it a property that must ALWAYS hold?  → **Invariant**

All three, or it is not an invariant:

- **The rule has no single natural home in code.** The prong is about the rule, not its guard:
  a rule that binds every writer across the system has no single home even when its guard lives in
  one place (a database trigger, a schema lint) — the guard is where it is enforced, the invariant
  is the record that the guard must exist and what it protects. A rule that binds one call site has
  a single home and belongs as a test. "An order cannot ship unpaid" has one — the aggregate, and a
  unit test — so it lives there, not here. "The production database is never
  overwritten" has none: it spans deploy scripts, migrations, ops and every engineer, which is why
  it needs a record and a guard. *Cross-cutting is the symptom of having no home, not the test.*
  Domain, bounded-context and sub-domain invariants live in code. Governance invariants are the
  ones code cannot hold in one place — architectural or business, both count.
- **Standing, not chosen.** It describes the system at every moment, not a decision taken once.
  "Sidecars are the only source of directives" — always. "We chose sidecars over sentinels" — once.
- **Falsifiable.** You can name the observation that would prove it broken. "A directive with no
  sidecar compiles" is an observation. "Governance is hard to bypass" is not.
- **Guardable.** A test, a check, a grep, or a hook can watch it. If nothing can watch it, it is a
  value, not an invariant.

**Violation is a defect.** Someone broke the system.

A note for users: governance is about your project and its codebase, never about a third-party tool. You never write invariants *about governance itself* (that ADRs are immutable,
that a missing check is not a pass). edikt carries those; thirteen of edikt's own twenty-one are of
that kind, which is why edikt's corpus is a poor example and a project corpus like bok-services is
the reference. A DDD invariant is a consistency rule enforced in a transaction; a governance
invariant is a structural or process property enforced at review, compile or hook time.

### Q2 — Is it a choice you made, where you could have chosen otherwise?  → **ADR**

All three, or it is not a decision:

- **A real rejected alternative existed.** Not "we could have done nothing" — a second option that
  someone could reasonably have picked, with its own costs. If there was only one sensible way, this
  is a fact about the world, and facts go in the code, the commit, or the README.
- **Reversing it costs something.** Structure, dependencies, interfaces, data, or a cross-cutting
  convention would have to change. If it can be flipped in one file next week, it is not a decision
  worth a record.
- **Without the record, it would be re-litigated.** The reason a future reader needs the ADR is that
  the alternative will look attractive again.

**Violation is not a concept.** You do not violate an ADR; you supersede it with a new one.

### Before Q3 — does it produce a default an agent follows *while editing*?  → if not, **documentation**

"How to run the project locally", "where the dashboards are", "how to get a database dump" are a
README or CONTRIBUTING, not governance. They carry no exception class and no editing-time default;
compiled as a guideline they become advisory directives that fire on every edit and say nothing.
The test: if the extractor would produce an imperative you would not want an agent to act on
mid-edit, it is documentation.

### Q3 — Is it a default with a NAMED exception?  → **Guideline**

Both, or it is not a guideline:

- **It is what you do unless…** — a convention that holds most of the time.
- **…and you can name the "unless".** A specific situation someone can check themselves against.
  "Use the shared error type, except in a hot path where allocation is measured" names the exception.
  "Use the shared error type where it makes sense" does not — that is taste, not a guideline.

**Violation is a conversation.** Someone should be able to say why, and be right.

### Q4 — None of the above  → **Nothing**

A commit message, a spec's provenance line, a comment in the code. **This is the expected most
common answer.** A healthy session produces zero governance artifacts.

**Example / Near miss, per kind:**

| kind | clean example (all prongs hold) | near-miss (one prong fails) | the near-miss becomes |
|---|---|---|---|
| **Invariant** | "The production database is never overwritten by a tool or a script." No single home (deploy, migrations, ops), falsifiable (one overwrite), guardable (a deploy assertion). | "The system is tool-agnostic." Standing and homeless, but no observation proves it broken. | An ADR, if a specific alternative (binding to one vendor) was rejected; otherwise a principle in the README. |
| **Invariant** | "Domain packages never import adapters." Guardable by an import test. | "An order cannot ship unpaid." Falsifiable and guardable, but it has one natural home: the aggregate and its unit test. | Code. |
| **Invariant** | "A manually authored source is never edited by a tool." Binds every writer; its guard is one trigger plus a test, and the record says the guard must exist. | "BM25 tests use committed fixture data." One helper, one test file: the rule and its guard share a single home. | Code. |
| **ADR** | "Hexagonal architecture with ports and adapters, over a layered architecture." Real alternative, costly reversal, re-litigated by every new joiner. | "The extractor model is Sonnet." Reversal is one config line and nobody rejected an alternative model on the record. | A line in the ADR that decided to pin a model at all, or the config value. |
| **ADR** | "Events go through a broker with at-least-once delivery, over direct HTTP calls." | "The pin lives in the agent's frontmatter." Forced by the earlier decision to pin; no second option was live. | A dated Amendment on the parent ADR. |
| **Guideline** | "Integration tests run behind a build tag, except the smoke suite that runs on every push." A default with a named exception. | "Use the shared error type where it makes sense." A default with no exception anyone can check themselves against. | Taste; nothing, until the exception can be named. |
| **Guideline** | "API documentation follows the Zalando guidelines, except internal admin endpoints." | "Run `make dev` to start the stack locally." No editing-time default and no exception class. | Documentation (README, CONTRIBUTING). |

**Prose rule (k):** an SDLC id (BRAIN, PRD, SPEC, PLAN) never appears in a governance artifact's body prose. It belongs in frontmatter references or a Provenance section.

**In plain terms:** every record opens with an `## In plain terms` section — one paragraph, declarative, present tense, no modal verb, no date.

The reviewer's `default: demote`. An ADR argues its way past guideline; a guideline past nothing.
<!-- edikt:artifact-kinds-contract:end -->


1. **Auto-reject screens** — upstream-docs restatement, derivable from code/tests, living state, task-scoped, reviewer-justification: refuse with the screen named.
2. **G0 — already captured**: search guideline rules, ADR titles, invariant statements. On a hit, refuse naming the existing artifact.
3. **G3 — guideline test**: a DEFAULT with a NAMED legitimate exception class, where violation is a discussion, not a failure. No nameable exception class and violation would be a defect → this is invariant-shaped: redirect to `/edikt:invariant:new` (G1). A one-time choice with a rejected alternative → `/edikt:adr:new` (G2). Otherwise refuse (G4).
4. The owner may override explicitly — record the overridden gate in the guideline body.

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

## When NOT to apply

{The named, legitimate exception class — not "rarely" or "use judgment," a specific situation someone could name and check themselves against. This is the G3 test: a default WITH a named exception. Cannot name one → this is invariant-shaped (redirect to /edikt:invariant:new) or there's nothing to write (G4).}

---

*Created by edikt:guideline — {date}*
```

### 5b. Structural gate (deterministic — GL-001 G3) — REQUIRED before the sidecar

Step 4g is a judgment call made by this session; this step is not. It is a grep against the file just written, and it REFUSES rather than warns.

```bash
GL_FILE="{guidelines_dir}/{slug}.md"
missing=()
grep -q '^## When NOT to apply' "$GL_FILE" || missing+=("When NOT to apply")
if [ "${#missing[@]}" -gt 0 ]; then
  printf 'REFUSED: guideline %s is missing required section(s): %s\n' "{slug}" "${missing[*]}"
  exit 1
fi
```

If this fails, STOP. Do not proceed to Step 6. If no named exception class can be given, this is either invariant-shaped (redirect per 4g) or there's nothing to write (G4) — do not invent an exception to satisfy the grep. This checks presence, not quality: an unfilled placeholder still passes the grep and is still wrong; quality is 4g's job, presence is this gate's.

### 6. Generate the sidecar

Directive metadata for every guideline lives only in the co-located `<slug>.edikt.yaml` sidecar — never in an in-body sentinel block. The guideline's own prose is yours to edit; edikt must never rewrite it. Dispatch the `sidecar-extractor` agent (`templates/agents/sidecar-extractor.md`) with the path of the guideline you just wrote. The agent runs in a forked subagent and writes `<slug>.edikt.yaml` next to the `.md`.

Use the Agent tool:
- `subagent_type: sidecar-extractor`
- `prompt: "Extract sidecar from {ABS_PATH_TO_GUIDELINE}"`

If the dispatch fails with `Agent type 'sidecar-extractor' not found` but `.claude/agents/sidecar-extractor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

If the agent fails, surface the error but do NOT roll back the guideline creation. The body is already written; the user can re-run sidecar generation via `/edikt:guideline:compile {slug}` once the issue is resolved.

If the sidecar is produced, it conforms to `templates/schemas/gov-sidecar.v2.schema.json` (v2) and contains topic, path, signals, and the directive list extracted from the `## Rules` section. Soft-language bullets that don't use MUST / NEVER / ALWAYS / SHOULD are skipped — guidelines compile only enforceable rules into directives.

The legacy auto-chain to `/edikt:guideline:compile` (which used to write an in-body sentinel block) is removed in v0.6.0. Existing guidelines created before this change still have in-body sentinels until `edikt migrate sidecars` lifts them out.

### 7. Run the verify gate

After both files are on disk, run the completion-evidence gate:

```bash
edikt verify gov {slug}
```

This walks the sidecar's `directives[].verify` and `verification[].verify` and runs each as a shell command. Exit 0 = all pass / all skipped; exit 1 = ≥ 1 failed.

- On exit 0 → proceed to Step 8. No surface.
- On exit 1 → surface a warning with the per-item failures. **Never auto-delete the artifact.**

### 8. Confirm

```
✅ Guideline created: {guidelines_dir}/{slug}.md
✅ Sidecar written: {guidelines_dir}/{slug}.edikt.yaml
✅ Verify: {n} of {m} passed.   (or)   ⚠ Verify: {failing_n} of {m} failed — see above.

  Topic: {Topic Title}
  Rules: {n}

  To refine the directives:
  - Edit the sidecar's directives[] directly to add, edit, or remove rules
  - Re-run /edikt:guideline:compile {slug} to regenerate the sidecar from prose

  Next: Run /edikt:gov:compile to include this guideline in governance.
```

---

REMEMBER: Guidelines are enforceable conventions, not suggestions. Every rule in the Rules section must use MUST or NEVER language. Soft language belongs in internal documentation, not in a guideline file that compiles into governance.
