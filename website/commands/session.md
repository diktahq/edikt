# /edikt:session

End-of-session sweep — summarize what was built and surface missed captures before context is lost.

## Usage

```bash
/edikt:session
```

## What it does

At the end of a work session, `edikt:session` reviews what changed (git diff, recent commits), checks the active plan's progress, and scans for architectural decisions, constraints, or doc gaps that should be captured — but haven't been yet.

Run it before ending a session or before `/compact`. The PreCompact hook reminds you automatically.

## What it checks

1. **Git summary** — what files changed, what was committed in the last few hours
2. **Plan progress** — which phases moved, what's still open
3. **Uncaptured decisions** — product architecture choices (a new boundary, a contract, a data store, a cross-context dependency, an external provider in the product's call path) that pass the GL-001 capture gates and are not reflected in any ADR. Implementation inside existing boundaries, tooling that does not ship, local dev setup and experiments are never surfaced, and every ADR candidate is a suggestion, never required
4. **Uncaptured constraints** — constraint language ("never", "always must") not reflected in any invariant
5. **Doc gaps** — new routes, env vars, or services not yet documented

Before surfacing any suggestion, it cross-references against existing files to avoid noise. Only genuinely missing captures are flagged.

## Output

```text
SESSION SUMMARY — 2026-03-08 17:42
─────────────────────────────────────────────────────
Built:    webhook delivery (3 files), DB migration
Commits:  feat(webhooks): delivery with retry logic
Updated:  PLAN-004 phase 2 → done

Possible captures:
  💡 ADR: Postgres over DynamoDB for the order store — rejected alternative, reversal cost and re-litigation risk all named
     → suggestion only — /edikt:adr:new if you want it recorded

  📄 Doc gap: POST /webhooks/retry — new endpoint not in API docs
     → Run /edikt:docs:review to review

─────────────────────────────────────────────────────
2 possible captures — none of them blocks anything; record only what you choose to.
```

If nothing was built:

```text
Nothing built in this session — no captures needed.
```

## PreCompact integration

The PreCompact hook (triggered automatically before the model compacts context) reminds you:

```text
⚠️ Context compacting. (1) Update your active plan's progress table NOW.
(2) Run /edikt:session to capture any decisions, constraints, or doc gaps before context is lost.
```

## Natural language triggers

- "end of session"
- "wrap up"
- "what did we miss?"
- "anything to capture?"
- "session summary"
