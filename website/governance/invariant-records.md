# Invariant Records

**Invariant Records** (short form `INV`) are edikt's formal artifact type for documenting hard architectural constraints — rules that must hold continuously, independent of any single decision. They're the enforcement counterpart to Architecture Decision Records (ADRs).

edikt formalizes architectural invariants as a governance artifact with a committed template, compile pipeline, and enforcement integration.

## Why formalize invariants?

ADRs have been formalized since [Michael Nygard's 2011 post](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions). The concept of architectural *invariants* has existed in computer science since at least [Hoare logic](https://en.wikipedia.org/wiki/Hoare_logic) in the 1960s — conditions that must hold before and after every operation. But the *documentation format* for architectural invariants was never standardized. Every team that documents invariants invents their own format.

edikt formalizes them: a committed template contract, a committed lifecycle, and compile pipeline integration that turns invariants into directives the model follows automatically.

## How Invariant Records differ from ADRs

| Aspect | ADR | Invariant Record |
|---|---|---|
| **Artifact type** | Historical record of a decision | Living rule that must remain true |
| **Written when** | A decision is made (one-time) | A constraint needs to be enforced |
| **Alternatives considered** | Yes, central to the format | No — invariants don't have alternatives |
| **Level of abstraction** | Can be implementation-specific | Must be constraint-level, implementation-agnostic |
| **Typical source** | A team discussion or design review | Regulation, incident, foundational principle, cross-cutting architectural concern |
| **Cross-cutting** | Usually not (narrow decision) | Yes — applies to many code paths |
| **Revision** | Immutable once accepted. Supersede via new ADR. | Content immutable. Status can change (Proposed → Active → Superseded/Retired). |
| **Relationship** | ADRs document *why* a decision was made. | Invariant Records document *what must remain true* as a consequence. |

Most invariants are NOT derived from ADRs. They're cross-cutting architectural principles, regulatory constraints, or foundational product rules that exist independent of any specific decision.

## The template

The Invariant Record template draws from the "constraint, not implementation" principle — the Statement describes what must be true, not how to achieve it. The template is customizable via `.edikt/templates/invariant.md` (see [Extensibility](extensibility)).

Every Invariant Record has six body sections (two optional). Compiled directives live in a co-located sidecar (`<INV>.edikt.yaml`); the prose body carries no in-body directives block in v0.6.0+.

```markdown
# INV-NNN: Short declarative title

**Date:** YYYY-MM-DD
**Status:** Active

## Statement

<One declarative sentence, present tense, stating the constraint.
No qualifications, no hedging.>

## Rationale

<Why this constraint exists. Regulatory requirement, lesson from
an incident, foundational architectural principle. Implementation-
agnostic.>

## Consequences of violation

<What specifically goes wrong when this is broken? Be concrete.>

## Implementation (optional but strongly encouraged)

<Concrete patterns that satisfy this invariant in the current stack.>

## Anti-patterns (optional but strongly encouraged)

<Concrete examples of violations and why they're wrong.>

## Enforcement

<At least one mechanism for catching violations. An invariant without
enforcement is a wish.>
```

**Four lifecycle states:**

- **Active** — currently enforced (the normal state)
- **Proposed** — under team discussion, not yet enforced
- **Superseded by INV-NNN** — replaced by a newer invariant
- **Retired (reason)** — no longer relevant, not replaced

## The constraint-not-implementation principle

The single most important rule for writing Invariant Records: **describe the constraint, not the implementation.**

**Test:** *"If our tech stack changed tomorrow, would this rule still apply?"* If yes, you're at the right level. If no, abstract up — the implementation belongs in an ADR.

```
❌ "Use UUIDv7 for primary keys"
✅ "Primary key identifiers are time-orderable"
```

UUIDv7 is today's implementation choice. The underlying constraint (time-orderability) is stable across tech changes. When UUIDv8 or a better ID scheme emerges, the invariant is unchanged — only the implementation ADR updates.

## How they compile into governance

Every Invariant Record has a co-located `<INV>.edikt.yaml` sidecar that holds the compiled directives. `/edikt:invariant:compile` regenerates that sidecar in a fresh subagent context with a locked extraction prompt — edikt never writes to the prose `.md`. The sidecar's `directives[]` entries come from the Statement and Enforcement sections, use MUST/NEVER language with literal code tokens, and get "No exceptions." appended automatically when the source uses absolute quantifiers ("every", "all", "total").

The sidecar also carries `manual_directives:` (user-added rules the extractor missed) and `suppressed_directives:` (auto entries you rejected). See [Sidecar Architecture](sidecar) for the full schema.

`/edikt:gov:compile` reads every sidecar and renders it to one of two places, depending on whether the invariant is scoped to specific files (`compile_schema_version: 3`, ADR-059):

- **Pathless invariants** — an invariant whose sidecar declares no `paths:` at all (it applies everywhere, unconditionally) renders into the **ambient core**, `.claude/rules/governance.md`. This file loads on every single edit. In edikt's own tree, as one worked example, it currently carries exactly one such constraint — the ADR-immutability rule (ref: INV-002) — followed by a one-line topic index; a project with no pathless invariants at all would have an empty ambient core save for that index. It's stated exactly once; the old model's practice of restating invariants a second time at the bottom of `governance.md` for recency bias was dropped with nothing replacing it.
- **Path-scoped invariant directives** — an invariant sidecar that does declare `paths:` (most invariants that only bind specific file types or directories) contributes its directive text to `directive-index.yaml`, a glob-keyed YAML file that is `bin/edikt hook match`'s exclusive input. Per ADR-060, this is what drives write-time enforcement, split by the directive's pinned grade: a `must`-grade invariant blocks the write with a synchronous PreToolUse deny naming the directive; an `advisory`-grade one is delivered as PostToolUse `additionalContext` after the write already happened.

Per ADR-066's single-home rule, a directive renders in exactly one of those two places, never both — `directive-index.yaml` fires precisely on the touched file via the write-time hook, while the ambient core is Claude Code's passive, always-loaded surface, so duplicating the same text into both added token cost without adding coverage. The topic file a path-scoped invariant's `topic:` groups it under (`.claude/rules/governance/<topic>.md`) still loads automatically via its `paths:` frontmatter when a matching file is touched, and still carries that topic's `Reminders` and `Verification Checklist` sections — but the invariant's own directive/prohibition text lives in `directive-index.yaml`, not in the topic file's compiled-directives region, which is empty for any topic whose contributing sidecars are all `paths:`-scoped. Hand-authored `ManualDirectives` are the exception: they're never extracted, have no other delivery channel, and always render into the topic file regardless of `paths:`.

## Why directive language matters

Experiments showed that *how* the directive is phrased changes whether the model follows it. The compile pipeline produces directives optimized for LLM compliance:

```
Prose (low compliance):
  "Log calls should include the tenant identifier"

Compiled directive (high compliance):
  "Every slog.Error call MUST include "tenant_id", tid. No exceptions. (ref: INV-012)"
```

The difference: literal code tokens (`slog.Error`, `"tenant_id"`, `tid`), MUST/NEVER language, and "No exceptions." reinforcement. Pre-registered experiments on Claude Opus 4.6 confirmed that the compiled format prevents violations the prose format misses — particularly on greenfield code and new domains where there are no existing patterns to copy.

Use `/edikt:gov:score` to measure how well your governance follows these patterns.

## Next steps

- **Read the canonical examples:** [tenant isolation](writing-invariants.md#example-1-tenant-isolation-is-total) and [money precision](writing-invariants.md#example-2-monetary-values-are-fixed-point-never-floating-point). Two worked examples covering different failure axes (security/isolation vs data correctness).
- **Read the writing guide:** [Writing Invariant Records](writing-invariants.md) — five qualities, seven traps, six bad-to-good rewrites, seven-question self-test, annotated canonical examples.
- **Create your first one:** `/edikt:invariant:new "your constraint here"` after running `/edikt:init` to set up templates.
