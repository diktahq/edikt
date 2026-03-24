---
layout: home

hero:
  name: 'edi<span style="color: var(--vp-c-brand-1)">kt</span>'
  text: "The governance layer for agentic engineering."
  tagline: Your coding standards, enforced. Your decisions, remembered. Every session, every engineer, every project.
  actions:
    - theme: brand
      text: Get Started — 5 minutes
      link: /getting-started
    - theme: alt
      text: What is edikt?
      link: /what-is-edikt

features:
  - title: Architecture governance & compliance
    details: "Capture architecture decisions, constraints, and team conventions. /edikt:compile reads all three, checks for contradictions, and produces a governance file Claude reads automatically — every session, before writing any code. Update a decision, recompile. One source of truth."
  - title: Correctness guardrails
    details: "20 rule packs install to .claude/rules/ and fire automatically — path-conditional, so Go rules only fire on .go files. Correctness guardrails that catch real bugs: hallucinated APIs, race conditions, placeholder code. The standard is the same whether it's your best engineer or your newest."
  - title: Agentic SDLC governance
    details: "PRD → spec → artifacts → plan → execute → drift detection. Each step feeds the next. Each is status-gated. Each is constrained by your compiled decisions and produces new decisions that feed back into governance. The full Agentic SDLC, governed."
  - title: Quality gates
    details: "Critical findings block progression automatically. A hardcoded JWT secret stops the build until resolved. Overrides are logged with git identity — you see who approved what, on which project. No silent failures."
  - title: Natural language, not commands
    details: "Say 'what's our status?' and Claude shows the governance dashboard. Say 'write a PRD for X' and Claude generates structured requirements. Say 'does the implementation match the spec?' and Claude runs drift detection. You talk. edikt handles the rest."
  - title: Zero dependencies
    details: "Every file is .md or .yaml. No build step, no runtime, no daemon, no lock-in. curl | bash to install. If you stop using edikt, the files stay — plain markdown you own, read, edit, and version-control."
---

Claude has memory. It doesn't have governance. Every session starts without standards — and decisions made yesterday get contradicted today.

**Solo engineers** use edikt to stop re-explaining their architecture every session. **Team leads** use it to enforce standards across every engineer's Claude — so code review catches design issues, not formatting. **Consultancies** use it to install their methodology on day one of every client project.
