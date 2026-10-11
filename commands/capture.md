---
name: capture
description: "Mid-session sweep — surface uncaptured ADRs, invariants, and doc gaps before moving on"
effort: normal
allowed-tools:
  - Read
  - Glob
  - Grep
---

# edikt:capture

Scan the current session context for uncaptured decisions, constraints, and documentation gaps before they're lost to compaction.

## Instructions

### 0. Config Guard

If `.edikt/config.yaml` does not exist, output:
```
No edikt config found. Run /edikt:init to set up this project.
```
And stop.

### 0b. Load the gates, and find what was asked

Read `.claude/skills/edikt-governance-authoring/SKILL.md`. It holds the gates that decide whether something is an invariant, an ADR, a guideline or nothing. This command does not restate them; every judgement below is made by reading that file. If it is not installed, say `governance-authoring skill not installed — run /edikt:upgrade` and give no verdict, because a verdict without the gates is the model's own judgement.

If the user asked about one specific thing ("record this", "should this be an ADR", "capture that"), that thing is the first candidate and is answered first, under its own heading in step 4, before anything else in the session is swept. Say so in the report; never fold it silently into the sweep. `$ARGUMENTS` may name it.

### 1. Scan for ADR Candidates

Review the conversation history for architectural decisions that were made but not captured. Look for signals:
- Explicit choices: "we decided", "going with", "we'll use X", "let's use", "the approach is"
- Trade-off reasoning: comparing two or more options and picking one
- A conclusion reached about a boundary, a contract, a data store, a cross-context dependency, or an external provider in the product's call path

For each candidate, note:
- The decision made (what was chosen)
- The alternatives considered (what was rejected)
- The rationale (why this was chosen)

An ADR candidate is a product architecture choice that constrains future structure, dependencies, interfaces, data, or security: a new boundary, a contract others depend on (an API, an event, a shared schema), a data store, a cross-context dependency, or an external provider in the product's call path.

Never surface any of these as an ADR candidate:
- implementation inside existing boundaries (algorithms, data structures, refactors, a library behind an existing port, variable names)
- tooling used to build the product that does not ship in it (linters, test runners, local tracing or observability stacks, editor and agent config)
- local dev setup (docker-compose for development, bootstrap scripts)
- experiments and measurements (spikes, benchmarks, prototypes)
- process scoped to one task or one release

When the product itself is a tool, its shipped tooling is product architecture; the exclusion covers only tooling that does not ship.

Steps 1 to 3 only collect candidates. Judging them is step 3b, and it applies to every candidate of every kind.

### 2. Scan for Invariant Candidates

Look for hard constraints mentioned in the conversation. Look for signals:
- Hard language: "never", "always must", "non-negotiable", "hard rule", "under no circumstances"
- Violation-consequence framing: "if we ever do X, Y will break"
- Explicit exclusions: "we're never doing X", "this is off-limits"

Collect these as candidates without filtering them here; step 3b decides which of them is an invariant, and a preference or a rule with a named exception is judged there, not dropped silently.

### 3. Scan for Documentation Gaps

Look for code-level decisions or explanations given in the conversation that should be in project documentation but likely aren't:
- "The reason we do it this way is..."
- Explanations of non-obvious behavior
- Design rationale buried in chat
- API contract decisions
- Schema or data model decisions without a corresponding spec

### 3b. Judge every candidate

Every candidate from steps 1 and 2, and the thing the user asked about, goes through the gates in the `edikt-governance-authoring` skill, in the order that skill gives, starting with whether it is already captured. The burden of proof is on recording: an ADR must argue past a guideline, a guideline past nothing, and a healthy session produces zero records. Each candidate ends with exactly one verdict:

- the kind it qualifies as (invariant, ADR or guideline), with the command that records it, offered as a suggestion only; or
- nothing to record, with the gate that fails (the skill's own label: already captured, the invariant gate, the ADR gate, the guideline gate, or the auto-reject class it falls in) and the reason, in one line.

A candidate with no real rejected alternative is a fact, not a decision: its verdict is nothing to record, naming the ADR gate.

### 4. Report Findings

Output findings in this format:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 CAPTURE SWEEP
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

You asked about: {the thing} (answered first)
  → {one verdict line, in the form below}

Verdicts ({n} candidates, one line each)

  1. {Decision or constraint, in a few words}
     → {Invariant | ADR | Guideline}: /edikt:invariant:new | /edikt:adr:new | /edikt:guideline:new if you want it recorded (its intake gates may still refuse it)
     Why: {the rejected alternative and the reversal cost for an ADR; the consequence of violation for an invariant; the named exception for a guideline}

  2. {Decision or constraint, in a few words}
     → Nothing to record — fails {gate}: {reason in one line}

  {repeat for each candidate}

Documentation Gaps ({n} found)

  1. {Description of the gap}
     Context: {where the decision/explanation appeared}
     → Run /edikt:docs:review for a full audit

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

Omit the "You asked about" block when the user did not ask about one thing. A candidate is never dropped without a line: a rule or decision that was stated and judged not worth recording appears as a "Nothing to record" verdict naming the gate that fails.

If no candidates were found at all, output for the verdicts section:
```
  None found — no decisions or constraints to judge in this session.
```

### 5. Confirm

```
✅ Capture sweep complete

Next: nothing above is required — a healthy session produces zero ADRs. Use /edikt:adr:new, /edikt:invariant:new or /edikt:guideline:new only for an item you choose to record.
```

---

REMEMBER: This command surfaces candidates — it does not capture them. Every finding is a suggestion the user may decline, not an automated write. Surface only product architecture choices and hard constraints — not every implementation detail that came up in the session. Never draft an ADR from this sweep, never draft one without asking the owner, and never treat a missing ADR as blocking: nothing here blocks a build, a review, or the end of the session.
