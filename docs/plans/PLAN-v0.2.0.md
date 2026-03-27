# Plan: edikt v0.2.0

**Status:** Planned
**Theme:** Claude Code Surface Sync + Rule Pack UX + Installer Safety + Real-World Compliance

---

## Overview

Five workstreams for v0.2.0:

1. **Codebase Pattern Learning** — edikt learns from the existing codebase and points Claude to real examples. When writing a new PR, Claude looks at how existing PRs are structured. When creating a handler, Claude finds a similar handler in the project to follow. Reduces hallucination by grounding Claude in what already exists.

2. **Claude Code Surface Sync** — Close the gap between edikt and Claude Code. Adopt agent governance fields (`maxTurns:`, `disallowedTools:`), new hooks (StopFailure, SessionEnd, SubagentStart), and the SendMessage migration.

3. **Rule Pack UX** — Rule packs installed by default can conflict with existing project conventions (e.g., `base/api.md` vs. an existing API guidelines file). The opacity is the real problem: Claude follows rules the user didn't author and doesn't know why. Design the right posture: conflict detection during init, opt-in vs. default-on, opinionated vs. principle-based.

4. **Installer Safety** — `install.sh` silently overwrites files on reinstall, including customized commands. No safety guarantees are documented. Design explicit guarantees: what edikt touches, what it never touches, what happens on reinstall.

5. **EXP-003: Real-World Compliance** — Test rule compliance under conditions that match actual edikt usage: 14+ rules loaded simultaneously, multi-turn conversations, context compaction recovery, real project conventions.

---

## Progress

| Phase | Theme | Status | Updated |
|-------|-------|--------|---------|
| 1a | Design session — codebase pattern learning | not started | — |
| 1b | Design session — Claude Code surface sync | not started | — |
| 1c | Design session — harness design audit | not started | — |
| 1d | Design session — rule pack UX | not started | — |
| 1e | Design session — installer safety | not started | — |
| 1f | Design session — dual-audience governance docs | not started | — |
| 2 | PRD-006 — v0.2.0 requirements | not started | — |
| 3 | Codebase pattern learning — scan, index, point Claude to examples | not started | — |
| 4 | Agent governance fields (`maxTurns:`, `disallowedTools:`) | not started | — |
| 5 | New hooks (StopFailure, SessionEnd, SubagentStart) | not started | — |
| 6 | Agent resume → SendMessage migration | not started | — |
| 7 | Rule pack UX — conflict detection + install posture | not started | — |
| 8 | Installer safety — guarantees + silent overwrite fix | not started | — |
| 9 | Dual-audience governance docs — sentinel-based guideline format | not started | — |
| 10 | Harness design audit — `/edikt:harness` command | not started | — |
| 11 | EXP-003: Real-world compliance experiment | not started | — |
| 12 | Website + docs update for v0.2.0 | not started | — |

**Shipped in v0.1.1 (removed from this plan):**
- ~~HTML sentinel migration~~ → shipped
- ~~Effort frontmatter on all commands~~ → shipped

---

## Phase 1a: Design Session — Codebase Pattern Learning

**Input:** Phase 3 description below + codebase examples
**Output:** Design decisions for how edikt learns and surfaces patterns
**Process:** Load context → review the concept → decide on scan approach, storage, user confirmation

## Phase 1b: Design Session — Claude Code Surface Sync

**Input:** `v0.2.0 Design Session — Claude Code Surface Sync.md` in Obsidian
**Output:** Design decisions for agent governance fields, new hooks, SendMessage migration
**Process:** Load context → review each feature → decide on approach → resolve open questions

## Phase 1c: Design Session — Harness Design Audit

**Input:** Anthropic's "Harness Design for Long-Running Application Development" (2026-03-24), existing `/edikt:doctor` command, EXP-004 results
**Output:** Design decisions for a `/edikt:harness` command and harness design guide
**Reference architecture (from article):**
- Planner → Generator → Evaluator separation. edikt's equivalent: Spec → Artifacts → Plan → Execute + specialist agent review. This mapping should be explicit in the guide.
- Sprint contracts = Completion Promises. The gap: edikt defines them in plan files but they aren't surfaced to evaluators as an explicit contract before execution starts.
- "Every harness component encodes an assumption about what the model can't do." This is the key design principle. `/edikt:harness` should surface the assumption behind each installed component — not just "PostCompact hook is installed" but "PostCompact hook exists because Claude loses plan context after compaction without it." As models improve, these assumptions are the things to stress-test.
- Self-evaluation bias: Claude generates AND evaluates its own output in most edikt commands. The generator-evaluator separation should be more explicit in harness guidance.
- Context resets outperform compaction for long tasks (Sonnet 4.5 exhibited "context anxiety"). edikt handles compaction recovery but has no intentional context reset pattern. Should edikt provide a reset strategy — intentional window clear with state preserved in the plan progress table?

**Key questions:**
- What does a complete harness look like? (rules + compiled governance + hooks + agents + gates + scoping)
- How do we audit completeness vs just setup correctness? (doctor checks "is it configured right," harness checks "is it configured enough")
- What's the gap between `/edikt:init` (install a harness) and `/edikt:harness` (audit/design a harness)?
- Should harness design be a guide, a command, or both?
- How do we surface harness assumptions per component so users can stress-test them as models evolve?
- Should edikt provide an intentional context reset pattern alongside compaction recovery?

## Phase 1d: Design Session — Rule Pack UX

**Problem (observed in real usage):** Rule packs installed by default conflict with existing project conventions. Example: `base/api.md` conflicts with an established API guidelines file already in the repo. The user doesn't understand why Claude is behaving differently — the rules are opaque.

**The tension:** Don't install → no visible value. Install by default → conflicts and user confusion. Prompt to review → most users skip it.

**Key questions:**
- Should rule packs be conflict-detected before install? (`edikt:init` scans CLAUDE.md + docs/guidelines, flags overlaps before writing any rule pack)
- What's the right default posture — install all, install none, install with explicit confirmation per pack?
- Should packs be opinionated defaults or principle-based suggestions that defer to project conventions?
- When a rule causes unexpected Claude behavior, how does the user trace it back to the rule pack? (transparency)
- Should `edikt:doctor` report active rule packs and their scope?

**Output:** Design decisions for Phase 7 implementation

## Phase 1e: Design Session — Installer Safety

**Problem:** `install.sh` silently overwrites files on reinstall — including any commands the user has customized in `~/.claude/commands/edikt/`. The `<!-- edikt:custom -->` protection exists in `edikt:upgrade` but not in `install.sh`. No safety guarantees are documented anywhere.

**Current behavior:**
- `curl -o` on each file → silent overwrite, no diff, no confirmation
- Does NOT touch `~/.claude/settings.json` (hooks are project-level via `edikt:init`)
- Does NOT touch files outside `~/.edikt/` and `~/.claude/commands/edikt/`
- Project-local install (`--project`) scopes everything to the current directory

**Key questions:**
- What guarantees should edikt make explicit? ("edikt never touches X, always preserves Y")
- Should reinstall check for customized commands (`<!-- edikt:custom -->`) before overwriting?
- Should install.sh have a `--force` flag for explicit overwrite vs. safe-by-default?
- Where should safety guarantees be documented — install.sh output, website, or both?

**Output:** Design decisions for Phase 8 implementation

## Phase 1f: Design Session — Dual-Audience Governance Docs

**Problem:** Governance documents serve two audiences with different needs. Humans need readable prose — context, rationale, nuance. LLMs need short, actionable directives — constraints, not explanations. Today, `/edikt:compile` guesses the directive from the prose. Teams write ADRs, invariants, and guidelines in whatever format they prefer — some are terse, some are verbose, some are structured, some aren't. Compile's extraction is lossy and inconsistent because it's inferring enforcement rules from human-oriented prose.

**The core insight:** The human writes for humans. The format doesn't matter — use MADR, lightweight ADRs, team templates, whatever works. But every governance document should also carry an explicit directive section that tells the LLM exactly what to enforce. The human owns the prose; edikt generates and maintains the directives. Compile reads directives, not prose. Review checks that they're aligned.

**The concept:** Every governance document (ADR, invariant, guideline) is dual-audience. A sentinel-delimited directive section lives inside the file alongside the human prose. The directive section is the contract between the document and the compiled governance output.

Example — ADR with team's preferred format:
```markdown
# ADR-007: Use cursor-based pagination

Date: 2026-03-27
Status: Accepted
Decision-makers: @daniel, @alice

## Why
Offset pagination breaks on large tables and is O(n) on most databases...

## Decision
All API endpoints returning collections must use cursor-based pagination...

## Consequences
- Frontend must store cursors, not page numbers
- Existing endpoints need migration path

[edikt:directives]: #
- All collection API endpoints MUST use cursor-based pagination. Never use offset pagination. (ref: ADR-007)
- Frontend clients MUST store opaque cursor tokens, not page numbers. (ref: ADR-007)
[edikt:directives-end]: #
```

Example — Invariant in terse style:
```markdown
# INV-003: No secrets in source

Never commit secrets, tokens, API keys, or passwords to source control.
Use environment variables or a secret manager.

[edikt:directives]: #
- NEVER commit secrets, tokens, API keys, or passwords to source control. Use environment variables or a secret manager. Violation is non-negotiable. (ref: INV-003)
[edikt:directives-end]: #
```

Example — Team guideline:
```markdown
# Error Handling Conventions

We use structured errors with error codes. All errors from our API
include a machine-readable `code` field and a human-readable `message`...

[edikt:directives]: #
- All API error responses MUST include a machine-readable `code` field and a human-readable `message` field. (ref: guidelines/error-handling.md)
- Error codes MUST be documented in the API contract. Do not return undocumented error codes. (ref: guidelines/error-handling.md)
[edikt:directives-end]: #
```

**Compile behavior — deterministic, never infers:**
1. For each governance doc with the right status (accepted ADR, active invariant, any guideline):
2. Check for `[edikt:directives]` sentinels
3. If present → read the directive section, compile into governance.md
4. If absent → **skip it**. Don't infer, don't guess. Surface it:
   ```
   ⚠ ADR-007 (accepted) has no directive section — skipped.
     Run /edikt:review-governance to generate directives for it.
   ```
5. Check for conflicts across all compiled directive sections before writing governance.md

Compile becomes a pure reader — it only compiles what's explicitly marked. No inference.

**Review-governance — the generative step:**
`/edikt:review-governance` is where prose becomes directives:
- Finds documents with the right status but no `[edikt:directives]` sentinels
- Reads the prose, generates candidate directives, asks the user to confirm
- Writes the sentinel section into the file
- Next compile run picks it up automatically

Also checks:
- Has the prose drifted from existing directives? (human edited the decision but didn't regenerate)
- Do directives conflict across documents?
- Are directives actionable and specific enough for enforcement?

**Creation commands generate directives on write:**
`/edikt:adr`, `/edikt:invariant`, and `/edikt:guideline` should generate the directive section when creating a new document. The user gets the dual-audience file from the start. But if they skip it or the document predates the feature, review-governance catches it.

**Directive scoring (review-governance):**
Review-governance scores prose and directives with different lenses:

| Audience | Scores on | Strong | Weak |
|----------|-----------|--------|------|
| Prose (human) | Clarity, context, rationale completeness | "Clear rationale, alternatives documented" | "Missing context — why was this chosen?" |
| Directives (LLM) | Enforceability, specificity, verifiability | "MUST use cursor pagination, never offset" | "Keep API design clean" |

Directive enforceability scale:
- **Strong:** MUST/NEVER/ALWAYS, references specific patterns or files, unambiguous, an LLM knows when it's violating this
- **Adequate:** SHOULD/PREFER, general concept but clear enough to follow
- **Weak:** Vague, aspirational, no concrete constraint — an LLM can't verify compliance

**Manual editing — the trust model:**
Directive sections are user-editable. The trust model:
- **Compile trusts what's in the sentinels.** It reads directives as-is, never second-guesses the user. It does structural validation only: empty sections, references to superseded ADRs, contradictions across documents.
- **Review-governance evaluates quality.** It detects prose-directive drift, scores enforceability, flags weak directives. But it never blocks — it reports and recommends.
- **The user decides.** They can accept review suggestions, edit manually, or override. edikt surfaces issues but the human has final say.

Compile structural checks (lightweight, every run):
- Directive section is empty → warn
- Directive references a superseded ADR → warn
- Two directives contradict each other → block (same as current contradiction check)
- Directive section present but document status is draft → skip (don't compile drafts)

Review-governance quality checks (on demand):
- Directive scored as weak → suggest strengthening
- Prose-directive drift detected → show diff, offer to regenerate
- Missing directive section → offer to generate

**Key questions:**
- What happens when an ADR is superseded? Clear the directive section, mark it inactive, or leave it for historical reference?
- What's the migration path for existing projects? `/edikt:review-governance --generate-all` flag?
- Should compile output which documents were compiled vs skipped? (yes — transparency matters)
- Should review-governance score prose and directives separately in the output, or a single combined score?

**Output:** Design decisions for Phase 9 implementation

## Phase 3: Codebase Pattern Learning

**The problem:** Claude hallucinates patterns, naming conventions, and structures instead of learning from the codebase. When asked to "write a new handler," Claude invents a structure rather than finding an existing handler and following it. When writing a PR description, Claude guesses the format rather than looking at recent merged PRs.

**The concept:** edikt scans the codebase during init and on-demand, identifies patterns, and compiles them into guidance Claude reads automatically. Examples:

- **PR template learning:** Scan recent merged PRs (via `gh pr list --state merged`), extract the structure, inject "When writing PRs, follow this pattern: [extracted template]" into compiled governance.
- **Code pattern learning:** "When creating a new HTTP handler, follow the pattern in `pkg/handler/orders.go`." Instead of rules that say "use hexagonal architecture," point Claude to a real file that demonstrates it.
- **Test pattern learning:** "Tests follow the pattern in `pkg/service/order_test.go` — table-driven tests with `testify/assert`."
- **Naming convention learning:** Scan existing files, infer naming patterns (snake_case files, camelCase functions), compile into a directive.

**What to design:**
- How does the scan work? (AST? grep? pattern matching?)
- Where do learned patterns get stored? (compiled into governance.md? separate file?)
- How does the user confirm/override learned patterns?
- How often does it re-scan? (on init only? on demand? each session?)
- How does it handle projects with inconsistent patterns?

## Phase 2: PRD-006

**Input:** Design decisions from Phase 1
**Output:** PRD-006 with numbered requirements and acceptance criteria
**Process:** `/edikt:prd`

## Phase 4: Agent Governance Fields

Add `maxTurns:` and `disallowedTools:` to all 18 agent templates. Values require design session (Phase 1b) to determine appropriate limits per agent type.

## Phase 5: New Hooks

Evaluate and implement: StopFailure, SessionEnd, SubagentStart, TaskCompleted, ConfigChange.
Highest value: SessionEnd (auto-session-sweep).

## Phase 6: Agent Migration

Search all templates for `resume` parameter usage. Replace with `SendMessage` pattern.

## Phase 7: Rule Pack UX

Implementation of Phase 1d design decisions. Likely includes:
- Conflict detection scan in `edikt:init` before installing rule packs
- Changes to install posture (opt-in vs. default, per-pack confirmation)
- Transparency improvements (doctor reports active packs + their scope)
- Possibly: rule pack content audit (more principle-based, less opinionated)

## Phase 8: Installer Safety

Implementation of Phase 1e design decisions. Likely includes:
- Check for `<!-- edikt:custom -->` marker before overwriting commands on reinstall
- Explicit safety guarantees documented in install output and website
- Possibly: `--force` flag for explicit overwrite behavior

## Phase 9: Dual-Audience Governance Docs

Implementation of Phase 1f design decisions. Likely includes:
- Directive sentinel support in `/edikt:adr`, `/edikt:invariant`, and new `/edikt:guideline` command
- `/edikt:compile` reads `[edikt:directives]` sections when present, warns on missing
- `/edikt:compile --generate-directives` to add directive sections to existing docs
- `/edikt:review-governance` detects prose-directive drift and cross-document conflicts
- Backward compat: documents without sentinels still compile via prose inference
- Superseded ADRs: directive section cleared or marked inactive

## Phase 10: Harness Design Audit

**The concept:** `/edikt:harness` audits the completeness and assumptions of the user's governance harness — not just "is it configured correctly" (that's `/edikt:doctor`) but "is it configured enough, and do you know why each piece exists?"

**Architecture frame (from Anthropic's harness design article):**
Every harness component encodes an assumption about what the model can't do on its own. Those assumptions should be visible, testable, and revisited as models improve. `/edikt:harness` should surface them explicitly:

| Component | Assumption |
|-----------|------------|
| PostCompact hook | Claude loses plan context after compaction without it |
| Quality gates | Claude misses critical findings without a blocking reviewer |
| Governance compiled | Claude won't honor ADRs it can't read |
| Specialist agents | Claude self-evaluates with bias — external evaluators are more accurate |
| Signal detection | Claude won't proactively capture architecture decisions mid-session |

**Generator-Evaluator separation guidance:** Most edikt commands have Claude generate AND evaluate its own output. The harness guide should teach explicit generator-evaluator separation — when to invoke a specialist agent as evaluator, when to use a second Claude turn, and what "sprint contracts" look like in the edikt execution model (Completion Promises surfaced before execution, not just stored in the plan file).

**Context management guidance:** Two strategies, different use cases:
- Compaction recovery (PostCompact hook) — for sessions that run long and compact naturally
- Intentional context reset — for tasks where coherence requires a clean window. Preserve state in the plan progress table, then start a fresh session. The article found this more effective than compaction for long tasks ("context anxiety" on Sonnet 4.5).

**What it checks:**
- Rules installed but no compiled governance? → "Run `/edikt:compile` to turn your ADRs into enforcement"
- Agents installed but no quality gates? → "Consider adding `gates: [security]` to block on critical findings"
- Hooks configured but no compaction recovery? → "Add PostCompact hook to survive context compaction"
- No signal detection? → "Enable stop hook to detect architecture decisions mid-session"
- Rules but no agent scoping? → "Consider restricting DBA to migration files only"
- Governance compiled but never verified? → "Run `/edikt:drift` to check compliance"
- No generator-evaluator separation? → "Your harness has no external evaluator — Claude is grading its own work"

**Output:** A harness maturity assessment with assumptions surfaced per component, actionable recommendations, and upgrade paths. Not pass/fail — guides users from basic setup to full harness design.

**Content:** "Designing your agent harness with edikt" guide on edikt.dev — positions edikt in the harness design conversation Anthropic just opened.

## Phase 11: EXP-003

**Brief:** `experiments/exp-003-real-world-compliance/BRIEF.md`
**Scope:** ~48 runs testing multi-rule, compaction, multi-turn, real conventions.
**Output:** EXP-003 write-up on website using experiment template.

## Phase 12: Website + Docs

Update website for v0.2.0 changes. CHANGELOG entry. Version bump.

---

## Dependencies

- Phase 1 design sessions block Phase 2 (PRD)
- Phase 2 (PRD) blocks Phases 3-10
- Phase 1d (rule pack design) can feed Phase 7 directly without waiting for PRD
- Phase 1e (installer safety) can feed Phase 8 directly without waiting for PRD
- Phase 1f (dual-audience docs) can feed Phase 9 directly without waiting for PRD
- Phase 11 (EXP-003) is independent — can run anytime
- Phase 12 (docs) depends on all other phases completing
