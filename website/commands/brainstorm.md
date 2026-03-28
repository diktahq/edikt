# /edikt:brainstorm

A thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge, converging toward a PRD or SPEC when ready.

## Usage

```bash
/edikt:brainstorm
/edikt:brainstorm webhook retry logic
/edikt:brainstorm multi-tenant isolation strategy
/edikt:brainstorm --fresh
/edikt:brainstorm --fresh rethink our auth approach
```

| Argument | Description |
|----------|-------------|
| (none) | Asks what you want to brainstorm |
| A topic or idea | Starts the brainstorm with that topic |
| `--fresh` | Unconstrained mode — skips project context, challenges existing decisions |

## How it works

The brainstorm has two phases: **open exploration** and **guided narrowing**.

**Open exploration** is free-form conversation. edikt asks open-ended questions — what problem does this solve, who benefits, what does success look like — and lets you think out loud. There's no structure yet. The goal is to understand scope, motivation, and possibilities before narrowing.

**Guided narrowing** kicks in when the conversation starts converging — when you're making decisions more than asking "what if." edikt captures the state:

```text
It sounds like we're converging. Let me capture what we've discussed so far:

  Problem:       Users can't retry failed webhooks without manual DB intervention
  Approach:      Async retry queue with exponential backoff
  Key decisions: Max 5 retries, dead letter after exhaustion
  Open questions: Should retry config be per-endpoint or global?
  Constraints:   ADR-003 (error handling), INV-001 (plain markdown)

Does this capture it? Anything to add or change?
```

You refine until it's right, then formalize.

## Context modes

**Grounded mode** (default) loads your project context — ADRs, invariants, active specs, project-context.md. The conversation is grounded in what exists. edikt references relevant decisions during exploration:

```text
📚 Loaded project context (grounded mode)
   5 ADRs, 2 invariants, 1 active spec
```

**Unconstrained mode** (`--fresh`) skips all context loading. Use this when you want to challenge existing decisions or explore ideas that might contradict current architecture. Brainstorm freely — contradictions are surfaced later, at formalize time:

```text
🧹 Fresh brainstorm (unconstrained mode)
   No project context loaded. Existing decisions will not constrain this session.
   Contradictions with existing governance will be surfaced when you formalize.
```

The safety net: unconstrained brainstorming is free, but formalization forces you to reconcile with reality.

## Specialist agents

Agents join the brainstorm proactively when edikt detects domain signals in the conversation — database terms trigger the DBA, security terms trigger the security agent, API terms trigger the API agent, and so on.

Each agent provides 2–3 brief observations, not a full review:

```text
🪝 edikt: security has thoughts on this...

💭 security:
  - JWT rotation adds complexity — consider whether short-lived tokens solve the same problem
  - If you go with refresh tokens, the revocation list needs its own storage strategy
```

You can also invoke agents on demand: "what does the architect think?" or "get DBA input" triggers the agent immediately.

Each agent is triggered proactively only once per session. On-demand triggers are unlimited.

## Output

When you're ready to formalize, edikt offers three options:

```text
Ready to formalize. What should this become?

1. PRD — product requirements document (feature with user-facing requirements)
2. SPEC — technical specification (technical feature, requirements are clear)
3. Save brainstorm only — keep the notes, formalize later
```

Choosing PRD or SPEC saves the brainstorm artifact and then runs `/edikt:prd` or `/edikt:spec` with the brainstorm content as input context.

The brainstorm artifact is saved to `docs/brainstorms/`:

```yaml
---
type: brainstorm
id: BRAIN-001
title: "Webhook retry logic"
status: draft
mode: grounded
created: 2026-03-28
participants: [user, claude]
agents_consulted: [security, dba]
produces: spec
---
```

Sections: Problem, Exploration, Decisions, Open Questions, Constraints, Next.

If the brainstorm ran in `--fresh` mode and you choose PRD or SPEC, edikt loads project context at that point and checks for contradictions with existing ADRs and invariants — surfacing them explicitly before proceeding.

## Natural language triggers

- "let's brainstorm"
- "brainstorm this"
- "explore options for X"
- "I have an idea"
- "let's think through X"

## What's next

- [/edikt:prd](/commands/prd) — product requirements document
- [/edikt:spec](/commands/spec) — technical specification from an accepted PRD
- [/edikt:plan](/commands/plan) — phased execution plan
