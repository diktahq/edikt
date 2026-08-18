# Governance Chain

The governance chain connects edikt's two systems — architecture governance & compliance and Agentic SDLC governance — into a single traceable path from intent to implementation to verification.

<div class="how-it-works-diagram">
<svg width="100%" viewBox="0 0 760 1020" xmlns="http://www.w3.org/2000/svg">
<defs>
<marker id="arr-slate" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-teal" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-amber" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="#D97706" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="arr-violet" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="#8B5CF6" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
</defs>

<!-- Column headers -->
<rect x="40" y="16" width="300" height="36" rx="4" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.75"/>
<text x="190" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" letter-spacing=".02em" fill="var(--vp-c-text-1)">Architecture governance &amp; compliance</text>

<rect x="420" y="16" width="300" height="36" rx="4" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" letter-spacing=".02em" fill="var(--vp-c-brand-1)">Agentic SDLC governance</text>

<!-- RIGHT: Brainstorm (entry point) -->
<rect x="455" y="68" width="230" height="52" rx="6" fill="var(--diagram-brainstorm-fill)" stroke="#8B5CF6" stroke-width="0.75"/>
<text x="570" y="86" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="14" font-weight="600" fill="#8B5CF6">Brainstorm</text>
<text x="570" y="106" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="var(--vp-c-text-2)">explore ideas · converge toward a decision</text>

<!-- Brainstorm → PRD -->
<line x1="570" y1="120" x2="570" y2="148" stroke="#8B5CF6" stroke-width="1.5" marker-end="url(#arr-violet)"/>

<!-- Brainstorm → ADR (curves left) -->
<path d="M455 94 L400 94 L400 160 L325 160" fill="none" stroke="#8B5CF6" stroke-width="1.0" stroke-dasharray="4 3" marker-end="url(#arr-violet)"/>
<text x="390" y="82" text-anchor="end" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="10" fill="#8B5CF6">ADR</text>

<!-- LEFT: Decisions -->
<rect x="55" y="100" width="270" height="116" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="124" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">Decisions</text>
<text x="190" y="148" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">ADRs — architecture choices</text>
<text x="190" y="170" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Invariants — hard constraints</text>
<text x="190" y="192" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Guidelines — team conventions</text>

<line x1="190" y1="216" x2="190" y2="248" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Compile -->
<rect x="105" y="248" width="170" height="44" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="270" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">/edikt:gov:compile</text>

<line x1="190" y1="292" x2="190" y2="324" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Enforcement surface (4 rendered surfaces, compile_schema_version: 3) -->
<rect x="45" y="324" width="290" height="58" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="346" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">Enforcement surface</text>
<text x="190" y="368" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1)">Ambient · topic · directive-index · skill</text>

<line x1="190" y1="382" x2="190" y2="414" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Hooks -->
<rect x="65" y="414" width="250" height="58" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-text-2)" stroke-width="0.5"/>
<text x="190" y="436" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">Lifecycle hooks</text>
<text x="190" y="458" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Context recovery · plan injection · gates</text>

<line x1="190" y1="472" x2="190" y2="504" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>

<!-- LEFT: Signal detection -->
<rect x="55" y="504" width="270" height="58" rx="6" fill="var(--diagram-left-fill)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="190" y="526" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Signal detection</text>
<text x="190" y="548" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Detects new decisions mid-session</text>

<!-- Feedback: signal → decisions (same relationship as "surfaces decisions" on the right — teal, same legend entry) -->
<path d="M55 533 L28 533 L28 160 L55 160" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-teal)"/>
<text x="18" y="347" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" font-weight="500" fill="var(--vp-c-brand-1)" transform="rotate(-90 18 347)">new ADR / invariant</text>

<!-- RIGHT: PRD -->
<rect x="435" y="148" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="168" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Requirements (PRD)</text>
<text x="570" y="190" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ pm agent · draft → accepted</text>

<line x1="570" y1="206" x2="570" y2="234" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Spec -->
<rect x="435" y="234" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="254" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Technical spec</text>
<text x="570" y="276" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ architect · dba · api · draft → accepted</text>

<line x1="570" y1="292" x2="570" y2="320" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Artifacts -->
<rect x="435" y="320" width="270" height="72" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="340" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Spec artifacts</text>
<text x="570" y="362" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ dba · api · qa agents</text>
<text x="570" y="380" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="#D97706">draft → accepted → in-progress → implemented</text>

<line x1="570" y1="392" x2="570" y2="420" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Plan -->
<rect x="435" y="420" width="270" height="80" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="440" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Plan + pre-flight</text>
<text x="570" y="462" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ specialists + evaluator pre-flight</text>

<line x1="570" y1="500" x2="570" y2="528" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Execute -->
<rect x="435" y="528" width="270" height="80" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="548" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Execute + post-flight</text>
<text x="570" y="570" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">governed session · quality gates · evaluator</text>

<!-- Evaluator loop: execute → plan (retry) -->
<path d="M705 558 L730 558 L730 458 L705 458" fill="none" stroke="#D97706" stroke-width="1.2" stroke-dasharray="4 3" marker-end="url(#arr-amber)"/>
<text x="744" y="508" text-anchor="start" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="10" font-weight="500" fill="#D97706" transform="rotate(90 744 508)">retry on FAIL</text>

<line x1="570" y1="608" x2="570" y2="636" stroke="var(--vp-c-brand-1)" stroke-width="1.5" marker-end="url(#arr-teal)"/>

<!-- RIGHT: Drift -->
<rect x="435" y="636" width="270" height="72" rx="6" fill="var(--vp-c-brand-soft)" stroke="var(--vp-c-brand-1)" stroke-width="0.75"/>
<text x="570" y="656" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-brand-1)">Drift detection</text>
<text x="570" y="678" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ architect · engineer · qa agents</text>
<text x="570" y="696" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="#D97706">auto-promote in-progress → implemented</text>

<!-- CROSS: constrains (left → right) -->
<path d="M335 344 C375 344, 390 263, 435 263" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-slate)"/>
<path d="M335 354 C380 354, 395 458, 435 458" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-slate)"/>
<path d="M335 366 C385 366, 400 565, 435 565" fill="none" stroke="var(--vp-c-text-2)" stroke-width="1.2" marker-end="url(#arr-slate)"/>
<text x="388" y="400" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-text-2)">constrains</text>

<!-- CROSS: surfaces (right → left, dashed) -->
<path d="M435 263 C390 263, 370 170, 325 170" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-teal)"/>
<path d="M435 458 C380 458, 360 180, 325 180" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-teal)"/>
<path d="M435 668 C375 668, 345 190, 325 190" fill="none" stroke="var(--vp-c-brand-1)" stroke-width="1.2" stroke-dasharray="5 3" marker-end="url(#arr-teal)"/>
<text x="388" y="600" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-brand-1)">surfaces</text>
<text x="388" y="616" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" font-weight="600" fill="var(--vp-c-brand-1)">decisions</text>

<!-- FLYWHEEL -->
<rect x="120" y="750" width="520" height="80" rx="6" fill="var(--vp-c-bg-alt)" stroke="var(--vp-c-border)" stroke-width="0.75"/>
<text x="380" y="776" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1)">The flywheel</text>
<text x="380" y="800" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1)">Brainstorm → Capture → Compile → Constrain → Execute → Evaluate → Verify</text>
<text x="380" y="820" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-2)">Decisions compound. Failures teach. Every session is more governed than the last.</text>

<!-- LEGEND -->
<line x1="120" y1="870" x2="152" y2="870" stroke="var(--vp-c-text-2)" stroke-width="1.5" marker-end="url(#arr-slate)"/>
<text x="162" y="870" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1)">Enforcement constrains lifecycle</text>

<line x1="440" y1="870" x2="472" y2="870" stroke="var(--vp-c-brand-1)" stroke-width="1.5" stroke-dasharray="5 3" marker-end="url(#arr-teal)"/>
<text x="482" y="870" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1)">A session surfaces new decisions</text>

<line x1="120" y1="898" x2="152" y2="898" stroke="#D97706" stroke-width="1.2" stroke-dasharray="4 3" marker-end="url(#arr-amber)"/>
<text x="162" y="898" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1)">Evaluator retry + lifecycle transitions</text>

<line x1="440" y1="898" x2="472" y2="898" stroke="#8B5CF6" stroke-width="1.2" stroke-dasharray="4 3" marker-end="url(#arr-violet)"/>
<text x="482" y="898" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1)">Brainstorm feeds SDLC or decisions</text>

<text x="120" y="930" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">⬡ = specialist agent</text>
<text x="250" y="930" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2)">18 domain agents: architect, dba, security, sre, api, qa, pm, and more</text>
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

:root {
  --diagram-left-fill: #E8ECF2;
  --diagram-brainstorm-fill: #F3EDFF;
}
.dark {
  --diagram-left-fill: #1E293B;
  --diagram-brainstorm-fill: #1E1533;
}
</style>

Two systems working together:

**Architecture governance & compliance** — ADRs, invariants, and guidelines are your current engineering decisions. They compile into four rendered surfaces under `.claude/rules/`: an always-loaded ambient core, per-topic files, a glob-keyed `directive-index.yaml` that drives write-time enforcement, and a manifest — see [Compile](compile) for the full render model. edikt's writes are confined to co-located `<artifact>.edikt.yaml` sidecars and the compiled surfaces above. The prose `.md` you author is never touched by `gov:compile` — the boundary between human-owned and tool-owned bytes is structural, not definitional. See [Sidecar Architecture](sidecar).

**Agentic SDLC governance** — (Brainstorm) → (Discovery) → PRD → Spec → Artifacts → Plan → Execute → Drift. Brainstorm and discovery are optional, conditional entry points — an idea can go straight to a PRD. Each step feeds the next. Each is constrained by compiled decisions and produces new ones. PRDs and specs use the same split as ADRs and invariants: a narrative `.md` for humans plus a co-located `.yaml` sidecar that's the actual structured source of truth — FRs, ACs, coverage mappings, revision history. See [Traceability](#traceability) below for what's really in each.

They connect at three points:
- Governance **constrains** the spec — existing ADRs and invariants inform the technical design
- Compiled **directives are active** during plan and execution — Claude follows them automatically
- Drift **verifies** the implementation against governance — did we build what we decided?

The details of each mechanism have their own pages:
- [Quality Gates](/governance/gates) — block on critical findings during plan and execution
- [Compiled Directives](/governance/compile) — how ADRs become enforcement
- [Drift Detection](/governance/drift) — the verification step
- [Specialist Agents](/guides/specialist-agents) — specialist review at each phase
- [Rule Packs](/rules/) — coding standards that fire automatically

## The conversation that drives it

You don't type commands in sequence — you tell Claude what you need, and Claude runs the right step.

The chain doesn't have to start at the PRD. When the idea itself isn't settled yet, two optional steps come before it — neither is mandatory, and they don't have to be used together.

> "Let's brainstorm this"

Claude opens a free-form, project-grounded conversation, pulling in specialist agents as relevant topics surface. When the conversation converges, you choose what it becomes: a PRD, a spec — brainstorm can formalize straight into either one — or just a saved brainstorm doc (`BRAIN-NNN`) to pick up later. Discovery is not a required stop in between.

> "Run discovery on this" (fresh, or `/edikt:sdlc:discovery BRAIN-003` to lift a saved brainstorm)

Use discovery when the idea has unknowns worth resolving before you commit to requirements — it's a peer command to brainstorm, not a wrapper around it, so it can start from scratch or lift a brainstorm's findings as a seed. It interviews you on what's known, what's uncertain, what would change your mind, and the smallest experiment that resolves the biggest unknown, then it always graduates into a PRD.

From here, the chain most people mean when they say "the governance chain":

> "Write a PRD for Stripe webhook delivery with retry logic and idempotency"

Claude generates structured requirements with acceptance criteria grounded in your project context. Review it, mark it accepted.

> "Write a spec for PRD-005"

Claude checks that PRD-005 is accepted, then routes to `architect`, scans your codebase and ADRs, and generates a technical specification. The spec references the PRD and any relevant ADRs.

> "Generate spec artifacts for SPEC-005"

Claude produces the implementable outputs: data model, API contracts, migrations, test strategy. Each artifact references the spec it came from.

> "Create a plan for SPEC-005"

Claude breaks the spec into phases, routes each to specialist agents for pre-flight review, and returns findings before any code is written.

::: tip What pre-flight and revision actually catch
Two things worth knowing before you get here. First: pre-flight criteria
validation reads the spec's acceptance criteria literally, against your
actual codebase — not against what sounds plausible. If SPEC-005 claims a
duplicate webhook delivery returns `409 Conflict`, but your error package
has no such response and every other conflict in the API returns `422
Unprocessable Entity`, pre-flight flags it before a plan phase inherits the
wrong criterion.

Second: some gaps don't show up in review at all — they show up while
writing the test. A retry scheduler can pass every code review and still
have no protection against double-delivery if the worker crashes and
restarts mid-retry. That's the kind of gap a "does the retry stop after N
attempts" test won't catch, but a "does a mid-retry crash-restart still
deliver exactly once" test will. When it does, the spec gets a new
requirement, and the chain records why.
:::

Then you execute. Claude builds with enforced standards, the active plan phase injected on every prompt.

Each phase's completion triggers post-flight — a composite review, not a single check. L1 is the phase's own criteria-verify verdict. L2 asks whether the phase's diff violates any compiled ADR or invariant. L3 routes the same diff to the domain specialists the changed files call for (`/edikt:sdlc:code-review`, also invocable standalone anytime you want a second opinion). A synthesizer dedupes all three into one report before the phase can flip to `done`.

> "Does the implementation match the spec?"

Claude runs drift detection — comparing what got built against the PRD acceptance criteria, spec requirements, artifact contracts, and ADR compliance.

The full sequence, with the two optional entry points in parentheses:

```text
(brainstorm) → (discovery) → PRD → spec → artifacts → plan → execute → drift detection
```

**Command references:** `/edikt:brainstorm`, `/edikt:sdlc:discovery`, `/edikt:sdlc:prd`, `/edikt:sdlc:spec`, `/edikt:sdlc:artifacts`, `/edikt:sdlc:plan`, `/edikt:sdlc:post-flight`, `/edikt:sdlc:code-review`, `/edikt:sdlc:drift`

## State machine

Each step in the chain has a status. The chain enforces a strict progression: each artifact must be accepted before the next step can begin.

| Step | Status values | Gate |
|------|--------------|------|
| PRD | `draft` → `accepted` | spec requires `accepted` PRD |
| Spec | `draft` → `accepted` | spec-artifacts requires `accepted` spec |
| Artifacts | `draft` → `accepted` (per artifact) | plan requires `accepted` artifacts |
| Plan phases | `pending` → `in-progress` → `done` (or `blocked`/`stuck`/`skipped`) | Execution proceeds phase by phase; post-flight gates each phase's `done` transition |
| Drift report | generated on demand | Closes the loop — accepted vs. built |

Attempting to write a spec on a draft PRD produces a hard block:

```text
BLOCKED  PRD-005 status is "draft".
         PRDs must be accepted before generating a spec.
         Review the PRD and change status to "accepted" first.
```

This isn't a suggestion. The gate exists because a draft PRD represents unresolved requirements — building a technical specification on top of unresolved requirements produces wasted work.

## What gets captured at each step

**PRD** — functional requirements, non-functional requirements, acceptance criteria, open questions. The product intent, in structured form.

**Spec** — architecture approach, components, trade-offs, alternatives considered, references to ADRs and invariants. The engineering response to the PRD.

**Artifacts** — design blueprints generated from the spec. Format depends on database type:
- `data-model.mmd` (SQL), `data-model.schema.yaml` (MongoDB), or `data-model.md` (DynamoDB/KV) — entities, relationships, indexes
- `contracts/api.yaml` — OpenAPI 3.0 endpoint definitions, request/response shapes, error codes
- `migrations/` — numbered SQL migration files with up/down/backfill/risk (SQL and mixed only)
- `test-strategy.md` — unit, integration, and edge case coverage
- `contracts/events.yaml` — AsyncAPI 2.6 event schemas, producers, consumers
- `fixtures.yaml` — portable seed data for dev and testing
- `config-spec.md` — environment variables, feature flags

**Plan** — phased execution with pre-flight specialist review. Each phase is reviewed by the domain agents before any code is written.

**Post-flight report** — composite of the phase's criteria-verify verdict (L1), a governance-diff check against compiled ADRs and invariants (L2), and specialist findings (L3), deduplicated by a synthesizer. Saved per phase to `.edikt/state/post-flight/`.

**Drift report** — comparison of implementation against spec, PRD acceptance criteria, artifact contracts, ADR decisions, and invariants. Saved to the spec folder.

## Traceability

Every artifact in the chain carries references to what it came from — but the identity fields you see in a `.md`'s frontmatter are only half the story. PRDs and specs each pair their narrative `.md` with a co-located `.yaml` sidecar, and the sidecar is where the actual traceability data — requirements, acceptance criteria, coverage mappings — lives. Same split as ADRs and invariants, applied to the SDLC side.

```yaml
# spec.md frontmatter — identity and references only
type: spec
id: SPEC-005
source_prd: PRD-005
references:
  adrs: [ADR-001, ADR-003]
  invariants: [INV-001]
status: accepted
```

```yaml
# spec.yaml — the sidecar, structured source of truth
source_prd_coverage:
  prd: PRD-005
  covered:
    - fr: FR-001
      by: [SR-001, SR-002]
  deferred: []
  uncovered: []
acceptance_criteria:
  - id: AC-001-1        # passed through from the PRD, byte-equal, never renumbered
    fr: FR-001
    given: "..."
    when: "..."
    then: "..."
  - id: SAC-001          # added by the spec itself — architectural-layer criteria
    source: spec
```

Every PRD `FR-NNN` must be covered by at least one spec `SR-NNN`, deferred with a stated reason, or explicitly listed as uncovered — nothing falls through silently. The spec's own `SPEC-{NNN}` folder holds `spec.md` + `spec.yaml` side by side; `/edikt:sdlc:spec-review` re-checks this coverage any time the spec is hand-edited after generation.

```yaml
# artifact frontmatter — no separate sidecar; this is the whole record
type: artifact
artifact_type: data-model
spec: SPEC-005
status: draft
reviewed_by: dba
```

When drift detection runs, it follows these references backward through the chain — checking implementation against artifacts, artifacts against the spec's FR coverage, spec against PRD acceptance criteria.

## Why this matters

Without the chain, the engineering cycle is scattered: requirements in Notion, decisions in Slack, specs in someone's head, verification by hope. The chain creates a single, version-controlled, machine-readable path from "what we decided to build" to "what we actually built."

The drift check closes the loop. It's not optional ceremony — it's the mechanism that makes the governance chain a governance chain rather than a documentation exercise.

## Artifact lifecycle

Every artifact in the chain follows a status lifecycle:

```
draft → accepted → in-progress → implemented → superseded
```

| Transition | Trigger | Who |
|-----------|---------|-----|
| draft → accepted | Change `status:` in frontmatter | Manual |
| accepted → in-progress | Plan starts a phase referencing this artifact | Auto |
| in-progress → implemented | Drift finds no violations | Auto |
| any → superseded | Create replacement artifact | Manual |

**Enforcement across commands:**
- `/edikt:sdlc:plan` warns when artifacts are still draft — lists them by name and offers to proceed with Known Risks or stop
- `/edikt:sdlc:drift` skips draft and superseded artifacts, validates the rest
- `/edikt:doctor` flags artifacts stuck in draft for more than 7 days

## When to use it

You don't have to use the full chain for every piece of work. For ad hoc tasks, edikt's rules and hooks govern the session without PRD or spec. The chain is for features where traceability matters — where you need to verify that implementation matches intent.

For new features, the chain is the right default. Tell Claude what you want to build, let it generate acceptance criteria, review and accept it, then proceed. If the feature itself isn't defined yet, start earlier — with a brainstorm to explore it, or a discovery doc if it has unknowns worth resolving before writing requirements.

See [/edikt:brainstorm](/commands/brainstorm), [/edikt:sdlc:discovery](/commands/sdlc/discovery), [/edikt:sdlc:prd](/commands/sdlc/prd), [/edikt:sdlc:spec](/commands/sdlc/spec), [/edikt:sdlc:artifacts](/commands/sdlc/artifacts), [/edikt:sdlc:plan](/commands/sdlc/plan), [/edikt:sdlc:drift](/commands/sdlc/drift).
