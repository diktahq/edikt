---
name: edikt:agents
description: "List, inspect, and manage specialist agents"
effort: low
allowed-tools:
  - Read
  - Write
  - Edit
  - Glob
  - Bash
---

# edikt:agents

List, inspect, and manage specialist agent templates installed in `.claude/agents/`.

## Arguments

- No argument: list installed and available agents
- `add {slug}`: install an agent from `~/.edikt/templates/agents/`
- `remove {slug}`: uninstall an agent from `.claude/agents/`
- `show {slug}`: print the full agent prompt
- `suggest`: recommend agents based on detected stack

## Instructions

### Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Parse argument

Check if an argument was provided and which subcommand it is.

### No argument — List agents

1. List installed agents:
   ```bash
   ls .claude/agents/*.md 2>/dev/null
   ```
   For each file, read the frontmatter to extract `name` and `description`.

2. List available templates not yet installed:
   ```bash
   ls ~/.edikt/templates/agents/*.md 2>/dev/null | grep -v '_registry'
   ```
   Cross-reference with installed list to find uninstalled ones.

3. Output:
   ```
   Installed agents ({n}):
     architect  — System design, ADRs, trade-off analysis
     engineer       — Implementation leadership, code review
     backend       — Backend implementation, business logic, APIs
     ...

   Available (not installed):
     data       — Data modeling, analytics, pipelines
     performance   — Performance profiling and optimization
     ...

   Usage:
     /edikt:agents add data
     /edikt:agents show security
     /edikt:agents remove pm
     /edikt:agents suggest          — recommend agents for this stack

   Next: Run /edikt:agents add {slug} to install more, or /edikt:agents suggest for recommendations.
   ```

   If no agents are installed yet:
   ```
   No agents installed yet.
   Run /edikt:init to install agents based on your stack, or:
     /edikt:agents add {slug}    — install a specific agent
     /edikt:agents suggest       — see recommendations for this stack
   ```

### `add {slug}` — Install an agent

1. Check if `~/.edikt/templates/agents/{slug}.md` exists.
2. If not found, list available agent slugs from `~/.edikt/templates/agents/`:
   ```
   Agent "{slug}" not found. Available agents:
     architect, engineer, backend, ...
   ```
3. If found:
   - Create `.claude/agents/` if it doesn't exist
   - Copy `~/.edikt/templates/agents/{slug}.md` to `.claude/agents/{slug}.md`
   - Output: `✅ Installed {slug} → .claude/agents/{slug}.md`
   - Show the agent's name and description
   - `  Next: The agent is now available for routing.`

### `remove {slug}` — Uninstall an agent

1. Check if `.claude/agents/{slug}.md` exists.
2. If not: output `Agent "{slug}" is not installed.`
3. If found: delete it and output: `Removed .claude/agents/{slug}.md`

### `show {slug}` — Show agent details

1. Check `.claude/agents/{slug}.md` first, then `~/.edikt/templates/agents/{slug}.md`.
2. Read and print the full agent content (frontmatter + system prompt).

### `suggest` — Recommend agents for this stack

1. Read `.edikt/config.yaml` for stack.
2. Read `~/.edikt/templates/agents/_registry.yaml`.
3. Match detected stack against registry to build recommended agent list.
4. Cross-reference with currently installed agents.
5. Output:
   ```
   Recommended agents for your stack ({stack}):

   Already installed:
     ✅ architect
     ✅ engineer
     ✅ backend

   Recommended (not installed):
     📦 dba    — Database design, query optimization, migration safety
     📦 qa         — Testing strategy, test writing, coverage analysis

   Run /edikt:agents add {slug} to install, or /edikt:init to install all recommended.
   ```
