# PROPOSAL-001: Project Adaptation & Quality Measurement

**Status:** Part 1 (v0.3.0) decisions locked in. ADR-008 written. Spec artifacts created. Ready for implementation. Part 2 (v0.4.0) design still open.
**Date:** 2026-04-09
**Last updated:** 2026-04-09 (Q1–Q6 resolved, three-list schema + hash caching fully designed, symmetric application across ADR/invariant/guideline confirmed, guideline:compile gap identified, spec directory created)
**Scope:** Two related feature areas that together define v0.3.0 and v0.4.0 themes

> **Implementing v0.3.0?** Do not read only this proposal document. The **machine-readable spec artifacts** live in [`PROPOSAL-001-spec/`](PROPOSAL-001-spec/) and they are the source of truth for the schema, hash algorithm, fixtures, and anti-patterns. This doc explains *why*; the spec directory explains *exactly what*. Both are required reading before writing code.
>
> The formal contract for the three-list schema + hash caching is in [ADR-008](../decisions/ADR-008-deterministic-compile-and-three-list-schema.md). It supersedes any ambiguity in this proposal.

This document captures a design discussion about two linked proposals that emerged from dogfooding edikt in a real project and hitting concrete quality problems with `/edikt:adr:new`. Part 1 is fully designed and ready to implement. Part 2 is scoped but has open questions. This doc exists so we don't lose the thinking and can come back to open questions cold.

**The two proposals:**
1. **Project adaptation** (v0.3.0 theme) — edikt detects existing artifact conventions, generates project templates matching them, and lets users take full control of the generated directive blocks across all compiled artifacts (ADRs, invariants, guidelines)
2. **Quality measurement** (v0.4.0 theme) — LLM-as-judge test suite that catches *outcome* regressions the current contract tests can't see

They're separated because they solve different problems (generation vs evaluation), but they're tightly linked: v0.3.0 introduces project templates + deterministic compile that v0.4.0 then measures the quality of.

---

## Part 1: Project Adaptation (v0.3.0)

### Context — the bug that surfaced this

A user reported hitting the following failure mode with `/edikt:adr:new`:

> The skill uses a rigid template (YAML frontmatter, Decision Drivers, Pros/Cons/Rejected, Confirmation, footer) regardless of project conventions. When existing ADRs use a different style, the output doesn't match and the user has to rewrite.

Same failure mode applies to `/edikt:invariant:new` and `/edikt:guideline:new` — template rigidity regardless of project style. And to `/edikt:init` — after scanning existing docs, edikt doesn't offer to adapt its templates to match the project's conventions; it just ignores them.

A second, deeper concern surfaced during design: `/edikt:adr:compile` (and its siblings) auto-generate directive blocks from the artifact body, but users need a way to **add** rules Claude missed, **suppress** rules Claude generated wrong, and **preserve** their edits across regenerations. Without that, the compile command is either destructive (user loses work) or untrustable (user stops using it).

### The core principle

**edikt should adapt to the project, not the other way around.**

We already do this for *paths* — v0.2.1 added Adopt/Migrate/Skip prompts when `/edikt:init` detects ADRs in a non-default folder. The logical next step is to do the same for *style* and *directive ownership*: when edikt detects existing artifacts with a consistent structure, it should offer to adapt its template to match, and users should be able to override any directive without losing that work on subsequent compiles.

### The "edikt only enforces sentinels" principle

edikt's **only** hard requirement on compiled artifacts is that they contain the directives sentinel block:

```markdown
[edikt:directives:start]: #
...
[edikt:directives:end]: #
```

Everything else — the human-facing structure, prose style, metadata, section names — is the user's concern. edikt provides default templates as a starting point ("goodies"), but does not enforce or assume any particular shape. **The sentinel block is the only machine-readable contract.**

This principle drives the entire Part 1 design. The template lookup chain, Adapt mode, migration decisions, and directive schema all flow from it.

### Scope: all three compiled artifact types

Everything in Part 1 — template adaptation, the directives schema, hash-based caching, interview flow, gov/compile merging — applies **symmetrically** to all three compiled artifact types:

| Artifact | New command | Compile command | Project template path |
|---|---|---|---|
| ADR | `/edikt:adr:new` | `/edikt:adr:compile` | `.edikt/templates/adr.md` |
| Invariant | `/edikt:invariant:new` | `/edikt:invariant:compile` | `.edikt/templates/invariant.md` |
| Guideline | `/edikt:guideline:new` | `/edikt:guideline:compile` ⚠ | `.edikt/templates/guideline.md` |

**Gap identified during design**: `/edikt:guideline:compile` does **not** currently exist. `commands/guideline/` has only `new.md` and `review.md`, while `adr/` and `invariant/` both have `compile.md`. **v0.3.0 adds the missing command** for parity. Guidelines become first-class governance sources — their directive blocks are read by `gov:compile` alongside ADRs and invariants.

Throughout this proposal, "compiled artifact" means ADR, invariant, or guideline. Examples may use ADRs for concreteness, but the machinery is artifact-agnostic.

### Init flow — three template prompts, not one

`/edikt:init` does not assume a default style for any artifact type. It asks for each one during onboarding.

**If existing artifacts are found (consistent style):**

```
Found 9 existing ADRs in docs/decisions/. They have a consistent style.
How should edikt handle them?

  [1] Adapt     — generate a project template from your existing style (recommended)
  [2] Start fresh — ignore existing style, pick one of the reference templates below
  [3] Write my own — I'll create .edikt/templates/adr.md manually
```

**If existing artifacts are inconsistent:**

```
Found 9 ADRs in docs/decisions/ with mixed styles (5 MADR, 4 Nygard).

Do you already have a team template? [y/n]:
```

If yes, the user points edikt at the team template and it's copied into `.edikt/templates/adr.md`.
If no, edikt offers:

```
  [1] Draft a template from the majority style (5 MADR ADRs) for your review
  [2] Pick from reference templates (Nygard-minimal or MADR-extended)
```

**If no existing artifacts (greenfield):**

```
No existing ADRs found. Pick a starting point for your project template:

  [1] Nygard-minimal — Title, Status, Context, Decision, Consequences (4 sections)
  [2] MADR-extended  — adds Decision Drivers, Alternatives, Consequences breakdown
  [3] Write my own   — I'll create .edikt/templates/adr.md manually
```

**The same three-part flow runs for invariants and guidelines.** Init produces up to three project templates per project.

Nygard and MADR exist as **reference examples** shipped in `~/.edikt/templates/examples/`, but they're not "defaults" — they're starting points init offers when the user picks "Start fresh". The user always explicitly chooses.

### Adapt mode — structural inference only

Adapt reads 2-3 existing artifacts and extracts the **structural** pattern to generate `.edikt/templates/<artifact>.md`. Structural means:

- Presence/absence of YAML frontmatter and which fields exist
- Heading levels (title as `#` or `##`, sections as `##` or `###`)
- Section names and their order
- Per-section format (bulleted list vs paragraph prose)
- Whether footers/references sections exist

**Structural is deterministic.** Read files, diff structure, emit the common skeleton. No LLM inference required.

**What Adapt does NOT infer:**

- **Voice** — narrative vs punchy, formal vs casual, first-person vs passive. This is how the writer fills in the scaffolding. Not something a template file can encode. The user writes in their own voice or notes preferences in a template comment.
- **Content quality** — whether the Decision section actually contains architectural detail. Handled by Part 2 (Tier 2 quality tests), not by templates.
- **Semantic conventions** — "this team always cites JIRA tickets" or "this team uses Why-Not sections for rejected alternatives". These require LLM inference and are unreliable to auto-detect. Users add them to their template manually.

**Three-layer separation:**

| Layer | Owner | Mechanism |
|---|---|---|
| Structure | Adapt mode | Deterministic pattern extraction |
| Voice | User | Template comments + their own writing |
| Content quality | Part 2 Tier 2 tests | LLM-judge scoring |

### Template lookup chain

`/edikt:<artifact>:new` finds its template via this precedence:

1. **Project override** — `.edikt/templates/<artifact>.md` (if it exists)
2. **Global reference** — `~/.edikt/templates/examples/<artifact>-*.md` (only when explicitly requested via init)
3. **Inline fallback** — a minimal stub defined inside `commands/<artifact>/new.md` for bootstrap scenarios

**No global default is auto-loaded.** `install.sh` does not ship an always-active `adr-default.md`. Reference templates live under `examples/` and are only copied into a project when the user picks them during init. Projects without an explicit choice get the hard refusal described below.

### Template-less refusal

If `/edikt:<artifact>:new` runs in a project that has no `.edikt/templates/<artifact>.md`, it **refuses with a clear error** pointing to init:

```
❌ No project template for ADR found.

edikt doesn't assume a style — your project owns this.
Run /edikt:init to set up templates, or create .edikt/templates/adr.md manually.
```

This is option (a) from the Q2 edge case discussion. `/edikt:<artifact>:new` is not a new-user-experience flow — init is. This prevents surprises where a user generates an ADR in edikt's guessed style and commits it before realizing they wanted something else.

### The three-list directive schema

Every compiled artifact's directives sentinel block contains **three declarative lists**:

```yaml
[edikt:directives:start]: #
source_hash: abc123def456        # hash of artifact body (excluding this block)
directives_hash: ghi789jkl012    # hash of the auto `directives:` list
compiler_version: 0.3.0          # edikt version that generated this block
paths:
  - "**/*.go"
  - "**/adapters/postgres/**"
scope:
  - implementation
directives:
  # Auto-generated from the artifact body by /edikt:<artifact>:compile.
  # Regenerated when you edit the body and re-run compile.
  - "Always use transactions for multi-table writes (ref: ADR-007)"
  - "Prefer RETURNING over SELECT after INSERT (ref: ADR-007)"
manual_directives:
  # User-authored. Preserved across regenerations.
  # Use for rules compile missed or couldn't infer.
  - "In tests, use a real postgres container, not a mock (ref: ADR-007)"
suppressed_directives:
  # Auto directives the user rejected.
  # Filtered out of the final enforced rule set at gov:compile time.
  - "Always use read replicas for reporting queries"
[edikt:directives:end]: #
```

**The three lists answer three different user intents:**

| User intent | List |
|---|---|
| "Claude missed a rule I want" | Add to `manual_directives:` |
| "Claude generated a rule that's wrong and I never want it" | Add to `suppressed_directives:` |
| "Claude generated a rule but I want it worded differently" | Add original to `suppressed_directives:` + reworded version to `manual_directives:` |
| "The artifact body needs updating to match reality" | Edit the body, re-run compile |

**The schema is backward compatible.** `manual_directives:` and `suppressed_directives:` are optional; absent means empty. Existing artifacts from v0.2.x continue to work without migration.

### Responsibility split: adr/compile vs gov/compile

The three lists are processed by different commands at different times:

| Command | Reads | Writes | Purpose |
|---|---|---|---|
| `/edikt:<artifact>:compile` | Artifact body, stored hashes, `directives:` list | `directives:` list, hashes | Generate auto directives from body. Cached via hashes. |
| `/edikt:gov:compile` | All three lists from every artifact | `.claude/rules/governance.md` + topic files | Merge all sources into enforced governance. Always fresh. |

**Key insight**: `<artifact>:compile` handles **generation** (expensive Claude call). `gov:compile` handles **application** (cheap string merging). Suppression and manual additions are application-time concerns — they don't trigger `<artifact>:compile` to re-run. They flow directly into `gov:compile` output on its next run.

**`gov:compile` behavior per artifact:**

1. Open the artifact file
2. Extract the full directives block via sentinels
3. Read all three lists
4. Compute `effective = (directives − suppressed_directives) ∪ manual_directives`
5. Merge into the topic file(s) the artifact routes to
6. De-dupe across artifacts (silent at merge time)
7. Detect contradictions (warn)
8. **Never writes back to the artifact file** — artifacts are read-only inputs

This means user edits to `manual_directives:` or `suppressed_directives:` are picked up **automatically on the next `gov:compile` run**, without needing `<artifact>:compile` to run first.

### Hash-based caching for determinism

**The problem**: Claude is stochastic. Running `<artifact>:compile` twice on the same body produces slightly different wording every time. Without determinism, exact-match suppression breaks, hand-edit detection fires false positives, and git shows churn in every artifact file even when nothing conceptually changed.

**The fix**: hash the artifact body, skip regeneration when the body hasn't changed. Claude is never re-invoked on unchanged inputs, so the output is stable by not being recomputed.

**Two hashes in the block:**

- **`source_hash`** — hash of the artifact body with the directives block excluded. Answers: "has the body changed since we last generated?"
- **`directives_hash`** — hash of the canonicalized auto `directives:` list. Answers: "has the user hand-edited Claude's output since we wrote it?"

Plus a **`compiler_version`** field to handle algorithm drift across edikt versions.

**Compile algorithm:**

```
1. Read artifact file
2. Compute current_source_hash = sha256(body, excluding directives block)
3. Compute current_directives_hash = sha256(canonical directives: list)
4. Read stored hashes from the block

5. Branch:

   (a) current_source_hash == stored_source_hash
       AND current_directives_hash == stored_directives_hash
       → FAST PATH: exit "up to date", no Claude call, no writes

   (b) current_source_hash != stored_source_hash
       → body changed
       → run Claude to regenerate directives from new body
       → write new directives: list
       → compute new hashes from new state
       → write both hashes to the block

   (c) current_source_hash == stored_source_hash
       AND current_directives_hash != stored_directives_hash
       → user hand-edited directives (body is stable)
       → run interview (see below)
       → after interview, compute new_directives_hash
       → update directives_hash only (source_hash unchanged)
```

**Hash computation:**

```bash
# source_hash — body with directives block excluded, normalized
awk '
  /^\[edikt:directives:start\]/ { skip=1; next }
  /^\[edikt:directives:end\]/   { skip=0; next }
  !skip
' <file> | \
  tr -d '\r' | \
  sed 's/[[:space:]]*$//' | \
  shasum -a 256 | \
  awk '{print $1}'
```

Normalization:
1. Remove the entire `[edikt:directives:start]...[edikt:directives:end]` block (inclusive)
2. Strip `\r` (normalize CRLF → LF)
3. Strip trailing whitespace per line
4. Leave everything else literal

**`directives_hash`** is computed as `sha256(join(directives list, '\n'))` over the YAML list items themselves (not the raw YAML text — that would be sensitive to indentation differences). Only the auto `directives:` list is hashed. `manual_directives:` and `suppressed_directives:` are NOT hashed — they're declarative user state that can change freely without triggering compile.

**`compiler_version` handles edikt upgrades.** If a future edikt version changes the hash algorithm or compile logic, existing blocks carry their `compiler_version` as a signal. Default behavior on version drift: warn, don't auto-regenerate. User explicitly opts in with `/edikt:<artifact>:compile --regenerate`.

### Hand-edit interview

When `<artifact>:compile` detects hand-edits (case c above), it runs an interactive interview **one question per detected edit** and asks the user to choose what happens.

**Interview flow:**

```
🔍 /edikt:adr:compile ADR-007

Reviewing directives: against current ADR body...
Found 2 lines that don't match auto-generated output.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
(1/2) Hand-added line in directives:

    "Always use JSON for serialization"

This line is in directives: but wouldn't be generated from the current
body. What should I do?

  [1] Move to manual_directives: (keep as user-authored rule)
  [2] Add to suppressed_directives: (pick this only if compile had
      generated this before and you want to prevent regeneration)
  [3] Delete entirely
  [4] Edit artifact body first so this becomes an auto directive
  [5] Skip for now (leave in directives: — will be overwritten next compile)

Choice [1]:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
(2/2) Deleted auto directive:

    "Prefer RETURNING over SELECT after INSERT"

Compile still wants to regenerate this from the body. What should I do?

  [1] Add to suppressed_directives: (keep deleted — compile filters it out)
  [2] Let compile regenerate (deletion was accidental)
  [3] Edit artifact body first to remove the source of this directive

Choice [1]:
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Applied:
  + manual_directives: "Always use JSON for serialization"
  + suppressed_directives: "Prefer RETURNING over SELECT after INSERT"
  ✓ directives: regenerated (2 lines)
```

**No silent auto-resolution.** Every detected edit gets an explicit user choice. Claude waits for each response before moving on. This matches edikt's pattern of "ask before destructive actions" and prevents the user from losing hand-edits they don't remember making.

**Headless mode fails loud:**

```
❌ /edikt:adr:compile ADR-007

Cannot run in headless mode: hand-edited lines detected requiring
user intervention.

  + "Always use JSON for serialization"
  - "Prefer RETURNING over SELECT after INSERT"

Options:
  1. Run interactively: /edikt:adr:compile ADR-007
  2. Override (destructive):  --strategy=regenerate  (discard hand-edits)
  3. Override (preserve):     --strategy=preserve    (leave file unchanged)

Exiting with error (2).
```

Two explicit `--strategy=` flags for automation. No silent defaults. CI breaks loud.

### Auto-chain from new → compile

`/edikt:<artifact>:new` automatically runs `/edikt:<artifact>:compile <ID>` at the end of its workflow. The sequence:

1. User runs `/edikt:adr:new "Use Redis for session cache"`
2. Claude generates the ADR body from the argument + interview
3. Writes the file with empty directive block stub
4. **Auto-chains**: runs `/edikt:adr:compile ADR-NNN` immediately
5. Populates the `directives:` list, writes both hashes
6. Returns with the fully-populated ADR

**This is safe** because on a brand new artifact there's nothing to preserve — no manual_directives, no suppressed_directives, no hand-edits. The interview never fires on first creation. Users never have to remember to run compile after new.

### Argument-aware sourcing — flexible prose input

`/edikt:<artifact>:new` treats its argument as **prose first, then mines for embedded references**:

1. **Empty argument** → infer from conversation context
2. **Non-empty argument** → treat as prose, scan for:
   - File paths matching existing files (`docs/specs/*.md`)
   - Identifiers that resolve (`SPEC-042`, `PRD-007`)
   - Branch names that exist (`feature/redis-cache`)
3. **For each extracted reference**, read the content into the source pool
4. **Use the full prose as framing** (scope, decision context)
5. **Interview fills gaps** — sections not covered by prose or extracted sources

**Examples:**

| Input | Behavior |
|---|---|
| `/edikt:adr:new` (empty) | Infer from conversation — what decision was just discussed? |
| `/edikt:adr:new "Use Redis for session cache"` | Pure prose, no refs, treat as title + interview |
| `/edikt:adr:new "Decide session cache strategy using docs/specs/cache.md"` | Prose with embedded path ref, read the spec as primary source |
| `/edikt:adr:new docs/specs/redis-cache.md` | Prose that IS a path, read file as primary source |
| `/edikt:adr:new SPEC-042` | Prose that IS an identifier, resolve and read |
| `/edikt:adr:new feature/redis-cache` | Prose that IS a branch, read branch content |

**This pattern already exists** in `/edikt:sdlc:plan` as of v0.1.3 ("flexible plan input"). v0.3.0 extends it to all three artifact types: `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new`.

**Implementation-wise, this is simpler than rigid type dispatch** — Claude is good at "scan this text for filenames and identifiers", worse at "classify this string into six categories".

### Where templates live — `.edikt/templates/`

Three reasons this is the right location (already specified in ADR-005):

1. **ADR-005 specifies the mechanism**: "Project templates in `.edikt/templates/` take precedence over edikt defaults. Lookup order: project override → edikt default." We documented this in v0.1.x but never wired it up for compiled artifact creation. v0.3.0 is the implementation.

2. **Templates must be versioned with the project**, not regenerated per-session. Team members need the same template so their artifacts look consistent without coordination. Git-tracked `.edikt/templates/*.md` is the only way.

3. **Generated templates are transparent artifacts.** The user can open them, see what Adapt inferred, and edit manually if the inference was wrong. Much better UX than "edikt learned your style, trust me".

### Phased implementation

**Phase 1 — Extensibility plumbing + guideline:compile**
- Add `commands/guideline/compile.md` (new command, mirrors `adr/compile.md` and `invariant/compile.md`)
- Extract inline templates from `commands/adr/new.md`, `commands/invariant/new.md`, `commands/guideline/new.md` into standalone reference files under `templates/examples/`
- Ship reference templates via `install.sh` to `~/.edikt/templates/examples/`
- Update `commands/*/new.md` to follow the lookup chain (project → inline fallback; no global default unless explicitly selected during init)
- Template-less refusal logic in all three `new.md` files
- No user-visible change for existing projects yet (they already have templates implied via inline)

**Phase 2 — Three-list schema + hash-based caching**
- Update all three `<artifact>:compile.md` commands with the three-list schema
- Hash computation algorithm in all three compile commands
- Fast path (hash match → skip)
- Slow path (body change → regenerate + update hashes)
- Hand-edit interview flow
- Headless mode `--strategy=` overrides
- Backward compat with legacy blocks (missing `manual_directives:`/`suppressed_directives:`)
- Update `gov:compile.md` to read all three lists from all three artifact types
- De-dup logic at gov:compile merge time
- Contradiction detection extended to within-artifact auto/manual/suppressed
- **Write ADR-008: Deterministic compile via content hashing and three-list directive schema**

**Phase 3 — Init style detection + Adapt mode**
- `/edikt:init` scans existing artifacts (three separate scans: ADRs, invariants, guidelines)
- Structural analysis: frontmatter, section names, heading levels, list-vs-paragraph format
- Three-choice prompt per artifact type (Adapt / Start fresh / Write my own)
- Adapt: generate `.edikt/templates/<artifact>.md` from detected pattern + always-present sentinel block
- "Start fresh" branch offers reference templates from `~/.edikt/templates/examples/`
- Inconsistent style fallback (ask if user has team template; if not, draft from majority)
- Grandfather flow for projects upgrading from v0.2.x

**Phase 4 — Flexible prose input**
- Update all three `<artifact>:new.md` with the prose-first dispatch logic
- Reference extraction: scan for paths, identifiers, branches
- Mixed input handling (prose with embedded refs)
- Empty input → conversation context fallback

**Phase 5 — doctor + upgrade integration**
- `doctor.md` reports project template state for all three artifact types
- `doctor.md` distinguishes "project template present / using edikt default / missing (post-v0.2.x)"
- `upgrade.md` never rewrites files under `.edikt/templates/`
- `upgrade.md` warns when templates are missing post-v0.2.x (graceful grandfathering)
- `upgrade.md` handles `compile_schema_version` drift (from v0.2.3 ADR-007)

**Phase 6 — Invariant Record story + experiments** *(added during Part 2 design)*
- Ship the canonical Invariant Record examples: tenant isolation and money precision
- Ship the writing guide (5 qualities, 7 traps, 6 rewrites, 7-question self-test, annotated examples)
- Website pages for governance/invariant-records, governance/writing-invariants, and the two annotated canonical examples
- Experiment infrastructure under `test/experiments/` with 3 pre-registered experiments
- Write ADR-009: Invariant Record terminology (coins the term "Invariant Record", short form `INV`)
- All artifacts follow the final template locked in during Part 2 design

## The Invariant Record coinage

Part of v0.3.0's story is that edikt formalizes a previously-unnamed artifact type: the **Invariant Record**. See [ADR-009](../decisions/ADR-009-invariant-record-terminology.md) for the full coinage rationale. Short version:

- **Architecture Decision Records (ADRs)** are a well-established term from Michael Nygard (2011), widely adopted, cite-able.
- **Invariants** as a concept are well-understood but have no standardized *documentation format*. Teams invent their own every time.
- **edikt coins "Invariant Record"** (short form `INV`, file naming `INV-NNN-title.md`) as a parallel artifact type with a committed template contract.
- **The term is honestly labeled as an edikt convention**, not imported from an external standard. We're formalizing something the industry has left ad-hoc, and we're transparent about it.
- **Guidelines do NOT receive a coined term** — "guideline" is already established English and doesn't need formalization.

The template, writing guide, and canonical examples all live in [`PROPOSAL-001-spec/`](PROPOSAL-001-spec/). Specifically:

- [`invariant-record-template.md`](PROPOSAL-001-spec/invariant-record-template.md) — the authoritative template (6 body sections, 4 lifecycle states)
- [`writing-invariants-guide.md`](PROPOSAL-001-spec/writing-invariants-guide.md) — the full writing guide
- [`canonical-examples/tenant-isolation.md`](PROPOSAL-001-spec/canonical-examples/tenant-isolation.md) — worked example 1
- [`canonical-examples/money-precision.md`](PROPOSAL-001-spec/canonical-examples/money-precision.md) — worked example 2

**The template principle that drives everything**: describe the **constraint**, not the **implementation**. An invariant that survives a tech stack change is at the right level of abstraction. An invariant tied to a specific library or version is an implementation detail that belongs in an ADR.

## Experiments for validation and iteration

Part 2 of the original proposal was going to defer quality measurement to v0.4.0. During design discussion, we decided to bundle a minimal version into v0.3.0 instead. The reason: v0.3.0 introduces claims about Claude behavior (invariants help Claude follow cross-cutting rules) that should be validated empirically before we build a narrative around them.

**Scope of Part 2 within v0.3.0**: three pre-registered experiments, not a full Tier 2 test suite.

### Methodology: publication-grade discipline, informal presentation

The experiments apply publication-grade discipline — pre-registration, human-natural prompts, committed assertions, N=10, honest reporting of negative results — **as a shield against self-deception, not as preparation for external peer review**. The distinction matters:

- We're not writing a paper. Presentation is informal (notebook-style results, no statistical tests).
- We are applying the discipline that would prevent us from unconsciously tuning experiments to produce favorable results.
- If we later decide results are worth publishing formally, the rigor is already there. Upgrade presentation only.

**Methodological commitments** (see [`experiments/README.md`](PROPOSAL-001-spec/experiments/README.md) for the full list):

1. Pre-registration before running — design committed to git first
2. Human-natural prompts reviewed for contamination
3. Assertion logic committed before running
4. N=10 per condition (baseline vs invariant-loaded)
5. Claude Code version + Claude model version recorded
6. Full transcripts preserved
7. No quiet deletion of failed experiments
8. Iteration allowed, but honestly documented

### The three experiments

| # | Invariant | Language | Hypothesis |
|---|---|---|---|
| [01](PROPOSAL-001-spec/experiments/01-multi-tenancy.md) | INV-012 Tenant isolation | Go | Baseline ≥5/10 violations, invariant-loaded ≤1/10 |
| [02](PROPOSAL-001-spec/experiments/02-money-precision.md) | INV-008 Money precision | Python | Baseline ≥5/10 violations, invariant-loaded ≤1/10 |
| [03](PROPOSAL-001-spec/experiments/03-timezone-awareness.md) | INV-016 Timezone awareness | Python | Baseline ≥5/10 violations, invariant-loaded ≤1/10 |

Each experiment has a pre-registered design with fixture, prompt, assertion logic, and expected outcomes — committed to git before any run.

### The decision loop

After running the experiments, we look at results honestly and decide:

| Outcome | Response |
|---|---|
| **Strong effect confirmed** | Evidence gets added to writing guide. Consider blog post. v0.3.0 release notes cite results. |
| **Weak effect** | Directional but not conclusive. Investigate: harder prompt, different fixture. Don't overclaim. |
| **No effect** | Claude already handles this well. Invariant still has value for teams, but "blind spot" framing doesn't apply. |
| **Inverted effect** | Invariant made things worse. Investigate, fix invariant or feature. |

**In all cases**: v0.3.0 feature ships regardless. The feature (template adaptation, three-list schema) is valuable even if the experimental hypothesis about Claude failure modes doesn't hold up. Experiments inform framing, they don't gate the release.

### Known limitations (acknowledged up front)

- **Context-size confound**: invariant-loaded condition has more tokens. Results could reflect "more context = more care" rather than the specific invariant content. Control condition deferred to v0.4.0+.
- **N=10 is small**: enough to see dramatic effects, not enough to distinguish subtle ones.
- **Single fixture, single prompt, single model per experiment**: results bounded by the specific setup.

These aren't flaws — they're honest scope bounds on what the experiments can and can't tell us. Documented in each experiment's design file.

### What Part 2 does NOT include in v0.3.0

- Full Tier 2 LLM-as-judge test suite (deferred to v0.4.0+ after v0.3.0 experiments produce signal)
- Rubric-based scoring via the evaluator agent
- Trend tracking in `evaluator-tuning.md`
- GitHub Actions CI integration (can't run Claude in CI anyway)
- Style conformance, source reading, decision weight, proportionality as separate categories (the experiments are narrower)

v0.3.0 ships the experiment *infrastructure* (fixtures, runner, results directory structure) and three specific experiments. The infrastructure is reusable by v0.4.0+ for a fuller Tier 2 suite if we decide to build one.

### Testing strategy

**Tier discipline**: v0.3.0 uses Tier 1 (bash contract tests) + Tier 2 (fixture-based integration via headless Claude) only. Tier 3 (LLM-as-judge) is explicitly deferred to v0.4.0.

**Test-first discipline**: every phase lands with its test files in the same commit. No "I'll add tests later". Contract tests are the spec for each phase.

**Shared helpers**: common logic (hash computation, schema parsing, fixture setup) lives in `test/helpers.sh` or `test/helpers/compile-schema.sh` to avoid 3× duplication across artifact types.

#### Tier 1 tests (bash contract)

Per phase, per artifact type:

- **Phase 1 — `test/test-template-fallback.sh`**
  - Template lookup chain: project override beats inline fallback
  - install.sh ships reference templates to `~/.edikt/templates/examples/`
  - Each reference template contains the required sentinel block
  - Template-less refusal documented in all three `new.md` files, error points to `/edikt:init`
  - `commands/guideline/compile.md` exists with the same structure as adr/compile.md

- **Phase 2 — `test/test-compile-schema.sh`**
  - Three-list schema documented in all three `<artifact>:compile.md`
  - Decision tree comment in reference templates
  - Hash algorithm documented (normalization steps, exclusion of directives block)
  - Interview flow documented (5 options per hand-edited line)
  - Headless mode `--strategy=` overrides documented
  - Backward compat: legacy blocks without new lists still parse
  - ADR-008 exists and is Accepted

- **Phase 2 — `test/test-compile-hashes.sh`**
  - Fixture artifacts with known bodies and precomputed expected hashes
  - Run the hash computation pipeline from the documented algorithm
  - Assert results match
  - Test normalization: trailing whitespace, CRLF, block exclusion
  - `compiler_version` field set correctly

- **Phase 3 — `test/test-init-adapt.sh`**
  - Three-choice prompt documented in init.md for each artifact type
  - Inconsistent style fallback logic documented
  - Adapt mode writes to `.edikt/templates/<artifact>.md`
  - Sentinel block always included in generated templates
  - Grandfather flow for v0.2.x projects documented

- **Phase 4 — `test/test-flexible-input.sh`**
  - Prose-first dispatch documented in all three `new.md` files
  - Reference extraction logic described (paths, identifiers, branches)
  - Mixed input handling documented
  - Conversation context fallback documented
  - Matches the existing `/edikt:sdlc:plan` pattern

- **Phase 5 — `test/test-v030-doctor-upgrade.sh`**
  - `doctor.md` checks for `.edikt/templates/<artifact>.md` presence (all three)
  - `doctor.md` reports "using project template" vs "using reference" vs "missing"
  - `upgrade.md` never modifies files under `.edikt/templates/`
  - `upgrade.md` documents the grandfather flow for v0.2.x → v0.3.0

#### Tier 2 tests (headless Claude + fixtures)

Fixtures under `test/fixtures/v030/`:

```
test/fixtures/v030/
├── init-adapt/
│   ├── nygard-minimal/        # 3 Nygard-style ADRs, expected template
│   ├── madr-extended/         # 3 MADR-style ADRs, expected template
│   ├── inconsistent/          # 5 MADR + 4 Nygard, expected prompt flow
│   ├── greenfield/            # no existing artifacts
│   ├── invariants-concise/    # 3 terse invariants
│   └── guidelines-narrative/  # 3 prose-style guidelines
├── compile-schema/
│   ├── body-unchanged.md      # precomputed hashes, expect fast path
│   ├── body-changed.md        # expect regeneration
│   ├── hand-edited.md         # directives_hash mismatch, expect interview (headless fails loud)
│   └── backward-compat.md     # legacy block without new lists
├── flexible-input/
│   ├── with-spec/             # spec file, argument references it
│   ├── with-identifier/       # SPEC-042 resolution
│   └── with-branch/           # branch content (may skip if flaky)
└── gov-compile-merge/
    ├── adr-with-manual.md
    ├── invariant-with-suppressed.md
    └── guideline-with-both.md
```

Test scripts:

- `test/test-init-adapt-e2e.sh` — runs `/edikt:init` via headless Claude in each fixture, asserts generated `.edikt/templates/*.md` matches expected structure
- `test/test-compile-e2e.sh` — runs `/edikt:<artifact>:compile` on each schema fixture, asserts hash behavior matches expected branch
- `test/test-flexible-input-e2e.sh` — runs `/edikt:<artifact>:new` with various argument types, asserts referenced content appears in output
- `test/test-gov-compile-merge-e2e.sh` — runs `/edikt:gov:compile` on fixtures with all three lists, asserts governance.md has correct merge

#### Known testing limitations

These are intentionally out of scope for v0.3.0 and documented as gaps:

1. **Interactive interview happy path** — can only test that the interview is *documented* (Tier 1) and the *headless failure* (Tier 2). Cannot test the interactive response flow without Claude Code features we don't have.
2. **Claude output correctness** — Tier 2 can verify the output file *contains* expected content via grep, but cannot verify "the ADR reads well" or "the Decision section is substantive". That's Tier 3 and belongs in v0.4.0.
3. **CI integration for Tier 2** — requires Claude Code CLI in the test runner. For v0.3.0, Tier 2 runs locally before releases and nightly if desired. GitHub Actions runs Tier 1 only. Revisit CI Tier 2 integration in v0.4.0 or later.

### Resolved open questions (Part 1)

All six v0.3.0 questions from the original proposal are resolved:

| # | Question | Resolution |
|---|---|---|
| Q1 | Phasing — bundle or split? | Bundle phases 1+2+3+4+5 as v0.3.0. Heavy release, but cohesive theme. |
| Q2 | Default template style — Nygard or MADR? | **No default.** Init captures per-artifact choice. Reference examples shipped but not auto-installed. |
| Q3 | Adapt mode inference depth — structural or prose? | Structural only. Voice → user responsibility. Content quality → Part 2. |
| Q4 | Migrate mode scope? | Deferred to a separate `/edikt:<artifact>:migrate` command. Not in init flow. |
| Q5 | Directives block wording? | Three-list schema + hashes + interview + auto-chain. Documented in ADR-008. |
| Q6 | Argument-aware sourcing timing? | In v0.3.0. Flexible prose input with reference extraction, matching `/edikt:sdlc:plan` pattern. |

### Resolved edge cases

All decided during the design discussion:

- **`/edikt:<artifact>:new` without template** → hard refuse with clear error pointing to init
- **Re-running init with templates present** → skip template step by default; `--reset-templates` flag for explicit regeneration
- **Skipping template step during init** → warn, `<artifact>:new` refuses until user comes back
- **No team template in existing project** → ask user: draft from majority OR pick from reference examples
- **Upgrade from v0.2.x** → warn during upgrade, prompt at point-of-need (on first `<artifact>:new` invocation)
- **Hand-edit auto-detection** → interview per edit, no silent moves, headless fails with `--strategy=` overrides
- **Duplicate rule in `directives:` and `manual_directives:`** → silent de-dup at gov:compile time
- **`suppressed_directives:` changes** → no `<artifact>:compile` re-run needed; `gov:compile` picks up on next run
- **`compiler_version` drift on edikt upgrade** → warn, don't auto-regenerate; user opts in with `--regenerate`

### Limitations

- **Structural inference cannot encode semantic conventions.** Teams that want "always cite JIRA tickets in Context" or "always use Why-Not sections for rejected alternatives" add these as template comments or prose-style notes. Auto-inference for semantics is unreliable and out of scope.
- **Voice is not encoded in templates.** Users write in their own voice or note preferences as template comments.
- **Interactive interview happy path is not tested** (see Testing / Known limitations).
- **First compile on fresh artifacts produces stochastic Claude output.** The committed directives become canonical via git; subsequent compiles are deterministic. Two team members running compile on the same fresh artifact in parallel may produce different initial wording — whoever commits first wins.

---

## Part 2: Quality Measurement — now inside v0.3.0 (minimal), fuller scope deferred

**Scope change from the original proposal**: Part 2 was originally designed as a separate v0.4.0 release. During Part 2 discussion, we decided to **bundle a minimal version into v0.3.0 instead** of deferring entirely.

What's in v0.3.0 (see the "Experiments for validation and iteration" section above, which is now inside Part 1's scope):

- Three pre-registered experiments with grep-based assertions (multi-tenancy, money precision, timezone)
- Experiment infrastructure at `test/experiments/` (runner, fixtures, results directory)
- Publication-grade methodological commitments (pre-registration, human-natural prompts, committed assertions, N=10)
- Canonical invariant examples that the experiments validate
- Honest reporting of outcomes, including negative results

What's deferred to v0.4.0 or later:

- **Full Tier 2 LLM-as-judge test suite** with the evaluator agent scoring output against rubrics
- **Four test categories** from the original proposal:
  - Style conformance (scored qualitatively via judge, not just grep assertions)
  - Source reading (does the ADR contain details from referenced source docs?)
  - Decision section weight (is the Decision section substantive or a summary?)
  - Proportionality (do close alternatives get proportional attention?)
- **Trend tracking** in `docs/architecture/evaluator-tuning.md` with rubric-versioned scoring over time
- **Rubric versioning** and golden-ADR calibration
- **CI integration** (still impossible under the cost constraint, but worth revisiting if a cheap local harness works well)

### Resolved Part 2 decisions

- ✅ **Cost budget**: No API spending. Local Claude Code subscription only. No GitHub Actions.
- ✅ **Infrastructure**: Local-only runs. No CI integration in v0.3.0.
- ✅ **Tier 2 merge gate**: Never. Tier 2 is informative only, never blocks commits or releases. Experiments are developer tools for validation, not gating mechanisms.
- ✅ **Methodology**: Publication-grade discipline applied as a shield against self-deception, not as preparation for peer review. Informal presentation, rigorous process.
- ✅ **Judge model pinning**: Record the Claude model version used, treat different versions as non-comparable baselines. No formal pinning mechanism in v0.3.0 — just documentation.
- ✅ **Experiment categories for v0.3.0**: three pre-registered experiments using grep-based assertions (not the four original LLM-judge categories). These are narrower but simpler and don't require a full rubric-scoring infrastructure.
- ✅ **Scope boundary**: v0.3.0 ships the experiments; v0.4.0+ can build a fuller Tier 2 suite on top of the infrastructure if the v0.3.0 experiments produce enough signal to justify it.

### Remaining open questions (deferred to v0.4.0 planning)

Not urgent. These become relevant only if we decide to expand Tier 2 into a fuller suite:

1. **Should the evaluator agent be used for qualitative scoring** (the original Tier 2 design), or are grep-based assertions enough? v0.3.0's three experiments stay with grep-based. If we want to measure things like "Decision section substantiveness" that can't be grepped, we need the evaluator.
2. **Rubric versioning** — how do rubrics evolve over time? How do we distinguish "rubric got stricter" from "feature got worse"?
3. **Golden ADR calibration** — should we maintain a set of known-good invariants/ADRs that the rubric must score above a threshold, as a sanity check on the rubric itself?
4. **Formal judge model pinning** — if we add LLM-judge scoring, do we commit to a specific Claude model version for stability? How do we handle model deprecation?
5. **Trend dashboards** — do results feed a dashboard (`evaluator-tuning.md` as a living doc) or do they stay as per-run snapshots?

None of these block v0.3.0. They're v0.4.0+ concerns.

---

## Cross-proposal: no longer separate releases

The original framing was "v0.3.0 = Adaptation, v0.4.0 = Measurement" as two halves of the same loop. During design discussion, we decided to bundle the minimal measurement story into v0.3.0 so the feature ships with empirical validation attached.

**Revised release theming:**

- **v0.3.0** = "edikt adapts to your project's conventions, with formal Invariant Records and pre-registered validation experiments"
- **v0.4.0+** = TBD, based on what v0.3.0's experiments reveal. Might be a fuller Tier 2 suite, might be new features we haven't thought of yet, might be iteration on v0.3.0's findings.

v0.3.0 is now a larger release than originally scoped, but it's self-contained: the feature, its canonical examples, its writing guide, and its empirical validation all ship together.

---

## Summary of open questions

**Part 1: Project adaptation (v0.3.0)**
✓ All resolved. ADR-008 + ADR-009 written. Ready for implementation.

**Part 2: Quality measurement (bundled into v0.3.0 as 3 pre-registered experiments)**
✓ All v0.3.0 decisions resolved. Remaining questions (full Tier 2 suite scope, rubric versioning, formal judge model pinning, trend dashboards) are deferred to v0.4.0+ planning and don't block v0.3.0.

---

## Next steps

**Immediate (v0.3.0 implementation path):**

1. ✅ **ADR-008 written** — [`ADR-008-deterministic-compile-and-three-list-schema.md`](../decisions/ADR-008-deterministic-compile-and-three-list-schema.md). Formal contract for the three-list schema and hash caching.
2. ✅ **ADR-009 written** — [`ADR-009-invariant-record-terminology.md`](../decisions/ADR-009-invariant-record-terminology.md). Formal coinage of "Invariant Record" and the template contract.
3. ✅ **Spec artifacts created** — [`PROPOSAL-001-spec/`](PROPOSAL-001-spec/) with schema.yaml, glossary.md, hash-reference.md, fixtures/, anti-patterns.md, file-changes.md, invariant-record-template.md, writing-invariants-guide.md, canonical-examples/, experiments/.
4. ✅ **Canonical invariant examples drafted** — [`canonical-examples/tenant-isolation.md`](PROPOSAL-001-spec/canonical-examples/tenant-isolation.md) and [`canonical-examples/money-precision.md`](PROPOSAL-001-spec/canonical-examples/money-precision.md).
5. ✅ **Writing guide drafted** — [`writing-invariants-guide.md`](PROPOSAL-001-spec/writing-invariants-guide.md).
6. ✅ **Three experiments pre-registered** — [`experiments/`](PROPOSAL-001-spec/experiments/) with design, methodology, runner spec, and pre-registration for multi-tenancy (Go), money precision (Python), timezone awareness (Python).
7. **Begin Phase 1** — follow [`PROPOSAL-001-spec/file-changes.md`](PROPOSAL-001-spec/file-changes.md) for exact file list. Extensibility plumbing + `/edikt:guideline:compile` creation.
8. **Test-first per phase** — contract tests land in the same commit as each phase's implementation, using fixtures from the spec directory.
9. **ADR-008 and ADR-009 guard the schema contracts** — any future change requires a new ADR superseding them.

**During v0.3.0 release validation (Phase 6):**

1. Build out the three experiment fixtures under `test/experiments/fixtures/`
2. Run the three experiments via `./test/experiments/run.sh`
3. Commit results to `test/experiments/results/`
4. Decide framing for release notes + blog post + website content based on what the experiments actually show
5. Ship v0.3.0 — feature, canonical examples, writing guide, and experimental validation together

---

## Document status

- **2026-04-09** — Created from design discussion. Part 1 Q1–Q6 locked in. Three-list schema + hash caching fully designed. Symmetric application across ADR/invariant/guideline confirmed. `/edikt:guideline:compile` gap identified. Testing strategy matrix drafted.
- **2026-04-09 (same day, later)** — ADR-008 written. Spec directory created with schema.yaml, glossary.md, hash-reference.md, fixtures/, anti-patterns.md, file-changes.md. Part 1 fully specified; ready for implementation without ambiguity.
- **2026-04-09 (Part 2 merged into v0.3.0)** — ADR-009 written (Invariant Record terminology coinage). Final Invariant Record template locked in (6 body sections, 4 lifecycle states, writing guidance comment). Canonical examples drafted for tenant isolation and money precision. Writing guide drafted with 5 qualities, 7 traps, 6 rewrites, 7-question self-test. Three experiments pre-registered with publication-grade methodology commitments. file-changes.md Phase 6 added. glossary.md expanded with Invariant Record terminology and experiments terminology.
- **Next update** — when v0.3.0 Phase 1 lands, mark "Phase 1 shipped" in the Phases section. When experiments run, link the results from this doc.
