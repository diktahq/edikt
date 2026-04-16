---
name: edikt:brainstorm
description: "Brainstorm features, explore design space, converge toward PRD or spec"
effort: high
argument-hint: "[topic or feature idea] [--fresh]"
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Grep
  - Bash
  - Agent
  - AskUserQuestion
---

# edikt:brainstorm

A thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge, converging toward a PRD or SPEC when ready.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Instructions

### Step 1: Determine Context Mode

Check if `--fresh` is in `$ARGUMENTS`. If present, strip it from arguments before passing to later steps.

- **Grounded mode** (default, no `--fresh`):
  Run `/edikt:context` logic. Load project context, ADRs, invariants, active specs, and config. This grounds the conversation in what exists.

  Read `.edikt/config.yaml` for paths. Read `docs/project-context.md` (or configured `paths.soul`). List ADRs, invariants, and active specs.

  Output:
  ```
  📚 Loaded project context (grounded mode)
     {n} ADRs, {m} invariants, {k} active specs
  ```

- **Unconstrained mode** (`--fresh`):
  Skip context loading entirely. Output:
  ```
  🧹 Fresh brainstorm (unconstrained mode)
     No project context loaded. Existing decisions will not constrain this session.
     Contradictions with existing governance will be surfaced when you formalize.
  ```

### Step 2: Understand the Topic

From `$ARGUMENTS` (after stripping `--fresh`):

- **If provided:** Use as the starting topic. Summarize your understanding and ask: "Is this what you want to explore?"
- **If empty:** Ask: "What do you want to brainstorm? Describe the idea, problem, or feature."

### Step 3: Open Exploration

Free-form conversation. The goal is to understand scope, motivation, constraints, and possibilities.

Ask open-ended questions:
- "What problem does this solve?"
- "Who benefits?"
- "What does success look like?"
- "What alternatives have you considered?"

**Grounded mode:** Connect to project context. Reference relevant ADRs, invariants, active specs, and existing architecture. Example: "We have ADR-003 about X — does this interact with that?", "The current spec-artifacts flow does Y — is this related?"

**Unconstrained mode:** Explore freely without referencing existing governance. The user chose `--fresh` because they want to challenge assumptions. Do not bring in ADRs or invariants during this phase.

Do NOT force structure yet. Let the conversation breathe.

**Specialist agent triggers (proactive):**

While the conversation flows, detect domain signals from the user's responses using the Domain Signal Detection table in the Reference section. When a domain is detected for the first time in the conversation, spawn the specialist agent with a brief, non-blocking input:

```
🪝 edikt: {agent} has thoughts on this...
```

Spawn an Agent:
```
Agent(
  prompt: "You are the {agent_name} specialist. Read this brainstorm conversation summary and provide 2-3 brief observations from your domain perspective. Do NOT do a full review. Keep it to 2-3 bullet points — observations, considerations, or questions that might shape the brainstorm. Format your response exactly as:

💭 {agent_name}:
  - {observation or consideration}
  - {observation or question}

Conversation summary: {summary of brainstorm so far}",
  description: "{agent_name} brief input on brainstorm"
)
```

Each agent is triggered proactively only ONCE per brainstorm session. Track which agents have already contributed.

**Specialist agent triggers (on-demand):**

If the user asks "what does {agent} think?" or "get {domain} input", spawn the agent immediately with the same brief format. On-demand triggers are not limited — the user can invoke any agent as many times as they want.

### Step 4: Guided Narrowing

When the conversation starts converging — decisions are being made, scope is narrowing, the user is saying "yes" more than "what if" — transition to guided mode:

```
It sounds like we're converging. Let me capture what we've discussed so far:

  Problem:       {summary}
  Approach:      {summary}
  Key decisions: {list}
  Open questions: {list}
  Constraints:   {grounded: relevant ADRs, invariants, project constraints that apply}
                 {unconstrained: "none loaded — will check at formalize time"}

Does this capture it? Anything to add or change?
```

Continue refining until the user is satisfied.

### Step 5: Formalize

When the user signals readiness ("looks good", "let's build this", "I'm ready"), offer the choice:

```
Ready to formalize. What should this become?

1. PRD — product requirements document (feature with user-facing requirements)
2. SPEC — technical specification (technical feature, requirements are clear)
3. Save brainstorm only — keep the notes, formalize later
```

- **If PRD:** Save the brainstorm artifact (Step 6), then run `/edikt:prd` with the brainstorm content as input context.
- **If SPEC:** Save the brainstorm artifact (Step 6), then run `/edikt:spec` with the brainstorm content as input context.
- **If save only:** Save the brainstorm artifact (Step 6) and stop.

**Unconstrained mode — contradiction check at formalize time:**

If the brainstorm ran in `--fresh` mode and the user chose PRD or SPEC, load project context NOW. Read ADRs, invariants, and active specs. Check the brainstorm decisions against existing governance for contradictions.

If contradictions found:
```
Checking brainstorm against existing governance...

⚠ This brainstorm challenges existing decisions:
  - {ADR-NNN}: "{title}" — brainstorm proposes {contradiction}
  - {INV-NNN}: "{title}" — brainstorm suggests {contradiction}

Options:
  1. Proceed — the PRD/SPEC will note these as proposed changes. You'll need to supersede the ADRs.
  2. Adjust — revisit the brainstorm decisions to align with existing governance.
  3. Cancel — save brainstorm only, formalize later.
```

If no contradictions found:
```
Checking brainstorm against existing governance...

✅ No contradictions with existing ADRs or invariants. Proceeding.
```

This is the safety net: unconstrained brainstorming is free, but formalization forces you to reconcile with reality.

### Step 6: Save Brainstorm Artifact

Resolve the brainstorms path from `.edikt/config.yaml` (`paths.brainstorms`, default: `docs/brainstorms/`).

Create the directory if it doesn't exist.

Count existing brainstorm files to determine the next number:
```bash
COUNT=$(ls docs/brainstorms/BRAIN-*.md 2>/dev/null | wc -l | tr -d ' ')
NEXT=$(printf "%03d" $((COUNT + 1)))
```

Write to `{brainstorms_path}/BRAIN-{NNN}-{slug}.md` using the Brainstorm Artifact Template in the Reference section.

Output:
```
✅ Brainstorm saved: {path}

  BRAIN-{NNN}: {Title}
  Mode: {grounded | unconstrained}
  Produces: {PRD-NNN | SPEC-NNN | pending}
```

---

REMEMBER: This is a builder tool, not a governance artifact. Keep the conversation natural. Don't over-structure the open phase. Let the user lead. Agents provide brief observations (2-3 points), NOT full reviews. The value is in the thinking, not the document. Each agent triggered proactively only ONCE per session.

## Reference

### Domain Signal Detection

| Domain | Signals | Agent |
|---|---|---|
| Database | SQL, query, schema, migration, index, database, db, table, foreign key, join, transaction, ORM, Postgres, MySQL, SQLite, MongoDB | `dba` |
| Infrastructure | deploy, docker, kubernetes, k8s, terraform, helm, CI, CD, infra, container, Dockerfile, compose, nginx, AWS, GCP, Azure, cloud | `sre` |
| Security | auth, JWT, OAuth, payment, PCI, HIPAA, token, secret, encrypt, credential, password, permission, role, RBAC, CORS, XSS, injection | `security` |
| API | API, endpoint, REST, GraphQL, route, webhook, contract, openapi, swagger, versioning, breaking change | `api` |
| Architecture | bounded context, domain, architecture, refactor, pattern, layer, dependency, coupling, abstraction, interface, hexagonal, clean arch | `architect` |
| Performance | performance, N+1, cache, latency, throughput, slow, optimize, index, query optimization, benchmark | `performance` |

### Agent Brief Format

Agents provide 2-3 brief observations — NOT full reviews.

```
💭 {agent_name}:
  - {observation or consideration}
  - {observation or question}
```

### Brainstorm Artifact Template

```markdown
---
type: brainstorm
id: BRAIN-{NNN}
title: "{title}"
status: draft
mode: {grounded | unconstrained}
created: {YYYY-MM-DD}
participants: [user, claude]
agents_consulted: [{list of agents that participated}]
produces: {prd | spec | pending}
---

# {Title}

## Problem
{what problem does this solve}

## Exploration
{key discussion points, options considered, trade-offs discussed}

## Decisions
{decisions made during the brainstorm}

## Open Questions
{unresolved questions}

## Constraints
{relevant ADRs, invariants, project constraints that apply}

## Next
{what this produces — PRD-NNN, SPEC-NNN, or "formalize later"}
```
