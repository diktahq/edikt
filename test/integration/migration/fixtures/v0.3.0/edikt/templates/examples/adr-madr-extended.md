---
status: accepted
date: {YYYY-MM-DD}
decision-makers: [{list of people who made the call}]
consulted: [{list of people consulted but not decision-makers}]
informed: [{list of people informed of the decision}]
supersedes: null
---

# ADR-NNN: {Short declarative title}

<!--
MADR-extended template (Markdown Any Decision Records — https://adr.github.io/madr/).

MADR extends Nygard's minimal format with structured sections for
Decision Drivers, Considered Options, and Pros/Cons analysis. It's
useful when decisions involve multiple stakeholders, significant
tradeoffs, or alternatives that need to be explicitly documented and
rejected with reasons.

edikt ships this as a reference template alongside adr-nygard-minimal.md.
Your project picks one (or writes its own) during /edikt:init.

Choose MADR-extended when:
- Decisions involve multiple stakeholders with competing interests
- The alternatives considered matter for future readers
- You want explicit Pros and Cons documented for each option
- Your team has time for a 30-minute ADR write-up on significant
  decisions

If you prefer a lighter format focused on brevity, see
adr-nygard-minimal.md.
-->

## Context and Problem Statement

{Describe the situation driving the decision. What forces are at play?
What constraints apply? What question are we trying to answer? Frame
the problem clearly enough that the alternatives make sense.}

## Decision Drivers

- {Driver 1 — e.g., "Must scale to 100k concurrent users"}
- {Driver 2 — e.g., "Must be operable by a team of 3 engineers"}
- {Driver 3 — e.g., "Compliance with SOC 2 required"}

## Considered Options

1. **{Option 1}** — {one-line summary}
2. **{Option 2}** — {one-line summary}
3. **{Option 3}** — {one-line summary}

## Decision Outcome

Chosen option: **{Option N}**, because {primary reason tied to decision drivers}.

### Consequences

**Positive:**
- {Positive consequence 1}
- {Positive consequence 2}

**Negative:**
- {Negative consequence 1 — what did we give up?}
- {Negative consequence 2 — what risks do we take on?}

**Neutral:**
- {Neutral consequence — changes to process or tooling}

## Pros and Cons of the Options

### {Option 1}

- ✅ {Pro 1}
- ✅ {Pro 2}
- ❌ {Con 1}
- ❌ {Con 2}

### {Option 2}

- ✅ {Pro 1}
- ❌ {Con 1 — the reason we rejected this}

### {Option 3}

- ✅ {Pro 1}
- ❌ {Con 1 — the reason we rejected this}

## Confirmation

{How will we know this decision is working? What metrics or observations
will confirm or refute the assumptions behind it? When will we re-evaluate?}

## More Information

{Links to related ADRs, design docs, benchmarks, or external references
that informed the decision.}

<!-- Directives for edikt governance. Populated by /edikt:adr:compile. -->
[edikt:directives:start]: #
[edikt:directives:end]: #
