# Glossary — PROPOSAL-001 / v0.3.0

This file defines every term used in PROPOSAL-001, ADR-008, and related v0.3.0 work. It exists to prevent terminology drift between the proposal, the ADR, the test code, and the command implementations.

**Usage rule:** if a term is defined here, use it exactly. Do not invent synonyms ("enforcement rules" instead of "directives"), do not mix singular/plural inconsistently, do not confuse terms (e.g., `suppressed_directives` vs `manual_directives` vs `directives`).

---

## Artifacts

### Compiled artifact
Any of: **ADR**, **invariant**, or **guideline**. These are markdown files that contain a directive sentinel block and are processed by `/edikt:gov:compile` into `.claude/rules/governance.md`.

**Never call this "source document"** — that term is ambiguous. Always "compiled artifact" or one of the specific three types.

### ADR (Architecture Decision Record)
A markdown file under `{base}/architecture/decisions/` (or the configured `paths.decisions`) documenting a specific architectural decision. Has a specific status (draft, accepted, deprecated, superseded).

Command: `/edikt:adr:new` (create), `/edikt:adr:compile` (generate directives), `/edikt:adr:review` (language quality).

### Invariant
A markdown file under `{base}/architecture/invariants/` (or the configured `paths.invariants`) documenting a hard constraint that must never be violated. Has a status (active, retired).

Command: `/edikt:invariant:new`, `/edikt:invariant:compile`, `/edikt:invariant:review`.

### Guideline
A markdown file under `{base}/guidelines/` (or the configured `paths.guidelines`) documenting a team convention or soft recommendation.

Command: `/edikt:guideline:new`, `/edikt:guideline:review`, **and `/edikt:guideline:compile` (NEW in v0.3.0)**.

---

## The directive sentinel block

### Directive sentinel block
The YAML-content block between `[edikt:directives:start]: #` and `[edikt:directives:end]: #` in any compiled artifact. Contains metadata (hashes, version, paths, scope) and three directive lists.

Sometimes shortened to "directive block" or "directives block". These all refer to the same thing.

**Never call this "rules block" or "enforcement block"** — stay with "directive(s) block" or "directive sentinel block".

### Sentinels
The two literal lines `[edikt:directives:start]: #` and `[edikt:directives:end]: #`. They are markdown link-reference definitions that delimit the directive block. See ADR-006 for why they look this way (previously HTML comments, changed in v0.2.0).

---

## The three directive lists

These three lists have strict, non-overlapping meanings. **Do not use them interchangeably.**

### `directives` (the auto list)
**Owner:** `/edikt:<artifact>:compile` (command-authoritative).
**Content:** auto-generated enforcement rules produced by Claude from the artifact body.
**Lifecycle:** rewritten on every body change. Hand-edits detected via `directives_hash`.
**Always rhymes with:** "auto directives", "Claude-generated directives", "the auto list".

**NEVER:**
- Call this "the main directives" (all three lists are valid rules).
- Say "user directives" (that's `manual_directives`).
- Say "raw directives" or "primary directives" — just "auto directives" or "the `directives:` list".

### `manual_directives` (the user-added list)
**Owner:** the user (read-only to all compile commands).
**Content:** user-authored enforcement rules the user added because compile missed them.
**Lifecycle:** never touched by `/edikt:<artifact>:compile`. Preserved across all regenerations.
**Always rhymes with:** "manual directives", "user-added directives", "user-authored directives".

**NEVER:**
- Call this "custom directives" (that implies it's a fork of something).
- Call this "extra directives" (that implies it's supplementary; they're first-class).
- Say "manual list" in isolation without "directives".

### `suppressed_directives` (the user-rejected list)
**Owner:** the user (read-only to all compile commands).
**Content:** auto directives the user rejects (hallucinated, wrong, nuance-shifted).
**Lifecycle:** never touched by `/edikt:<artifact>:compile`. Applied by `/edikt:gov:compile` as a filter on `directives:`.
**Always rhymes with:** "suppressed directives", "rejected directives", "filtered directives".

**NEVER:**
- Call this "blacklist" or "blocklist" (connotation is wrong).
- Call this "disabled directives" (sounds like a toggle).
- Confuse with "retired" (that's an invariant status, not a directive state).

---

## Commands

### `/edikt:<artifact>:compile`
The family of three commands: `/edikt:adr:compile`, `/edikt:invariant:compile`, `/edikt:guideline:compile` (new in v0.3.0). Each generates auto directives from its artifact type's body.

When referring to one of them, be specific. When referring to the pattern, write `/edikt:<artifact>:compile`.

### `/edikt:gov:compile`
The command that merges directives from ALL compiled artifacts into `.claude/rules/governance.md` and topic files. See ADR-007 for the topic file structure.

**Critical distinction:**
- `<artifact>:compile` = **generation** (runs Claude, writes auto directives).
- `gov:compile` = **application** (merges lists, no Claude call, writes governance.md).

Never conflate these. Never say "compile" without a qualifier — always specify `<artifact>:compile` or `gov:compile`.

### `/edikt:<artifact>:new`
The family: `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new`. Creates a new compiled artifact from flexible prose input and auto-chains to the corresponding `<artifact>:compile`.

---

## Hash fields

### `source_hash`
SHA-256 hex digest of the artifact body (directives block excluded) after normalization. Tells `<artifact>:compile` whether the body has changed since the last compile run.

**Never** compute this over the full file — the directives block MUST be excluded.

### `directives_hash`
SHA-256 hex digest of the canonicalized `directives:` list items joined with `\n`. Tells `<artifact>:compile` whether the user hand-edited the auto list.

**Never** compute this over `manual_directives:` or `suppressed_directives:`. Only the auto `directives:` list is hashed.

### `compiler_version`
String matching edikt's semver at compile time (e.g., `"0.3.0"`). Used to detect algorithm drift across edikt versions.

---

## Compile states

### Fresh artifact
An artifact that has never been compiled. Its directive block is missing, empty, or contains only the sentinel lines. First compile runs Claude, writes all lists + hashes.

### Legacy artifact (v0.2.x)
An artifact with a directive block but missing `source_hash`, `directives_hash`, `compiler_version`, `manual_directives`, and `suppressed_directives`. First compile after v0.3.0 upgrade migrates silently — regenerates auto directives and writes the new fields.

### Clean artifact
An artifact where both `source_hash` and `directives_hash` match the current state. `<artifact>:compile` uses the fast path: no Claude call, no writes, exit "up to date".

### Body-changed artifact
An artifact where `source_hash` does not match the current body. Claude regenerates auto directives; new hashes are written.

### Hand-edited artifact
An artifact where `source_hash` matches but `directives_hash` does not. The user modified `directives:` after compile wrote it. Interview triggers (or `--strategy=` override in headless mode).

---

## Compile algorithm phases

### Fast path
When both hashes match → do nothing, exit "up to date". No Claude call, no writes.

### Slow path
When `source_hash` mismatches → run Claude to regenerate directives, write new directives + new hashes.

### Interview path
When `source_hash` matches but `directives_hash` does not → interactive questions per detected edit, one choice per edit.

### Headless fail path
When interview would trigger but there's no TTY → exit with error and `--strategy=` override hint.

---

## Effective rule set

The final rules written to `.claude/rules/governance.md` (or topic files) after `gov:compile` merges everything.

**Formula per artifact:**

```
effective_rules = (directives - suppressed_directives) ∪ manual_directives
```

Where:
- `-` is set difference by exact string match
- `∪` is set union preserving document order

**Then across artifacts:** de-duplicate by exact string match so each unique rule appears once in the output.

---

## Templates

### Project template
A markdown file at `.edikt/templates/<artifact>.md` defining the scaffolding for new artifacts of that type. Generated by `/edikt:init`'s Adapt mode from existing artifacts, or selected from reference templates. Git-tracked, team-shared.

### Reference template
A markdown file at `~/.edikt/templates/examples/*.md` shipped by `install.sh`. Used as a starting point when a user chooses "Start fresh" during init. Examples: `adr-nygard-minimal.md`, `adr-madr-extended.md`.

### Default template
**⚠ This term is FORBIDDEN in v0.3.0+**. We do not ship default templates. There is no implicit default. Users must explicitly choose during init or the command refuses.

If you see "default template" in any doc, assume it's a leftover from a pre-v0.3.0 draft and should be removed or rewritten.

### Inline fallback
The minimal stub defined inside `commands/<artifact>/new.md` used only when no project template exists AND no reference template was selected. Exists purely for bootstrap scenarios.

---

## Hand-edit interview options

Exactly five options for hand-ADDED lines (one new option, one question per detected edit):

1. Move to `manual_directives:`
2. Add to `suppressed_directives:`
3. Delete entirely
4. Edit artifact body first
5. Skip for now (leave in `directives:`, will be overwritten next compile)

Exactly three options for hand-DELETED auto directives:

1. Add to `suppressed_directives:` (keep deleted)
2. Let compile regenerate (deletion was accidental)
3. Edit artifact body first

**Do not invent other options.** Any implementation that offers different choices is violating ADR-008.

---

## Strategy overrides (headless)

### `--strategy=regenerate`
Force full Claude re-run, discard hand-edits in `directives:`. Still preserves `manual_directives:` and `suppressed_directives:`.

### `--strategy=preserve`
Skip this artifact entirely. Leave the file unchanged. Exit 0.

### (default, no flag)
Run interview interactively. In headless mode, fail with error 2 and print available overrides.

---

## Invariant Record terminology (ADR-009)

### Invariant Record
The formal name for a governance artifact documenting a hard architectural constraint. Coined by edikt in ADR-009 as parallel to "Architecture Decision Record" (ADR). Always labeled as "an edikt convention, not an external standard" in documentation.

**Short form:** `INV`. File naming: `INV-NNN-short-title.md`.

### INV
Three-letter short form of "Invariant Record". **Never** use `IR` — it collides with Intermediate Representation (compilers), Information Retrieval, Incident Response, and Infrared.

### Constraint vs implementation (level of abstraction)
The core distinction in writing a good invariant. A **constraint** is a rule that holds regardless of the specific technology used. An **implementation** is a specific technology choice. Invariants describe constraints. Implementations belong in ADRs.

**Test:** "If our tech stack changed tomorrow, would this rule still apply?" Yes → constraint (good invariant). No → implementation (belongs in an ADR).

### Canonical example (of an invariant)
An invariant shipped with edikt as a reference for users writing their own. v0.3.0 ships two: **tenant isolation** (multi-tenant data scoping) and **money precision** (fixed-point monetary values). Canonical examples live at `templates/examples/invariants/` and on the edikt website.

### Writing guide (for invariants)
The guide at [`writing-invariants-guide.md`](writing-invariants-guide.md) teaching how to write good Invariant Records. Contains five qualities, seven traps, six bad-to-good rewrites, and a seven-question self-test. Lives on the website and is shipped alongside the canonical examples.

### Invariant states
Four lifecycle states for an Invariant Record:

- **Active** — currently enforced. `gov:compile` reads it. The normal state.
- **Proposed** — under team discussion. `gov:compile` skips it. Directives not yet enforced.
- **Superseded by INV-NNN** — replaced by a newer invariant. `gov:compile` skips it. Kept for historical record.
- **Retired (reason)** — no longer relevant, not replaced. `gov:compile` skips it. Status line includes the reason.

## Experiments terminology

### Pre-registration
The practice of committing the experiment design (fixture, prompt, invariant, assertion logic, expected outcomes) to git BEFORE running the experiment. Binds us to the design and prevents post-hoc rationalization. Committed in the experiments directory before any run.

### Baseline condition / invariant-loaded condition
The two conditions compared in each experiment. **Baseline** = Claude runs the prompt without any invariant in its context. **Invariant-loaded** = Claude runs the same prompt with the invariant's canonical example loaded into context. The difference in failure rates between the two is the measured effect of the invariant.

### Human-natural prompt
A prompt written the way a real engineer would phrase the task while working. Crucially: contains no words that hint at the invariant being tested (e.g., "tenant", "secure", "precise", "decimal"). Contamination by hint words invalidates the experiment.

### Assertion (experiment)
A committed-before-running script that takes Claude's output and returns pass/fail. Tests whether Claude's generated code follows the invariant. Written before running so results can't be manipulated by adjusting what "pass" means.

### Publication-grade discipline
Methodology rigor applied as a shield against self-deception, not as preparation for external peer review. Includes: pre-registration, human-natural prompts, committed assertions, N=10 per condition, model version pinning, honest reporting of negative results. We're not writing a paper; we're preventing ourselves from lying to ourselves.

### N=10 per condition
The default experiment size: 10 runs in baseline, 10 runs in invariant-loaded condition. Small enough to be fast, large enough to see dramatic effects. Subtle effects may require larger N in follow-up experiments.

## Things that are NOT v0.3.0

To prevent Claude from conflating features:

- **Tier 2 LLM-as-judge tests** — v0.4.0 only.
- **Voice inference** — user's responsibility, not edikt's.
- **Migrate mode in init** — deferred to a separate `/edikt:<artifact>:migrate` command in v0.3.1+.
- **Semantic style inference** — out of scope. Structural only.
- **`gov:compile` hash caching** — not in v0.3.0. Only `<artifact>:compile` has hash-based skip.
- **Auto-resolution of hand-edits** — forbidden. Always interview.
- **Running tests in CI with API costs** — forbidden. Local-only via user subscription.

If any of these show up in a v0.3.0 PR, it's wrong.
