---
title: "FAQ — edikt Governance for Claude Code"
description: "Common questions about edikt: governance chain, drift detection, quality gates, compiled directives, team setup, and Claude Code compatibility."
---

# FAQ

## Do I commit `.claude/rules/` to git?

**Yes.** Commit everything edikt generates — `.claude/rules/`, `.claude/CLAUDE.md`, `.claude/settings.json`, `.claude/agents/`, `docs/`, `.edikt/config.yaml`. This is how the whole team benefits. When a teammate opens the project in Claude Code, they get the same governance automatically.

## Will edikt overwrite my existing CLAUDE.md?

No. edikt generates `.claude/CLAUDE.md` (inside the `.claude/` directory), not the root `CLAUDE.md`. If you have an existing root `CLAUDE.md`, edikt won't touch it.

## What's the difference between edikt and just writing a CLAUDE.md?

A hand-written CLAUDE.md is a suggestion the model reads once. edikt compiles your decisions into path-conditional rules, lifecycle hooks, and quality gates that fire without anyone remembering to invoke them — and on a team, that removes the per-engineer discipline a shared CLAUDE.md depends on.

A CLAUDE.md drifts. A governance layer compounds.

**Full argument:** [What is edikt?](/what-is-edikt)

## What's the governance chain?

The sequence from intent to implementation to verification. You drive it through natural language:

- (Optional) "Let's brainstorm this" → open-ended exploration that converges into a PRD, a spec, or a saved brainstorm doc to formalize later
- (Optional, conditional) "Run discovery on this" → for ideas with unknowns worth resolving first; can start fresh or lift a saved brainstorm, and always graduates into a PRD
- "Write a PRD for [feature]" → structured requirements with acceptance criteria
- "Write a spec for PRD-005" → technical specification from the accepted PRD
- "Generate spec artifacts for SPEC-005" → data model, API contracts, migrations, test strategy
- "Create a plan for SPEC-005" → phased execution with specialist pre-flight review
- Execute — the model builds with enforced standards
- "Does the implementation match the spec?" → drift detection closes the loop

Brainstorm and discovery are entry points, not required stops — an idea can go straight to a PRD. From the PRD on, each step references the one before it, and each must be accepted before the next begins.

**Full explanation:** [Governance Chain](/governance/chain)
**Command references:** `/edikt:brainstorm`, `/edikt:sdlc:discovery`, `/edikt:sdlc:prd`, `/edikt:sdlc:spec`, `/edikt:sdlc:artifacts`, `/edikt:sdlc:plan`, `/edikt:sdlc:drift`

## What's drift detection?

Ask "does the implementation match the spec for SPEC-005?" and it compares the implementation against the technical specification and the original PRD. It identifies divergences — features that were specified but not built, patterns that were decided but not followed, acceptance criteria that aren't covered by tests.

Drift detection is the verification step that closes the governance chain.

**Command reference:** `/edikt:sdlc:drift SPEC-005`

## What are quality gates?

When a specialist agent detects a critical finding — a hardcoded secret, a migration without a rollback, an API breaking change — the model presents it to you and blocks progression. You can override the gate, but overrides are logged with your git identity.

Gates fire automatically via the SubagentStop hook and pre-flight review. You don't trigger them.

Quality gates make enforcement visible. They're the difference between "we have standards" and "standards are enforced."

## Why did a write get refused that used to go through?

Because as of v0.7.0 a `must`-grade write is refused *before* it hits disk. A deny-channel bug
previously let that write land and only killed the assistant's turn afterward; that's fixed. Landing
alongside it, grade derivation now reads the actual obligation strength of what you wrote — a plain,
unconditional "MUST" grades as `must`, not just "MUST NOT"/"NEVER" as before — which reclassified 404
of 420 previously-`advisory` directives in this project's own corpus. Together, on a corpus you didn't
change, those 404 rules went from informational-only to enforced.

This isn't a bug to route around — it's an existing rule finally enforcing what it says. If a specific
refusal looks wrong for what you actually meant, reword the directive to genuinely conditional language
(`SHOULD`, `MAY`, or a qualified `MUST`) rather than bypassing the gate. Full detail:
[Upgrading to v0.7.0](/guides/v0.7.0-upgrade).

## How do I compile governance?

After capturing decisions with "Save this decision" or adding invariants with "That's a hard rule", say: "Compile governance."

edikt reads your accepted ADRs, active invariants, and team guidelines and produces four things under `.claude/rules/`: an always-on core loaded every session, per-topic files (under `.claude/rules/governance/`) that load when you touch matching code, a machine-readable index the write-time hooks read, and a manifest that proves nothing drifted. The ADRs are the source of truth. The compiled output is the enforcement format.

**Command reference:** `/edikt:gov:compile`

## Does edikt replace my linter or CI pipeline?

No. edikt works upstream — it tells the model the standards before code is written, so violations are prevented rather than caught. Your linter still runs. Your CI still validates. edikt's job is to make the linter boring.

## Can I use edikt on a team?

Yes. Commit the generated files. Every teammate gets identical governance — same standards, same agents, same decisions, same quality gates. No per-developer setup, no drift.

## Can I use edikt across multiple projects?

Yes. Run `/edikt:init` in each project. Each project gets its own rules matched to its stack, its own decisions, its own agents. The governance chain and quality gates work independently per project.

If you want a shared baseline — say, your consultancy's core standards — set up a base `.edikt/config.yaml` template and customize per project. The methodology stays constant; the stack-specific rules vary.

## What's the maintenance overhead?

Low. Rules update when you re-run the install script (new templates from upstream). Decisions update when you compile governance. There's no daemon running, no service to maintain, no subscription to manage. The files are in your repo — version-controlled like everything else.

Per project, maintenance is: update edikt when a new version ships, recompile when you capture new decisions. Minutes per month, not hours.

## What happens when Claude Code updates?

edikt uses Claude Code's official platform primitives — rules, hooks, agents, settings.json. These are Anthropic's documented surface area. When Claude Code ships new hook types or rule capabilities, edikt adopts them.

edikt has tracked every Claude Code platform change since rules were introduced. Breaking changes are rare; when they happen, edikt ships a patch.

## Does it work with Cursor or other AI tools?

The knowledge base (project-context.md, ADRs, specs, product docs) is plain markdown that works anywhere. But the governance loop — lifecycle hooks, path-conditional rules, quality gates, specialist agents, slash commands — currently only works in Claude Code, the most advanced harness for the primitives that loop depends on. Claude Code today — other harnesses are the stated direction, not a closed door. See [Why Claude Code today](/what-is-edikt#why-claude-code-today) for the full breakdown.

## How do I update rules after changing config?

Edit `.edikt/config.yaml`, then run `/edikt:init` again. edikt regenerates rules from the updated config without touching files you've manually edited.

## Something broke. How do I reset?

Delete `.claude/rules/` and `.edikt/config.yaml`, then run `/edikt:init` again. Or run `/edikt:doctor` to diagnose the issue first.

## How do I roll back a bad release?

```bash
edikt rollback
```

This reverts `~/.edikt/current` to the previous generation. Note: rollback is payload-only. Structural migrations are permanent and are not reversed by rollback — this includes the v0.4.x → v0.6.0 sentinel-to-sidecar migration and the v0.7.0 sidecar schema v1 → v2 upgrade (`edikt migrate to-v2`). Directive grades recompiled under v0.7.0's fixed derivation also stay recompiled; rollback does not un-grade a directive. If you need to go back further than one generation, use `edikt use <version>` with a version from `edikt list`.

## Can I pin edikt per project?

Yes. Run from inside the project directory:

```bash
edikt upgrade --pin v0.7.1
```

The pin is stored in `~/.edikt/lock.yaml` (global mode) or `.edikt/lock.yaml` (project mode). Subsequent `edikt upgrade` calls are no-ops until you clear it with `edikt upgrade --pin clear`.

## What happened to my old `~/.edikt/hooks/`?

v0.6.0 migrated the flat layout to a versioned one. Your hooks are now at `~/.edikt/versions/<version>/hooks/` and `~/.edikt/hooks` is a symlink to `~/.edikt/current/hooks`. Nothing was deleted. Run `edikt doctor` to verify the symlink health.

## Why did `brew upgrade edikt` run but `edikt upgrade` still says there's an update?

They update different things. `brew upgrade edikt` updates the launcher binary (`bin/edikt`) — the small shell script that manages payload versions. `edikt upgrade` updates the payload — templates, commands, hooks, agents. They're versioned independently. After `brew upgrade edikt`, run `edikt upgrade` to also update the payload.

---

Still have questions? [Open an issue on GitHub](https://github.com/diktahq/edikt/issues).

Ready to try it? [Get started in 5 minutes](/getting-started).
