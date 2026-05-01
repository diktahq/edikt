---
name: edikt:capture
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
- A conclusion reached about a technology, pattern, or design

For each candidate, note:
- The decision made (what was chosen)
- The alternatives considered (what was rejected)
- The rationale (why this was chosen)

Distinguish between implementation details and architectural decisions. Implementation details (variable names, minor refactors) are not ADR candidates. Decisions about structure, patterns, tools, or constraints are.

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
     → Run /edikt:adr:new to capture this

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

Next: Run /edikt:adr:new or /edikt:invariant:new for any items above.
```

---

REMEMBER: This command surfaces candidates — it does not capture them. Every finding is a prompt for the user to act, not an automated write. Surface only genuine architectural decisions and hard constraints — not every implementation detail that came up in the session.
