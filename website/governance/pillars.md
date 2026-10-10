# The Two Pillars: Extraction and Delivery

Governance is two questions. **What are the rules?** **When does the model see them?**
Extraction answers the first. Delivery answers the second. Everything else in edikt
serves one of the two.

<div class="how-it-works-diagram">
<svg width="100%" viewBox="0 0 760 640" xmlns="http://www.w3.org/2000/svg" role="img" aria-label="The two pillars of governance: extraction on the left turns your decisions into directives; delivery on the right puts them in front of the model on three surfaces and verifies the work at phase end.">
<defs>
<marker id="p-arr-slate" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="p-arr-teal" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
<marker id="p-arr-violet" viewBox="0 0 10 10" refX="8" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M2 1L8 5L2 9" fill="none" stroke="#8B5CF6" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"/></marker>
</defs>

<!-- Column headers -->
<rect x="40" y="16" width="300" height="36" rx="4" fill="var(--diagram-left-fill, var(--vp-c-bg-soft, #f3f4f6))" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="0.75"/>
<text x="190" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" fill="var(--vp-c-text-1, #111827)">EXTRACTION — what are the rules?</text>
<rect x="420" y="16" width="300" height="36" rx="4" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.75"/>
<text x="570" y="34" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" fill="var(--vp-c-text-1, #111827)">DELIVERY — when does the model see them?</text>

<!-- LEFT: your decisions -->
<rect x="55" y="76" width="270" height="112" rx="6" fill="var(--diagram-left-fill, var(--vp-c-bg-soft, #f3f4f6))" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="0.5"/>
<text x="190" y="98" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Your decisions</text>
<text x="190" y="122" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1, #111827)">ADRs — a choice with a rejected alternative</text>
<text x="190" y="144" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1, #111827)">Invariants — a property that always holds</text>
<text x="190" y="166" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1, #111827)">Guidelines — a default with a named exception</text>
<line x1="190" y1="188" x2="190" y2="218" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="1.5" marker-end="url(#p-arr-slate)"/>
<text x="200" y="203" text-anchor="start" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="var(--vp-c-text-2, #6b7280)">plain markdown, yours</text>

<!-- LEFT: extractor -->
<rect x="55" y="220" width="270" height="76" rx="6" fill="var(--diagram-left-fill, var(--vp-c-bg-soft, #f3f4f6))" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="0.5"/>
<text x="190" y="242" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Extractor</text>
<text x="190" y="264" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1, #111827)">one sentence per rule · force · intent</text>
<text x="190" y="282" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2, #6b7280)">never invents, never paraphrases</text>
<line x1="190" y1="296" x2="190" y2="326" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="1.5" marker-end="url(#p-arr-slate)"/>

<!-- LEFT: sidecar -->
<rect x="55" y="328" width="270" height="62" rx="6" fill="var(--diagram-left-fill, var(--vp-c-bg-soft, #f3f4f6))" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="0.5"/>
<text x="190" y="350" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Sidecar</text>
<text x="190" y="372" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="13" fill="var(--vp-c-text-1, #111827)">directives with provenance — the source line</text>
<line x1="190" y1="390" x2="190" y2="420" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="1.5" marker-end="url(#p-arr-slate)"/>

<!-- LEFT: compile -->
<rect x="105" y="422" width="170" height="44" rx="6" fill="var(--diagram-left-fill, var(--vp-c-bg-soft, #f3f4f6))" stroke="var(--vp-c-text-2, #6b7280)" stroke-width="0.5"/>
<text x="190" y="444" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">/edikt:gov:compile</text>
<text x="190" y="482" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2, #6b7280)">deterministic merge · drift and grounding measured</text>

<!-- BRIDGE: compile → the three surfaces (teal spine) -->
<path d="M275 444 L380 444 L380 98 L432 98" fill="none" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>
<path d="M380 158 L432 158" fill="none" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>
<path d="M380 218 L432 218" fill="none" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>
<text x="386" y="300" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="var(--vp-c-brand-1, #0f766e)" transform="rotate(-90 386 300)">compiled surfaces</text>

<!-- RIGHT: three surfaces -->
<rect x="435" y="76" width="270" height="44" rx="6" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.5"/>
<text x="570" y="90" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Ambient core</text>
<text x="570" y="108" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1, #111827)">always loaded — the non-negotiables</text>
<rect x="435" y="136" width="270" height="44" rx="6" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.5"/>
<text x="570" y="150" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Topic files</text>
<text x="570" y="168" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1, #111827)">loaded when a matching file is touched</text>
<rect x="435" y="196" width="270" height="44" rx="6" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.5"/>
<text x="570" y="210" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Skill packages</text>
<text x="570" y="228" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1, #111827)">loaded when the task calls for the topic</text>
<line x1="570" y1="240" x2="570" y2="270" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>
<text x="580" y="255" text-anchor="start" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="var(--vp-c-text-2, #6b7280)">read by judgement</text>

<!-- RIGHT: the model -->
<circle cx="570" cy="302" r="30" fill="var(--vp-c-bg, #ffffff)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5"/>
<text x="570" y="302" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="14" font-weight="600" fill="var(--vp-c-text-1, #111827)">the model</text>
<line x1="570" y1="332" x2="570" y2="362" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>

<!-- RIGHT: the work -->
<rect x="480" y="364" width="180" height="40" rx="6" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.5"/>
<text x="570" y="384" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">the work</text>
<line x1="570" y1="404" x2="570" y2="434" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="1.5" marker-end="url(#p-arr-teal)"/>
<text x="580" y="419" text-anchor="start" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="var(--vp-c-text-2, #6b7280)">at the end of a phase</text>

<!-- RIGHT: phase-end review -->
<rect x="435" y="436" width="270" height="58" rx="6" fill="var(--vp-c-brand-soft, #d9efec)" stroke="var(--vp-c-brand-1, #0f766e)" stroke-width="0.5"/>
<text x="570" y="456" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="15" font-weight="600" fill="var(--vp-c-text-1, #111827)">Phase-end review</text>
<text x="570" y="477" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-1, #111827)">verifies the work against the same directives</text>

<!-- LOOP: review cites the sidecar (violet, dashed) -->
<path d="M435 465 L400 465 L400 520 L190 520 L190 392" fill="none" stroke="#8B5CF6" stroke-width="1.0" stroke-dasharray="4 3" marker-end="url(#p-arr-violet)"/>
<text x="296" y="510" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="11" fill="#8B5CF6">cites the directives it used — uncited ones are pruned by evidence</text>

<!-- FOOT: the one thing edikt blocks -->
<rect x="40" y="556" width="680" height="56" rx="6" fill="none" stroke="#D97706" stroke-width="1.0" stroke-dasharray="5 3"/>
<text x="380" y="576" text-anchor="middle" dominant-baseline="central" font-family="Space Grotesk, sans-serif" font-size="13" font-weight="600" fill="#D97706">edikt does not inject on every edit and does not deny writes</text>
<text x="380" y="596" text-anchor="middle" dominant-baseline="central" font-family="IBM Plex Sans, sans-serif" font-size="12" fill="var(--vp-c-text-2, #6b7280)">It blocks exactly one thing: edits to its own managed regions. Delivery is “load the right rules, then check the work.”</text>
</svg>
</div>


## Extraction: from your decisions to rules a model can follow

You write decisions, invariants and guidelines in plain markdown. edikt extracts each
into **directives**: one sentence per rule, its force stated as *must* or *should*, its
intent, the line it came from, and the observation that would prove it broken.

The extractor never invents and never paraphrases. Every directive traces back to a
sentence you wrote, and you can follow the trace in the sidecar next to the artifact.

Quality is measured, not assumed. Compile checks that every directive is grounded in
its source, that the source still says what the directive says, and that nothing has
drifted since the last extraction.

| | You own | edikt owns |
|---|---|---|
| **Input** | artifacts that extract well: the right kind (invariant, decision, guideline), one rule per sentence, force and intent stated | the extractor, its contract, and its pins |
| **Output** | — | the sidecar, the compiled surfaces, the drift report |
| **Measured by** | — | grounding, provenance, drift, double-loading |

## Delivery: the right rule in front of the model at the right time

Rules reach the model on three surfaces, chosen by *when* each rule is needed:

- **The ambient core.** A small set of non-negotiables, loaded in every session.
- **Topic files.** Loaded when the model touches a file matching the topic's paths.
- **Skill packages.** Loaded when the task calls for the topic, before any file is
  touched.

The model reads them by judgement. Then, at the end of a phase, a review verifies the
work against the same directives and cites the ones it used.

Delivery is *load the right rules, then check the work*. It is not *block the keystroke*.

### What edikt does not do

edikt does not inject rules into every edit and does not deny writes. Deterministic
checks in the write path are flaky, fire on correct code, and their effect on behaviour
was never measured. The one delivery mechanism with a measured effect is the ambient
core: in the counter-pattern experiment, loading it moved rule-following from zero of
nine sessions to four of seven, against a codebase that demonstrated the violation.
That is what edikt ships and stands behind.

edikt blocks exactly one thing: edits to its own managed regions in the files it
generates. It says so when it does.

## How the pillars meet

Extraction quality is delivery cost. Every directive extracted is a directive loaded, so
an artifact that extracts into noise is paid for on every session. Two tools keep the
two honest:

- **The session tally** shows which rules were in scope for the files you touched.
- **Prune by evidence.** A directive that a review never cites is a candidate for
  removal, decided by the count, not by opinion.

## Where to go next

- [How Governance Compiles](/governance/compile) — the extraction pipeline in detail.
- [Sidecar Architecture](/governance/sidecar) — what a directive record contains.
- [Architecture Decisions](/governance/architecture-decisions), [Invariant Records](/governance/invariant-records), [Guidelines](/governance/guidelines) — the three kinds of artifact and when to write each.
