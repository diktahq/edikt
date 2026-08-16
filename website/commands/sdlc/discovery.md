# /edikt:sdlc:discovery

Capture a structured discovery document for a feature you don't know enough about yet.

Use this when an idea has unknowns that matter enough to resolve before writing a PRD. It's a peer to `/edikt:brainstorm`, not a wrapper — discovery has its own sections (Known, Unknown, Kill Criteria, Discovery Plan) and evolves independently.

## When to use which

| Tool | Use when |
|------|----------|
| `/edikt:brainstorm` | Open-ended exploration. You're not yet sure what you're solving. |
| `/edikt:sdlc:discovery` | You have a candidate feature, but the unknown-to-known ratio is high. Reduce uncertainty before scoping. |
| `/edikt:sdlc:prd` | You already know the feature. Now define what done means. |

A brainstorm can graduate into a discovery. A discovery can graduate into a PRD.

## Usage

```bash
/edikt:sdlc:discovery realtime collaboration for the editor
/edikt:sdlc:discovery BRAIN-007                   # lift an existing brainstorm
/edikt:sdlc:discovery                             # asks what to explore
```

## What the command asks

Four questions, one at a time. Each is intentionally hard to skip — the answers are the document.

1. **What do you know for certain?** Facts, data, prior research, confirmed user statements. Be precise: "3 of 12 users said X" beats "users seem to want X."
2. **What are you most uncertain about?** Three to five unknowns, ordered by impact on the decision — which one, if resolved, most changes whether you build this. Not alphabetical, not easiest-first.
3. **What would change your mind?** Kill criteria. "If fewer than N% of users X, we stop." Concrete and measurable where possible.
4. **What's the smallest experiment that reduces the biggest uncertainty?** The discovery plan. Aim for the cheapest signal that moves the biggest unknown.

## Output

Writes `DISCOVERY-NNN-<slug>.md` to `docs/product/discovery/`:

```markdown
---
type: discovery
id: DISCOVERY-001
title: "Realtime collaboration for the editor"
status: active
graduates_to: null
---

## Context
<1-2 paragraphs — what this is about, why it matters now>

## Known
- 12 of 30 customer interviews mentioned losing edits during multi-user sessions
- Existing CRDT library benchmarked at 50ms p99 with 10 concurrent users

## Unknown
1. **Whether last-write-wins is acceptable for our 95th percentile workflow** — drives the entire engineering approach
2. **How much network jitter our enterprise customers tolerate** — sets the offline-first decision
3. **Whether undo across sessions is table-stakes** — large UX surface area

## Kill Criteria
- If fewer than 30% of pilot users edit collaboratively in week 1, we stop
- If CRDT memory cost exceeds 100KB per active doc, we pick a different approach

## Discovery Plan
| # | Experiment | Method | Success signal | Timeline |
|---|------------|--------|----------------|----------|
| 1 | Last-write-wins prototype | 2-day spike with 5 internal users | <2 reported conflicts/day | Week 1 |
| 2 | Memory benchmark | Yjs synthetic load | <100KB at 1k ops | Week 1 |

## Assumptions Register
| Assumption | Confidence | Source |
|-----------|-----------|--------|
| Users want simultaneous editing | medium | 12/30 interviews |

## Outcome
_Filled when discovery concludes._
```

The document has its own status (`active`, `concluded`, `killed`) and its own revision flow. You update Known/Unknown as findings come in — the document is alive while the question is open.

## Graduating to a PRD

When uncertainty is reduced enough to commit, run:

```bash
/edikt:sdlc:prd DISCOVERY-001
```

This pre-populates the PRD's Problem from your Known section and the Open Questions from anything still in Unknown. The discovery's frontmatter gets `graduates_to: PRD-NNN` so the trail is bidirectional.

## Why discovery exists separately from brainstorm

Brainstorms are open-ended. Specialist agents join as topics emerge. The artifact looks like a working memo.

Discoveries are structured uncertainty reduction. The four sections are not optional — they're what makes the document useful three months later when someone asks "why did we decide X?"

The duplication is deliberate. Each can evolve independently of the other.

## What's next

- [/edikt:brainstorm](/commands/brainstorm) — open-ended exploration
- [/edikt:sdlc:prd](/commands/sdlc/prd) — graduate this discovery into a PRD
- [PRD v2 Deep Dive](/guides/prd-v2) — what comes after discovery concludes
