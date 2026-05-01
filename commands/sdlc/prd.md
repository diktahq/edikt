---
name: edikt:sdlc:prd
description: "Write a Product Requirements Document for a feature"
effort: high
argument-hint: "<feature description>"
allowed-tools:
  - Read
  - Write
  - Bash
  - Glob
---
!`bash -c 'CFG=""; D="$PWD"; while [ "$D" != "/" ]; do [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break; D=$(dirname "$D"); done; [ -z "$CFG" ] && { printf "<!-- edikt:live -->\nNext PRD number: PRD-001\nExisting PRDs: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; PROOT=$(dirname "$(dirname "$CFG")"); REL=$(grep "^  prds:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); if [ -z "$REL" ]; then BASE=$(grep "^base:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); BASE="${BASE:-docs}"; REL="$BASE/product/prds"; fi; case "$REL" in /*) DIR="$REL" ;; *) DIR="$PROOT/$REL" ;; esac; COUNT=$(find "$DIR" -maxdepth 1 -type f -name "PRD-*.md" 2>/dev/null | wc -l | tr -d " "); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(find "$DIR" -maxdepth 1 -type f -name "PRD-*.md" 2>/dev/null | sort | xargs -I{} basename {} .md | tr "\n" "," | sed "s/,$//"); printf "<!-- edikt:live -->\nNext PRD number: PRD-%s\nExisting PRDs: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"'`

# edikt:sdlc:prd

Write a Product Requirements Document (PRD) for a feature or change.

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

Read `.edikt/config.yaml`. Resolve paths from the `paths:` section (fall back to defaults if not configured):

- PRDs: `paths.prds` (default: `docs/product/prds`)
- Project Context: `paths.project-context` (default: `docs/project-context.md`)
- Template override: check if `.edikt/templates/prd.md` exists — if yes, use it as the output template instead of the built-in template below

### 2. Load Context

```bash
ls {prds_path}/*.md 2>/dev/null | sort
```

Read `{project_context_path}` for project identity, users, and stack.
Read `{BASE}/product/spec.md` if it exists — for roadmap context.

The correct next PRD number is provided at the top of this prompt in the `<!-- edikt:live -->` block. Use it exactly — do not guess or count files yourself.

### 3. Clarify Requirements

If `$ARGUMENTS` is vague or missing, present all clarifying questions in a **single batched message** per Opus 4.7 best-practices guidance. Use this format:

```
I need a few answers before writing the PRD. Answer any subset — defaults apply for skipped [optional] items.

1. [required] Who is this for? (which user type)
2. [required] What problem does it solve?
3. [optional — default: "measurable user outcome"] What does success look like?
```

If `$ARGUMENTS` is clear enough, skip the interview and proceed directly.

### 4. Write the PRD

Create `{BASE}/product/prds/PRD-{NNN}-{slug}.md`:

```markdown
---
type: prd
id: PRD-{NNN}
title: {Feature Title}
status: draft
author: {git user.name}
stakeholders: []
created_at: {ISO8601 timestamp}
references:
  adrs: []
  invariants: []
---

# PRD-{NNN}: {Feature Title}

**Status:** draft
**Date:** {today}
**Author:** {git user.name}

---

## Problem

{What problem this solves. Who has it. How painful it is. Include evidence if available.}

## Users

{Who this is for — be specific. Reference project-context.md user types if defined.}

## Goals

- {What success looks like — measurable where possible}
- {What we're optimizing for}

## Non-Goals

- {What this explicitly does NOT solve}
- {Out of scope for this version}

## Requirements

### Must Have
- FR-001: {Requirement} [MUST]
- FR-002: {Requirement} [MUST]

### Should Have
- FR-003: {Requirement} [SHOULD]

### Won't Have (v1)
- FR-004: {Deferred requirement}

## User Stories

**P1** — **As a** {user type}, **I want** {action} **so that** {benefit}.

**P2** — **As a** {user type}, **I want** {action} **so that** {benefit}.

## Acceptance Criteria

- [ ] AC-001: {Testable criterion} — Verify: {how to check — automated test, manual review, or command}
- [ ] AC-002: {Testable criterion} — Verify: {how to check}
- [ ] AC-003: {Testable criterion} — Verify: {how to check}

## Technical Notes

{Constraints, dependencies, integration points — or "TBD".}
{If anything is unclear, mark it:}
- NEEDS CLARIFICATION: {question that must be answered before implementation}

## Open Questions

- {Question that needs resolution before implementation}

---

*Written by edikt:prd — {date}*
```

---

REMEMBER: Every requirement needs a numbered ID (FR-NNN) and every acceptance criterion needs a verification method (AC-NNN: criterion — Verify: method). If something is unclear, mark it NEEDS CLARIFICATION — never invent details.

### 5. Confirm

```
✅ PRD created: {BASE}/product/prds/PRD-{NNN}-{slug}.md

  PRD-{NNN}: {Feature Title}
  Status: draft

  Review and change status to "accepted" when ready.
  Next: Run /edikt:sdlc:spec for PRD-{NNN}

  Want pm to review this? Say "review this PRD"
```
