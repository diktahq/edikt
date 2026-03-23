# edikt — Project Context

## Vision

Your architecture decisions governed and enforced across every line of AI-generated code. The full Agentic SDLC governed from requirements to verification.

## Mission

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification — closing the gap between what you decided and what gets built.

## What Is This

edikt is the governance layer for agentic engineering. It delivers on the vision through two systems that work independently but reinforce each other.

**Architecture governance & compliance.** Capture architecture decisions (ADRs), hard constraints (invariants), and team conventions (guidelines). `/edikt:compile` reads all three, checks for contradictions, and produces a single governance file Claude reads automatically — every session, before writing any code. Rule packs add correctness guardrails to the same enforcement surface. Signal detection suggests capturing new decisions mid-session. Together, governed decisions and compiled enforcement define everything Claude must follow.

**Agentic SDLC governance.** A status-gated chain — PRD → spec → artifacts → plan → execute → drift detection — where each step is reviewed by specialist agents, constrained by compiled governance, and produces new decisions that feed back into the next compilation.

The two systems create a flywheel: the lifecycle produces engineering decisions, and compiled decisions govern the lifecycle. Decisions compound rather than decay. Every session is more governed than the last.

## Problem It Solves

Claude Code has memory — but it's local to each engineer, never shared, never consistent across a team. There's no governance: no enforced standards, no persistent decisions, no feedback loop when Claude drifts from what the team agreed on.

edikt fixes this by:

1. **Governing architecture and compiling into enforcement** — ADRs, invariants, and guidelines are captured, managed, and compiled into directives Claude follows automatically every session
2. **Governing the Agentic SDLC** — PRD → spec → artifacts → plan → execute → drift detection, with status enforcement at each transition
3. **Enforcing through hooks** — 9 lifecycle hooks: auto-format, plan injection, compaction recovery, signal detection, quality gates
4. **Specialist review** — 18 domain agents (DBA, security, SRE, architect) review plans and implementations
5. **Detecting drift** — Verifies implementation matches the spec, PRD, and ADRs with confidence-based severity

## Who Uses It

Engineers who own the full spec-to-implementation cycle with Claude Code — solo agentic engineers, team leads standardizing AI across their team, and consultancies applying a repeatable methodology across client projects.

## Core Principles

- **Governance over context** — Context makes Claude informed; governance makes Claude consistent
- **Enforcement over suggestion** — Rules in `.claude/rules/` fire automatically; hooks run without being invoked
- **Traceability over trust** — Every decision traced from PRD through implementation to verification
- **Decisions compile into enforcement** — ADRs are the source of truth; compiled directives are the enforcement format
- **Infer, don't interrogate** — User describes, edikt figures out the rest
- **Plain markdown, no magic** — No build step, no runtime, no proprietary formats
- **Claude Code native** — Uses the platform's primitives (rules, agents, hooks) instead of reinventing them

## Non-Negotiables

- Commands are `.md` files — no compiled code, no build step
- Installation is copy files — no npm, no dependencies
- The governance chain enforces status transitions — PRD accepted before spec, spec before artifacts
- ADRs are the source of truth — no manual edits to compiled governance directives
- Works without external services (no Linear/Jira required for core features)

## Stack

- Markdown for commands, templates, and governance directives
- YAML for configuration (`.edikt/config.yaml`)
- Shell scripts for hooks (bash, no external dependencies)
- No runtime dependencies — pure Claude Code slash commands

## Name

edikt — an authoritative decree. You capture engineering decisions. edikt compiles them into decrees your AI agent follows automatically.

Part of the K-family: dikta (platform) → edikt (governance) → verikt (architecture validation). All derived from Latin authority words with an engineered K.

## Category

Agentic Engineering Governance

## Voice

Precise. Direct. Authoritative. Forward.
