# Anti-Patterns — PROPOSAL-001 / v0.3.0

Explicit "don't do this" list. If you see any of these in an implementation or review, reject it immediately. Each anti-pattern is grounded in a decision from Part 1 of the proposal or ADR-008.

---

## Schema anti-patterns

### ❌ Using different field names than the schema

The schema names are **exactly**: `source_hash`, `directives_hash`, `compiler_version`, `paths`, `scope`, `directives`, `manual_directives`, `suppressed_directives`.

**Do not use**: `auto_directives`, `user_directives`, `extra_directives`, `custom_directives`, `rejected_directives`, `blocked_directives`, `rules`, `enforcement`, anything else.

**Do not pluralize/singularize inconsistently**: `directive` vs `directives`. It's always plural.

**Do not use camelCase**: `manualDirectives`. It's always snake_case.

### ❌ Making optional fields required (or vice versa)

**REQUIRED after first compile**: `source_hash`, `directives_hash`, `compiler_version`, `paths`, `scope`, `directives`.

**OPTIONAL (default empty list)**: `manual_directives`, `suppressed_directives`.

**ABSENT in v0.2.x legacy**: all three hash fields.

If you mark `manual_directives` as required, you break backward compat with v0.2.x. If you mark `source_hash` as optional after first compile, you break the cache contract. Get this right.

### ❌ Adding new fields to the block without a new ADR

If you think the block needs a new field (`last_modified`, `author`, `priority`, whatever), **don't just add it**. Write a new ADR superseding ADR-008 that explains why, what the migration story is, and how downstream tools handle the new field.

The schema is a contract. Contracts don't change silently.

---

## Compile command anti-patterns

### ❌ `<artifact>:compile` reading `manual_directives:`

The `<artifact>:compile` commands (adr, invariant, guideline) MUST NOT read `manual_directives:`. Period. They don't need it. Reading it creates an implicit coupling and can lead to accidental modification.

Same for `suppressed_directives:`. Only `gov:compile` reads those two lists.

### ❌ `<artifact>:compile` filtering `directives:` based on `suppressed_directives:`

Filtering is `gov:compile`'s job. `<artifact>:compile` writes the full auto output to `directives:`. Full stop.

If you filter at `<artifact>:compile` time, you lose the ability to undo a suppression (the filtered-out directive would be gone from the file and the next compile would regenerate it with different wording).

### ❌ Running Claude when `source_hash` and `directives_hash` both match

The fast path exists precisely to avoid this. Calling Claude on a cache hit wastes the user's subscription and produces stochastic wording drift.

### ❌ Silently auto-resolving hand-edits

When `directives_hash` mismatches with an unchanged body, the command MUST run the interactive interview OR fail loud in headless mode. It MUST NOT:

- Silently move the hand-added line to `manual_directives:`
- Silently add the deleted line to `suppressed_directives:`
- Silently regenerate and discard the user's edits
- Use a heuristic to guess the user's intent

The only options are interactive interview or explicit `--strategy=` flag.

### ❌ Writing hashes the user didn't ask for

On the fast path (both hashes match), do NOT write anything to the file. Not even "no change" metadata. The fast path is a pure no-op.

### ❌ Computing `source_hash` over the full file

The directive block MUST be excluded from `source_hash`. If you include it, writing new directives immediately invalidates the hash, and the cache never works.

### ❌ Computing `directives_hash` over the full YAML block

Only the `directives:` list items get hashed. Not the YAML surrounding them. Not `manual_directives:` or `suppressed_directives:`. Not `paths:` or `scope:`. Just the auto list items, joined with `\n`.

### ❌ Forgetting to normalize line endings / trailing whitespace

A file edited on Windows MUST produce the same `source_hash` as the same file edited on macOS. Normalize CRLF → LF and strip trailing whitespace before hashing. See `hash-reference.md` for the exact steps.

### ❌ Using uppercase hex or base64 for hash values

Lowercase hex, 64 characters. Always. Don't let PowerShell or some clever shortcut change this.

---

## gov:compile anti-patterns

### ❌ `gov:compile` writing to artifact files

`gov:compile` reads artifact files. It writes `.claude/rules/governance.md` and the topic files under `.claude/rules/governance/`. It MUST NOT write anything back to ADR/invariant/guideline files. Ever.

### ❌ `gov:compile` running Claude to regenerate directives

`gov:compile` is pure merge + filter. No Claude call for the merge logic. Contradiction detection might use Claude, but that's orthogonal to the three-list merge.

### ❌ `gov:compile` caching with hashes

v0.3.0 does NOT add hash-based caching to `gov:compile`. Only `<artifact>:compile` has hash-based skip. If `gov:compile` performance becomes a problem with large projects, we revisit in a later version.

### ❌ Getting the merge formula wrong

The formula is:

```
effective = (directives - suppressed_directives) ∪ manual_directives
```

Exactly this. Not `(directives ∪ manual_directives) - suppressed_directives`. Not `(directives ∪ manual_directives ∪ suppressed_directives)`. Not anything else.

The difference matters: if a user manually adds "Foo" AND that same "Foo" is in suppressed_directives, the formula says it should be in the effective set (because the filter applies to the auto list only, not to manual). If you change the formula order, you'd filter manual additions too, which is wrong.

---

## Init flow anti-patterns

### ❌ Auto-installing a "default" template

v0.3.0 does NOT ship a default template that gets installed for new projects. Reference templates under `~/.edikt/templates/examples/` are only copied when the user explicitly selects them during init.

If you see `install.sh` copying `adr-default.md` to `.edikt/templates/adr.md` automatically, that's wrong.

### ❌ `<artifact>:new` running without a project template

If `.edikt/templates/<artifact>.md` doesn't exist, the command MUST refuse with a clear error pointing to `/edikt:init`. It MUST NOT:

- Fall back to a hardcoded inline template silently
- Copy a reference template automatically
- Start an ad-hoc template selection flow

### ❌ Assuming the project's preferred style

If existing artifacts are detected, init's Adapt mode reads them to extract structural patterns. If they're inconsistent (mixed styles), init MUST ask the user what to do. It MUST NOT:

- Pick the majority style without asking
- Pick the most recent style
- Fail with an error
- Pick a reference template without asking

### ❌ Overwriting an existing project template on init re-run

If `.edikt/templates/<artifact>.md` already exists, init MUST skip the template step by default. `--reset-templates` is the only flag that allows regeneration.

### ❌ Requiring migration of existing artifacts at init time

Migration of existing ADRs to match a new template is a SEPARATE command (`/edikt:<artifact>:migrate`, deferred to v0.3.1+). Init's Adapt mode creates the template. It MUST NOT modify existing artifacts.

---

## Template content anti-patterns

### ❌ Shipping a template without the sentinel block

Every template (reference, generated, or inline fallback) MUST include the `[edikt:directives:start]: #` / `[edikt:directives:end]: #` block, even if empty. This is the only hard contract edikt enforces.

### ❌ Opinionated sections in default templates

Defaults should be minimal. Don't ship `adr-default.md` with 8 sections that reflect one person's preferences. Nygard-minimal is the floor. MADR-extended is a reference.

### ❌ Using HTML comments as sentinels

Sentinels are markdown link-reference definitions: `[edikt:directives:start]: #`. Not HTML comments. See ADR-006.

---

## Argument-aware sourcing anti-patterns

### ❌ Classifying input into rigid types

`/edikt:<artifact>:new` treats input as prose first, then scans for references. Do NOT:

- Use a regex to detect "is this a file path?" and branch on it.
- Fail when input doesn't match a known type.
- Require specific prefixes or delimiters.

The input is always prose. References embedded within it are extracted and resolved.

### ❌ Ignoring the existing `/edikt:sdlc:plan` pattern

v0.1.3 added "flexible input" to `/edikt:sdlc:plan`. v0.3.0 extends the same pattern to `<artifact>:new`. If you invent a different dispatch mechanism, you're creating inconsistency between commands.

---

## Test anti-patterns

### ❌ Tests that require API costs

v0.4.0 Tier 2 tests use Claude Code headless mode + the user's Claude subscription. v0.3.0 tests (Tier 1 + fixture-based Tier 2) MUST NOT require an API key or incur costs.

If a test needs to call Claude, it uses `claude -p` headless and assumes a local subscription. If that's not available, the test is skipped, not failed.

### ❌ Tests that gate merges on stochastic output

Only deterministic tests can gate merges. LLM-as-judge scores are stochastic — they feed the quality trend doc, they don't block PRs.

### ❌ Tests that depend on specific Claude model versions

Tests should work across Claude Sonnet 3.5, 3.6, 3.7, etc. If a test is brittle to model version, it's probably testing the wrong thing (testing Claude, not testing edikt).

### ❌ Copy-pasting hash/schema logic across test files

All three artifact types (ADR, invariant, guideline) share the same schema and hash algorithm. Test helpers for parsing, computing, and validating the block MUST live in `test/helpers.sh` or `test/helpers/compile-schema.sh` and be used by all three artifact type test files.

---

## Documentation anti-patterns

### ❌ Inconsistent terminology

Use the glossary. If you see "auto directives" in one doc and "generated directives" in another, it's the same thing, pick one (`auto directives` per the glossary).

### ❌ Using "default template" in v0.3.0 docs

There is no default template in v0.3.0. The term is forbidden. If you see it, it's a leftover from a pre-v0.3.0 draft.

### ❌ Treating ADRs as the primary example and ignoring invariants/guidelines

The schema is symmetric. Examples should rotate between the three types to reinforce symmetry. If every example in a doc is an ADR, it implies (incorrectly) that invariants and guidelines have a different contract.

### ❌ Missing backward compatibility notes

Anywhere the schema is documented, the backward compatibility story for v0.2.x MUST be mentioned. Missing it invites Claude to break migration.

---

## Meta anti-pattern

### ❌ "Close enough" to the spec

The schema, hash algorithm, and compile logic are contracts. "Close enough" is not good enough. If your implementation produces a hash that's one character different from the reference implementation, it's broken, not approximate.

If you find yourself thinking "this should work in most cases" — stop. The spec exists so "most cases" becomes "all cases".
