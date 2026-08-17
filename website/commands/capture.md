# /edikt:capture

Mid-session sweep — surfaces uncaptured ADRs, invariants, and documentation gaps in the current conversation before they're lost to compaction. **Read-only**: it surfaces candidates, it doesn't create anything.

## Usage

```bash
/edikt:capture
```

## What it does

Scans the conversation for three signal types:

| Signal | Surfaced as | Routes to |
|--------|-------------|-----------|
| Explicit technical choice with rejected alternatives | ADR candidate | [`/edikt:adr:new`](/commands/adr/new) |
| Hard constraint where violation causes real harm | Invariant candidate | [`/edikt:invariant:new`](/commands/invariant/new) |
| Design rationale or decisions missing from project docs | Documentation gap | [`/edikt:docs:review`](/commands/docs/review) |

Every candidate runs through the GL-001 capture gates before being surfaced — burden of proof is on capture, not silence, and a candidate with no real rejected alternative isn't proposed at all. Then it reports what it found:

```text
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
 CAPTURE SWEEP
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

ADR Candidates (1 found)

  1. Use Redis for session storage
     Decision: Redis over in-memory sessions
     Alternatives considered: in-memory store (doesn't survive restarts)
     Rationale: sessions must persist across deploys
     → Run /edikt:adr:new to capture this

Invariant Candidates (1 found)

  1. Sessions must expire within 24h
     Consequence of violation: stale sessions accumulate, memory grows unbounded
     → Run /edikt:invariant:new to capture this

Documentation Gaps (0 found)

  None found — no uncaptured decisions in this category.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
✅ Capture sweep complete

Next: Run /edikt:adr:new or /edikt:invariant:new for any items above.
```

Nothing gets created automatically — every finding is a prompt to run the command it points at, not an automated write.

## When to use

Run at the end of a conversation where multiple governance-worthy things happened. Instead of remembering to run four different commands, run one.

The `Stop` hook proactively suggests specific capture commands during conversations. `/edikt:capture` is the catch-all when you want to sweep the whole conversation at once.

## Natural language triggers

- "capture everything from this conversation"
- "let's save what we decided"
- "wrap up this conversation"

## What's next

- [/edikt:session](/commands/session) — end-of-session sweep across recent work
- [/edikt:adr:new](/commands/adr/new) — capture a specific architecture decision
- [/edikt:invariant:new](/commands/invariant/new) — capture a specific hard constraint
