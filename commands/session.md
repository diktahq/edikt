---
name: session
description: "End-of-session sweep — summarize what happened, surface missed captures before context is lost"
effort: normal
context: fork
allowed-tools:
  - Read
  - Glob
  - Grep
  - Bash
---

You are performing an end-of-session sweep. Your job is to summarize what happened this session and surface any missed captures (ADRs, invariants, doc gaps) before context is lost.

## Step 0: Resolve Paths

Read `.edikt/config.yaml`. If not found, output:

```
No edikt config found. Run /edikt:init to set up this project.
```

And stop. Otherwise resolve paths from the `paths:` section:

- Plans: `paths.plans` (default: `docs/plans`)
- Decisions: `paths.decisions` (default: `docs/architecture/decisions`)

## Step 1: What changed (git)

Run these commands to understand what happened this session:

```bash
git diff HEAD --name-only 2>/dev/null
git diff --staged --name-only 2>/dev/null
git log --since="3 hours ago" --oneline 2>/dev/null
```

Group the changed files into logical areas (e.g., "webhook delivery (3 files)", "DB migration", "auth routes"). If there are no changes and no recent commits, output:

```
Nothing built in this session — no captures needed.
```

And stop. (If the user asked about one specific thing, answer for that thing first with the gates in Step 3, then stop.)

## Step 2: Plan progress

Find the most recently modified plan file under `{paths.plans}/`:

```bash
ls -t {paths.plans}/*.md 2>/dev/null | head -1
```

Read it and check if any phases have moved to `done` or `in-progress` recently (look at the progress table and updated timestamps). Report what changed, or "no plans updated" if nothing moved.

## Step 3: Scan for uncaptured decisions

Review the current conversation context for decision language:
- "we decided to...", "going with...", "chose X over Y", "trade-off is...", "because of..."

Then check existing ADRs:

```bash
ls {paths.decisions}/*.md 2>/dev/null || ls docs/architecture/decisions/*.md 2>/dev/null
```

Read the ADR list, then read `.claude/skills/edikt-governance-authoring/SKILL.md` (if it is absent, say `governance-authoring skill not installed — run /edikt:upgrade` and give no verdict). **Run every candidate, of every kind, through the gates in that skill**, the same ones `/edikt:capture` uses, and give each one a verdict (see Output format). If the user asked about one specific thing ("record this", "should this be an ADR"), answer for that thing first and say so, before the sweep. Burden of proof is on CAPTURE, not silence; the default is DEMOTE, and a healthy session produces zero ADRs. Auto-reject candidates that restate upstream docs or defaults, are derivable from code or tests, are living state, are task-scoped, or exist to justify a change to a reviewer. G0: skip anything already covered by an existing ADR or invariant. Then G2 needs all three: a REAL rejected alternative, a choice that is expensive to reverse or constrains future structure, dependencies, interfaces, data, or security, and a re-litigation risk without the record. A candidate with no real rejected alternative is a fact, not a decision — its verdict is "nothing to record", naming the ADR gate.

Surface an ADR candidate only for a product architecture choice: a new boundary, a contract others depend on (an API, an event, a shared schema), a data store, a cross-context dependency, or an external provider in the product's call path.

Never surface any of these as an ADR candidate:
- implementation inside existing boundaries (algorithms, data structures, refactors, a library behind an existing port, style choices)
- tooling used to build the product that does not ship in it (linters, test runners, local tracing or observability stacks, editor and agent config)
- local dev setup (docker-compose for development, bootstrap scripts)
- experiments and measurements (spikes, benchmarks, prototypes)

Every ADR candidate is a suggestion. Never draft one from this sweep, and never treat a missing ADR as blocking.

## Step 4: Scan for uncaptured constraints

Collect candidates from the conversation's constraint language:
- "never...", "always must...", "required on all...", "violating this would..."

Then check existing invariants:

```bash
ls docs/invariants/*.md 2>/dev/null || ls docs/architecture/invariants/*.md 2>/dev/null
```

Do not drop a constraint here: it is a candidate. Judge it with the gates in the `edikt-governance-authoring` skill (the same ones as Step 3). A constraint already captured in an existing invariant file is judged as already captured.

## Step 5: Scan for doc gaps

From the git diff, look for:
- New HTTP routes or endpoints added
- New environment variables introduced
- New services or infrastructure components
- Breaking changes to existing APIs

Same logic as `/edikt:docs:review`, but scoped to this session's changes only. Skip internal refactors, test changes, and formatting.

## Step 6: Cross-reference to avoid noise

Before surfacing any suggestion:
- Verify it's not already in `docs/decisions/` (or `docs/architecture/decisions/`)
- Verify it's not already in `docs/invariants/` (or `docs/architecture/invariants/`)
- Verify it's not already documented

Only surface genuinely missing captures as possible captures; a candidate that was judged and rejected still gets its one-line "nothing to record" verdict.

## Output format

```
SESSION SUMMARY — {date} {time}
─────────────────────────────────────────────────────
Built:    {changed areas — e.g., "webhook delivery (3 files), DB migration"}
Commits:  {recent commit messages, or "none yet"}
Updated:  {plan progress changes, or "no plans updated"}

You asked about: {the thing} (answered first; only when the user asked about one thing)
  → {a verdict line, in one of the two forms below}

Possible captures:
  💡 ADR: {decision description} — {the ADR gate's verdict: the rejected alternative, the reversal cost, the re-litigation risk}
     → suggestion only — /edikt:adr:new if you want it recorded

  💡 Invariant: {constraint description} — {the invariant gate's verdict: why no single home in code, how it is falsified, what guards it}
     → /edikt:invariant:new if you want it recorded

  💡 Guideline: {default and its named exception}
     → /edikt:guideline:new if you want it recorded

  ➖ Nothing to record: {candidate} — fails {gate}: {reason in one line}

  📄 Doc gap: {new public surface} — not in {doc file}
     → Run /edikt:docs:review to review

─────────────────────────────────────────────────────
{count} possible captures, {m} judged not worth recording — none of them blocks anything; record only what you choose to.
```

If there are no candidates at all, output instead:
```
✅ Session looks complete — nothing missed.

  Next: Start a fresh session for your next task.
```
