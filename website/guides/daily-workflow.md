# Daily Workflow

The problem edikt solves is session-to-session drift — Claude forgetting your standards, losing your decisions, starting over every time. The daily workflow is how you prevent that.

```
Start session → Load context → Build → Review → Capture → End session
```

Most of this is automatic. The explicit steps are one command each.

---

## 1. Starting a session

**The problem without edikt:** You open Claude Code, start describing what you want to build, and realize five messages in that Claude has no idea about your architecture or patterns.

**What happens automatically with edikt:**

The session refresh fires automatically and tells you what changed since last time:

```
📋 edikt — since your last session (2d ago):
   3 migration/schema files changed
   Relevant agents: dba
   Run /edikt:context to load full project context.
```

If nothing significant changed:
```
📋 edikt — 2d since last session. Run /edikt:context to load context.
```

**Then run:**
```
/edikt:context
```

This loads your project context, active plan, decisions, invariants, and installed rules into Claude's session. After this, Claude knows your project — not just your files.

---

## 2. Planning work

**The problem without edikt:** You describe a feature, Claude starts coding, and halfway through you discover the migration is missing a rollback, there's no index on the query column, and the API contract breaks a mobile client.

**With edikt:**
```
/edikt:plan add webhook delivery with retry logic
```

edikt interviews you (3–6 focused questions), scans the codebase for relevant patterns, breaks work into phases, then runs a pre-flight specialist review before execution:

```
PRE-FLIGHT REVIEW
─────────────────────────────────────────────────────
Domains detected: database, api

DBA
  🔴  Migration has no rollback — add DOWN migration
  🟡  No index on webhooks.status — queried in retry loop

API
  🟢  Endpoint contract looks stable
─────────────────────────────────────────────────────
1 critical, 1 warning. Address before executing?
```

Fix the migration gap now. It takes 5 minutes. After implementation it takes an hour.

Skip pre-flight for simple tasks:
```
/edikt:plan --no-review fix typo in error message
```

---

## 3. Building

Just build. edikt works in the background:

**Auto-format** runs after every edit — gofmt, prettier, black, rubocop. No manual formatter runs.

**Signal detection** watches every response for decisions worth capturing:

```
💡 ADR candidate — run /edikt:adr to capture it.
📄 Doc gap: new /webhooks/retry endpoint — run /edikt:docs to review.
🔒 Security-sensitive domain — run /edikt:audit before shipping.
```

When you see one of these, run the command. That decision becomes a permanent record, not a conversation that gets lost.

**Update your plan progress table** as phases complete — it's the state that survives context compaction:

```
| Phase | Status | Updated    |
|-------|--------|------------|
| 1     | done   | 2026-03-08 |
| 2     | in-progress | 2026-03-08 |
```

---

## 4. Reviewing what you built

**The problem without edikt:** You push code that has a missing index, a security gap you missed at 6pm, or an API response that breaks an existing client. You find out in code review or production.

**With edikt:**
```
/edikt:review             ← review last commit
/edikt:review --branch    ← review everything on this branch
```

edikt classifies changed files by domain and routes to the right specialists automatically:

```
IMPLEMENTATION REVIEW
─────────────────────────────────────────────────────
Scope: 5 files changed
Domains: database, api

DBA
  🔴  Missing index on webhooks.delivered_at
  🟢  Transaction boundaries look correct

API
  🟢  No breaking changes detected
─────────────────────────────────────────────────────
1 critical. Address before shipping?
```

For security-sensitive features (auth, payments, PII), also run:
```
/edikt:audit
```

---

## 5. Ending a session

**The problem without edikt:** You make three architectural decisions during a session. Context compacts. Next session you can't remember why you chose exponential backoff, what the new endpoint is called, or whether you captured the constraint about idempotency keys.

**What happens automatically:**

The PreCompact hook fires before context compression:
```
⚠️ Context compacting. Update your active plan's progress table NOW.
   Run /edikt:session to capture decisions before context is lost.
```

**Run explicitly at end of day:**
```
/edikt:session
```

```
SESSION SUMMARY — 2026-03-08
─────────────────────────────────────────────────────
Built:    webhook delivery (5 files), DB migration
Commits:  feat(webhooks): delivery with retry logic
Updated:  PLAN-007 phase 2 → done

Possible captures:
  💡 ADR: exponential backoff over fixed intervals
     → Run /edikt:adr to capture

  📄 Doc gap: POST /webhooks/retry — new endpoint
     → Run /edikt:docs to review
─────────────────────────────────────────────────────
```

Capture what matters. What you save in `docs/decisions/` is available to Claude in every future session.

---

## The full loop at a glance

| Step | Automatic | Explicit |
|------|-----------|---------|
| Session start | SessionStart hook surfaces git changes | `/edikt:context` |
| Planning | — | `/edikt:plan` |
| Pre-flight | Runs at end of `/edikt:plan` | `--no-review` to skip |
| Building | PostToolUse formats, Stop hook flags signals | `/edikt:adr`, `/edikt:invariant` |
| Review | — | `/edikt:review` |
| Security | Pre-push hook scans on push | `/edikt:audit` |
| End of session | PreCompact reminds you | `/edikt:session` |
