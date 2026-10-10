---
name: adr:new
description: "Capture an architecture decision record — from scratch or from the current conversation"
effort: normal
argument-hint: "[decision topic] — omit to extract from conversation"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---
!`edikt next-id adr`

# edikt:adr:new

Create an Architecture Decision Record (ADR). Two modes:

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

- **With argument** — `/edikt:adr:new use postgres for persistence` — works through the decision from scratch
- **No argument** — `/edikt:adr:new` — extracts the decision from the current conversation

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 1. Resolve Paths

Read `.edikt/config.yaml`. Resolve paths from the `paths:` section:

- Decisions: `paths.decisions` (default: `docs/architecture/decisions`)

### 1a. Resolve Template (lookup chain)

Project templates always take precedence over edikt's built-in defaults — your project owns its template shape. Follow this precedence when selecting the template:

1. **Project template**: if `.edikt/templates/adr.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Refuse**: if no project template exists, refuse with a clear error:
   ```
   ❌ No project ADR template found.

   edikt requires an explicit
   project template. edikt doesn't assume a style — your project owns this.

   To set up templates, run:
     /edikt:init                     (interactive setup — pick from
                                      Adapt, Start fresh, or Write my own)
     /edikt:init --reset-templates   (regenerate templates, overwriting
                                      any existing ones)

   Or create .edikt/templates/adr.md manually as plain prose. Under
   the template MUST NOT contain an
   in-body [edikt:directives:start] block — compiled directives live
   in the co-located <name>.edikt.yaml sidecar that /edikt:adr:compile
   generates next to the .md.

   See docs/internal/product/prds/PRD-NNN-spec/invariant-record-template.md
   for the template contract (the ADR template follows the same pattern).
   ```
   Do NOT fall back to the inline template. Do NOT write the ADR. Exit.

**No global default**: edikt does NOT ship a single "default ADR template" that is auto-installed into every project. Projects either explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only.

### 2. Load Existing ADRs

```bash
ls {decisions_path}/*.md 2>/dev/null | sort
```

The correct next ADR number is provided at the top of this prompt in the `<!-- edikt:live -->` block. Use it exactly — do not guess or count files yourself.

### 3. Determine Mode — flexible prose input with reference extraction

The argument is **always prose first**, then mined for embedded references. This is the same pattern `/edikt:sdlc:plan` has used since v0.1.3. Do NOT classify the input into rigid types — treat the whole argument as natural language and scan it for things that resolve to content.

#### 3a. Empty argument → infer from conversation

**If `$ARGUMENTS` is empty**, scan the current conversation for a significant technical or architectural choice that was recently discussed:
- A choice between two or more approaches
- Reasoning about trade-offs
- A conclusion that was reached

If no clear decision is found in conversation context:
```
I couldn't identify a clear architectural decision in our conversation.

An ADR captures a significant technical choice — not implementation details.
Describe the decision to capture: /edikt:adr:new <decision topic>

Examples:
  /edikt:adr:new "Use Redis for session cache"
  /edikt:adr:new "Decide session cache using docs/specs/cache.md"
  /edikt:adr:new docs/specs/redis-decision.md
  /edikt:adr:new SPEC-942
```
And stop.

If a clear decision IS found, use it as the framing prose (skip sections 3b and 3c below) and proceed to 3d (Interview for gaps).

#### 3b. Non-empty argument → extract embedded references

Treat `$ARGUMENTS` as prose. Scan it for references of three kinds, resolving each to content that feeds the ADR body:

**Reference kind 1: file paths**

Any substring in the prose that looks like a file path AND resolves to an existing file in the repository. Examples Claude should recognize:
- `docs/specs/redis-cache.md`
- `./design/session-store.md`
- `internal/orders/repository.go`
- `README.md`

Detection: a token containing at least one `/` OR ending in a common code extension (`.md`, `.go`, `.py`, `.ts`, `.tsx`, `.js`, `.jsx`, `.rb`, `.php`, `.rs`, `.java`, `.kt`, `.sql`, `.yaml`, `.yml`, `.toml`, `.json`). Verify existence with `ls {path}` or `test -f {path}` before accepting it as a reference. If the file doesn't exist, treat it as plain prose (do NOT error).

For each path reference that resolves: read the file content. Add it to the source pool used to draft the ADR.

**Reference kind 2: identifiers**

Tokens matching known edikt artifact ID patterns:
- `ADR-NNN` — resolve to a file in `{paths.decisions}` whose name contains the ID
- `INV-NNN` — resolve to a file in `{paths.invariants}`
- `SPEC-NNN` — resolve to a file in `{paths.specs}` (default `docs/product/specs`)
- `PRD-NNN` — resolve to a file in `{paths.prds}` (default `docs/product/prds`)
- `PLAN-NNN` — resolve to a file in `{paths.plans}` (default `docs/plans`)

Read `paths:` from `.edikt/config.yaml` to resolve the directory for each identifier kind. If the identifier doesn't resolve (no matching file), treat the token as plain prose — do NOT error. The user might be referencing an ADR they haven't created yet, or citing a number from memory.

For each identifier that resolves: read the corresponding file. Add it to the source pool.

**Reference kind 3: branch names**

Any token matching `{prefix}/{name}` where `{prefix}` is one of: `feature`, `feat`, `fix`, `hotfix`, `refactor`, `chore`, `docs`, `release`, `dev`, `spike`, or `experiment`. Verify existence with:
```bash
git rev-parse --verify "refs/heads/{branch}" 2>/dev/null || git rev-parse --verify "refs/remotes/origin/{branch}" 2>/dev/null
```

If the branch exists: read the branch's diff against the default branch (`git diff main...{branch}` or equivalent) and any notes/commits on the branch that look relevant (commit messages, new spec/design files on the branch). Add this context to the source pool.

If the branch doesn't resolve or `git` isn't available, treat as plain prose. Do NOT error.

#### 3c. Build the source pool and framing

After scanning:

- **Framing prose** = the full `$ARGUMENTS` string with references left inline. This sets the tone and scope of the ADR ("the user wants to decide X in the context of Y").
- **Source pool** = the concatenated content of every resolved reference, labeled by origin:
  ```
  --- from docs/specs/redis-cache.md ---
  {content}
  --- from ADR-942 ---
  {content}
  --- from feature/redis-migration (branch diff) ---
  {content}
  ```
- **Primary sources**: resolved references dominate the draft. If `docs/specs/redis-cache.md` contains benchmarks, the ADR's Decision section cites those benchmarks. If ADR-942 established a related constraint, the new ADR's Consequences section acknowledges it.
- **Secondary source**: the framing prose provides context and intent. Use it to understand *why* the user wants this ADR, not to fill content.

**If only framing prose resolved (no references found)**:
- Proceed as before: treat the prose as the decision topic and drive the ADR entirely through the interview in 3d.
- This is the classic `/edikt:adr:new "Use Redis for session cache"` path.

**If one or more references resolved**:
- Skip or truncate the interview. Use the source pool to fill the ADR body directly.
- The interview becomes "fill gaps", not "start from scratch". Ask ONLY about things not present in the source pool.

#### 3d. Interview for gaps (batched presentation per Opus 4.7 guidance)

Based on the source pool and framing, identify what's missing for a complete ADR:
- **Context** — what problem is this solving? (Often in the source pool already if a spec was referenced)
- **Alternatives** — what else was considered? (May or may not be in the source pool)
- **Trade-offs** — what are we accepting? (Rarely in specs, usually needs interview)
- **Confirmation** — how will we know this is working? (Rarely in specs)

**Present ALL gap questions in a single message as a numbered list — do NOT ask one-at-a-time.** Every user turn adds reasoning overhead; batching respects the user's attention budget. Each question must be labeled:
- `[required]` — blocking; the ADR cannot be written without this
- `[optional — default: <value>]` — default applied silently if skipped

Accept a single reply covering any subset. For skipped `[optional]` questions, apply the documented default. For skipped `[required]` questions, re-ask only those.

Skip the interview entirely when the source pool covers all four elements.

Example batched message:
```
I need a few answers before writing ADR-NNN. Answer any subset — defaults apply for skipped [optional] items.

1. [required] What specifically is being decided? (one sentence)
2. [optional — default: two alternatives inferred from framing] What other approaches did you consider?
3. [optional — default: no explicit trade-offs] What are you accepting by choosing this?
4. [optional — default: "runtime behavior matches spec"] How will we verify this decision is being followed?
```

**Examples:**

| Input | Behavior |
|---|---|
| `/edikt:adr:new` (empty) | Scan conversation, extract recent decision, interview for gaps |
| `/edikt:adr:new "Use Redis for session cache"` | Pure prose, no refs, interview all sections |
| `/edikt:adr:new docs/specs/redis-cache.md` | Path resolves, read file, use as primary source, interview for gaps (likely trade-offs + confirmation) |
| `/edikt:adr:new "Decide session cache using docs/specs/redis-cache.md and SPEC-942"` | Prose with embedded path and identifier; read both, use as primary sources, interview only for what's missing |
| `/edikt:adr:new SPEC-942` | Identifier resolves, read spec, treat as primary source |
| `/edikt:adr:new feature/redis-migration` | Branch resolves, read diff + commits, use as primary source |
| `/edikt:adr:new "We need to cache sessions, the team discussed Redis vs Memcached last week"` | Pure prose with conversational framing, no resolvable refs, interview for specifics |

### 3e. Quality check the drafted Decision section

Before writing the ADR file (in Section 5), draft the Decision section and validate it against the quality criteria in Section 4 below. If the draft fails the quality criteria, iterate with the user before persisting.

### 4. Draft and Validate the Decision Section

Before writing the file, draft the Decision section and validate each directive against these quality criteria. Every statement in the Decision section becomes a compiled governance directive — weak language here means weak enforcement later.

**Write with enforcement-grade language from the start.** Every statement in the Decision section becomes a compiled governance directive. Write them as if they're rules Claude will follow literally — because they are.

Rules for writing the Decision section:

1. **Hard constraints use MUST or NEVER** (uppercase) with a one-clause reason after the dash. Example: "Domain classes MUST NOT import from infrastructure namespaces — dependency inversion keeps the domain testable without framework coupling."
2. **Name specific things** — namespaces, tools, patterns, file paths. "Use hexagonal architecture" is vague. "Domain and application layers MUST NOT import from `Symfony\*`, `Doctrine\*`, or any infrastructure namespace" is enforceable.
3. **One directive per sentence.** Don't combine "use CQRS and event sourcing" — split them.
4. **Every directive must be verifiable.** If you can't grep for it, test for it, or check it in code review with specific criteria, rewrite it until you can.

Do NOT write soft language ("should", "try to", "consider", "prefer") for decisions that are meant to be enforced. If it's a preference, it belongs in `docs/guidelines/`, not in an ADR Decision section.

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

Run the capture gates BEFORE drafting the file. Burden of proof is on CAPTURE, not silence; the reviewer default is DEMOTE.

1. **Auto-reject screens** — if the candidate restates an upstream tool's docs/defaults, is derivable from code or tests, is living state that will rot, is scoped to one task, or exists to justify a change to a reviewer: refuse with the screen named. Suggest a commit message or spec provenance instead.
2. **G0 — already captured**: search ADR/INV titles, compiled sidecar `signals`, and spec-SR provenance for the candidate's subject terms. On a hit, refuse: name the existing artifact.
3. **G2 — ADR test, ALL THREE required**: (a) a REAL rejected alternative existed — interview for it; if none can be named, this is a fact, not a decision: refuse. (b) expensive to reverse OR constrains future structure/dependencies/interfaces/data/security — this fills the required "Reversal cost" section. (c) re-litigation risk without the record.
4. On G2 failure that still shows a default-with-exceptions shape, REDIRECT to `/edikt:guideline:new` naming the failed prong. On full failure, refuse (G4): a healthy session produces zero ADRs.
5. The owner may override any verdict explicitly — state the overridden gate in the ADR's Context.

### 5. Write the ADR

Create `{BASE}/decisions/{NNN}-{slug}.md`.

```markdown
---
type: adr
id: ADR-{NNN}
title: {Title}
status: proposed
decision-makers: [{git user.name}]
created_at: {ISO8601 timestamp}
supersedes:        # optional — ADR-NNN if replacing a previous decision
references:
  adrs: []
  invariants: []
  prds: []
  specs: []
---

# ADR-{NNN}: {Title}

**Status:** proposed
**Date:** {today}
**Decision-makers:** {git user.name}

---

## Context and Problem Statement

{Background and forces at play. End with the question this ADR answers:}

How should we {the specific decision question}?

## Decision Drivers

- {Most important quality or concern}
- {Second priority}
- {Third priority}

## Considered Options

1. {Option A} — {one-line description}
2. {Option B} — {one-line description}
3. {Option C} — {one-line description}

## Decision

We will {active voice — what was decided, specifically and concretely}.

## Alternatives Considered

### {Option A}
- **Pros:** {benefits}
- **Cons:** {drawbacks}
- **Rejected because:** {specific reason}

### {Option B}
- **Pros:** {benefits}
- **Cons:** {drawbacks}
- **Rejected because:** {specific reason}

## Consequences

- **Good:** {benefit}
- **Bad:** {accepted trade-off}
- **Neutral:** {side effect that is neither good nor bad}

## Reversal Cost

{What it costs to undo this decision, concretely — what would need to change, what data/interfaces/dependents would break, and how far the decision has already propagated. This is the G2(b) prong: expensive-to-reverse or future-constraining. Cannot fill → not an ADR (G2 failure).}

## Confirmation

How to verify this decision is being followed:
- {Automated: command, test, or hook that checks compliance}
- {Manual: what a reviewer should look for in code review}

<!--
Compiled directives live in the co-located ADR-{NNN}-{slug}.edikt.yaml sidecar
. edikt never writes to this .md — edit prose only;
run /edikt:adr:compile to regenerate the sidecar.
-->

---

*Captured by edikt:adr — {date}*
```

An ADR should cover ONE decision, not a design document. If it exceeds 2 pages, it's probably a spec. Keep it focused: one question, one decision, one set of consequences.

---

REMEMBER: An ADR captures a DECISION with trade-offs, not a preference. It must include a Confirmation section describing how to verify the decision is being followed. If it exceeds 2 pages, it's a spec, not an ADR.

### 5b. Structural gate (deterministic — GL-001 G2) — REQUIRED before the sidecar

Step 4g is a judgment call made by this session; a session under pressure can talk itself past it. This step is not judgment — it is a grep against the file that was just written, and it REFUSES rather than warns.

```bash
ADR_FILE="{BASE}/decisions/{NNN}-{slug}.md"
missing=()
# Case-insensitive on the heading text, and a suffix after it is fine
# (`## Reversal Cost (REQUIRED — GL-001 G2)` is the shipped template's form).
grep -qiE '^## considered options' "$ADR_FILE" || missing+=("Considered Options")
grep -qiE '^## reversal cost' "$ADR_FILE" || missing+=("Reversal Cost")
if [ "${#missing[@]}" -gt 0 ]; then
  printf 'REFUSED: ADR-{NNN} is missing required section(s): %s\n' "${missing[*]}"
  exit 1
fi
```

If this fails, STOP. Do not proceed to Step 6. Report exactly which section(s) are missing — do not silently fill them with placeholder text to pass the check, and do not delete the artifact. If neither section can be filled with real content (no real rejected alternative, no real reversal cost), the correct outcome was never filing the ADR — surface that per 4g Step 4 (redirect to guideline, or refuse per G4) rather than inventing content to satisfy the grep.

This checks presence, not quality: a heading with placeholder text (`{Option A}`, `{What it costs...}`) still left unfilled passes this grep and is still wrong. Presence is what a script can verify; content quality is 4g's job, done once, before the file exists.

### 6. Acceptance gate (SPEC-020 SR-001) — refuses at every step, never edits around a refusal  edikt-guard:allow

The ADR was just written with `status: proposed`. This step is the ONLY thing that may flip it to `accepted`, and it does so by running four checks in order, refusing and stopping at the first one that fails. A refusal leaves the artifact on disk exactly as `status: proposed` — never deleted, never silently re-attempted, never accepted anyway because the rest of the session is confident. **Never edit the ADR's prose to route around a refusal from this step; a refusal is the acceptance step's verdict, not a drafting bug to patch.**

**Step 6a — mechanical citations first.** Every citation the draft makes must resolve before any judgement is spent on it (D95, this ADR's own contract):

```bash
EDIKT_INVOKER=/edikt:adr:new edikt gov audit-cite ADR-{NNN}
```

- Exit 0 → continue to 6b.
- Exit 1 → REFUSE. Print the binary's output verbatim (it already names each unresolved citation, what was looked for, and both the additive and subtractive remedy). The mechanical result is never taken from the auditor's own report in 6b and the auditor cannot override it — audit-cite's exit code is authoritative for this check, full stop.
- Exit 3 → usage/internal error; report and stop, do not proceed to 6b.

**Step 6b — the governance-auditor's judgement.** Dispatch the locked read-only auditor over the draft plus the corpus:

- `subagent_type: governance-auditor`
- `prompt: "Read the draft artifact at {ABS_PATH_TO_ADR}. Read the corpus of accepted ADRs, invariants, and guidelines under {decisions_dir}, {invariants_dir}, {guidelines_dir}. Grade every applicable decision prong, run restatement detection against the corpus, grade the five prose checks, and emit exactly one audit-report JSON object."`

If the dispatch fails with `Agent type 'governance-auditor' not found` but `.claude/agents/governance-auditor.md` exists (installed this session), use the fallback in `commands/_shared-agent-routing.md` § "Fallback: agent installed this session".

Validate the report before reading anything out of it (INV-011 — fail closed on a missing, empty, or schema-nonconforming report; absence of output is never evidence of completion):  edikt-guard:allow

- **Missing, empty, or does not parse as JSON conforming to `templates/agents/audit-report.schema.json`** → REFUSE. Report the validation failure. Never treat "the agent said it was done" as evidence — the report is the evidence.
- **`meta.stubbed: true`** → REFUSE. A canned report standing in for real judgement must never gate a real acceptance; this only fires under `EDIKT_AUDIT_STUB=1`, which a real acceptance run never sets.
- **`class.value != "adr"`** → REFUSE. Print `class.value` and every entry in `class.ruled_out` — the auditor's reasoning for what this draft actually is, not merely that it isn't an ADR. Per D118, this is exactly the check that catches a documentation gap that grew a citation looking for authority to attach to.
- **Any `prose_checks[]` entry with `grade: "fail"`** → REFUSE (a blocking prose finding). Print the check, its quote, and its note.
- **`overall_verdict: "fail"` for any other reason** (a decision-prong failure, a restatement) → REFUSE, printing the failing prong or restatement verdict verbatim.

Only `overall_verdict: "pass"` with `class.value: "adr"` and no `stubbed: true` proceeds to 6c.

**Step 6c — compile through the Phase A dispatch contract, scoped to this one artifact.** This is what generates the sidecar; there is no separate sidecar-generation step.

`--bootstrap` is correct here and is NOT boilerplate to copy elsewhere. This ADR was written
seconds ago by this command, so it has never had a sidecar and there is nothing a
regeneration could destroy. Everywhere else, a missing sidecar might be one that was LOST —
and its `manual_directives` would be destroyed silently — so `phasea plan --artifact`
refuses without this flag and asks. Never add it to a re-extraction to get past that refusal.

`--bootstrap` is also what lets this plan compile an ADR that is still `status: proposed`. The
corpus excludes proposed ADRs, and `phasea plan --artifact` refuses one in every other case. A
declared bootstrap of a proposed ADR with no sidecar on disk is the single exception, because
6d may flip the status only after this step's verify exits 0. A rejected or superseded ADR, or a
proposed one that already has a sidecar, is still refused.

Follow `commands/gov/compile.md`'s Extract Directives procedure (steps A1–A4) exactly, with the plan scoped to just this ADR:

```bash
edikt phasea plan --artifact {ABS_PATH_TO_ADR} --bootstrap
```

Exit 4 is a refusal — report it verbatim and leave the ADR `proposed`. Besides a contract or path problem, it fires when an earlier plan for this artifact was never closed (dispatched and not restored, mid-retry, or FAILED), for instance from an interrupted earlier run of this command; the message names that plan and what to do.

Then dispatch the `sidecar-extractor` agent for the single planned item (A2), `edikt phasea restore --plan <plan_id>` (A3, not optional — A4 refuses without it), and `edikt phasea verify --plan <plan_id>` (A4). Compile.md's own exit-code table and re-dispatch-as-correction-pass rules apply unchanged; this step does not re-derive them. Three of its rules decide what this step reports:

- **A stall** is a final response from the extractor that is anything other than exactly one `SIDECAR WRITTEN: <path>` line — including an empty response, a watchdog, a kill or a crash. Pass its `item_id` to A4 as `edikt phasea verify --plan <plan_id> --stalled <item_id>`.
- **Restore owns `paths:`.** It writes `paths:` and `paths_approval:` back exactly as they stood before the dispatch (a new ADR has none), and drops a `paths:` the extractor wrote on a sidecar that had none — scope enters only through `proposed_paths` and its approval. A hand-back saying scope was lost or needs `edikt sidecar approve --kind paths` is wrong — do not act on it.
- **Report from the receipt, never from the extractor's hand-back:** the item's `status`, `anchors_valid`, `anchors_whitespace_only` and `needs_review`.

**Read the receipt's existence BEFORE the exit code** (never branch on a parsed field when the exit code is the contract). No receipt at `.edikt/state/phasea/<plan_id>.receipt.json` is a refusal at the plan/restore layer, not an extraction outcome — report the message verbatim.  edikt-guard:allow

**Step 6d — flip status, only on 6c's verify exiting 0.** Edit the ADR's frontmatter `status:` and the body's `**Status:**` line from `proposed` to `status: accepted` and `**Status:** accepted`, in that one edit, only after `phasea verify` for this artifact's plan returned exit 0 with the receipt on disk. Any other outcome (1, 2, 3, 5, 6, 7) leaves the artifact `proposed` and reports per compile.md's table — never retried automatically, never accepted anyway.

### 7. Run the verify gate

After both the `.md` and the `.edikt.yaml` are on disk, run the completion-evidence gate:

```bash
edikt verify gov ADR-{NNN}
```

This walks the new sidecar's `directives[].verify`, `prohibitions[].verify`, and `verification[].verify` and runs each as a shell command. Exit codes: `0` = all pass or all skipped; `1` = ≥ 1 failed.

- On exit 0 → proceed to Step 8 (Confirm). No surface.
- On exit 1 → surface a warning with the per-item failures and tell the user to either fix the directive prose / sidecar verify command. **Never auto-delete the artifact** — the user needs the file on disk to inspect and iterate.

Sample warning shape:

```
⚠ ADR-{NNN} created, but {failing_n} of {total} verify(s) failed:
  directive[0]: exit=1 — `! rg -P 'pattern' src/` matched lines
Fix the directive prose, edit the sidecar's verify: line, or remove it.
The artifact is on disk at {BASE}/decisions/{NNN}-{slug}.md.
```

### 8. Confirm

```
✅ ADR created: {BASE}/decisions/{NNN}-{slug}.md
✅ Sidecar written: {BASE}/decisions/{NNN}-{slug}.edikt.yaml
✅ Verify: {n} of {m} passed.   (or)   ⚠ Verify: {failing_n} of {m} failed — see above.

  ADR-{NNN}: {Title}
  Status: accepted (passed the Step 6 acceptance gate — audit-cite, governance-auditor, and phasea verify all held)

  To refine the directives:
  - Edit the sidecar's directives[] directly to add, edit, or remove rules
  - Re-run /edikt:adr:compile ADR-{NNN} to regenerate the sidecar from prose

  Next: Run /edikt:gov:compile to update governance directives.

  Want architect to review this? Say "review this ADR"
```
