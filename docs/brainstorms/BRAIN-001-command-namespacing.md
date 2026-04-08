---
type: brainstorm
id: BRAIN-001
title: "Command Namespacing and Restructuring"
status: draft
mode: grounded
created: 2026-03-31
participants: [user, claude]
agents_consulted: []
produces: plan
---

# Command Namespacing and Restructuring

## Problem

25 flat commands give no signal about which artifacts they touch or how they relate to each other. A new user sees `/edikt:` and gets an undifferentiated list. The SDLC chain (prd → spec → artifacts → plan → review → drift) isn't discoverable from command names alone. Governance commands (compile, review-governance, sync, rules-update) don't signal they all touch `.claude/rules/`. `review-governance` is a confusing name after the v0.2.0 redesign.

Nested namespacing in Claude Code was confirmed working via live test (`edikt:test:hello` resolved from `~/.claude/commands/edikt/test/hello.md`).

## Approach

Artifact-centric namespacing for decisions. Chain-centric for SDLC. Assembly-centric for governance. Flat for setup and daily use. Old commands deprecated with pointer to new name, removed in v0.4.0.

## Key Decisions

**Final command structure:**

```
edikt:adr:new / :compile / :review
edikt:invariant:new / :compile / :review
edikt:guideline:new / :review

edikt:gov:compile / :review / :rules-update / :sync

edikt:sdlc:prd / :spec / :artifacts / :plan / :review / :drift / :audit

edikt:docs:review / :intake

edikt:capture
edikt:init
edikt:upgrade
edikt:doctor
edikt:status
edikt:context
edikt:brainstorm
edikt:session
edikt:team
edikt:agents
edikt:mcp
```

**Namespace identities:**
- `adr/invariant/guideline:` — the decision artifacts
- `gov:` — everything that touches `.claude/rules/`
- `sdlc:` — the chain from requirements to verification
- `docs:` — documentation quality and structure

**Deprecation rules:**
- All 25 old commands stay as stubs in v0.3.0, pointing to new names
- Deprecated stubs removed in v0.4.0
- Standard deprecation format across all stubs

**`edikt:adr:compile` vs `edikt:gov:compile`:**
- `adr:compile` / `invariant:compile` — targeted, regenerate sentinel for one specific document (or all of that type)
- `gov:compile` — full assembly: generates missing sentinels across all types, then compiles to topic files

**New commands (net new, written from scratch):**
- `edikt:capture` — mid-session sweep: ADR candidates, invariant candidates, doc gaps
- `edikt:guideline:new` — create a guideline file
- `edikt:guideline:review` — review guideline language quality
- `edikt:adr:compile` — generate/update sentinel for one ADR or all
- `edikt:invariant:compile` — generate/update sentinel for one invariant or all
- `edikt:gov:review` — review compiled governance output quality

## Open Questions

- None — all resolved during brainstorm

## Constraints

- INV-001: commands are plain markdown only, no compiled code
- ADR-001: Claude Code only
- Nested namespacing confirmed working (live test 2026-03-31)
- Deprecation period: v0.3.0 keeps stubs, v0.4.0 removes them

## Implementation Checklist

Things that must be audited to avoid breaking changes:

1. All 25 command `.md` files renamed/moved to new subdirectory structure
2. All old command names → deprecated stubs with standard format
3. `install.sh` — create subdirectories when copying commands
4. `CLAUDE.md` template — natural language intent table updated
5. `CLAUDE.md` (dogfood) — same
6. Website pages — all command references updated, URL redirects for old paths
7. README — command list updated
8. Test suite — all command name references updated
9. `templates/agents/_registry.yaml` — any command references
10. Hook files — any command references in hook output
11. New commands written from scratch: `capture`, `guideline:new`, `guideline:review`, `adr:compile`, `invariant:compile`, `gov:review`

## Next

Plan: implement command namespacing as v0.3.0
