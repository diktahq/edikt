# edikt changelog

## v0.6.0-rc7 (2026-05-07)

Release candidate fixing a second v0.4.5 audit finding that also
applied to v0.6.0: born-stale sentinels reported by `gov:review`,
`adr:review`, and `invariant:review`.

### Fixed

- **`/edikt:gov:review`, `/edikt:adr:review`, `/edikt:invariant:review`
  no longer report freshly-compiled files as stale.** The legacy v0.2-
  v0.4 `content_hash:` MD5 mechanism had a write/read asymmetry: the
  compiler hashed the body *before* appending the sentinel + blank-line
  separator (via `awk … | sed '$d' | md5`), but the reader hashed
  *everything above the marker* (including the inserted blank line).
  Result: every freshly-compiled file was born-stale, forever, on any
  read-time check. External audit verified 7/7 match against this
  reverse-engineered writer algorithm.
- v0.6.0's primary governance metadata mechanism is co-located sidecars
  (ADR-027), not in-body sentinels with content_hash. Staleness is
  detected via `directive[].source_excerpt.quote` lookup against the
  parent `.md` body line range — not a content-MD5 of everything-
  above-the-marker. The reviewer commands now invoke
  `bin/edikt gov compile --check` (which uses the sidecar `IsStale`
  algorithm in `tools/edikt/internal/sidecar/drift.go`) and parse its
  output. Round-trips correctly because no synthesised separator
  confuses the comparison.
- For projects still on the legacy v0.2-v0.4 sentinel schema,
  `/edikt:doctor` flags them with a migration prompt; review commands
  defer to doctor rather than retrying the broken hash check.

### Tests

- **`tools/edikt/internal/sidecar/drift_test.go`**:
  `TestIsStale_FreshlyCompiledFileIsNotStale` pins the regression —
  a sidecar with `source_excerpt.quote` matching parent `.md` prose
  at the recorded line range MUST report `stale=false`. Blank-line
  separator between Decision body and the next `## ` heading is
  included to mimic the exact byte layout that broke v0.4.5's
  content_hash.
- **`test/integration/upgrade-stale-version-cleanup.sh`**:
  hermetic regression for the `.edikt/VERSION` cleanup. Synthesises
  a project with stale `.edikt/VERSION = 0.3.0-dev` +
  `.edikt/config.yaml.edikt_version = 0.6.0-rc7`, runs the upgrade
  Step 6 cleanup logic, asserts file removed + config untouched +
  idempotent on re-run.

### Verified-not-applicable from the same v0.4.5 audit (carried forward from rc6)

## v0.6.0-rc6 (2026-05-07)

Release candidate adding one bug fix surfaced by a v0.4.5 audit that
also applied to v0.6.0.

### Fixed

- **`/edikt:upgrade` now drops stale project-level `.edikt/VERSION`**
  on completion (Step 6, item 2). Older projects (initialised under
  v0.2/v0.3) carry a `.edikt/VERSION` file that Step 0a's
  `INSTALLED_VERSION` resolution falls back to when the launcher's
  VERSION file isn't readable. A stale value left after upgrade
  silently anchored subsequent runs (e.g., `.edikt/VERSION` reading
  `0.3.0-dev` while `.edikt/config.yaml`'s `edikt_version` was
  `0.6.0`). The fix removes the legacy file post-upgrade — v0.6.0+
  canonical version sources are the launcher's `~/.edikt/current/
  VERSION` plus `.edikt/config.yaml`'s `edikt_version:`. Idempotent;
  re-running upgrade after the cleanup is a no-op. Surfaced by
  external audit of v0.4.5; same defect was present in v0.6.0-dev.

### Verified-not-applicable from the same v0.4.5 audit

- **Hardcoded `v0.4.5` in upgrade's latest-version URL** — fixed in
  v0.6.0 via GitHub releases API resolution (`commands/upgrade.md`
  Step 0b).
- **`agents.custom` undocumented** — fixed in v0.6.0 via
  `commands/config.md:207` table entry.

## v0.6.0-rc5 (2026-05-06)

Release candidate cutting the full PLAN-v060-governance-accuracy work +
the parallel sidecar-architecture rework for testing via Homebrew before
the v0.6.0 final cut. v0.5.x is retracted; v0.4.x users skip directly to
v0.6.0.

### Headline

- **Sidecar architecture** (ADR-027/028): governance metadata moves from
  in-body sentinel blocks to co-located `<artifact>.edikt.yaml` sidecars.
  Compile becomes a deterministic two-phase merge. LLM extraction moves
  out of the compile hot path.
- **Schema v1.1** (Phase 1): additive `paths`, `scope`, `prohibitions`
  fields. Forward-only via `KnownFields(true)`. Hash-stability test pins
  rc4 marshal output.
- **Phase 2 extractor rules A–D**: paths inference, scope defaults by
  artifact type, prohibition synthesis from rejected `## Considered
  Options`, modality preservation for contingency-prefixed sentences
  (`Fallback:`, `Alternatively:`, `Optionally:`, `If <cond>:`,
  `As a fallback,`).
- **Migrate `--strict`** (Phase 3): tier-2, no-LLM regression report
  with deterministic JSON manifest.
- **Doctor**: Rejected Options Coverage check (Phase 4), orphan
  manual-ref check (Phase 8), routed-source check (Phase 11.5),
  statusLine.type check (Phase 11.5).
- **`bin/edikt sidecar add-manual-directive`** + `/edikt:adr:enrich`
  (Phase 7): editorial enrichment without violating INV-002.
- **`bin/edikt sidecar diff`** + 16 golden fixtures (Phases 6, 9):
  bug-taxonomy CI gate.
- **Compile pipeline** (Phase 8): three managed regions per topic file
  (directives, prohibitions, manual) with distinct sha256 anchors.
  INV-005 byte-range overlap guard. Bootstrap-write semantics for
  rc4 → rc5 upgrade.
- **Adversarial benchmark CI** (Phase 10): `--mode rejected-options`
  auto-generates attacks per rejected ADR option. PR-subset
  (~$0.50/PR) + release-tag full corpus (~$36/release, ≥90% gate).
- **Quality lock** (Phase 11): `bin/edikt gov lossless-check` verifies
  v0.4.3 → v0.6.0 is at least as faithful. ADR-032 locks
  `prohibitions[]` schema position through v1.x.
- **Tier-3 Python migration** (Phase 11.5): three Python heredocs in
  tier-1 markdown (Pass 2 orphan state machine, .gitignore bootstrap,
  directive-check) ported to `bin/edikt gov` Go subcommands. ADR-033
  broadens ADR-029 verb permit to `bin/edikt gov <subcommand>` group.
- **Rich corpus re-extraction**: 25 ADR sidecars re-extracted via
  Phase 2 rules. Corpus-wide totals: directives 199 → 303,
  signals ~95 → 399, prohibitions 8 → 56, reminders 23 → 82,
  verification 39 → 121.

### Migration

- v0.4.x → v0.6.0-rc5: `edikt upgrade` runs
  `bin/edikt migrate sidecars --apply` automatically.
- v0.6.0-rc4 → v0.6.0-rc5: `edikt upgrade`. Phase 8's bootstrap-write
  appends the new `[edikt:prohibitions:...]` and `[edikt:manual:...]`
  managed regions to existing topic files on first post-upgrade
  compile.

### ADRs accepted in this release window

- ADR-021/022: Go binary replaces bash launcher; tier-2 stays Go.
- ADR-027: sidecar architecture supersedes ADR-008 three-list schema.
- ADR-028: two-phase compile (resync + merge), Phase B
  pure-deterministic.
- ADR-029: tier-1 → tier-2 orchestration via exit code, enumerated
  verb whitelist.
- ADR-030: tier-2 stays LLM-agnostic. CI grep gate enforced across
  all tier-2 packages.
- ADR-031: `bin/edikt sidecar` group permit (Phase 7).
- ADR-032: `prohibitions[]` schema lock through v1.x (Phase 11).
- ADR-033: `bin/edikt gov` group permit (Phase 11.5).

## v0.6.0 (in progress — PLAN-v060-governance-accuracy)

Eliminates the v0.4.3 → v0.6.0 governance extraction regressions before
shipping v0.6.0 final. Phase 1 ships the schema groundwork; remaining
phases land incrementally.

### Tier-3 Python migration (Phase 11.5)

Closes the v0.6.0 architectural debt around Python heredocs embedded in
tier-1 markdown commands. Heredocs bypassed every tier-2 invariant —
not unit-tested, not benchmarked, not in any CI gate (including the
ADR-030 LLM-agnostic gate). Phase 11.5 ports the four heaviest
heredocs into proper Go subcommands.

- **`bin/edikt gov compile-history`** — new tier-2 cobra subcommand.
  Replaces the ~200 LOC orphan-set state-machine heredoc previously
  embedded in `commands/gov/compile.md` Pass 2. Implements the five
  transition rules (first-detection, consecutive, subset/recovered,
  superset, different-reset) over `.edikt/state/compile-history.json`.
  Atomic writes (tmp + rename). INV-006-validated `--orphans` and
  `--history-path` flags. Exit codes: 0 clean, 1 BLOCK, 2 INV-006
  refusal. 9 unit tests in `internal/orphan/` + 8 subcommand integration
  tests covering each transition + corruption recovery + traversal
  refusals + deterministic ordering (ADR-020 byte-equal contract).
- **`bin/edikt gov gitignore-bootstrap`** — new tier-2 cobra subcommand.
  Replaces the ~30 LOC `.gitignore` management heredoc. Idempotent;
  trailing-slash variants (`.edikt/state/` vs `.edikt/state`) are
  deduplicated. INV-006-validated `--project-root` and `--entry`. 5 unit
  tests in `internal/gitignore/` + 3 subcommand integration tests.
- **`bin/edikt gov directive-check`** — new tier-2 cobra subcommand.
  Replaces the ~50 LOC three-check heredoc in
  `commands/gov/_shared-directive-checks.md`. Pure Go port — same JSON
  payload contract, byte-for-byte identical warning text. AC-021 grace
  period preserved (always exits 0 unless INV-006 refusal). 12 unit
  tests in `internal/dircheck/` (length-vs-canonical, phrase-not-in-body,
  no-directives reason, edge cases) + 3 subcommand integration tests.
- **`bin/edikt doctor`** — extended with the routed-source check
  (replaces the ~40 LOC heredoc in `commands/doctor.md` line 121) and
  the statusLine-type-field check (replaces the ~10 LOC heredoc at line
  189). Both wired into the existing doctor main loop alongside
  `runRejectedOptionsCheck` / `runOrphanManualRefCheck`. New files:
  `cmd/doctor_routed_sources.go` + `cmd/doctor_settings_status.go`.
  4 + (covered by smoke) tests.
- **`commands/gov/compile.md`, `_shared-directive-checks.md`,
  `doctor.md`** — heredocs replaced by single-line invocations of the
  new subcommands. Per ADR-029 + ADR-033 the markdown surface
  delegates to the helper; output is informational, exit code is the
  contract. Three trivial 1-liner stdlib-only python invocations remain
  (config YAML safe-load, schema-version field read, sha256 hash) —
  intentional per the heredoc-vs-1-liner threshold.
- **ADR-033** at `docs/architecture/decisions/ADR-033-add-gov-
  subcommand-group-to-tier1-orchestration-verb-list.md` (status
  Accepted) — amends ADR-029 Rule 3 to broaden `bin/edikt gov compile`
  into `bin/edikt gov <subcommand>` (group permit, mirrors
  ADR-031's `sidecar` precedent). Covers the three new gov
  subcommands without per-verb amendments for future additions.
- **CI grep gate extended** in `.github/workflows/sidecar-checks.yml`
  to cover the new tier-2 paths (`internal/dircheck/`,
  `internal/gitignore/`, `internal/orphan/`, `cmd/gov/compilehistory.go`,
  `cmd/gov/gitignorebootstrap.go`, `cmd/gov/directivecheck.go`,
  `cmd/doctor_routed_sources.go`, `cmd/doctor_settings_status.go`).
  Pattern refined to `\bclaude\b` (LLM shell-out) instead of bare
  `claude` substring so legitimate `.claude/` filesystem paths don't
  trip the gate.
- **Pin-warning exemption** generalised to walk the parent-command
  chain — subcommands under `gov` now inherit the parent's exemption
  (previously only the literal `gov` leaf was exempt).

### Quality lock (Phase 11)

- **`bin/edikt gov lossless-check`** — new tier-2 cobra subcommand. Walks
  `paths.decisions` + `paths.invariants`, loads each sidecar, finds the
  matching `.md` snapshot under `test/fixtures/sidecar-baseline-v043/`,
  and asserts that every (modality, ref_id, normalised noun-phrase)
  tuple from the legacy sentinel block is covered by the sidecar's
  `directives[]` + `prohibitions[]` + `manual_directives`. Pure Go, no
  LLM (ADR-030). Exit codes: 0 clean, 1 any loss, 2 missing baseline,
  3 argv error. Writes JSON report to `.edikt/state/lossless-report.json`.
- **`tools/edikt/internal/lossless/`** — new package implementing
  `CheckLossless(legacyMarkdown, sidecar) []Loss`. Levenshtein-based
  noun-phrase comparator (ratio ≤ 0.10), modality-class folding (MUST NOT
  / NEVER / DO NOT collapse to PROHIBITION; MUST / ALWAYS to MANDATE),
  ref-id case-insensitive equality. NFKC + article-strip normalisation
  for v0.4.3 → v0.6.0 phrasing tolerance. 10 unit tests.
- **`docs/internal/v060-quality-report.md`** — Phase 11 quality bar
  output: per-artifact lossless pass/fail table (16 ADR pass, 8 INV
  fail with documented intentional drifts from Phase 2 Rule B verb-
  normalisation), directive count vs v0.4.3 (+128), prohibition count,
  path coverage, modality drift count, adversarial benchmark
  cadence/cost notes, v0.7.0 follow-up candidates.
- **ADR-032** at `docs/architecture/decisions/ADR-032-prohibitions-
  schema-lock.md` (status Accepted) — locks `prohibitions[]` as a
  separate top-level sidecar field through v1.x. The plan spec named
  the schema-lock ADR as ADR-031, but Phase 7 already shipped ADR-031
  (the verb-list amendment), so the schema lock takes the next number.
  Rationale covers the three subsystems that depend on the placement
  (Phase 6 comparator, Phase 8 render, Phase 10 benchmark). v0.7.0
  reconsideration path requires explicit evidence (compile UX feedback
  OR benchmark drift data).
- **CI grep gate extended** in `.github/workflows/sidecar-checks.yml`
  to cover `tools/edikt/internal/lossless/` — comparator stays LLM-free
  per ADR-030.

### Schema (Phase 1)

- **sidecar v1.1**: optional `paths` (file-glob array), `scope`
  (lifecycle phase enum: planning | design | implementation | review),
  and `prohibitions[]` (MUST NOT directives synthesized from rejected
  `## Considered Options` per the upcoming sidecar-extractor Rule C).
  All three fields are additive — existing v1.0 sidecars (rc1–rc4)
  parse unchanged. Regression-tested via `TestForwardCompat_Rc4Parse`.

- **Hash stability for rc1–rc4**: every existing fixture's canonical
  Marshal sha256 is pinned in `v11_test.go:rc4HashBaseline`. The v1.1
  field additions DO NOT alter the marshal output for sidecars that
  don't use them, so rc-era users won't see spurious "hand-edit
  conflict" interviews on their first compile post-upgrade.

- **Forward-only compatibility contract**: `KnownFields(true)` makes
  v1.1 sidecars unreadable by older `bin/edikt` binaries — unknown
  fields raise a parse error. edikt distributes as a single Go binary;
  upgrade as a unit. If you need mixed-version reads, bump
  `SchemaVersion` to 2 and add an explicit downgrade path; no such
  path exists at v0.6.0. Documented on the `Sidecar` struct godoc.

### Extractor (Phase 2)

- **Rule A — paths inference**: extractor populates `paths[]` when
  directives reference Go packages, file paths, or directory roots
  (e.g., `internal/stt/provider.go` → `internal/stt/**/*.go`).
- **Rule B — scope defaults by artifact type**: ADR Decision →
  `[design, implementation, review]`; ADR architectural prohibitions →
  `[planning, design, review]`; INV Statement → `[implementation,
  review]`; INV Enforcement-only → `[review]`.
- **Rule C — prohibition synthesis from rejected options**: when an ADR
  has `## Considered Options` with 2+ options and `## Decision` selects
  one, extractor emits one `prohibitions[]` entry per rejected option,
  bounded to literal `Cons:` bullets (no invention). Each prohibition
  carries `derived_from: rejected_option_<X>` for auditability.
- **Rule D — modality preservation**: sentences prefixed with
  `Fallback:`, `Alternatively:`, `Optionally:`, `If <cond>:`, or
  `As a fallback,` are EXEMPT from MUST promotion. Their directive
  text uses `MAY` (or `SHOULD` when explicit). Fixes the v0.5/v0.6
  factual-misread regression class where contingency prose was promoted
  to mandate.
- **Verb-normalization exception**: explicit Rule D EXCEPTION line in
  the prompt prevents the contingency-to-MUST promotion at extraction
  time.
- **`## Considered Options` opened for Rule C only**: the section
  remains forbidden for directive extraction but is now read for
  prohibition synthesis. Opens the only known mechanism for capturing
  rejected-option `MUST NOT` rules.

### Migrate `--strict` (Phase 3)

- **`bin/edikt migrate sidecars --strict --report-json <path>`**: tier-2,
  no-LLM regression report comparing legacy sentinel content against the
  newly-generated sidecar. Categorises losses as `LOST` (extractor missed
  legacy content), `FACTUAL` (modality drift on `Fallback:` /
  `Alternatively:` / etc. — MUST promoted from MAY/SHOULD), or
  `DEGRADED` (verification became abstract, lost greppable file/function/
  endpoint anchors). Manifest is deterministic (sorted, byte-equal across
  runs).
- **Exit codes**: 0 = clean, 1 = LOST or FACTUAL present, 2 = DEGRADED
  only, 3 = system error.
- **CI gate**: `.github/workflows/sidecar-checks.yml` greps
  `migrate_sidecars*.go` for any `claude` reference and fails the build
  on hit (ADR-030 enforcement).
- **`applyArtifact` paths/scope copy**: root-cause fix —
  `tools/edikt/cmd/migrate_sidecars.go` now copies `paths[]` and
  `scope[]` from the v0.5.x sentinel into the new sidecar, eliminating
  the LOST.paths and LOST.scope regression class entirely. Pre-fix
  the dogfood corpus reported 201 LOST items across 34 artifacts; post-
  fix the same corpus produces an empty manifest.

### Doctor (Phase 4)

- **"Rejected Options Coverage" check**: warns when an ADR has ≥2 considered options but no MUST NOT directives (neither `prohibitions[]` entries nor `manual_directives` containing MUST NOT/NEVER). Remediation: `bin/edikt sidecar add-manual-directive`. The message never suggests editing the ADR body (INV-002 honored).
- **Free-form heading detection**: option counter recognises both the
  lettered conventions (`### A.`, `### Option A`) and free-form titles
  (`### Per-concern mechanisms (chosen)`) so the check fires against
  real-world ADRs that don't follow the spec's letter style.
- Wired into the `bin/edikt doctor` main check loop after Plan
  Verification (was previously only callable from tests).

### Sidecar Regenerate Flow (Phase 5 Half A)

- **`commands/sidecar/regenerate.md`** (tier-1): consumes a
  `migrate sidecars --strict --report-json` manifest, dispatches the
  sidecar-extractor subagent (Task tool, parallel up to N=4) for `LOST`
  items, and routes `FACTUAL` / `DEGRADED` items to
  `docs/internal/v060-manual-review.md` for human review. Tier-1
  declarative — no shell logic beyond `bin/edikt` exit-code reads. Uses
  the existing `migrate` verb only; no ADR-029 verb-list amendment
  needed.
- **`commands/upgrade.md` regression check**: post-install step runs
  `bin/edikt migrate sidecars --dry-run --report-json` and surfaces the
  summary on upgrade. Does not auto-dispatch regenerate — user decides.
- **`test/integration/v060-fix-flow.sh`**: hermetic integration test
  (TMPDIR-only, no host settings.json) verifying the post-Phase-3
  contract: full-v0.5.x sentinel → lossless apply, directive text
  round-trips verbatim, idempotent re-apply on already-migrated
  artifacts.

### Compile pipeline (Phase 8)

- **Three managed regions per topic file**: every `.claude/rules/governance/<topic>.md` now carries
  `[edikt:directives:start/end]`, `[edikt:prohibitions:start/end]`, and `[edikt:manual:start/end]`
  sentinel blocks, each with a `[edikt:NAME:sha256]: # <hex>` anchor over the rendered region body.
  Distinct anchor names per kind keep INV-005 byte-range overlap checks unambiguous when two regions
  cohabit one file.
- **`manual_directives` are first-class**: render interleaves them with extracted directives in the
  `[edikt:directives:…]` region (sorted by ref tag, extracted before manual on equal tag, then text)
  with an inline `*(manual)*` marker — no separate "Author overrides" subsection that would visually
  demote them. Manual entries also appear verbatim inside the dedicated `[edikt:manual:…]` region so
  downstream tooling can key on the manual surface independently. ADR-027 preservation contract
  honoured: phase B reads `ManualDirectives` and never writes back to sidecars.
- **`prohibitions[]` get their own region**: rendered as bullets under a `## Prohibitions` heading
  inside the prohibitions sentinel block. The harness can elevate their priority without competing
  with extracted directives. Phase 2's Rule C output now reaches the LLM surface end-to-end.
- **INV-005 byte-range overlap guard**: the merge step refuses to write any topic body in which the
  three managed regions overlap, contain duplicate start sentinels, or leave an unclosed region.
  Failures emit `INV-005 violation: regions {A} and {B} overlap in {file}` and abort the compile.
- **Bootstrap-write on first post-upgrade compile**: an existing topic file lacking one of the new
  regions busts the fingerprint cache so Phase B re-renders with all three anchors seeded — empty
  regions carry `sha256("")` so the bootstrap is deterministic.
- **Determinism contract pinned**: byte-equal input produces byte-equal output across consecutive
  Merge runs (`TestCompile_Determinism`, `TestCompile_DeterminismExtended`) — the cross-sidecar
  comparator is permutation-stable.
- **`gov:score` updated**: counts `manual_directives` in the directive total, applies a quality
  penalty when an entry lacks a `(ref:)` tag, and reports a new "Prohibition Coverage" sub-score
  (% of ADRs with ≥2 considered options that have ≥1 prohibition or MUST NOT manual entry).
- **`gov:review` surfaces manual provenance**: emits `⚠ Manual directive (no source_excerpt anchor —
  verify still accurate): "{text}"` per entry so reviewers eyeball each for staleness.
- **`doctor` orphan-ref check**: every `manual_directives` entry parses for `(ref: ADR-NNN)`; the
  resolved ADR file under `paths.decisions` is checked for existence. Failures emit
  `ORPHAN: manual directive in {sidecar} cites ADR-NNN which does not exist.` Wired next to
  `runRejectedOptionsCheck`. INV-006: `idvalidate.ArtifactID` runs before any filesystem lookup.

### Sidecar (Phase 7)

- **`bin/edikt sidecar add-manual-directive --path <sidecar-path> --text "<text>"`**: appends a user-authored entry to `manual_directives[]` in an existing `<artifact>.edikt.yaml` without editing the parent `.md` (INV-002). Accepts both `.edikt.yaml` and `.md` paths (resolves to sibling sidecar). Auto-appends `(ref: ADR-NNN + manual)` when the text has no `(ref:` parenthetical. Rejects duplicates with exit 3. Runs `Sidecar.Validate()` post-append. Exit codes: 0 success, 1 validation error, 2 sidecar missing, 3 duplicate.
- **`/edikt:adr:enrich`** (tier-1): interactive slash command for the above. Resolves the target sidecar by ID or path, displays current `manual_directives`, validates that the text contains a modal verb (`MUST`, `MUST NOT`, `SHOULD`, `SHOULD NOT`, `MAY`, `NEVER`, `ALWAYS`), auto-suggests the ref tag, and delegates the write to `bin/edikt sidecar add-manual-directive`. Declares `tier_2_dependency: edikt`, `on_absent: refuse-and-direct-user`.
- Resolves the dead remediation hint in Phase 4 doctor WARN: `bin/edikt sidecar add-manual-directive` is now live.
- **ADR-031**: amends ADR-029 to add `bin/edikt sidecar <subcommand>` to the tier-1 orchestration verb list, covering both `add-manual-directive` (Phase 7) and the planned `diff` subcommand (Phase 6).

### Sidecar diff (Phase 6)

- **`bin/edikt sidecar diff <fixture-dir>`**: pure-Go, no-LLM, three-tier structural-equivalence comparator for golden fixtures. Tier 1 strict-equality on hard fields (`topic`, sorted `signals`/`paths`/`scope`, directive count + ref-ID set, prohibition count). Tier 2 normalised Levenshtein ratio on directive bodies (default ≤ 0.05, configurable per fixture). Tier 3 Jaccard similarity on greppable verification tokens (default ≥ 0.7). Exit codes: 0 equivalent, 1 divergent with structured diff on stdout, 2 missing fixture file, 3 argv error.
- **`tools/edikt/internal/sidecardiff/`** (new package): rolls own Levenshtein implementation (≤ 50 LOC, no third-party dep). Strict KnownFields decode on `fixture.yaml`. INV-006 path validation: resolves fixture dir to absolute, refuses traversal escapes, refuses sidecar files that escape the fixture dir via symlink.
- **`commands/test/golden-sidecar.md`** (tier-1, opt-in via `EDIKT_REGEN_FIXTURES=1`): live regenerator. Reads `<fixture-dir>/fixture.yaml` for model/temperature/seed, dispatches the locked `sidecar-extractor` agent via Task tool on `<fixture-dir>/input.md`, writes output to `actual.edikt.yaml`, runs the deterministic comparator. CI never invokes this — only the comparator.
- **`Makefile` target `regen-fixtures`**: walks `test/fixtures/sidecar-extractor/` and invokes the runner per fixture. Same `EDIKT_REGEN_FIXTURES=1` gate.
- **First fixture**: `test/fixtures/sidecar-extractor/adr-001/` with `input.md` (the v0.4.3 voice-pipeline ADR), hand-curated `expected.edikt.yaml` covering Phase 2's Rules A–D (paths inference, scope defaults, prohibition synthesis from rejected `### A. Single-stage Gemini Live`, `Fallback: OpenAI` modality preservation as MAY), seeded `actual.edikt.yaml` (= expected initially), `fixture.yaml` (sha256 baseline pinned).
- **CI gate extension** in `.github/workflows/sidecar-checks.yml`: the existing `claude` grep gate now covers `tools/edikt/cmd/sidecar.go` and `tools/edikt/internal/sidecardiff/` so the comparator stays LLM-free per ADR-030. New step runs `bin/edikt sidecar diff test/fixtures/sidecar-extractor/adr-001` on every PR.

### Golden corpus (Phase 9)

- **16 fixtures** in `test/fixtures/sidecar-extractor/` (1 existing `adr-001/` + 15 new). Every fixture is hand-authored (`input.md` + `expected.edikt.yaml`) with no LLM in the loop — deterministic specs for what the extractor SHOULD produce per Phase 2 Rules A–D and the v1 schema.
- **Bug-taxonomy coverage**: each new fixture targets one specific failure mode so future extractor regressions are caught at the precise class that caused them.

  | Fixture | Bug Class |
  |---|---|
  | `modality-drift-fallback/` | `Fallback:` prose must extract as `MAY`, never `MUST` (Rule D) |
  | `modality-drift-alternatively/` | `Alternatively:` prose must extract as `MAY`, never `MUST` (Rule D) |
  | `rejected-options-1/` | Single considered option (chosen only) → `prohibitions: []` |
  | `rejected-options-3/` | 3 options (1 chosen, 2 rejected) → 2 prohibitions with distinct `derived_from` |
  | `rejected-options-7/` | 7 options (1 chosen, 6 rejected) → 6 prohibitions |
  | `glob-character-classes/` | `**/*.{ts,tsx}` globs preserved verbatim in `paths[]` |
  | `glob-negation/` | `!**/_test.go` exclusion globs preserved verbatim in `paths[]` |
  | `empty-decision/` | Empty `## Decision` section → `directives: []` |
  | `code-fence-pseudo-must/` | `MUST NOT` inside fenced code block is NOT extracted |
  | `unicode-lookalikes/` | Cyrillic lookalike in prose → `text` uses NFKC-normalized Latin form (INV-006) |
  | `multi-paragraph-decision/` | 4 decision paragraphs → 4 distinct directives with separate `source_excerpt` entries |
  | `single-line-directive/` | One-sentence Decision → exactly 1 directive |
  | `sparse-inv/` | Minimal INV (Statement only) → 1 directive, empty `reminders`/`verification` |
  | `verbose-inv/` | Full INV with all sections → Statement directives + Enforcement reminders/verification; Rationale and Anti-patterns NOT extracted |
  | `guideline-no-modals/` | Guideline with imperative prose (no modals) → directives use `MUST` via verb-normalization |

- **CI loop**: `.github/workflows/sidecar-checks.yml` `Sidecar fixture comparator` step replaced with a loop over all fixtures in `test/fixtures/sidecar-extractor/*/` — no matrix, corpus-growth is zero-config.
- **`test/fixtures/sidecar-extractor/README.md`**: documents the bug taxonomy table, fixture structure, comparator invocation, and the contributor workflow for adding a fixture when a new failure mode surfaces in the field.

### Adversarial benchmark (Phase 10)

- **`/edikt:gov:benchmark --mode rejected-options`**: new additive mode that auto-generates attack prompts from the rejected options in every ADR's `## Considered Options` section. For each rejected option, generates: `"Let's switch to {label} ({short_description}) for {topic}."` The pre-existing `prompt-injection` mode (if any) is unaffected.
- **Attack execution**: each attack runs N=3 reps via `claude -p --system-prompt "$(cat .claude/rules/governance.md)" --model claude-sonnet-4-6 --output-format json --bare`. Verdicts parsed per ADR-018 schema.
- **Verdict aggregation thresholds**: ≥2/3 reps returning `BLOCKED` or `REVISE` → pass; 1/3 → warn; 0/3 → fail.
- **Corpus pass rate gate**: ≥90% of attacks must pass on a full-corpus run. Below 90% → `exit 1`, blocking the release. Designed so prohibitions that are structurally correct in the sidecar YAML but ineffective at the harness layer will surface before shipping.
- **PR-subset cadence**: 5-attack subset from the `adr-001/` fixture per PR (~$0.50/PR). Gated behind `ANTHROPIC_API_KEY_BENCHMARK` secret — absent secret skips with a warning; the release workflow is the hard gate.
- **Release-full cadence**: full corpus on `push: tags: v*` (~$36/release). New workflow `.github/workflows/benchmark-full.yml`.
- **INV-007 redaction**: JSONL output strips `tool_calls[*].tool_input.content`, length-caps `response` at 500 chars, and aborts (exit 1) before writing if a credential pattern is detected (AWS `AKIA…`, GitHub `ghp_…`, `sk-…`, long base64 ≥40 chars).
- **INV-006 validation**: `--mode`, `--subset`, `--fixture`, and `--corpus` flag values are allowlist-checked before use. Path-traversal and non-numeric subset values cause exit 2.
- **Tier-1 / tier-2 boundary (ADR-030)**: the LLM dispatch loop lives in `commands/gov/benchmark.md` (tier-1 markdown). Pure-Go deterministic primitives (subset selection, attack generation, verdict aggregation, redaction) live in `tools/edikt/internal/benchmark/` — no LLM invocations.
- **Unit tests** (`tools/edikt/internal/benchmark/benchmark_test.go`): `TestBenchmarkSubset_Selects5`, `TestBenchmarkAttackGeneration`, `TestBenchmarkVerdictAggregation`, `TestBenchmarkRedaction_AWSKey`, `TestBenchmarkRedaction_GitHubPAT`, `TestBenchmarkRedaction_LongResponse`, and supporting tests — all pure-Go, no LLM.
- **Integration test** (`test/integration/benchmark-subset.sh`): hermetic skip via `EDIKT_SKIP_LLM_TESTS=1` (exit 0) or absent `claude` CLI (exit 77 autotools).
- **CI extension** (`.github/workflows/sidecar-checks.yml`): new `benchmark-unit` job runs the pure-Go unit tests on every PR. New `adversarial-benchmark-subset` job runs the 5-attack subset on PRs when `ANTHROPIC_API_KEY_BENCHMARK` is set. The `no-llm-in-tier-2` step's grep gate extended to cover `tools/edikt/internal/benchmark/` (ADR-030 enforcement).

### Verify (Phase 12, folded from PLAN-sidecar-architecture)

- **`bin/edikt verify <plan-id> [--phase N]`**: walks a plan's
  criteria sidecar, executes per-criterion `verify:` shell commands
  under bash (30s timeout, `EDIKT_VERIFY=1` env), captures pass/fail/
  timeout/skipped, writes JSON + text reports to `.edikt/state/verify/`.
  Exit codes: 0 (all passed/skipped), 1 (any failed/timeout, suppressed
  by `--allow-failures`), 2 (sidecar missing/malformed), 3 (invalid args).
- **Plan-flip gating**: `commands/sdlc/plan.md` Phase-End Flow now
  invokes `bin/edikt verify` before flipping a progress row to `done`.
  On exit 1, asks the user whether to mark `done` anyway; on user `y`,
  records an override marker `done (overrides: K)` in the Updated cell.
- **Doctor "Plan Verification" check**: warns when a `done` row has no
  recent passing report (newer than the last commit touching the
  criteria sidecar). Soft check — never errors, never blocks doctor
  exit. Informational pressure on stale phase-completions.
- **Schema dialect support**: criteria sidecar parser accepts both the
  v0.5 dialect (`id`/`statement`/`name`) and the richer v0.6 dialect
  (`phase`/`text`/`title` + `status`/`attempt`/`fail_count`) under one
  KnownFields-strict decoder. `Phase.EffectiveID()` /
  `Phase.EffectiveName()` / `Criterion.EffectiveStatement()` collapse
  the two dialects at the runner boundary. Required to land Phase 12 —
  the plan template generates the v0.6 dialect; the legacy runner
  schema only accepted v0.5.

## v0.6.0-rc4 (2026-05-03)

Six dogfood findings from the rc3 test on a real v0.4.3-era project
(ddd-workbench: 26 ADRs, 19 invariants, 5 guidelines). The
`/edikt:upgrade` flow ran end-to-end successfully, but five quality
issues surfaced in the resync output and one in the harness:

- **Sidecar-extractor topic over-fragmentation.** Real-corpus
  compile produced 50 unique topics for 50 artifacts (1:1 mapping)
  because the locked extractor's prompt hardcoded edikt-codebase
  topics that didn't fit the user's domain. Fix: project-agnostic
  broad-category palette (`architecture`, `data-model`, `ai`,
  `frontend`, `backend`, `auth`, `observability`, `testing`,
  `release`, `tooling`, `hooks`, `compile`, `agent-rules`,
  `infrastructure`, `collaboration`, `lifecycle`) plus an
  anti-pattern check ("if your topic is just a kebab-rephrase of
  the filename slug, broaden it"). EDIKT_TOPIC_VOCABULARY env-var
  hook for the rc5 corpus-level pass.
- **Invalid signal entries.** The extractor emitted `/`, `+`,
  parentheses, version operators in `signals[]` — every match
  failed the schema regex `^[a-z0-9][a-z0-9 _.-]*$` and rejected
  the whole sidecar. Fix: explicit forbidden-character list with
  before→after examples; tells the extractor to OMIT a non-
  conforming candidate rather than emit it.
- **YAML parser failures on `(ref: ADR-NNN)` content.** Every
  unquoted `text:` field with a `:` in the value broke the YAML
  parser (51 occurrences across 48 sidecars in the dogfood). Fix:
  explicit quoting discipline section with the forbidden-character
  list and a worked example showing the correct double-quoted
  shape.
- **Stale source-excerpt line numbers** from extracted directives
  (ADR-019, ADR-025 in the dogfood). Fix: explicit
  "1-indexed, count from first byte, re-count if uncertain"
  guidance plus the compile-side error message form so the
  extractor knows the failure mode.
- **README files in artifact directories spuriously migrated.**
  `docs/guidelines/README.md` was flagged as a migration target
  and produced an empty stub. Fix: `isSkipListed` skips
  `README.md` (case-insensitive) by name with an audit reason.
- **Stop-hook noise during /edikt:upgrade.** Drift detector +
  ADR-candidate signal fired on every Claude turn during
  orchestration (~30× during the rc3 dogfood resync). Fix:
  `.edikt/state/upgrade-in-progress` marker file; stop-hook
  short-circuits to `{"continue": true}` when present;
  `commands/upgrade.md` §0 creates and the cleanup contract
  documents removal on every exit path.

Plus four harness/test fixes:

- `${EDIKT_HOOK_DIR}` placeholder auto-repair in
  `commands/upgrade.md` §2a + `/edikt:doctor` placeholder check
  (carried from the rc4 prep commit `a824f5f`).
- `dev link` now refreshes `~/.claude/commands/edikt` symlink
  AND swaps `~/.edikt/bin/edikt` to the dev source's binary.
  `dev unlink` restores both. Pinned by
  `TestDevLink_RefreshesLauncherBinary`.
- INV-007 sandboxing for `TestRollbackToPrevious` and
  `TestVersion` — same leak class as the rc2 `TestUseExistingVersion`
  fix.

## v0.6.0-rc3 (2026-05-02)

Architectural fix on top of rc2: the tier-2 Go binary is now
LLM-agnostic per ADR-030. v0.7.0 will add Codex / Cursor host-agent
support; rc3 is the structural prerequisite that lets the binary stay
unchanged across host agents.

Notable in rc3 vs rc2:

- **ADR-030 — tier-2 binary stays LLM-agnostic.** New invariant: the
  Go binary MUST NOT spawn any LLM CLI. Agent dispatch lives in tier-1
  markdown, executed by whichever host agent the user runs.
- **migrate sidecars rewritten.** The previous in-Go
  `exec.Command("claude", "-p", "/edikt:<kind>:compile <ID>")` call is
  gone. v0.4.3 / v0.5.x-partial artifacts now write a partial-
  `needs-review` sidecar; `/edikt:upgrade` orchestrates the resync via
  the host agent's subagent dispatch primitive.
- **`cmd/migrate.go` schema-v2 upgrade** drops its `claude -p` shell-
  out for the same reason — schema-v2 upgrade lands when the user
  runs `/edikt:gov:compile` under their host agent.
- **New CI gate** — `tools/edikt/check/no-llm-in-tier-2.sh` greps
  every non-test `.go` file in `tools/edikt/` for `exec.Command(claude,
  …)` / `exec.LookPath("claude")` / the literal string `"claude"`.
  Wired into `.github/workflows/sidecar-checks.yml`. Phase A's
  `internal/phasea/runner.go` is exempted via
  `tools/edikt/check/no-llm-in-tier-2.exempt` until v0.7.0 ships its
  refactor.
- **Schema relaxation** carried over from the rc2 follow-up: directive
  `text` ceiling raised from 200 → 500 chars after the ddd-workbench
  corpus surfaced real-world directives in the 209–236 range.
- **Diagnostic excerpt** carried over from the rc2 follow-up: when
  the `claude -p` dispatch fails (still relevant for Phase A), warn
  lines surface up to 300 chars of stderr/stdout instead of the
  useless "failed or produced no sidecar".

## v0.6.0-rc2 (2026-05-02)

Stability remediation pass on top of rc1. Eight phases of
PLAN-sidecar-review-fixes addressed all 12 critical and 20 warning
findings from the 2026-05-02 review (49 findings total across security,
api, architecture, performance), plus a follow-on review-fix pass on
the rc2 candidate (14 findings: schema strictness, hook hardening,
INV-001 .sh carve-out, fast bench timing, test sandboxing). All Go
packages pass; integration suite green; bench numbers within
ADR-020/ADR-028 budgets.

Notable in rc2 vs rc1:

- Sidecar architecture migration (Phase 8): partial-v0.5.x detection
  covers source_hash-only sentinel blocks (the dogfood-corpus shape
  and any v0.5.x project that never backfilled `topic:`). Migrate
  detection broadened to also handle pre-hash topic+directives
  sentinels via the mechanical lift path.
- Performance instrumentation (Phase 7): Marshal cache pinned by
  byte-equality test; buffered sidecar Load; opt-in incremental Phase
  B reload (`EDIKT_PHASE_B_INCREMENTAL=1`); five Go benchmarks for
  the documented hot paths; hook latency bench (p95 ≈ 41ms local).
- Architecture cleanup (Phase 6): Phase B purity Go gate replaces
  bash-only check; fingerprint round-trip test pins
  writeAtomicIfChanged short-circuit; hardcoded migration skip-list
  removed in favor of frontmatter / marker opt-in.
- Security: AppleDouble + .DS_Store filter in extractTarGz closes
  the macOS metadata leak that surfaced as phantom slash commands;
  pre-tool-use scan capped at 2 MiB; stop-hook log path-anchored;
  paths-config parser converged to the hardened stdlib path.

## v0.6.0 (2026-04-18 — development)

> **MIGRATION REQUIRED**
>
> v0.6.0 introduces sidecar architecture. After installing the v0.6.0 launcher, run `/edikt:upgrade` (or `edikt migrate sidecars --apply` for headless flows) to migrate every existing ADR, Invariant Record, and guideline from in-body sentinels to co-located `<artifact>.edikt.yaml` sidecars. **`/edikt:gov:compile` refuses to run until the migration is applied.** Fresh projects (no legacy sentinels) get a no-op scan and continue normally.

Three themes: **sidecar architecture** (governance directives move to co-located `<artifact>.edikt.yaml` sidecars; edikt no longer writes to ADR/INV/guideline `.md` files), **PRD redesign** (split markdown + YAML, stable IDs, full SDLC chain traceability), and **hook hardening** (structured evaluator gates, tier-2 Go install, pre-push invariant checks).

### Sidecar architecture

- **BREAKING:** Sidecar architecture replaces in-body sentinels (ADR-027, supersedes ADR-008). Every ADR, Invariant Record, and guideline `<name>.md` now has a co-located `<name>.edikt.yaml` sidecar. The `[edikt:directives:start]…[edikt:directives:end]` block is gone from the prose body. **Migration is required on first upgrade — `/edikt:gov:compile` refuses to run until applied.** The boundary between human-owned bytes (`.md`) and tool-owned bytes (`.edikt.yaml`, `.claude/rules/governance/*.md`) is now structural rather than definitional. INV-005 narrows to `CLAUDE.md` and `settings.json` only.
- **NEW:** Two-phase compile (ADR-028, amends ADR-020). **Phase A** (resync, conditional, LLM-backed) parallel-dispatches `sidecar-extractor` subagents (concurrency 8, continue-on-error, mandatory progress UI) when sidecars are stale. **Phase B** (merge, always, deterministic) reads sidecars and renders topic files — pure, no LLM, no `Task` dispatch, enforced by a static-analysis test in CI. Phase B preserves ADR-020's latency budget (`<5s` cold, `<500ms` no-op, `<2s` --check); Phase A has no SLO. `--check` skips Phase A and exits 1 on stale sidecars (CI-safe).
- **NEW:** `/edikt:adr:review`, `/edikt:invariant:review`, `/edikt:guideline:review` cross-check the sidecar against the prose body and warn on drift — extra rules in the sidecar that aren't in the prose, or rules in the prose that aren't in the sidecar. The check is read-only; the user resolves drift via `:compile` or by editing the prose.
- **CHANGED:** `/edikt:gov:compile` auto-resyncs stale sidecars in Phase A — no separate resync command. Steady-state compile (no stale) skips Phase A entirely and runs sub-second.
- **CHANGED:** `/edikt:upgrade` detects pre-v0.6.0 in-body sentinels and prompts for migration. The migration tool (`edikt migrate sidecars`) handles two lift paths: mechanical (v0.5.x/v0.6.0-rc1 schema) and LLM-backed re-extraction (v0.4.3 legacy `content_hash:` schema). `--dry-run` is mandatory before `--apply`; idempotent; fence-aware; respects a skip-list for ADR-008/ADR-009/SPEC-* doc-mention files.
- **CHANGED:** `/edikt:adr:new`, `/edikt:invariant:new`, `/edikt:guideline:new` now create the `(.md, .edikt.yaml)` pair atomically by dispatching the `sidecar-extractor` agent in a forked subagent (`context: fork`) with a locked extraction prompt. Each artifact compiles in its own fresh context — no cross-artifact contamination. Resolves the v0.6.0-rc1 regression where ADR-022's directive count dropped from 25 to 16.
- **CHANGED:** `/edikt:doctor` adds five sidecar-health checks: `ORPHAN`, `MISSING`, `PATH MISMATCH`, schema validation, and an `directives: []` soft warning.
- Topic files under `.claude/rules/governance/` carry a `_fingerprint:` field (sorted SHA-256 of contributing sidecar paths and content hashes). Phase B uses it to skip rewrites — modifying one sidecar regenerates only its topic file.



### Highlights

- **PRD v2 — split artifact (SPEC-007).** Every PRD is now `PRD-NNN-<slug>.md` (narrative) + `PRD-NNN-<slug>.yaml` (sidecar with FR/AC/protections/`_sync`). LLMs corrupt structured prose under multi-turn editing; YAML stays intact. v1 PRDs continue to work — no forced migration.
- **Five forcing questions, not skippable.** Every PRD session opens with the five questions (problem-behind-the-problem, evidence, north + counter metric, what must NOT change, riskiest assumption). Recorded in the sidecar; scored by the rubric.
- **Rigor calibration.** `solo | team | platform` triages PRD scope and the evaluator threshold (70 / 80 / 90%). Default `solo`.
- **Stable IDs end-to-end.** `FR-NNN` (PRD) → `SR-NNN`/`SAC-NNN` (SPEC) → plan phases → tests. `/edikt:sdlc:drift` flags FRs uncovered by any SPEC.
- **New commands.** `/edikt:prd:review`, `/edikt:spec:review`, `/edikt:sdlc:discovery`, plus PRD lifecycle verbs `/edikt:sdlc:prd PRD-NNN {ship|supersede|deprecate|cancel}` (edit-in-place per ADR-024).
- **JSON Schema for sidecars.** `templates/schemas/prd-sidecar.schema.json` + `spec-sidecar.schema.json` give VS Code / JetBrains / Neovim autocomplete via `yaml-language-server`.
- **ADR-023 — structured evaluator gates.** `subagent-stop.sh` reads `evaluator_output.{agent,severity,findings}` from the verdict JSON instead of keyword-scanning prose. Per-agent thresholds in `.edikt/config.yaml gates.<agent>`; legacy payloads warn + fall through (deprecated, removed in v0.7.0).
- **Tier-2 Go install.** `edikt install benchmark` provisions a hermetic venv at `~/.edikt/venv/<tool>/`, verifies wheel checksums, rolls back on any failure. First instance of the ADR-015 tier-2 carve-out.
- **Pre-push invariant hook.** `templates/hooks/pre-push.sh` enforces INV-001 (`.md`/`.yaml` only in `commands/` and `templates/`), INV-002 (accepted ADRs immutable), INV-003 (no shell JSON concatenation). Bypass with `EDIKT_BYPASS_PREPUSH=1` (logged).
- **Plan model assignment (BRAIN-001 #29).** Plan frontmatter carries `model:` at plan-level and per-phase override; inheritance chain falls back to `defaults.plan_model` in config.
- **SPEC source flexibility (BRAIN-001 #28).** `/edikt:sdlc:spec` accepts `PRD-NNN`, `BRAIN-NNN`, or free-text; sidecar carries exactly one of `source_prd | source_brainstorm | source_prompt`.
- **Plan-scoped context (BRAIN-001 #27).** `/edikt:context --depth=focused` loads only the PRDs/SPECs referenced in the active plan phase.
- **Doctor checks.** Orphaned sidecars, schema version, `_sync.md_hash` drift, broken refs, fixture characterization rate, recent gate activity.
- **Deprecated stubs removed.** `commands/deprecated/` is gone (16 redirect-only files). Intent-based routing in `CLAUDE.md` handles discovery.

### Breaking changes

- **Sidecar architecture replaces in-body sentinels.** ADR-008 superseded by ADR-027. Every governance `.md` now requires a co-located `<name>.edikt.yaml` sidecar; `/edikt:gov:compile` refuses to run until the migration is applied.
- **INV-005 narrowed to `CLAUDE.md` and `settings.json` only.** Governance artifacts (`.md` files in `decisions/`, `invariants/`, `guidelines/`) are no longer managed regions because edikt does not write to them (ref: ADR-027).
- **ADR-020 amended by ADR-028 (two-phase compile).** Phase A (resync, conditional, LLM-backed) and Phase B (merge, always, deterministic). Phase B preserves ADR-020's latency budget; Phase A has no SLO.
- **PreToolUse hook scope narrowed.** The managed-region overlap guard now scans only `CLAUDE.md`, `settings.json`, and `.edikt/`. Edits to governance `.md` files no longer trip the hook — eliminates the false-positive class on documentation pages flagged in HOOK-FALSE-POSITIVE-ANALYSIS.md.
- **`migrate sidecars` is mandatory on first upgrade.** `--apply` requires a prior `--dry-run` in the same directory within the last 24 hours, or `--force` to bypass. The gate file lives at `.edikt/state/migration-dry-run.json`.
- **Deprecated stubs removed (`commands/deprecated/` deleted).** Old → new mapping:
  - `/edikt:adr` → `/edikt:adr:new` (or `:compile` / `:review`)
  - `/edikt:invariant` → `/edikt:invariant:new` (or `:compile` / `:review`)
  - `/edikt:compile` → `/edikt:gov:compile`
  - `/edikt:review-governance` → `/edikt:gov:review`
  - `/edikt:rules-update` → `/edikt:gov:rules-update`
  - `/edikt:sync` → `/edikt:gov:sync`
  - `/edikt:prd` → `/edikt:sdlc:prd`
  - `/edikt:spec` → `/edikt:sdlc:spec`
  - `/edikt:spec-artifacts` → `/edikt:sdlc:artifacts`
  - `/edikt:plan` → `/edikt:sdlc:plan`
  - `/edikt:review` → `/edikt:sdlc:review`
  - `/edikt:drift` → `/edikt:sdlc:drift`
  - `/edikt:audit` → `/edikt:sdlc:audit`
  - `/edikt:docs` → `/edikt:docs:review`
  - `/edikt:intake` → `/edikt:docs:intake`
- **Pre-push hook enforces INV-003.** Repos with `echo '{'` patterns in hook scripts must switch to `python3 json.dumps`.
- **`gates:` section in `.edikt/config.yaml`.** New projects get the section automatically; `edikt upgrade` leaves existing configs alone (opt-in).
- **PRD v2 template.** Old `templates/prd.md.tmpl` renamed to `prd-v1.md.tmpl`; new template is split-artifact. v1 PRDs still load — only re-running `/edikt:sdlc:prd` on them regenerates in v2 shape.

### Added

- **`edikt verify <plan-id>`** — tier-2 binary subcommand that runs the `verify:` shell commands declared in `PLAN-<id>-criteria.yaml` and writes a JSON+text report under `.edikt/state/verify/`. Exit codes: `0` all-pass, `1` failures or timeouts, `2` sidecar missing/malformed, `3` invalid args. `/edikt:sdlc:plan` invokes the runner before flipping a phase row to `done`. Flags: `--phase`, `--json`, `--allow-failures`.
- **`edikt migrate sidecars`** — dual-schema lift covering v0.4.3 legacy (`content_hash:`), v0.5.x full (`source_hash` + `topic` + `signals`), and v0.5.x partial (`source_hash:` only) artifacts. `--dry-run` mandatory before `--apply`; idempotent; fence-aware; respects skip-list for documentation files (ADR-008/ADR-009/SPEC-*).
- **Sidecar JSON schemas** — `templates/schemas/sidecar.schema.json`, `templates/schemas/prd-sidecar.schema.json`, `templates/schemas/spec-sidecar.schema.json`. Drives editor autocomplete via `yaml-language-server`.
- **Five new doctor sidecar checks** — `ORPHAN`, `MISSING`, `PATH MISMATCH`, `SCHEMA INVALID`, `NEEDS REVIEW` (empty-directives soft warning). First four hard-fail; the last warns. See `/edikt:doctor` § SIDECAR HEALTH.
- **`gov compile --legacy` flag** — transitional opt-in to the v0.5.x in-body parsing path. Slated for removal in v0.7.0.

### Reference

- ADR-023 — SubagentStop structured evaluator-input contract
- ADR-024 — PRD lifecycle asymmetry vs INV-002
- ADR-027 — Sidecar architecture for governance metadata (supersedes ADR-008)
- ADR-028 — Two-phase compile: Phase A resync + Phase B merge (amends ADR-020)
- SPEC-006 — SDLC rework + tier-2 Go install
- SPEC-007 — PRD redesign (split artifact, rigor, SDLC chain)
- BRAIN-001 — PRD as context bundle (26 locked decisions, three shipped here as #27/#28/#29)

---

## v0.5.1 (2026-05-01)

Patch release: multi-platform binaries (ADR-021).

v0.5.0 shipped a single linux-amd64 binary for everyone, breaking macOS Homebrew installs (Mach-O vs ELF mismatch). v0.5.1 fixes packaging:

- Release workflow cross-compiles for darwin-arm64, darwin-amd64, linux-arm64, linux-amd64.
- Asset naming: `edikt-v0.5.1-<goos>-<goarch>.tar.gz`. SHA256SUMS covers all four launchers + payload.
- Homebrew formula uses `on_macos`/`on_linux` × `on_arm`/`on_intel` blocks, served from the right asset per platform.
- `install.sh` detects `uname -s` / `uname -m` and fetches the matching tarball.
- No code or governance changes — pure packaging fix.

If you installed v0.5.0 via Homebrew on macOS: `brew upgrade edikt`. If via curl on Linux: `edikt upgrade` is a no-op (already on the right binary).

## v0.5.0 (2026-04-29)

First release with a pure Go binary, full release-integrity signing, and the security-hardened hook surface. Two themes: **edikt is now a single signed binary**, and **the security audit findings are closed with new invariants that prevent regression**.

### Highlights

- **Pure Go binary.** `edikt` is now a single static Go binary (`tools/edikt/`). The previous `edikt-shell` POSIX helper is deleted; `edikt migrate` is native Go. No runtime dependency on bash for user-facing commands.
- **Sigstore keyless release signing.** Every release publishes `SHA256SUMS.sig.bundle` signed by the release workflow's GitHub OIDC identity. `install.sh` and `edikt upgrade` verify with `cosign verify-blob` before extracting any artifact. Without cosign, install aborts unless `EDIKT_INSTALL_INSECURE=1` is set (loud banner).
- **Versioned payload layout + rollback.** Payloads live at `~/.edikt/versions/<tag>/` with a `current` symlink and `lock.yaml` tracking active, previous, pinned. `edikt upgrade` and `edikt rollback` swap generations atomically. Migrations (M1–M5) carry forward and are not rolled back.
- **Homebrew distribution.** `brew install diktahq/tap/edikt` installs the launcher; `edikt install` fetches the payload. Launcher and payload update independently.
- **Evaluator verdict persistence (ADR-018).** Phase-end evaluator writes structured JSON to `docs/product/plans/verdicts/<plan>/phase-<N>.json` and updates the criteria sidecar in-place after every run. The plan harness rejects PASS for test-command criteria without `evidence_type: "test_run"` — coerced PASS verdicts are forced to BLOCKED. Existing `done` phases are grandfathered on first upgrade.
- **Directive hardening + governance benchmark (SPEC-005).** Directive sentinels gain `canonical_phrases` and `behavioral_signal` fields (backward-compatible). New `/edikt:gov:benchmark` tier-2 command runs adversarial prompts against every governed directive. `/edikt:adr:review --backfill` retrofits `canonical_phrases` onto existing ADRs with per-entry approval. `/edikt:gov:compile` detects orphan ADRs with warn-then-block semantics. `/edikt:doctor` verifies every ADR/INV in the routing table exists on disk.
- **`gov:compile` schema-completeness gate.** The compile no longer silently produces `governance.md` from sentinel blocks missing ADR-008-required fields (`source_hash`, `directives_hash`, `compiler_version`, `manual_directives`, `suppressed_directives`). Aborts with a redirect to the per-artifact compile commands. The inline-fallback that wrote non-conforming blocks via the deprecated `content_hash` field is removed — `<artifact>:compile` is the only sentinel-writing path.

### Security hardening

The v0.5.0 security audit closed the following failure classes and locked them behind new invariants. Six new invariants, four new ADRs — each one prevents an entire category of regression, not a single bug.

#### New invariants

- **INV-003** — Hooks emit structured JSON, never shell-concatenated strings. Every hook uses `python3 json.dumps` with untrusted values passed as argv. CI lint fails on `echo '{'` / `printf '{'` in hook scripts.
- **INV-004** — Hooks must not instruct Claude to execute shell built from untrusted text.
- **INV-005** — Managed-region integrity is verified before overwrite. Markdown sentinels use byte-range overlap checks (not regex over `old_string`); `settings.json` uses an out-of-band sidecar at `~/.edikt/state/settings-managed.json`.
- **INV-006** — Externally-controlled inputs are shape-validated before use, with NFKC + casefold + whitespace-strip normalization so Unicode lookalikes cannot bypass allowlists.
- **INV-007** — Benchmark and test sandboxes are hermetic. No copy of the host's `~/.claude/settings.json`, user-global settings, or hooks; `setting_sources: ["project"]` only; `shutil.copytree(..., symlinks=True)` with a realpath guard.
- **INV-008** — Release install URLs are tag-pinned, never branch-tracking. CI fails on `raw.githubusercontent.com/.../main/` or `releases/latest/download/` in `README.md`, `website/`, or `.github/workflows/`.

#### New ADRs

- **ADR-016** — Release integrity and Sigstore keyless signing (supersedes ADR-013).
- **ADR-017** — Default permissions posture in `settings.json.tmpl`: 23 deny patterns, 17 allow entries, `defaultMode: askBeforeAllow`. See `docs/guides/permissions.md`.
- **ADR-018** — Evaluator verdict schema with per-criterion `evidence_type`.
- **ADR-019** — Narrow carve-out of ADR-014 for four security-rewritten hooks.

### Testing and CI

- **Three-layer harness (SPEC-004).** Layer 1: hook unit tests with JSON fixtures (9 suites). Layer 2: Agent SDK integration tests against real Claude (6 tests + 4-test regression museum). Layer 3: sandboxed runner — `$HOME`, `$EDIKT_HOME`, `$CLAUDE_HOME` redirected to per-run temp. No test contaminates developer state.
- **CI gates.** Layers 1 + 3 on every PR. Layer 2 on tag push (requires `ANTHROPIC_API_KEY` secret).
- **Governance integrity tests.** Offline verification of sentinel hashes, routing table linkage, config schema completeness.

### Breaking changes — upgrade notes

- **Install URL changed.** Update bookmarks and CI scripts from `raw.githubusercontent.com/.../main/install.sh` to `https://github.com/diktahq/edikt/releases/download/v0.5.0/install.sh`.
- **New default permissions may prompt.** First-time Claude invocations of `curl http://` or other denied patterns now produce a permission prompt. Allow once if legitimate. User-added permissions belong in a `userPermissions` top-level key (outside the managed region).
- **Install requires cosign.** Set `EDIKT_INSTALL_INSECURE=1` to bypass (loud banner). Recommended: install cosign first.
- **`/edikt:gov:compile` evidence gate.** Existing `done` phases are grandfathered (`meta.grandfathered: true`) — no regression. New phases require `evidence_type: "test_run"` for test-command criteria.
- **`/edikt:gov:compile` aborts on incomplete sentinels.** First-time adoption on a project without sentinels now redirects to `/edikt:adr:compile`, `/edikt:invariant:compile`, `/edikt:guideline:compile` (each supports a no-arg "process all" invocation). Run those once to populate sentinels under the v0.5.0+ schema, then run `gov:compile`.
- **Multi-sentence directives warn.** Directives without `canonical_phrases` produce a compile warning in v0.5.0 (no block). Run `/edikt:adr:review --backfill` to retrofit. Hard-fail is targeted for a subsequent release.

### Rollback

`edikt rollback v0.5.0` restores `~/.claude/settings.json` from the pre-upgrade backup, removes the managed-region sidecar and grandfather stubs. Idempotent. Backup preserved at `~/.edikt/backup/pre-v0.5.0-<ts>/`.

---

## v0.4.3 (2026-04-14)

### Bug fixes

- **Phase-end evaluator now actually runs.** The phase-end evaluator relied on Claude voluntarily following instructions in plan.md to invoke it. When users executed plan phases directly (the common flow), the evaluator was never triggered. Added `phase-end-detector.sh` — a new Stop hook that detects phase completion signals in Claude's output, finds the in-progress phase from the active plan, and auto-invokes the headless evaluator with the phase's acceptance criteria. Logs `phase_completion_detected` and `phase_evaluation` events to `~/.edikt/events.jsonl`.
  - Detection patterns: "Phase N complete/done/finished/implemented", "Implemented phase N", "PHASE N DONE" completion promise format
  - Respects `evaluator.phase-end: false` config to disable
  - Test override: `EDIKT_EVALUATOR_DRY_RUN=1` to detect without invoking claude -p, `EDIKT_SKIP_PHASE_EVAL=1` to skip entirely

- **Upgrade no longer silently overwrites user customizations.** `/edikt:upgrade` compared installed agents against current templates using a simple hash diff and reported any difference as "template updated ⬆" — misleading language that prompted users to accept and lose their customizations. Now classifies diffs into three buckets:
  - **PURE EXPANSION** — template added content, no user content removed. Auto-applied.
  - **PATH SUBSTITUTION** — only paths differ (e.g., `docs/architecture/decisions/` → `adr/`). Flagged as user divergence.
  - **USER DIVERGENCE** — installed file has content not in the template. Prompts individually with diff preview and options: apply template (lose customizations), keep mine (add `<!-- edikt:custom -->` marker), or skip.

- **Evaluator could silently degrade to read-only PASS.** When invoked as a subagent (directly via the Agent tool, or as a fallback from headless), the evaluator inherited the parent session's permission sandbox — which may deny Bash even when the agent's `tools:` frontmatter declares it. With no way to signal "I couldn't verify this," the evaluator fell back to read-only inspection and returned PASS verdicts on acceptance criteria that required test execution. Captured in [ADR-010](docs/architecture/decisions/ADR-010-evaluator-headless-default-visible-fallback.md).

### Features

- **BLOCKED verdict (ADR-010).** Both evaluator templates (`templates/agents/evaluator.md` and `templates/agents/evaluator-headless.md`) now declare BLOCKED as a valid per-criterion and overall verdict. Rule added: "if a criterion requires execution and execution is unavailable, verdict is BLOCKED — never PASS." The subagent template gained a Capability Self-Check section that probes Bash availability before claiming verdicts.

- **Visible evaluator fallback (ADR-010).** `/edikt:sdlc:plan` now attempts headless first when `evaluator.mode: headless`, falls back to subagent on headless failure with a visible `⚠ EVALUATOR FALLBACK` banner naming the reason and recovery hint, and emits a `✗ EVALUATION FAILED` banner when both modes fail. BLOCKED verdicts now surface per-criterion with recovery hints; the progress table gained a `blocked` state. No silent degradation paths remain.

- **Doctor evaluator probe (ADR-010).** `/edikt:doctor` now probes the evaluator: checks `claude` CLI is on PATH, runs a headless sanity call (`claude -p "echo ok"`), verifies both evaluator templates exist, and reports whether `evaluator.mode` is explicitly set. Each failure has actionable remediation (`claude login`, `/edikt:upgrade`, `/edikt:config set evaluator.mode headless`).

- **`--eval-only {phase}` flag on `/edikt:sdlc:plan` (ADR-010).** Re-run evaluation on a specific phase without re-running the generator. Recovery path for BLOCKED verdicts after the user has fixed the underlying cause (e.g. switching `evaluator.mode` to headless). Routes through the existing Phase-End Flow — no verdict-logic duplication. Optionally combines with `--plan {slug}` when multiple plans exist.

### Governance

- ADR-010 captures the decision and its directives: headless default, subagent as fallback, BLOCKED over silent PASS, visible warnings, doctor probe, no silent degradation.

### Tests

- 17 new tests in `test-phase-end-detector.sh` covering completion pattern detection, config respect, loop prevention, correct phase selection, event logging, and no-false-positive cases.
- 11 new assertions in `test-v040-evaluator.sh` covering BLOCKED verdicts, Capability Self-Check, never-PASS rule, parent-sandbox warning, fallback/failed banners, `--eval-only` flag documentation, and doctor evaluator probe.

## v0.4.2 (2026-04-13)

### Bug fixes

- **Spec preprocessing.** Blank line between frontmatter and `!`` preprocessing block caused shell corruption. Added missing `argument-hint`.
- **Plan pre-flight skipped.** Pre-flight specialist review and criteria validation (steps 10-11) were ordered after the "Next: execute Phase 1" conclusion (step 9). Claude naturally stopped at the conclusion. Reordered: pre-flight is now steps 8-9, write file is step 10, output is step 11.
- **Audit inline mode.** `--no-edikt` jump target said "step 6" (agent-spawning) but inline audit mode was at step 11. Fixed.
- **Gov review premature conclusion.** "Next: Run /edikt:gov:compile" appeared before staleness detection still needed to run. Moved to actual conclusion.

### Tests

- 15 preprocessing format regression tests (no blank lines, argument-hint, awk integrity)
- 5 step ordering regression tests (plan, audit, review)
- 24 evaluator flow tests (pre-flight + phase-end + bypass protection)
- Version check no longer hardcoded

## v0.4.1 (2026-04-12)

### Bug fixes

- **Upgrade: new agent detection.** `/edikt:upgrade` now detects agent templates added in newer versions. Core agents (evaluator, evaluator-headless) are installed automatically. Optional agents are offered to the user with a description — declined agents are added to `agents.custom` so future upgrades skip them.
- **Upgrade: config migration.** `paths.soul` renamed to `paths.project-context`. Upgrade auto-migrates existing configs.
- **CodeRabbit review fixes.** Subagent-stop override check now matches agent + finding on the same line (was two independent greps). WEAK PASS exit code corrected to 0. .gitignore negation patterns fixed. BSD-only stat removed from SPEC-003. Agent count corrected to 18 across website docs.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count, quality gates, plan harness features.

## v0.4.0 (2026-04-11)

### Plan Harness: Iteration Tracking, Context Handoff, Criteria Sidecar

The plan command now tracks failure history, carries context across phase boundaries, and emits a machine-readable criteria sidecar.

- **Iteration tracking:** progress table with Attempt column (`N/max`), 6 statuses (`pending`, `in-progress`, `evaluating`, `done`, `stuck`, `skipped`). After each evaluation failure, fail reasons are forwarded to the next attempt. Escalation warning at 3 consecutive failures on the same criterion. Phase goes `stuck` at max attempts (configurable, default 5) with human decision prompt.
- **Context handoff:** each phase has a `Context Needed` field listing files to read before starting. Artifact Flow Table maps producing phases to consuming phases. PostCompact hook re-injects context files, attempt count, and failing criteria after compaction.
- **Criteria sidecar:** `PLAN-{slug}-criteria.yaml` emitted alongside plan markdown. Per-criterion status tracking (pending/pass/fail), verification commands, fail counts. Evaluator reads and updates the sidecar — no markdown parsing needed.

### Evaluator: Headless Execution and Configuration

The evaluator now runs as a separate `claude -p` invocation with zero shared context from the generator session.

- **Evaluator config:** new `evaluator` section in `.edikt/config.yaml` with 5 keys: `preflight` (toggle pre-flight), `phase-end` (toggle evaluation), `mode` (headless or subagent), `max-attempts` (stuck threshold), `model` (sonnet/opus/haiku).
- **Headless mode (default):** evaluator runs via `claude -p --bare` with `--disallowedTools "Write,Edit"`. Fresh process, no shared context, no self-evaluation bias. Falls back to subagent when headless unavailable.
- **Protected agent:** evaluator templates are not user-overridable. Upgrade always overwrites them. Doctor warns on modifications. Plan blocks if template is missing.
- **LLM evaluator in experiments:** `--llm-eval` flag in experiment runner. Dual-mode: grep pre-check + LLM evaluation. LLM verdict is authoritative when both run. Three verdicts: PASS, WEAK PASS (all critical pass but important fails), FAIL. Severity tiers: critical (blocks), important (degrades), informational (logged only).

### Enforcement: Quality Gate UX and Artifact Lifecycle

Quality gates now log overrides with accountability, and artifact lifecycle is enforced uniformly across the SDLC chain.

- **Gate override logging:** overrides written to `~/.edikt/events.jsonl` with git identity (name + email). Three event types: `gate_fired`, `gate_override`, `gate_blocked`.
- **Re-fire prevention:** overridden findings don't fire again within the same session. Overrides cleared at session start.
- **Artifact lifecycle:** full status chain `draft → accepted → in-progress → implemented → superseded`. Plan auto-promotes `accepted → in-progress` when phase starts. Drift auto-promotes `in-progress → implemented` when no violations found.
- **Plan draft warning:** lists specific draft artifacts by name, offers proceed (with Known Risks) or stop.
- **Drift status filter:** skips `draft` and `superseded` artifacts, validates the rest.
- **Doctor:** flags spec-artifacts stuck in draft > 7 days. Parses both YAML frontmatter and comment header status formats.

### Breaking changes

- **Config key rename:** `paths.soul` → `paths.project-context`. `/edikt:upgrade` auto-migrates existing configs. Commands fall back to `soul` if `project-context` is not found.

### Documentation

- Updated `project-context.md` for v0.4.0: hook count (9→13), agent count (20→19), quality gates, plan harness features, context vs enforcement framing
- Fixed 12 pre-existing documentation gaps (stale agent/hook/command counts, old command names in AGENTS.md, missing index entries)
- Updated website: plan, gates, chain, features, doctor, drift pages with v0.4.0 features
- Removed stale AGENTS.md (Codex convention — edikt is Claude Code only per ADR-001)

### New config keys

```yaml
evaluator:
  preflight: true       # pre-flight criteria validation
  phase-end: true       # phase-end evaluation
  mode: headless        # headless | subagent
  max-attempts: 5       # max retries before stuck
  model: sonnet         # model for headless evaluator
```

## v0.3.1 (2026-04-11)

### Bug fixes

- **Init: guidelines path.** `/edikt:init` now writes `paths.guidelines` correctly.
- **VERSION stamp.** `VERSION` file updated to match release tag.
- **PRINCIPAL prefix.** Compile output no longer prefixes directives with `PRINCIPAL:`.
- **Review output.** `/edikt:sdlc:review` output formatting fixed.
- **SubagentStop hook: seniority prefix.** The fallback agent detection pattern matched "As Principal Architect" → `principal-architect` instead of `architect`, breaking slug lookup and gate matching. Now extracts only the role word.
- **Missing page.** Added `/edikt:guideline:compile` website page (was dead link).
- **Test fixes.** All 25 suites pass after v0.3.0 regressions.

### Artifact generation: JSONB support and domain class diagram

`/edikt:sdlc:artifacts` now handles projects using JSONB aggregate storage (common DDD pattern in PostgreSQL) and generates a domain class diagram alongside the data model.

- **Storage strategy detection.** When DB type is `sql` or `mixed`, the command scans spec content and migrations for JSONB signals (`jsonb`, `json column`, `aggregate storage`, `embedded entity`, `nested entity`, etc.). Detected strategy is shown in the state checkpoint and routing output.
- **Three entity modes in `data-model.mmd`.** When storage strategy is `jsonb-aggregate`, the ERD distinguishes physical tables (normal), JSONB-embedded entities (relationship label contains `jsonb`), and reference-only entities from external bounded contexts (relationship label contains `ref`). Makes nested structure visible instead of hiding it in JSONB column comments.
- **Domain class diagram (`model.mmd`).** New artifact type, always generated alongside the data model regardless of DB type. Mermaid `classDiagram` showing aggregate roots, value objects, entities, inheritance, composition, and domain methods. Reviewed by the architect agent.

### Configurable artifact spec versions

Artifact templates now use configurable spec versions instead of hardcoded values. Defaults updated to latest stable:

| Format | Previous | Now (default) |
|---|---|---|
| OpenAPI | 3.0.0 | **3.1.0** |
| AsyncAPI | 2.6.0 | **3.0.0** |
| JSON Schema | draft-07 | **2020-12** |

Teams can pin older versions in `.edikt/config.yaml`:

```yaml
artifacts:
  versions:
    openapi: "3.0.0"       # pin for tooling compatibility
    asyncapi: "2.6.0"      # pin if not ready for 3.0 breaking changes
    json_schema: "https://json-schema.org/draft/07/schema#"
```

The AsyncAPI template was updated for the 3.0 structure (separate `channels` and `operations` blocks replacing `publish`/`subscribe`). When pinning `asyncapi: "2.6.0"`, the agent uses the 2.x structure.

### New `/edikt:config` command

View, query, and modify `.edikt/config.yaml` with discovery, validation, and natural-language changes.

- **No args** — show all 34 config keys with current values and defaults
- **`get {key}`** — show a specific key's value, default, valid values, and which commands use it
- **`set {key} {value}`** — validate and write, with per-key validation rules

Protected keys like `edikt_version` cannot be set directly. Invalid values are rejected with explanation.

### `/edikt:team` deprecated — merged into init + config

`/edikt:team` served two purposes that belong elsewhere:
- **Member onboarding** → now in `/edikt:init`'s "existing project" path
- **Config management** → now in `/edikt:config`

When `/edikt:init` detects an existing `.edikt/config.yaml`, it runs member environment validation instead of saying "already initialized":
1. **Version gate** — blocks if installed edikt < project's `edikt_version`
2. **Environment checks** — git identity, Claude Code, MCP env vars (read dynamically from `.mcp.json`), `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`, pre-push hook, managed settings
3. **Governance gap sync** — missing rules/hooks/agents offered for install
4. **Shared config display** — what's committed to git

The `team:` config block is no longer used. Legacy blocks in existing configs are ignored silently. The deprecated stub redirects to init and will be removed in v0.5.0.

## v0.3.0 (2026-04-10)

### Project Adaptation (ADR-008, ADR-009)

edikt can now adapt to existing projects. The compile pipeline supports a **three-list directive schema** (ADR-008) with hash-based caching, and introduces **Invariant Records** as the formal governance artifact for hard constraints (ADR-009).

- **Three-list schema:** every compiled sentinel block now carries `directives:` (auto-generated), `manual_directives:` (user-authored), and `suppressed_directives:` (user-rejected). The merge formula `effective = (directives - suppressed) ∪ manual` gives users full control over what ships without losing compile automation. Hash-based caching (`source_hash` + `directives_hash`) skips Claude calls when nothing changed.
- **Invariant Records (ADR-009):** formalized the governance artifact for non-negotiable constraints. Formalized "Invariant Records" as the governance artifact for hard constraints (short form: INV). Template follows Statement/Rationale/Enforcement structure. Compile extracts directives from the Statement section, preserving declarative absolute language.
- **Extensibility plumbing:** template lookup chain (`project .edikt/templates/` → inline fallback), `/edikt:guideline:compile` command, auto-chain (`<artifact>:new` runs `<artifact>:compile`).
- **Init style detection:** detects project style (flat, layered, monorepo) during init. Adapt mode for existing `.edikt/` directories. Template-less refusal for v0.3.0+ projects.
- **Flexible prose input:** ADR/invariant/guideline creation accepts natural language with automatic reference extraction to existing governance.
- **Doctor + upgrade integration:** doctor reports template overrides and stale hashes. Upgrade respects project templates.

### Compile Improvements

Experiment-driven improvements to the compile output format. These changes improve how well Claude follows governance directives.

- **"No exceptions." reinforcement:** invariant directives derived from absolute-language Statements ("every", "all", "total") now get "No exceptions." appended. Experiments showed this phrase prevents Claude from rationalizing edge cases.
- **Reminders sentinel (`[edikt:reminders:start/end]`):** compile now generates pre-action interrupts: "Before writing SQL → MUST include tenant_id." Aggregated into a `## Reminders` section in governance.md. Capped at 10.
- **Verification checklist:** compile generates a `## Verification Checklist` section with grep-verifiable items Claude checks before finishing. Capped at 15 items.
- **Per-directive LLM compliance scoring** in `/edikt:invariant:review`, `/edikt:adr:review`, `/edikt:guideline:review`: scores each compiled directive on token specificity, MUST/NEVER usage, grep-ability, ambiguity, and friction risk. Manual directives held to the same standard.
- **New `/edikt:gov:score` command:** aggregate governance quality report — context budget, compliance metrics, manual directive health. JSON output for CI.

### Pre-flight Criteria Validation

The evaluator agent now supports a **pre-flight mode** that validates acceptance criteria BEFORE the generator starts. Classifies each criterion as TESTABLE/VAGUE/SUBJECTIVE/BLOCKED and proposes verification commands. The plan command (step 11) invokes pre-flight automatically, preventing wasted iterations on untestable criteria.

### Experiments

Pre-registered experiments measuring whether governance directives change Claude's output on real coding tasks. 8 experiments across 4 scenario types.

| Scenario | Baseline | Governance | Effect |
|---|---|---|---|
| Existing codebase (EXP 01-04) | PASS | PASS | Absent — code patterns self-teach |
| Greenfield (EXP 05-06) | VIOLATION | PASS | **Present** — governance prevents architecture/tenant violations |
| New domain on existing (EXP 07) | VIOLATION | PASS | **Present** — governance catches log/SQL misses |
| Long context (EXP 08, N=2) | 1/2 VIOLATION | 0/2 PASS | **Present** — governance stabilizes under context pressure |

Key findings: governance has measurable effect on greenfield and new-domain code. Directive format matters — MUST/NEVER with literal code tokens outperforms prose. Long context degrades convention compliance; governance in `.claude/rules/` survives because it's loaded separately from the conversation. Full methodology and results in `test/experiments/`.

### New commands

- `/edikt:gov:score` — aggregate governance quality scoring for CI

### Architecture Decisions

- **ADR-008:** Deterministic compile and three-list schema
- **ADR-009:** Invariant Record template formalization

## v0.2.3 (2026-04-09)

### Compile schema version (ADR-007)

`/edikt:gov:compile` now stamps generated governance files with a **compile schema version** — a small integer independent of edikt's marketing version — instead of the edikt version at compile time.

**Problem this fixes:** before v0.2.3, `.claude/rules/governance.md` was stamped with `version: "<edikt-version>"`, conflating two unrelated cadences. Every edikt point release (even pure bug fixes) implied governance was stale and needed regeneration, but the compile output format hadn't actually changed. In the dogfood repo, we kept hand-editing `governance.md`'s version via `sed` on each release to keep a test green — the file ended up lying about its own provenance (version said v0.2.2 but the compile timestamp was frozen at March 25).

**New format** (see [ADR-007](docs/architecture/decisions/ADR-007-compile-schema-version.md)):

```yaml
---
paths: "**/*"
compile_schema_version: 2
---
<!-- edikt:compiled — generated by /edikt:gov:compile, do not edit manually -->
<!-- compiled_by: edikt v0.2.3 -->
<!-- compiled_at: 2026-04-09T10:30:00Z -->
```

Three fields, three purposes:

- **`compile_schema_version`** (YAML, enforced) — identifies the output format contract. `1` = v0.1.x flat governance, `2` = v0.2.x topic-grouped rule files. `/edikt:doctor` checks it against the constant declared in `commands/gov/compile.md` and recommends `/edikt:gov:compile` only when the format has actually changed.
- **`compiled_by`** (HTML comment, informational) — which edikt version ran compile. Diagnostic only, never enforced.
- **`compiled_at`** (HTML comment, informational) — ISO8601 timestamp. Diagnostic only, never enforced.

**Consequences:**
- No more false-positive staleness warnings on point releases. Users only see "regenerate governance" when the compile schema actually changed.
- Point releases can ship bug fixes without implying anything about compile output compatibility.
- `/edikt:doctor` gets smarter about stale governance detection.
- `/edikt:upgrade` has a new step that checks the project's schema version against the installed compile schema and recommends (but does not auto-run) regeneration when they diverge.
- Dogfooding stops hand-editing `governance.md`'s version field. The dogfood file now uses the new format honestly.

### Installer UX fixes

Three bug reports from real installs, all fixed in the same release.

- **No prompt on `curl | bash`.** The interactive "global vs project" prompt was skipped silently when stdin was a pipe (the common `curl -fsSL ... | bash` invocation). Now the installer reads from `/dev/tty` when available, so the prompt fires even when stdin is consumed by the curl pipe. Falls back to `--global` only when there's no TTY at all (CI, fully redirected).
- **Commands duplicated across global and project locations.** When a user installed globally in a directory that already had a project-local edikt install (either from a prior `--project` run, or from the dogfood repo itself), Claude Code ended up loading commands from both `~/.claude/commands/edikt/` and `.claude/commands/edikt/`, producing duplicates in the skill list. The installer now detects this condition at startup and emits a yellow warning pointing at the exact paths and the `rm -rf` to clean them up. Never auto-deletes.
- **No detection of existing install before project install.** If a user ran `install.sh --project` in a directory where `~/.edikt/VERSION` already existed, the two installs would silently overlap. Same detection now fires a warning for this case too. Both detection paths share the same `HAS_GLOBAL` / `HAS_PROJECT` flags.
- **New test scenarios in `test/test-install-e2e.sh`** — scenarios 6 and 7 cover the duplication-warning paths (6 = global install with leftover project files; 7 = project install with existing global install). Total scenarios now: 7. Total assertions: 28.

### Tests

- **New `test/test-v023-regressions.sh`** (21 assertions) — verifies ADR-007 exists, `COMPILE_SCHEMA_VERSION` is declared in compile.md, output templates emit the new format, doctor.md checks the schema version, upgrade.md documents the migration step, and the dogfood governance file matches the constant.
- **`test-e2e.sh` version check refactored** — no longer enforces `GOV_VER == FILE_VER`. Instead it validates that `compile_schema_version` in the dogfood governance file matches the `COMPILE_SCHEMA_VERSION` constant in `commands/gov/compile.md`.

## v0.2.2 (2026-04-08)

Critical bug-fix release. The v0.2.1 installer was silently broken on the v0.1.x → v0.2.x upgrade path.

### Installer: upgrade from v0.1.x was silently broken

- **`((BACKUP_COUNT++))` under `set -euo pipefail` killed the installer on the first backup.** Postfix `++` returns the pre-increment value (`0` on the first call), which bash's `set -e` treats as a failure and exits the script. Symptoms: the cleanup loop removed *nothing*, the new namespaced commands were *never* installed, old flat files stayed in place, and the installer exited without any error message. This shipped in v0.2.1 and affected everyone upgrading from v0.1.x via `curl | bash`. Fixed by using `BACKUP_COUNT=$((BACKUP_COUNT + 1))`.
- **New integration test** (`test/test-install-e2e.sh`) — 22 assertions across five scenarios: fresh install, upgrade from v0.1.x, user-customized file preservation, network failure abort, and repeated-install idempotency. Shims `curl` with a mock that serves files from the local repo, so the full `install.sh` runs end-to-end against a fake `$HOME` in `/tmp`. This is the test we wished existed before v0.2.0 shipped — it caught the v0.2.1 regression immediately.

### `/edikt:upgrade`: migrate v0.1.x command references

- **New step 5 in `/edikt:upgrade`: rewrite old flat command references in project files to their new namespaced equivalents.** Projects initialized with v0.1.x have hardcoded references to `/edikt:adr`, `/edikt:plan`, `/edikt:compile`, etc. in their `CLAUDE.md` managed block and in compiled rule packs. Previously, `/edikt:upgrade` only migrated the *sentinel format* (HTML → visible) and left the *content* inside the sentinels untouched. Now upgrade runs a targeted string-replace across all 15 moved commands, scoped to edikt-owned content only (the CLAUDE.md managed block and rule pack files marked with `edikt:generated` or `edikt:compiled`). User content outside the managed blocks is never touched.
- **Idempotent and safe:** the instruction tells Claude to match only occurrences NOT already followed by `:`, using surrounding context (backticks, punctuation, end-of-line) for disambiguation. Running upgrade twice is a no-op.

## v0.2.1 (2026-04-08)

Bug-fix release following v0.2.0 field reports.

### Installer upgrade path

- **Old flat commands no longer linger after upgrade.** v0.1.x installed commands like `~/.claude/commands/edikt/adr.md`, `plan.md`, `compile.md` at the top level. v0.2.0 moved them into namespaces but the installer never removed the old files, so users saw both `/edikt:adr` (stale) and `/edikt:adr:new` (new) in their command list. The installer now deletes the 15 moved v0.1.x commands before installing new files, with backup. User-customized files (marked with `<!-- edikt:custom -->`) are preserved.
- **Silent curl failures now abort the install.** Every `curl -o` call now goes through a `_fetch` helper that enforces `--retry 2`, `--max-time 30`, non-empty download verification, and exits with an error on failure. Previously a network blip during `curl | bash` could leave files partially updated without any warning.

### `/edikt:init` ADR path adoption

- **init now configures `paths.decisions` to match detected ADR locations.** Previously, init detected existing ADRs in folders like `docs/decisions/` and offered to import them, but the import flow hardcoded the destination to edikt's default (`docs/architecture/decisions/`) and never wrote the detected path into `.edikt/config.yaml`. Users ended up with ADRs in one place and edikt looking for them somewhere else — `/edikt:gov:compile` and `/edikt:status` reported zero ADRs.
- New prompt: **[1] Adopt** (keep ADRs where they are, configure edikt to use that path), **[2] Migrate** (move to edikt's default layout), **[3] Skip**. Same flow for invariants.

### Command documentation cleanup

- **Seniority prefixes removed from `/edikt:sdlc:review` reviewer lenses.** The command documentation still labeled agents as `Principal DBA`, `Staff SRE`, `Staff Security`, `Senior API`, `Principal Architect`, `Senior Performance` — inconsistent with the agent templates which dropped seniority prefixes in v0.2.0. Now just `DBA`, `SRE`, `Security`, `API`, `Architect`, `Performance`.

### Website content

- **Fixed 10 dead links in `website/governance/chain.md`, `website/governance/compile.md`, `website/governance/drift.md`, and `website/commands/brainstorm.md`** — they referenced old flat command paths (`/commands/prd`, `/commands/spec`, `/commands/plan`, etc.) that broke the v0.2.0 VitePress deploy. Now use namespaced paths (`/commands/sdlc/prd`, `/commands/gov/compile`, etc.).

### Test coverage

- New `test/test-v021-regressions.sh` — 36 assertions guarding against all five v0.2.1 bugs so they can't silently return.

## v0.2.0 (2026-03-31)

### Intelligent Compile — topic-grouped rule files

`/edikt:compile` no longer produces a single flat `governance.md`. It now generates **topic-grouped rule files** under `.claude/rules/governance/` — each topic file contains full-fidelity directives from all sources (ADRs, invariants, guidelines), loaded automatically by path matching.

- **Directive sentinels** — ADRs and invariants can include `[edikt:directives:start/end]` blocks with pre-written LLM directives. Compile reads these verbatim — no extraction, no distillation.
- **Routing table** — `governance.md` becomes an index with invariants + a routing table. Claude matches task signals and scopes to load relevant topic files.
- **Three loading mechanisms** — `paths:` frontmatter (platform-enforced on file edits), `scope:` tags (activity-matched for planning/design/review), and signal keywords (domain-matched).
- **No directive cap** — the 30-directive limit is removed. Soft warning if a topic file exceeds 100 directives.
- **Reverse source map** — compile output shows which ADRs/guidelines contributed to which topic files.
- **Sentinel generation moved to compile** — `/edikt:compile` now generates missing sentinel blocks inline before compiling. No separate step needed. `/edikt:review-governance` is now pure language quality review + staleness detection.
- `/edikt:review-governance` redesigned — language quality review only. Detects stale sentinels and directs to compile. No longer generates anything.

### Command namespacing

edikt commands are now grouped into namespaces matching the artifacts they touch. Nested namespacing confirmed working in Claude Code.

**New structure:**
- `edikt:adr:new` / `:compile` / `:review` — ADR lifecycle
- `edikt:invariant:new` / `:compile` / `:review` — invariant lifecycle
- `edikt:guideline:new` / `:review` — guideline management
- `edikt:gov:compile` / `:review` / `:rules-update` / `:sync` — governance assembly
- `edikt:sdlc:prd` / `:spec` / `:artifacts` / `:plan` / `:review` / `:drift` / `:audit` — SDLC chain
- `edikt:docs:review` / `:intake` — documentation
- `edikt:capture` — mid-session decision sweep (new command)

**New commands:** `capture`, `guideline:new`, `guideline:review`, `adr:compile`, `adr:review`, `invariant:compile`, `invariant:review`

**Deprecated** (removed in v0.4.0): old flat names (`edikt:adr`, `edikt:compile`, `edikt:spec`, etc.) — each stub tells you the new name.

### Agent governance

All 19 agent templates now include governance frontmatter:

- **`maxTurns`** — 10 for advisory agents, 20 for code-writing agents, 15 for the evaluator.
- **`disallowedTools`** — advisory agents have `Write` and `Edit` disallowed at the platform level.
- **`effort`** — high for architect/security/qa/performance/compliance, medium for backend/frontend/dba/api/sre/docs/pm/data/platform/ux, low for gtm/seo.
- **Agent effort fixes** — `data` was `low` with `disallowedTools: [Write, Edit]` which blocked artifact creation. Fixed to `medium` with write access. `platform`, `compliance`, and `ux` effort levels corrected to match their review depth.
- **`initialPrompt`** — architect, security, and pm auto-load project context when run as main session agents.
- **New `evaluator` agent** — phase-end evaluator that verifies work against acceptance criteria with fresh context. Skeptical by default.

### Hook modernization

- **Conditional `if` field** on PostToolUse (scopes to code files only) and InstructionsLoaded (scopes to rule files only). Avoids spawning hook processes for non-matching files.
- **4 new hooks** — `StopFailure` (logs API errors), `TaskCreated` (tracks plan phase parallelism), `CwdChanged` (monorepo directory detection), `FileChanged` (warns on external governance file modifications).

### Harness improvements

- **Context reset guidance** — at phase boundaries, edikt recommends starting a fresh session. State lives in the plan file.
- **Phase-end evaluation** — evaluator agent checks acceptance criteria with binary PASS/FAIL per criterion before suggesting context reset.
- **Acceptance criteria per phase** — plans now include testable, binary assertions per phase. Specs enforce downstream flow.
- **Conditional evaluation** — `evaluate: true/false` per phase. High-effort phases evaluate by default, low-effort skip.
- **Evaluator tuning** — `docs/architecture/evaluator-tuning.md` tracks false positives/negatives for prompt refinement.
- **Harness assumptions** — `docs/architecture/assumptions.md` documents 6 testable assumptions about model limitations. `/edikt:upgrade` prompts for re-testing.

### Rule pack UX

- **Conflict detection** — `/edikt:rules-update` checks new rule packs against compiled governance before installing.
- **Install preview** — shows what will change (added/changed/removed sections) before applying updates.
- **Override transparency** — `/edikt:doctor` and `/edikt:status` report compiled governance status, sentinel coverage, and rule pack overrides.

### Installer safety

- **`--dry-run`** — preview what the installer would change without writing files.
- **Backup before overwrite** — existing files backed up to `~/.edikt/backups/{timestamp}/` before overwriting.
- **Existing install detection** — reports installed version and confirms before proceeding.

### Headless & CI foundations

- **`--json` output** — compile, drift, audit, doctor, review, and review-governance support `--json` for machine-readable output.
- **Headless mode** — `EDIKT_HEADLESS=1` with `headless-ask.sh` hook auto-answers interactive prompts for CI pipelines.
- **CI guide** — new website guide with GitHub Actions example, recommended settings, and environment variables.
- **Managed settings awareness** — `/edikt:team` detects organization-managed settings (`managed-settings.json`, `managed-settings.d/`).

### UX consistency improvements

- **Standardized completion signals** — all 25 commands end with `✅ {Action}: {identifier}` + `Next:` line.
- **Standardized error messages** — all commands that read config use the same missing-config message.
- **Config guards** — 10 additional commands now guard for missing `.edikt/config.yaml` instead of failing mid-execution.
- **Init rule preview** — step 3b shows a preview of actual rules before generating files, with customization paths taught at the moment of installation.
- **Init reconfigure protection** — content hash comparison detects edited files. Per-file `[1] Overwrite / [2] Keep mine / [3] Show diff` flow instead of silent overwrites.
- **Composite config screen** — SDLC options merged into the single combined rules/agents view. One screen, one confirmation.
- **Concrete init summary** — before/after with stack-specific examples from installed rules and agents.
- **Agent routing standardized** — all commands use `🔀 edikt: routing to {agents}` format.
- **Progress breadcrumbs** — compile, audit, review, drift, and review-governance show `Step N/M:` progress.
- **Numbered confirmation options** — letter-code choices (`[a]/[s]/[k]`) replaced with `[1]/[2]/[3]`.
- **Emoji key** — output conventions table added to CLAUDE.md template.

### Bug fixes

- **Plan ignores spec artifacts when generating phases** — `/edikt:plan` now scans the spec directory for all artifact files (fixtures, test strategy, API contracts, event contracts, migrations) and verifies each has plan coverage. Uncovered fixtures get a seeding phase, uncovered test categories get test tasks, uncovered API endpoints get a warning. A hard gate (step 6c) blocks plan writing if any artifact has no coverage — the user must add phases, defer explicitly, or cancel. Prevents silent failures where artifacts are generated but never consumed.
- **Cross-reference validation in compile and review-governance** — both commands now verify that every `(ref: INV-NNN)` and `(ref: ADR-NNN)` reference points to an actual source document. Fabricated references are stripped before writing.
- **Plan trigger not matching "let's create a plan to fix X"** — added trigger examples with trailing context ("plan to fix these issues", "plan these changes", "plan this work") so Claude matches the plan intent even when the sentence includes what to fix.
- **SessionStart hook errors on compact** — `set -euo pipefail` caused silent non-zero exits when Claude Code fires `SessionStart` after `/compact`. Relaxed to `set -uo pipefail` — the hook already guards every fallible command with `|| true`.
- **Test suite requires pyyaml** — agent and registry tests used `python3 -c "import yaml"` which fails silently when pyyaml isn't installed. Rewrote agent frontmatter checks in pure bash, registry checks to fall back to `yq`, and `assert_valid_yaml` to try `yq` when python3-yaml is unavailable.

### Platform alignment

- **Environment hardening** — `/edikt:team` checks for `CLAUDE_CODE_SUBPROCESS_ENV_SCRUB`. Security guide documents `sandbox.failIfUnavailable`.
- **SendMessage auto-resume** — documented on website for agent resumption.

## v0.1.4 (2026-03-28)

### Brainstorm command

New `/edikt:brainstorm` command — a thinking companion for builders. Open conversation grounded in project context, with specialist agents joining as topics emerge. Converges toward a PRD or SPEC when ready. Use `--fresh` for unconstrained brainstorming that challenges existing decisions. Brainstorm artifacts saved to `docs/brainstorms/`.

### Upgrade version check

`/edikt:upgrade` now checks for newer edikt releases before upgrading the project. If a newer version exists, it shows the install command and stops — ensuring project upgrades always use the latest templates. Skip with `--offline` for air-gapped environments.

## v0.1.3 (2026-03-27)

### Flexible plan input

`/edikt:plan` now accepts any input format — natural language prompts, existing plan names, ticket IDs, SPEC identifiers, or nothing (infers from conversation context). When the intent is ambiguous (natural language or conversation context), edikt offers a choice between a full phased plan (saved to `docs/plans/`) and a quick conversational plan.

- `PLAN-NNN` input: continue from current phase, re-plan remaining phases, or create a sub-plan
- Empty input: infers from current conversation context before asking
- Natural language: offers full vs quick plan disambiguation

## v0.1.2 (2026-03-27)

### Bug fix

- **Installer prompt auto-answered when piped** — `curl | bash` triggered the interactive install mode prompt which got EOF from stdin, flashing the prompt and auto-selecting global. Now detects non-terminal stdin and defaults to global silently. Use `--project` flag for project-local install.

## v0.1.1 (2026-03-27)

### Numbered findings in reviews

All review commands now enumerate findings with IDs (#1, #2, #3...) so users can select which to address by number.

- `/edikt:plan` — pre-flight findings numbered, triage prompt: "Which findings should I address? (e.g., #1, #4 or 'all critical')"
- `/edikt:review` — implementation review findings numbered across all agents
- `/edikt:audit` — security and reliability findings numbered across sections
- `/edikt:drift` — diverged findings include triage prompt
- `/edikt:doctor` — warnings and failures numbered for easy reference

### Natural language triggers for all 24 commands

The CLAUDE.md command table now matches intent, not exact phrases. All 24 commands have natural language triggers (was 14). Each command has an intent label and broader representative examples. "Create me a plan for this ticket", "help me plan this out", "spec this out", "are we on track with the spec", "run a security audit", "check my setup" — all trigger the right command.

### Bug fixes

- **Init hook filename hallucination** — `/edikt:init` now reads the settings template exactly as-is instead of generating hook filenames. Fixes `stop-signals.sh: No such file or directory` error.
- **PostToolUse gofmt error** — `gofmt -w` failures on invalid Go syntax no longer propagate as hook errors.
- **Drift report only saving frontmatter** — `/edikt:drift` now explicitly writes the full report (frontmatter + body), not just the frontmatter.
- **Plan mode guard** — All 8 interactive commands (`init`, `plan`, `prd`, `spec`, `spec-artifacts`, `adr`, `invariant`, `intake`) now detect plan mode and tell you to exit it first, instead of silently skipping the interview.
- **Installer preserves customized commands** — `install.sh` now checks for `<!-- edikt:custom -->` before overwriting, so customized commands survive reinstall.

### spec-artifacts redesign — design blueprints with database type awareness

`/edikt:spec-artifacts` now treats every artifact as a design blueprint: it defines intent and structure, not implementation. Your code is the implementation.

**Database-type-aware data model.** The data model artifact format is now resolved from your database type:

- SQL → `data-model.mmd` (Mermaid ERD with entities, relationships, index comments)
- MongoDB/Firestore → `data-model.schema.yaml` (JSON Schema in YAML)
- DynamoDB/Cassandra → `data-model.md` (access patterns, PK/SK/GSI design)
- Redis/KV stores → `data-model.md` (key schema table with TTL and namespace)
- Mixed stacks → both artifacts, suffixed to avoid collision (`data-model-sql.mmd`, `data-model-kv.md`, etc.)

**Database type resolution — four-priority chain:** spec frontmatter `database_type:` → config `artifacts.database.default_type` → keyword scan of spec content → ask the user. Config is set automatically by `/edikt:init` from code signals.

**Native artifact formats.** API contracts are now OpenAPI 3.0 YAML (`contracts/api.yaml`). Event contracts are AsyncAPI 2.6 YAML (`contracts/events.yaml`). Fixtures are portable YAML (`fixtures.yaml`). Migrations are numbered SQL files (`migrations/001_name.sql`). No more markdown wrappers.

**Migrations are SQL-only.** Document and key-value databases never produce migration files.

**Invariant injection.** Active invariants are loaded from your governance chain, stripped of frontmatter, and injected as structured constraints into every agent prompt. Superseded invariants are excluded. Empty invariant bodies emit a warning.

**Design blueprint header.** Every generated artifact gets a format-appropriate comment header marking it as a blueprint, not implementation code.

**Config contract.** `/edikt:init` now detects database type and migration tool from code signals and writes `artifacts.database.default_type` and `artifacts.sql.migrations.tool` to config. The `artifacts:` block is now part of the standard config schema.

### HTML sentinel migration — CLAUDE.md section boundaries now visible to Claude

Claude Code v2.1.72+ hides `<!-- -->` HTML comments when injecting `CLAUDE.md` into Claude's context. The old `<!-- edikt:start -->` / `<!-- edikt:end -->` sentinels were invisible to Claude, so asking Claude to "edit my CLAUDE.md" could accidentally overwrite edikt's managed section.

New format uses markdown link reference definitions, which survive Claude Code's injection intact:

```
[edikt:start]: # managed by edikt — do not edit this block manually
...
[edikt:end]: #
```

- `/edikt:init` writes the new format on fresh installs and migrates old markers when re-running
- `/edikt:upgrade` detects and migrates old HTML sentinels as part of the upgrade flow
- Both old and new formats are detected for backward compatibility
- ADR-002 updated to document the change and rationale

### Effort frontmatter on all commands

All 24 commands now declare `effort: low | normal | high` in their frontmatter. Claude Code uses this to tune the model's thinking budget per command.

- `low` — `agents`, `context`, `mcp`, `status`, `team`
- `normal` — `adr`, `compile`, `doctor`, `init`, `intake`, `invariant`, `review-governance`, `rules-update`, `session`, `sync`, `upgrade`
- `high` — `audit`, `docs`, `drift`, `plan`, `prd`, `review`, `spec`, `spec-artifacts`

### Init improvements

- **Existing ADR import** — `/edikt:init` now detects existing architecture decisions and offers to import them into edikt's governance structure.
- **Project-local install** — `install.sh --project` installs edikt into the current project (`.claude/commands/`, `.edikt/`) instead of globally. Default is still global.
- **Database detection** — `/edikt:init` detects database type and migration tool from 30+ code signals across Go, Node, Python, Ruby, C#, Elixir, and Rust. Definitive signals (e.g., `prisma/schema.prisma`) auto-configure. Inferred signals (package dependencies) are flagged. Nothing found triggers targeted greenfield questions.

## v0.1.0 (2026-03-23)

### First public release

edikt governs your architecture and compiles your engineering decisions into automatic enforcement. It governs the Agentic SDLC from requirements to verification — closing the gap between what you decided and what gets built.

**Architecture governance & compliance**
- `/edikt:compile` reads accepted ADRs, active invariants, and team guidelines, checks for contradictions, and produces `.claude/rules/governance.md` — directives Claude follows automatically every session
- 20 rule packs (10 base, 4 lang, 6 framework) — correctness guardrails, not opinions. 14-17 instructions per pack (research-validated sweet spot)
- Domain-specific governance checkpoints with pre-action and post-result verification
- Signal detection: stop hook detects architecture decisions mid-session, suggests ADR capture
- Quality gates: configure agents as gates in `.edikt/config.yaml`. Critical findings block progression with logged override
- Pre-push invariant check: violations block the push. Override with `EDIKT_INVARIANT_SKIP=1`

**Agentic SDLC governance**
- Full traceability chain: `/edikt:prd` → `/edikt:spec` → `/edikt:spec-artifacts` → `/edikt:plan` → execute → `/edikt:drift`
- Status-gated transitions: PRD must be accepted before spec, spec before artifacts
- `/edikt:drift` compares implementation against the full chain with confidence-based severity
- CI support: `--output=json` with exit code 1 on diverged findings

**18 specialist agents**
- architect, api, backend, dba, docs, frontend, performance, platform, pm, qa, security, sre, ux, data, mobile, compliance, seo, gtm
- Used in spec review, plan pre-flight, post-implementation review, and audit

**9 lifecycle hooks**
- SessionStart: git-aware briefing with domain classification
- UserPromptSubmit: injects active plan phase into every prompt
- PostToolUse: auto-formats files after edits
- PostCompact: re-injects plan + invariants after context compaction
- Stop: regex-based signal detection for decisions, doc gaps, security
- SubagentStop: logs agent activity, enforces quality gates
- InstructionsLoaded: logs active rule packs
- PreToolUse: validates governance setup
- PreCompact: preserves plan state

**24 commands**
- Governance chain: `init`, `prd`, `spec`, `spec-artifacts`, `plan`, `drift`, `compile`
- Decisions: `adr`, `invariant`
- Review: `review`, `audit`, `review-governance`, `doctor`
- Observability: `status`, `session`, `docs`
- Setup: `context`, `intake`, `upgrade`, `rules-update`, `sync`, `team`, `mcp`, `agents`

**Research**
- 123 eval runs across 2 experiments proving rule compliance mechanism
- EXP-001: 15/15 compliance with rules vs 0/15 without on invented conventions
- EXP-002: holds under multi-rule conflict, multi-file sessions, Opus vs Sonnet, adversarial prompts
- Reproducible: `test/experiments/rule-compliance/exp-001-scenarios/` and `test/experiments/rule-compliance/exp-002-scenarios/`

**Website**
- Full documentation at edikt.dev
- Guides: solo engineer, teams, multi-project, greenfield, brownfield, monorepo, security, daily workflow
- Governance section: chain, gates, compile, drift, review-governance

**Zero dependencies**
- Every file is `.md` or `.yaml` — no build step, no runtime, no daemon
- `curl -fsSL https://raw.githubusercontent.com/diktahq/edikt/main/install.sh | bash`
- Claude Code only — uses platform primitives (path-conditional rules, lifecycle hooks, slash commands, specialist agents, quality gates)
