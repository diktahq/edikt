# /edikt:capture

Capture the current conversation into the right governance artifact — ADR, invariant, guideline, or PRD — without having to decide upfront.

## Usage

```bash
/edikt:capture
```

## What it does

Reads the current conversation and classifies what happened:

| Signal | Routes to |
|--------|-----------|
| Significant technical choice with trade-offs | [`/edikt:adr:new`](/commands/adr/new) |
| Hard constraint where violation causes harm | [`/edikt:invariant:new`](/commands/invariant/new) |
| Team standard or coding convention | [`/edikt:guideline:new`](/commands/guideline/new) |
| Clearly-defined feature requirement | [`/edikt:sdlc:prd`](/commands/sdlc/prd) |

After classification, edikt shows what it found and confirms before creating anything:

```text
This conversation contains:
  → A significant technical decision (use Redis for session storage)
  → A hard constraint (sessions must expire within 24h)

Create:
  1. ADR: use Redis for session storage
  2. Invariant: sessions must expire within 24h

Proceed? (y/n/edit)
```

You can confirm, edit the classification, or cancel individual items.

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
