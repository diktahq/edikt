# File-by-File Change Checklist — v0.3.0

This file lists every file that must be created, modified, or verified during v0.3.0 implementation, organized by phase. Use as a work-breakdown and review checklist.

**Convention:**
- ✨ = new file
- ✏️ = modified existing file
- 🔍 = verify (no changes expected but must be checked)
- 🧪 = test file

Paths are relative to the repo root unless noted.

---

## Cross-cutting (lands before Phase 1)

These land first because everything else depends on them.

- ✨ `docs/architecture/decisions/ADR-008-deterministic-compile-and-three-list-schema.md` (already written)
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/README.md`
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/schema.yaml`
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/glossary.md`
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/hash-reference.md`
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/fixtures/*.md` (all fixture files)
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/anti-patterns.md`
- ✨ `docs/architecture/proposals/PROPOSAL-001-spec/file-changes.md` (this file)
- ✏️ `docs/architecture/proposals/PROPOSAL-001-adapt-and-measure.md` (link the spec directory)

**Commit message:** `docs: ADR-008 + PROPOSAL-001 spec artifacts for v0.3.0`

---

## Phase 1 — Extensibility plumbing + guideline:compile

**Scope:** extract templates, create missing command, wire up lookup chain, no user-visible change for existing projects.

### New files

- ✨ `commands/guideline/compile.md` — the missing command, mirrors `commands/adr/compile.md` in structure
- ✨ `templates/examples/adr-nygard-minimal.md` — reference template (Nygard-minimal shape + sentinel block)
- ✨ `templates/examples/adr-madr-extended.md` — reference template (MADR-extended shape + sentinel block)
- ✨ `templates/examples/invariant-minimal.md` — reference invariant template
- ✨ `templates/examples/invariant-full.md` — reference invariant template with rationale + consequences
- ✨ `templates/examples/guideline-minimal.md` — reference guideline template
- ✨ `templates/examples/guideline-extended.md` — reference guideline template with examples + when-not-to-apply

### Modified files

- ✏️ `commands/adr/new.md` — add template lookup chain, template-less refusal logic
- ✏️ `commands/invariant/new.md` — same
- ✏️ `commands/guideline/new.md` — same
- ✏️ `install.sh` — copy `templates/examples/*` to `~/.edikt/templates/examples/` on install. Do NOT auto-install any as `adr.md`/`invariant.md`/`guideline.md` in project-mode.

### Test files

- 🧪 `test/test-template-fallback.sh` — verifies lookup chain: project `.edikt/templates/<artifact>.md` → inline fallback. No global default. Reference templates are NOT auto-loaded.
- 🧪 `test/test-guideline-compile-exists.sh` — verifies `commands/guideline/compile.md` exists and has the same structure (frontmatter, argument handling, schema awareness) as `adr/compile.md`.
- 🧪 `test/test-template-refusal.sh` — verifies all three `new.md` files document the template-less refusal with clear error pointing to init.
- 🧪 `test/test-install-ships-examples.sh` — extends existing `test-install-e2e.sh` with new scenarios for reference template installation.

### Verify

- 🔍 Existing `test/test-install-e2e.sh` still passes (sanity check for install.sh changes).
- 🔍 `install.sh` template handling does NOT auto-install anything as a default.

**Commit:** `feat(v0.3.0 phase 1): extensibility plumbing + guideline:compile command`

---

## Phase 2 — Three-list schema + hash-based caching

**Scope:** the heart of ADR-008. Schema, hashes, interview, auto-chain.

### Modified files

- ✏️ `commands/adr/compile.md` — add three-list schema contract, hash algorithm, interview flow, headless strategy flags
- ✏️ `commands/invariant/compile.md` — same
- ✏️ `commands/guideline/compile.md` — created in Phase 1, now gets the full schema + hash logic
- ✏️ `commands/gov/compile.md` — update to read all three lists from each artifact, apply merge formula `(directives - suppressed_directives) ∪ manual_directives`, de-dup across artifacts, contradiction detection extended
- ✏️ `commands/adr/new.md` — auto-chain to `adr:compile` at end
- ✏️ `commands/invariant/new.md` — auto-chain to `invariant:compile`
- ✏️ `commands/guideline/new.md` — auto-chain to `guideline:compile`

### Test files

- 🧪 `test/test-compile-schema.sh` — contract tests: all three compile commands document the schema, hash algorithm, interview flow, --strategy flags; decision tree comment in reference templates
- 🧪 `test/test-compile-hashes.sh` — hash computation tests using the test vectors from `hash-reference.md`. Use shared helper `test/helpers/compile-schema.sh`.
- 🧪 `test/test-auto-chain.sh` — contract tests: all three `new.md` files document auto-chain to compile
- 🧪 `test/test-gov-compile-merge.sh` — uses fixtures from `docs/architecture/proposals/PROPOSAL-001-spec/fixtures/` to verify merge formula
- 🧪 `test/fixtures/compile-schema/` — copies of the PROPOSAL-001-spec fixtures for test use (or symlinks, or references)
- 🧪 `test/helpers/compile-schema.sh` — shared helper with hash computation, schema parsing, fixture loading

### Tier 2 tests (headless Claude)

- 🧪 `test/test-compile-e2e.sh` — runs `/edikt:adr:compile` via `claude -p` on fixtures, verifies:
  - Fresh artifact → populates block with hashes
  - Fast path → exits "up to date" without writes
  - Body change → regenerates and updates hashes
  - Hand-edited → headless fails with `--strategy=` hint
  - Same scenarios for invariant:compile and guideline:compile

### Verify

- 🔍 Existing v0.2.x artifacts (dogfood `docs/architecture/decisions/ADR-00*.md`) still work with v0.3.0 gov:compile (backward compat)
- 🔍 No artifact file modifications from `gov:compile` run

**Commit:** `feat(v0.3.0 phase 2): three-list schema + hash-based caching (ADR-008)`

---

## Phase 3 — Init style detection + Adapt mode

**Scope:** init flow update, three prompts per artifact type, Adapt/Start fresh/Write my own.

### Modified files

- ✏️ `commands/init.md` — add style detection step, three-choice prompts for each artifact type (ADR, invariant, guideline), Adapt mode logic, inconsistent-style fallback, grandfather flow for v0.2.x projects

### Test files

- 🧪 `test/test-init-adapt.sh` — contract tests: init.md documents the three-choice prompt for all three artifact types, inconsistent style handling, grandfather flow
- 🧪 `test/fixtures/init-adapt/nygard-minimal/` — 3 Nygard-style ADRs + expected generated template
- 🧪 `test/fixtures/init-adapt/madr-extended/` — 3 MADR-style ADRs + expected generated template
- 🧪 `test/fixtures/init-adapt/inconsistent/` — 5 MADR + 4 Nygard ADRs + expected prompt flow
- 🧪 `test/fixtures/init-adapt/greenfield/` — no existing artifacts + expected reference template choice
- 🧪 `test/fixtures/init-adapt/invariants-*/` — parallel fixtures for invariants
- 🧪 `test/fixtures/init-adapt/guidelines-*/` — parallel fixtures for guidelines

### Tier 2 tests (headless Claude)

- 🧪 `test/test-init-adapt-e2e.sh` — runs `/edikt:init` via `claude -p` on each fixture, asserts the generated `.edikt/templates/<artifact>.md` matches expected structure

### Verify

- 🔍 Re-running init on a project with existing templates skips the template step (unless `--reset-templates`)
- 🔍 Skipping the template step warns but doesn't block init

**Commit:** `feat(v0.3.0 phase 3): init style detection + Adapt mode for all three artifact types`

---

## Phase 4 — Flexible prose input

**Scope:** argument-aware sourcing in all three `new` commands.

### Modified files

- ✏️ `commands/adr/new.md` — replace argument handling with prose-first dispatch + reference extraction
- ✏️ `commands/invariant/new.md` — same
- ✏️ `commands/guideline/new.md` — same

### Test files

- 🧪 `test/test-flexible-input.sh` — contract tests: all three `new.md` files document the prose-first dispatch, reference extraction, conversation-context fallback
- 🧪 `test/fixtures/flexible-input/with-spec/` — project with a spec doc, test argument references it
- 🧪 `test/fixtures/flexible-input/with-identifier/` — project with a PRD/SPEC that can be resolved by identifier
- 🧪 `test/fixtures/flexible-input/with-branch/` — optional, may skip if git branch handling is too flaky

### Tier 2 tests (headless Claude)

- 🧪 `test/test-flexible-input-e2e.sh` — runs `/edikt:adr:new "Decide X using docs/specs/foo.md"` via `claude -p`, asserts the generated ADR contains content from `docs/specs/foo.md` that wouldn't come from the prose argument alone

### Verify

- 🔍 `/edikt:sdlc:plan` still works as before (same pattern, separate command, no regression)

**Commit:** `feat(v0.3.0 phase 4): flexible prose input with reference extraction`

---

## Phase 6 — Invariant Record story + experiments

**Scope:** Ship the Invariant Record coinage, the writing guide, the canonical examples as reference templates, the website pages, and the experiment infrastructure. This phase implements ADR-009 and puts the "edikt adapts to your project" story into the world with real content.

### New files (shipped with install.sh)

- ✨ `templates/examples/invariants/tenant-isolation.md` — canonical example, from `PROPOSAL-001-spec/canonical-examples/tenant-isolation.md`
- ✨ `templates/examples/invariants/money-precision.md` — canonical example
- ✨ `templates/examples/invariants/README.md` — index, points at writing guide
- ✨ `templates/examples/invariants/WRITING-GUIDE.md` — condensed version of the full guide (~500 words, shipped locally for users who don't want to visit the website)

### New website files

- ✨ `website/governance/invariant-records.md` — landing page for the Invariant Record concept, coinage, and template
- ✨ `website/governance/writing-invariants.md` — full writing guide (from `PROPOSAL-001-spec/writing-invariants-guide.md`)
- ✨ `website/governance/canonical-invariants/tenant-isolation.md` — annotated canonical example
- ✨ `website/governance/canonical-invariants/money-precision.md` — annotated canonical example
- ✏️ `website/.vitepress/config.ts` — add the new governance pages to the sidebar

### Experiment infrastructure

- ✨ `test/experiments/README.md` — orientation, points at `PROPOSAL-001-spec/experiments/`
- ✨ `test/experiments/run.sh` — orchestrator (per `PROPOSAL-001-spec/experiments/runner-spec.md`)
- ✨ `test/experiments/fixtures/01-multi-tenancy/` — Go fixture + prompt + assertion + invariant copy
- ✨ `test/experiments/fixtures/02-money-precision/` — Python fixture + prompt + assertion + invariant copy
- ✨ `test/experiments/fixtures/03-timezone-awareness/` — Python fixture + prompt + assertion + invariant copy
- ✨ `test/experiments/results/` — results directory (initially empty; populated during release validation)

### Install.sh changes

- ✏️ `install.sh` — ship `templates/examples/invariants/*` to `~/.edikt/templates/examples/invariants/`

### Test files

- 🧪 `test/test-invariant-record-story.sh` — contract tests:
  - ADR-009 exists and is Accepted
  - Canonical examples follow the template (6 sections, directives block, frontmatter)
  - Writing guide has the 5 qualities + 7 traps + 6 rewrites + self-test
  - `/edikt:invariant:new` uses the shipped template
  - `/edikt:init` offers the canonical examples under "Start fresh" for invariants
  - Reference templates at `~/.edikt/templates/examples/invariants/` after install
  - `test/experiments/` structure matches the runner-spec
  - Pre-registration files for the three experiments exist and are committed

### Verify (during release validation)

- 🔍 Run the three experiments via `./test/experiments/run.sh`
- 🔍 Commit results to `test/experiments/results/`
- 🔍 Decide: blog post? release notes framing? more experiments?
- 🔍 If experiments reveal a feature bug, fix and re-run before release

**Commit:** `feat(v0.3.0 phase 6): invariant record story + canonical examples + experiment infrastructure`

---

## Phase 5 — doctor + upgrade integration

**Scope:** make doctor and upgrade aware of project templates and the new schema.

### Modified files

- ✏️ `commands/doctor.md` — add checks for `.edikt/templates/<artifact>.md` presence (all three), `compile_schema_version` field in governance.md (ADR-007), three-list schema validity in artifact files
- ✏️ `commands/upgrade.md` — add Phase 5 logic:
  - Never overwrite `.edikt/templates/*.md`
  - Warn when project lacks templates post-v0.2.x upgrade
  - Prompt-at-point-of-need flow for first `<artifact>:new` after upgrade
  - Handle `compile_schema_version` drift (from v0.2.3)
  - Handle missing hash fields in artifacts (legacy v0.2.x)

### Test files

- 🧪 `test/test-v030-doctor-upgrade.sh` — contract tests: doctor checks template state for all three types, upgrade never modifies `.edikt/templates/`, upgrade documents grandfather flow

### Tier 2 tests (headless Claude)

- 🧪 Extend `test/test-install-e2e.sh` with a scenario: upgrade a project with a custom `.edikt/templates/adr.md`, verify it's preserved

### Verify

- 🔍 `test/test-v023-regressions.sh` (from v0.2.3) still passes
- 🔍 `/edikt:doctor` on the dogfood repo reports "using project template" for each artifact type

**Commit:** `feat(v0.3.0 phase 5): doctor + upgrade integration for project templates`

---

## Final v0.3.0 release commit

After all five phases have landed as separate commits, the release commit handles version bump + CHANGELOG:

### Modified files

- ✏️ `VERSION` — bump to `0.3.0`
- ✏️ `.edikt/config.yaml` — update `edikt_version: "0.3.0"`
- ✏️ `.claude/rules/governance.md` — run `/edikt:gov:compile` in the dogfood repo to regenerate with new schema (all three artifact types produce three-list blocks now)
- ✏️ `CHANGELOG.md` — comprehensive v0.3.0 entry describing all five phases, ADR-008, guideline:compile addition, migration notes
- ✏️ `docs/architecture/proposals/PROPOSAL-001-adapt-and-measure.md` — update status to "Part 1 shipped in v0.3.0", link to actual release

### Verify

- 🔍 `./test/run.sh` passes all suites
- 🔍 Pipeline green (all GitHub Actions pass)
- 🔍 ADR-008 is cited in dogfood ADRs for any new directives added during v0.3.0 development
- 🔍 Dogfood ADRs' directive blocks are regenerated using v0.3.0 compile (should produce three-list format)

### Release actions

- Create commit: `feat: edikt v0.3.0`
- Tag: `git tag -a v0.3.0 -m "edikt v0.3.0"`
- Push to origin
- `gh release create v0.3.0 --title "v0.3.0" --notes-file /tmp/v030-notes.md`

---

## What's NOT in v0.3.0

Confirming out-of-scope items so they don't creep in:

- ❌ Full Tier 2 LLM-as-judge test suite with trending, evaluator-tuning integration, rubric-versioning (v0.4.0+). v0.3.0 ships only 3 pre-registered experiments with grep-based assertions — not a general quality test infrastructure.
- ❌ `/edikt:<artifact>:migrate` command (v0.3.1+)
- ❌ Voice inference in Adapt mode (structural only per Q3)
- ❌ Semantic style extraction beyond structural
- ❌ `gov:compile` hash-based caching (wait for real performance data)
- ❌ GitHub Actions running Claude in CI (local-only; user's subscription per cost constraint)
- ❌ Auto-resolution of hand-edits in any form (interactive interview only)
- ❌ API-cost-incurring tests
- ❌ Shipped default templates that auto-install (init captures per-artifact choice)
- ❌ Guideline Record coinage (guidelines stay as "guideline"; only ADR-009 coins "Invariant Record")
- ❌ Formal statistical analysis of experiment results (publication-grade discipline, informal presentation)

Any PR that adds these is out of scope for v0.3.0.

---

## File count summary

| Phase | New files | Modified files | Test files | Fixtures |
|---|---|---|---|---|
| Cross-cutting | 11 (spec + ADRs + Invariant artifacts) | 1 (proposal doc) | — | — |
| Phase 1 | 8 (templates + command) | 4 (new.md × 3 + install.sh) | 4 | — |
| Phase 2 | — | 7 (compile.md × 4 + new.md × 3) | 4 + helper | 7+ |
| Phase 3 | — | 1 (init.md) | 1 | ~10 |
| Phase 4 | — | 3 (new.md × 3) | 1 | ~3 |
| Phase 5 | — | 2 (doctor.md + upgrade.md) | 1 | — |
| **Phase 6** | **~12 (canonical examples + website pages + experiment infra)** | **1 (install.sh)** | **1** | **3 experiment fixtures** |
| Release | — | 4 (VERSION, config, governance, CHANGELOG) | — | — |

**Total: ~30 new files, ~23 modified files, ~12 new test files, ~25+ fixture files.**

This is a substantial release. Plan accordingly. Phase 6 is new since the original scope — it carries the "Invariant Record" coinage story (ADR-009), the canonical examples, the writing guide on the website, and the experiment infrastructure. All of these were added during Part 2 design when we decided to bundle validation into v0.3.0 instead of deferring to v0.4.0.
