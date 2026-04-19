---
name: edikt:sdlc:discovery
description: "Structured discovery doc — define what you know, what you don't, and how to find out"
effort: medium
argument-hint: "<description or BRAIN-NNN to lift>"
allowed-tools:
  - Read
  - Write
  - Glob
  - Grep
  - Bash
---
!`bash -c 'CFG=""; D="$PWD"; while [ "$D" != "/" ]; do [ -f "$D/.edikt/config.yaml" ] && CFG="$D/.edikt/config.yaml" && break; D=$(dirname "$D"); done; [ -z "$CFG" ] && { printf "<!-- edikt:live -->\nNext DISCOVERY number: DISCOVERY-001\nExisting discoveries: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; PROOT=$(dirname "$(dirname "$CFG")"); BASE=$(grep "^base:" "$CFG" 2>/dev/null | awk "{print \$2}" | tr -d "\""); BASE="${BASE:-docs}"; DIR="$PROOT/$BASE/product/discovery"; [ ! -d "$DIR" ] && { printf "<!-- edikt:live -->\nNext DISCOVERY number: DISCOVERY-001\nExisting discoveries: (none yet)\n<!-- /edikt:live -->\n"; exit 0; }; COUNT=$(find "$DIR" -maxdepth 1 -type f -name "DISCOVERY-*.md" 2>/dev/null | wc -l | tr -d " "); NEXT=$(printf "%03d" $((COUNT + 1))); EXISTING=$(find "$DIR" -maxdepth 1 -type f -name "DISCOVERY-*.md" 2>/dev/null | sort | xargs -I{} basename {} .md | tr "\n" "," | sed "s/,$//"); printf "<!-- edikt:live -->\nNext DISCOVERY number: DISCOVERY-%s\nExisting discoveries: %s\n<!-- /edikt:live -->\n" "$NEXT" "${EXISTING:-(none yet)}"'`

# edikt:sdlc:discovery

Structured discovery document for features with high uncertainty. Peer command to `/edikt:brainstorm`, not a wrapper — discovery evolves independently with its own sections: Evidence, Discovery Plan, Kill Criteria, Assumptions Register.

Use discovery when a feature idea has unknowns that matter enough to resolve before writing a PRD. Use `/edikt:brainstorm` for open-ended exploration. Use `/edikt:sdlc:prd` when you already know the feature and are defining it.

CRITICAL: This command requires interactive input. If you are in plan mode (you can only describe actions, not perform them), output this and stop:
```
⚠️  This command requires user interaction and cannot run in plan mode.
Exit plan mode first, then run the command again.
```

## Arguments

- `<description>` — short description of the feature or area to explore
- `BRAIN-NNN` — lift an existing brainstorm doc into a structured discovery
- Empty — ask what to explore

## Instructions

### Step 0: Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### Step 1: Resolve Arguments

Read `.edikt/config.yaml`. Resolve paths:
- `base` defaults to `docs`
- Discovery directory: `{base}/product/discovery/`
- Brainstorms directory: `{paths.brainstorms}` (default: `docs/brainstorms/`)

Inspect `$ARGUMENTS`:

- If `$ARGUMENTS` matches `BRAIN-\d+`:
  - Read the brainstorm file from the brainstorms directory (or `docs/internal/brainstorms/` fallback)
  - Seed content: use the brainstorm's Problem, Exploration, and Decisions sections as starting material for the discovery sections below
- If `$ARGUMENTS` is empty:
  - Ask: "What are you exploring? Short description or a BRAIN-NNN identifier."
- Otherwise:
  - Use `$ARGUMENTS` verbatim as the description seed

### Step 2: Discovery Interview

Ask these four questions, one at a time, waiting for the answer before asking the next. Do NOT ask them all as a single batch — discovery quality depends on the user thinking about each one in isolation.

1. **What do you know for certain?**
   Facts, data, prior research, confirmed user statements. Be precise: "3 of 12 users in last week's interviews said X" is better than "users seem to want X."

2. **What are you most uncertain about?**
   List 3-5 unknowns. Order them by **impact on the decision** — which unknown, if resolved, most changes whether you build this. Not alphabetical, not easiest-first.

3. **What would change your mind?**
   Kill criteria. "If fewer than N% of users X, we stop" or "If Y technical constraint holds, we pick a different approach." Concrete, measurable where possible.

4. **What's the smallest experiment that reduces the biggest uncertainty?**
   The discovery plan. Aim for the cheapest signal that moves the biggest unknown from Q2. User interviews, prototype, data query, spike, prior-art review.

### Step 3: Generate Discovery Document

Write `DISCOVERY-NNN-<slug>.md` to `{base}/product/discovery/`. Create the directory if it doesn't exist. The slug is a kebab-case summary of the topic (3-5 words).

Document structure:

```markdown
---
type: discovery
id: DISCOVERY-NNN
title: "<title>"
status: active
author: <git user name>
created_at: <ISO8601>
source_brainstorm: <BRAIN-NNN or null>
graduates_to: null   # PRD-NNN when discovery concludes
---

# DISCOVERY-NNN: <title>

**Status:** Active
**Author:** <author>
**Created:** <date>
**Source:** <BRAIN-NNN if lifted, or "fresh">

## Context

<1-2 paragraph description — what this is about, why it matters now>

## Known

<From Q1 — facts, data, prior research. Bullet list with evidence.>

## Unknown

<From Q2 — ordered by impact on decision. Each unknown has a short note on why it matters.>

1. **<unknown>** — <why this matters most>
2. **<unknown>** — <why second>
...

## Kill Criteria

<From Q3 — what would make us stop or change course. Concrete and measurable.>

- If <condition>, we <action>
- If <condition>, we <action>

## Discovery Plan

| # | Experiment | Method | Success signal | Timeline |
|---|-----------|--------|----------------|----------|
| 1 | <from Q4> | <interview/prototype/query/etc> | <what data proves/disproves> | <week N> |

## Assumptions Register

| Assumption | Confidence | Source |
|-----------|-----------|--------|
| <assumption> | low \| medium \| high | <why we believe this> |

## Outcome

_Filled when discovery concludes. Options: graduate to PRD, kill, or iterate._

---

*This is a discovery document. When findings converge, graduate with `/edikt:sdlc:prd DISCOVERY-NNN` to generate a PRD pre-populated from the Known + Unknown sections.*
```

### Step 4: Update Revision Summary

Output to the user:

```
✅ DISCOVERY-NNN created

  {path}/DISCOVERY-NNN-<slug>.md

  Known:        {count} facts
  Unknown:      {count} open questions (ranked)
  Kill criteria: {count}
  Experiments:  {count}

Next steps:
  • Run the experiments from the Discovery Plan
  • Update the Known/Unknown sections as findings come in
  • When ready to build: /edikt:sdlc:prd DISCOVERY-NNN
```

## Design Notes

- **Discovery vs brainstorm.** `/edikt:brainstorm` is open-ended thinking with specialist agents joining as topics emerge. `/edikt:sdlc:discovery` is structured uncertainty reduction with defined sections. A brainstorm can graduate into a discovery; a discovery can graduate into a PRD.
- **Discovery vs PRD.** A PRD describes WHAT to build. A discovery describes what we need to learn to decide what to build. If the unknown-to-known ratio is high, do discovery first.
- **Duplicated, not wrapped.** This command duplicates structure with `/edikt:brainstorm` deliberately so each can evolve independently. When both converge to the same sections, consolidation is a future decision — not a v0.6.0 concern.

## Related Commands

- `/edikt:brainstorm` — open-ended exploration
- `/edikt:sdlc:prd` — PRD authoring (accepts a DISCOVERY-NNN as seed argument)
- `/edikt:sdlc:spec` — technical spec from accepted PRD

Next: When experiments are complete and unknowns are resolved, run /edikt:sdlc:prd DISCOVERY-NNN to author the PRD.
