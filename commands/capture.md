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

**Run every candidate through the GL-001 capture gates before proposing it.** Burden of proof is on CAPTURE, not silence; the reviewer default is DEMOTE — an ADR must argue past guideline, a guideline past nothing, and a healthy session produces zero ADRs. Auto-reject candidates that restate upstream docs/defaults, are derivable from code or tests, are living state, are task-scoped, or exist to justify a change to a reviewer. G0: skip anything already captured (ADR/INV titles, sidecar signals, spec provenance). A candidate with no REAL rejected alternative is a fact, not a decision — propose nothing, or at most a commit-message note. Present each surviving candidate WITH its gate verdict (G1/G2/G3) so the user rules on classification, not just capture.

### 2. Scan for Invariant Candidates

Look for hard constraints mentioned in the conversation. Look for signals:
- Hard language: "never", "always must", "non-negotiable", "hard rule", "under no circumstances"
- Violation-consequence framing: "if we ever do X, Y will break"
- Explicit exclusions: "we're never doing X", "this is off-limits"

Filter out preferences and guidelines — only flag things where violation causes real harm.

### 3. Scan for Documentation Gaps

Look for code-level decisions or explanations given in the conversation that should be in project documentation but likely aren't:
- "The reason we do it this way is..."
- Explanations of non-obvious behavior
- Design rationale buried in chat
- API contract decisions
- Schema or data model decisions without a corresponding spec

### 4. Report Findings

Output findings in this format:

```
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 CAPTURE SWEEP
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ADR Candidates ({n} found)

  1. {Decision topic}
     Decision: {what was decided}
     Alternatives considered: {what was rejected}
     Rationale: {why}
     → Suggestion only — /edikt:adr:new if you want it recorded (its intake gates may still refuse it)

  {repeat for each candidate}

Invariant Candidates ({n} found)

  1. {Constraint}
     Consequence of violation: {what breaks}
     → Run /edikt:invariant:new to capture this

  {repeat for each candidate}

Documentation Gaps ({n} found)

  1. {Description of the gap}
     Context: {where the decision/explanation appeared}
     → Run /edikt:docs:review for a full audit

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
```

If no candidates found in any category, output for that section:
```
  None found — no uncaptured decisions in this category.
```

### 5. Confirm

```
✅ Capture sweep complete

Next: nothing above is required — a healthy session produces zero ADRs. Use /edikt:adr:new or /edikt:invariant:new only for an item you choose to record.
```

---

REMEMBER: This command surfaces candidates — it does not capture them. Every finding is a suggestion the user may decline, not an automated write. Surface only product architecture choices and hard constraints — not every implementation detail that came up in the session. Never draft an ADR from this sweep, never draft one without asking the owner, and never treat a missing ADR as blocking: nothing here blocks a build, a review, or the end of the session.
