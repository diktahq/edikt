---
name: edikt:adr:new
description: "Capture an architecture decision record — from scratch or from the current conversation"
effort: normal
argument-hint: "[decision topic] — omit to extract from conversation"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---
!`bash -c 'CFG=""; D="$PWD"; while [ "$D" != "/" ]; do [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break; D=$(dirname "$D"); done; [ -z "$CFG" ] && { printf "<!-- edikt:live -->\nNext ADR number: ADR-001\nExisting ADRs: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; PROOT=$(dirname "$(dirname "$CFG")"); REL=$(grep "^  decisions:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); if [ -z "$REL" ]; then BASE=$(grep "^base:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); BASE="${BASE:-docs}"; REL="$BASE/architecture/decisions"; fi; case "$REL" in /*) DIR="$REL" ;; *) DIR="$PROOT/$REL" ;; esac; COUNT=$(find "$DIR" -maxdepth 1 -type f -name "ADR-*.md" 2>/dev/null | wc -l | tr -d " "); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(find "$DIR" -maxdepth 1 -type f -name "ADR-*.md" 2>/dev/null | sort | xargs -I{} basename {} .md | tr "\n" "," | sed "s/,$//"); printf "<!-- edikt:live -->\nNext ADR number: ADR-%s\nExisting ADRs: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"'`

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

edikt v0.3.0+ supports project-level template overrides per [ADR-005](../../docs/architecture/decisions/ADR-005-extensibility-model.md) and [ADR-009](../../docs/architecture/decisions/ADR-009-invariant-record-terminology.md). Follow this precedence when selecting the template for the new ADR:

1. **Project template**: if `.edikt/templates/adr.md` exists in the project, use it as the output template. This is the highest priority — user projects own their template shape.
2. **Inline fallback (v0.2.x legacy projects only)**: if no project template exists AND the project's `edikt_version` in `.edikt/config.yaml` is `< 0.3.0` or missing, use the inline template shown in step 5 below. Print a one-time warning:
   ```
   ⚠ No project ADR template found. Using the legacy inline fallback.
     Run /edikt:upgrade followed by /edikt:init to set up project templates
     and get the full v0.3.0 template adaptation feature.
   ```
   This keeps v0.2.x projects working during the upgrade window.
3. **Refuse (v0.3.0+ projects with missing templates)**: if no project template exists AND the project's `edikt_version` is `>= 0.3.0`, refuse with a clear error:
   ```
   ❌ No project ADR template found.

   This project is on edikt v{version}, which requires an explicit
   project template. edikt doesn't assume a style — your project owns this.

   To set up templates, run:
     /edikt:init                     (interactive setup — pick from
                                      Adapt, Start fresh, or Write my own)
     /edikt:init --reset-templates   (regenerate templates, overwriting
                                      any existing ones)

   Or create .edikt/templates/adr.md manually with the required
   [edikt:directives:start]: # / [edikt:directives:end]: # sentinel block.

   See docs/internal/product/prds/PRD-001-spec/invariant-record-template.md
   for the template contract (the ADR template follows the same pattern).
   ```
   Do NOT fall back to the inline template. Do NOT write the ADR. Exit.

**No global default**: edikt does NOT ship a single "default ADR template" that is auto-installed into every project. Projects either explicitly pick a template during init, write their own, or fall back to the inline template in v0.2.x legacy mode only. See ADR-009 and PROPOSAL-001 for rationale.

**Checking the project edikt_version**: extract it from `.edikt/config.yaml`:
```bash
PROJECT_EDIKT_VERSION=$(grep '^edikt_version:' .edikt/config.yaml 2>/dev/null | awk '{print $2}' | tr -d '"')
```
Compare to `0.3.0` using semver ordering (not string comparison). A missing `edikt_version` line means v0.2.x legacy.

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
  /edikt:adr:new SPEC-042
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
  --- from ADR-042 ---
  {content}
  --- from feature/redis-migration (branch diff) ---
  {content}
  ```
- **Primary sources**: resolved references dominate the draft. If `docs/specs/redis-cache.md` contains benchmarks, the ADR's Decision section cites those benchmarks. If ADR-042 established a related constraint, the new ADR's Consequences section acknowledges it.
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
| `/edikt:adr:new "Decide session cache using docs/specs/redis-cache.md and SPEC-042"` | Prose with embedded path and identifier; read both, use as primary sources, interview only for what's missing |
| `/edikt:adr:new SPEC-042` | Identifier resolves, read spec, treat as primary source |
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

### 5. Write the ADR

Create `{BASE}/decisions/{NNN}-{slug}.md`:

```markdown
---
type: adr
id: ADR-{NNN}
title: {Title}
status: accepted
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

**Status:** accepted
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

## Confirmation

How to verify this decision is being followed:
- {Automated: command, test, or hook that checks compliance}
- {Manual: what a reviewer should look for in code review}

## Directives

LLM-enforceable rules compiled from this ADR. Generated by `/edikt:gov:review`, consumed by `/edikt:gov:compile`.

[edikt:directives:start]: #
paths:
  - {glob patterns for files this ADR applies to — e.g. "**/*.go", "docs/architecture/**"}
scope:
  - {activity scopes — e.g. planning, design, review, implementation}
directives:
  - {Each decision statement rewritten as an enforceable directive}
  - {Use MUST/NEVER for hard constraints, specific names for patterns}
  - {One directive per line, include (ref: ADR-{NNN}) suffix}
[edikt:directives:end]: #

---

*Captured by edikt:adr — {date}*
```

An ADR should cover ONE decision, not a design document. If it exceeds 2 pages, it's probably a spec. Keep it focused: one question, one decision, one set of consequences.

---

REMEMBER: An ADR captures a DECISION with trade-offs, not a preference. It must include a Confirmation section describing how to verify the decision is being followed. If it exceeds 2 pages, it's a spec, not an ADR.

### 6. Auto-chain to /edikt:adr:compile (ADR-008)

Per ADR-008, this command auto-chains to `/edikt:adr:compile ADR-{NNN}` at the end of its workflow so the newly-created ADR has its directive sentinel block populated immediately. Fresh artifacts have nothing to preserve (no manual directives, no suppressed directives, no hand-edits), so the compile runs the slow path cleanly and the user never has to remember to compile after new.

Run `/edikt:adr:compile ADR-{NNN}` now. If it produces an error (e.g., headless mode with strategy flags), surface the error but do NOT roll back the ADR creation — the body is already written and the user can run compile manually later.

If the compile succeeds, the directive block is populated with:
- `source_hash` — SHA-256 of the ADR body (excluding the block)
- `directives_hash` — SHA-256 of the auto `directives:` list
- `compiler_version` — current edikt version
- `directives:` — auto-generated from the `## Decision` section
- `manual_directives: []` — empty; user adds rules compile missed
- `suppressed_directives: []` — empty; user adds rules compile got wrong

### 7. Confirm

```
✅ ADR created: {BASE}/decisions/{NNN}-{slug}.md
✅ Directives compiled: {k} auto directives

  ADR-{NNN}: {Title}
  Status: accepted

  Review it and change status to "proposed" if it needs team sign-off first.

  To refine the directives:
  - Add rules compile missed → edit manual_directives: in the block
  - Reject wrong auto rules → add to suppressed_directives: in the block
  - Re-read ADR-008 for the three-list schema contract

  Next: Run /edikt:gov:compile to update governance directives.

  Want architect to review this? Say "review this ADR"
```
