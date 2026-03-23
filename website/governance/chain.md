# Governance Chain

The governance chain connects edikt's two systems — architecture governance & compliance and Agentic SDLC governance — into a single traceable path from intent to implementation to verification.

<div class="how-it-works-diagram">
<svg width="100%" viewBox="0 0 760 820" xmlns="http://www.w3.org/2000/svg">
<defs>
<marker id="arr-slate" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-teal" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-stone" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="#A0936D" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-constrain" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-surfaces" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
</defs>

<!-- Column headers -->
<rect x="40" y="16" width="300" height="36" rx="4" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.75"/>
<text x="190" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" letter-spacing=".02em" fill="var(--vp-c-text-1)">Architecture governance &amp; compliance</text>

<rect x="420" y="16" width="300" height="36" rx="4" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" letter-spacing=".02em" fill="var(--vp-c-brand-1)">Agentic SDLC governance</text>

<!-- LEFT: Decisions -->
<rect x="55" y="80" width="270" height="116" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="104" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">Decisions</text>
<text x="190" y="128" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">ADRs — architecture choices</text>
<text x="190" y="150" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Invariants — hard constraints</text>
<text x="190" y="172" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Guidelines — team conventions</text>

<line x1="190" y1="196" x2="190" y2="228" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Compile -->
<rect x="105" y="228" width="170" height="44" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="250" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">/edikt:compile</text>

<line x1="190" y1="272" x2="190" y2="304" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Enforcement surface -->
<rect x="45" y="304" width="290" height="80" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="328" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">Enforcement surface</text>
<text x="190" y="352" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Compiled directives + rule packs</text>
<text x="190" y="372" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">.claude/rules/ — auto-loaded every session</text>

<line x1="190" y1="384" x2="190" y2="416" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Hooks -->
<rect x="65" y="416" width="250" height="58" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="438" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">9 lifecycle hooks</text>
<text x="190" y="460" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Re-inject after compaction</text>

<line x1="190" y1="474" x2="190" y2="506" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Signal detection -->
<rect x="55" y="506" width="270" height="58" rx="6" fill="var(--diagram-left-fill)" stroke="#A0936D" stroke-width="0.75"/>
<text x="190" y="528" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="#A0936D">Signal detection</text>
<text x="190" y="550" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Detects new decisions mid-session</text>

<!-- Feedback: signal → decisions -->
<path d="M55 535 L28 535 L28 140 L55 140" fill="none" stroke="#A0936D" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-stone)"/>
<text x="18" y="335" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" font-weight="500" fill="#A0936D" transform="rotate(-90 18 335)">new ADR / invariant</text>

<!-- RIGHT: PRD -->
<rect x="435" y="80" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="102" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Requirements (PRD)</text>
<text x="570" y="124" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ pm agent reviews</text>

<line x1="570" y1="138" x2="570" y2="166" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Spec -->
<rect x="435" y="166" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="188" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Technical spec</text>
<text x="570" y="210" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ architect · dba · api agents</text>

<line x1="570" y1="224" x2="570" y2="252" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Artifacts -->
<rect x="435" y="252" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="274" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Spec artifacts</text>
<text x="570" y="296" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ dba · api · qa agents</text>

<line x1="570" y1="310" x2="570" y2="338" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Plan -->
<rect x="435" y="338" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="360" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Plan + pre-flight</text>
<text x="570" y="382" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ dba · security · sre agents</text>

<line x1="570" y1="396" x2="570" y2="424" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Execute -->
<rect x="435" y="424" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="446" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Execute</text>
<text x="570" y="468" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">governed session · quality gates</text>

<line x1="570" y1="482" x2="570" y2="510" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Drift -->
<rect x="435" y="510" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="532" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Drift detection</text>
<text x="570" y="554" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ architect · engineer · qa agents</text>

<!-- CROSS: constrains (left → right) -->
<path d="M335 345 C380 345, 395 195, 435 195" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-constrain)"/>
<path d="M335 355 C385 355, 400 367, 435 367" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-constrain)"/>
<path d="M335 365 C390 365, 405 453, 435 453" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-constrain)"/>
<text x="383" y="296" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-text-2)">constrains</text>

<!-- CROSS: surfaces (right → left, dashed) -->
<path d="M435 195 C390 195, 370 150, 325 150" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-surfaces)"/>
<path d="M435 367 C385 367, 365 160, 325 160" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-surfaces)"/>
<path d="M435 539 C380 539, 350 170, 325 170" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-surfaces)"/>
<text x="383" y="490" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-brand-1)">surfaces</text>
<text x="383" y="506" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-brand-1)">decisions</text>

<!-- FLYWHEEL -->
<rect x="120" y="610" width="520" height="80" rx="6" fill="var(--vp-c-bg-alt)" stroke="var(--vp-c-border)" stroke-width="0.75"/>
<text x="380" y="636" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">The flywheel</text>
<text x="380" y="660" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Capture → Compile → Constrain → Surface → repeat</text>
<text x="380" y="680" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-2)">Decisions compound rather than decay</text>

<!-- LEGEND -->
<line x1="120" y1="726" x2="152" y2="726" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-constrain)"/>
<text x="162" y="726" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Enforcement constrains lifecycle</text>

<line x1="430" y1="726" x2="462" y2="726" stroke="var(--vp-c-brand-1)" stroke-width="1.5" stroke-dasharray="5 3" marker-end="url(#arr-surfaces)"/>
<text x="472" y="726" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Lifecycle surfaces new decisions</text>

<text x="120" y="758" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ = specialist agent</text>
<text x="250" y="758" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">18 agents: architect, dba, security, sre, api, qa, pm, and more</text>
</svg>
</div>

<style>
.how-it-works-diagram {
  margin: 24px 0 32px;
  overflow-x: auto;
}
.how-it-works-diagram svg {
  min-width: 640px;
}
.how-it-works-diagram .fill-left { fill: var(--vp-c-bg-soft); }
.how-it-works-diagram .fill-right { fill: var(--vp-c-brand-soft); }

:root {
  --diagram-left-fill: #E8ECF2;
}
.dark {
  --diagram-left-fill: #1E293B;
}
</style>

Two systems working together:

**Architecture governance & compliance** — ADRs, invariants, and guidelines are your current engineering decisions. They compile into `governance.md` — directives Claude reads every session.

**Agentic SDLC governance** — PRD → Spec → Artifacts → Plan → Execute → Drift. Each step feeds the next. Each is constrained by compiled decisions and produces new ones.

They connect at three points:
- Governance **constrains** the spec — existing ADRs and invariants inform the technical design
- Compiled **directives are active** during plan and execution — Claude follows them automatically
- Drift **verifies** the implementation against governance — did we build what we decided?

The details of each mechanism have their own pages:
- [Quality Gates](/governance/gates) — block on critical findings during plan and execution
- [Compiled Directives](/governance/compile) — how ADRs become enforcement
- [Drift Detection](/governance/drift) — the verification step
- [Agents](/agents) — specialist review at each phase
- [Rule Packs](/rules/) — coding standards that fire automatically

## The conversation that drives it

You don't type commands in sequence — you tell Claude what you need, and Claude runs the right step.

> "Write a PRD for Stripe webhook delivery with retry logic and idempotency"

Claude generates structured requirements with acceptance criteria grounded in your project context. Review it, mark it accepted.

> "Write a spec for PRD-005"

Claude checks that PRD-005 is accepted, then routes to `architect`, scans your codebase and ADRs, and generates a technical specification. The spec references the PRD and any relevant ADRs.

> "Generate spec artifacts for SPEC-005"

Claude produces the implementable outputs: data model, API contracts, migrations, test strategy. Each artifact references the spec it came from.

> "Create a plan for SPEC-005"

Claude breaks the spec into phases, routes each to specialist agents for pre-flight review, and returns findings before any code is written.

Then you execute. Claude builds with enforced standards, the active plan phase injected on every prompt.

> "Does the implementation match the spec?"

Claude runs drift detection — comparing what got built against the PRD acceptance criteria, spec requirements, artifact contracts, and ADR compliance.

The full sequence:

```
PRD → spec → artifacts → plan → execute → drift detection
```

**Command references:** `/edikt:prd`, `/edikt:spec`, `/edikt:spec-artifacts`, `/edikt:plan`, `/edikt:drift`

## State machine

Each step in the chain has a status. The chain enforces a strict progression: each artifact must be accepted before the next step can begin.

| Step | Status values | Gate |
|------|--------------|------|
| PRD | `draft` → `accepted` | spec requires `accepted` PRD |
| Spec | `draft` → `accepted` | spec-artifacts requires `accepted` spec |
| Artifacts | `draft` → `accepted` (per artifact) | plan requires `accepted` artifacts |
| Plan | `draft` → `in-progress` → `complete` | Execution proceeds phase by phase |
| Drift report | generated on demand | Closes the loop — accepted vs. built |

Attempting to write a spec on a draft PRD produces a hard block:

```
BLOCKED  PRD-005 status is "draft".
         PRDs must be accepted before generating a spec.
         Review the PRD and change status to "accepted" first.
```

This isn't a suggestion. The gate exists because a draft PRD represents unresolved requirements — building a technical specification on top of unresolved requirements produces wasted work.

## What gets captured at each step

**PRD** — functional requirements, non-functional requirements, acceptance criteria, open questions. The product intent, in structured form.

**Spec** — architecture approach, components, trade-offs, alternatives considered, references to ADRs and invariants. The engineering response to the PRD.

**Artifacts** — the implementable outputs of the spec:
- `data-model.md` — entities, relationships, indexes
- `contracts/api.md` — endpoint definitions, request/response shapes, error codes
- `migrations.md` — schema changes with up and down migrations
- `test-strategy.md` — unit, integration, and edge case coverage
- `contracts/events.md` — event schemas, producers, consumers
- `config-spec.md` — environment variables, feature flags

**Plan** — phased execution with pre-flight specialist review. Each phase is reviewed by the domain agents before any code is written.

**Drift report** — comparison of implementation against spec, PRD acceptance criteria, artifact contracts, ADR decisions, and invariants. Saved to the spec folder.

## Traceability

Every artifact in the chain carries references to what it came from:

```yaml
# spec frontmatter
type: spec
id: SPEC-005
source_prd: PRD-005
references:
  adrs: [ADR-001, ADR-003]
  invariants: [INV-001]
status: accepted
```

```yaml
# artifact frontmatter
type: artifact
artifact_type: data-model
spec: SPEC-005
status: draft
reviewed_by: dba
```

When drift detection runs, it follows these references backward through the chain — checking implementation against artifacts, artifacts against spec, spec against PRD acceptance criteria.

## Why this matters

Without the chain, the engineering cycle is scattered: requirements in Notion, decisions in Slack, specs in someone's head, verification by hope. The chain creates a single, version-controlled, machine-readable path from "what we decided to build" to "what we actually built."

The drift check closes the loop. It's not optional ceremony — it's the mechanism that makes the governance chain a governance chain rather than a documentation exercise.

## When to use it

You don't have to use the full chain for every piece of work. For ad hoc tasks, edikt's rules and hooks govern the session without PRD or spec. The chain is for features where traceability matters — where you need to verify that implementation matches intent.

For new features, the chain is the right default. Tell Claude what you want to build, let it generate acceptance criteria, review and accept it, then proceed.

See [/edikt:prd](/commands/prd), [/edikt:spec](/commands/spec), [/edikt:spec-artifacts](/commands/spec-artifacts), [/edikt:plan](/commands/plan), [/edikt:drift](/commands/drift).
