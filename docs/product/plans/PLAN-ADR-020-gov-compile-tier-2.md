# Plan: ADR-020 — gov-compile tier-2 migration

## Overview

**Task:** Implement ADR-020. Migrate `/edikt:gov:compile`'s deterministic transformations from LLM round-trips to a tier-2 Go binary helper per ADR-015 + ADR-021. Keep LLM calls only for sentinel generation on new artifacts, hand-edit conflict interview, and contradiction-warning wording.

**Source ADRs:** [ADR-020](../../architecture/decisions/ADR-020-gov-compile-tier-2-migration.md) (the migration decision) + [ADR-021](../../architecture/decisions/ADR-021-go-as-tier-2-language.md) (Go as the language).
**Dependencies:** ADR-008 (hash format + three-list schema), ADR-015 (tier-2 tooling contract), ADR-016 (release signing), ADR-021 (Go binary contract).
**Target release:** v0.5.x follow-up (decision captured in v0.5.0).
**Total Phases:** 5
**Estimated Cost:** ~$0.64 (1 opus + 3 sonnet + 1 haiku)
**Created:** 2026-04-17

## Progress

| Phase | Status | Attempt | Updated |
|-------|--------|---------|---------|
| 1     | -      | 0/3     | -       |
| 2     | -      | 0/5     | -       |
| 3     | -      | 0/3     | -       |
| 4     | -      | 0/3     | -       |
| 5     | -      | 0/3     | -       |

## Model Assignment

| Phase | Task | Model | Reasoning | Est. Cost |
|-------|------|-------|-----------|-----------|
| 1 | Add `topic:` field to existing sentinel blocks | haiku | Mechanical edit across ~40 files | $0.01 |
| 2 | Write `tools/gov-compile/` Go module (cmd + pkg + tests) | opus | Novel Go module, ports 600 lines of compile.md logic | $0.80 |
| 3 | Update `commands/gov/compile.md` to delegate to helper | sonnet | Markdown + flow changes | $0.08 |
| 4 | Regression test: byte-equal output across runs | sonnet | Python pytest + fixture harness | $0.08 |
| 5 | CHANGELOG + integration with `edikt install gov-compile` | haiku | Docs + install manifest | $0.01 |

## Execution Strategy

| Phase | Depends On | Parallel With |
|-------|------------|---------------|
| 1     | None       | —             |
| 2     | None       | 1             |
| 3     | 2          | —             |
| 4     | 3          | —             |
| 5     | 3,4        | —             |

Phase 1 and Phase 2 are parallel (edit existing sentinels vs write new helper). Phase 3 depends on 2 (helper must exist). Phase 4 depends on 3 (can't test the delegation until it's wired). Phase 5 closes the loop.

---

## Phase 1: Add `topic:` field to every existing sentinel block

**Objective:** Eliminate the "LLM decides topic grouping" step at compile time. Every sentinel block gets an explicit `topic:` field pulled from the current compiled output's `<!-- topic: X -->` header.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `TOPICS SEEDED`
**Dependencies:** None

**Prompt:**
```
For each accepted ADR under docs/architecture/decisions/ and each active
INV under docs/architecture/invariants/:

1. Read the compiled output at .claude/rules/governance/<topic>.md and map
   each source file to its current topic via the `<!-- sources: ADR-NNN, ...`
   header.
2. Edit the source file's [edikt:directives:start]: # sentinel block to add
   a `topic:` field immediately after `compiler_version:`. Value is the topic
   filename without .md (e.g. "architecture", "hooks", "release").
3. INV files: all invariant directives land in governance.md (the index),
   not in a topic file. Use `topic: invariants` for these.

Accept-only constraint: do NOT re-edit directives or paths — just add the
topic: field. Touch nothing else inside the sentinel block.

INV-002 note: accepted ADRs are immutable EXCEPT for the sentinel-block
mutations that compile already performs (source_hash, directives_hash,
compiler_version, now topic). Adding `topic:` is a structural addition,
not a content edit, so it's in the same class as those fields.

When complete, output: TOPICS SEEDED
```

---

## Phase 2: Write `tools/gov-compile/` (Go)

**Objective:** Port the deterministic parts of `commands/gov/compile.md` into a Go binary (~500-800 lines across a few files) that can be invoked standalone. All logic except the three LLM-required cases moves here. Per ADR-021, tier-2 helpers are Go modules producing single static binaries.
**Model:** `opus`
**Max Iterations:** 5
**Completion Promise:** `HELPER READY`
**Dependencies:** None (can run in parallel with Phase 1)

**Prompt:**
```
Create tools/gov-compile/ with:

- go.mod / go.sum — Go module. Primary external dep: gopkg.in/yaml.v3
  for frontmatter and sentinel-block parsing. Everything else is stdlib
  (crypto/sha256, text/template, path/filepath, regexp, io/fs, os,
  encoding/json). No other deps unless proven necessary.
- cmd/gov-compile/main.go — CLI entry point. Flags: --check, --json,
  [paths]. Parses args, loads config, dispatches.
- internal/config/ — load .edikt/config.yaml, resolve paths.
- internal/parse/ — frontmatter + sentinel-block extraction.
- internal/hash/ — source_hash + directives_hash per ADR-008.
- internal/compile/ — three-list merge, effective_rules, topic grouping.
- internal/render/ — text/template rendering for topic files + index.
- internal/orphan/ — orphan detection + atomic state write (port the
  Python block from commands/gov/compile.md §12d).
- templates/topic.md.tmpl — text/template for a single topic file.
- templates/index.md.tmpl — text/template for governance.md index.
- *_test.go files next to each package — Go test convention.
- README.md — build / test / contribute section.

Build:
- `go build -o gov-compile ./cmd/gov-compile` produces a static binary.
- Verify with `file gov-compile` (should report "statically linked").
- Binary size target: under 15 MB.

The helper must:
1. Read `.edikt/config.yaml` for `paths:`. Resolve ADR/INV/guideline dirs.
2. Scan each dir for .md files. Parse YAML frontmatter (PyYAML safe_load).
3. Filter ADRs by `status: accepted`; INVs by `status: active` (or legacy
   no-status). Skip `superseded`, `deprecated`, `revoked`.
4. For each source file, extract the sentinel block between
   `[edikt:directives:start]: #` and `[edikt:directives:end]: #`. Parse
   its YAML. Read directives, manual_directives, suppressed_directives,
   topic, paths, scope, reminders, verification, canonical_phrases,
   behavioral_signal.
5. Compute `source_hash` (SHA-256 over body with sentinel block excluded,
   CRLF→LF normalized, trailing whitespace stripped per line).
6. Compute `directives_hash` (SHA-256 over `directives:` list joined by \n).
7. Fast-path skip: if BOTH hashes match the stored values in the sentinel
   block AND the compiled output is fresh, exit without writes.
8. Compute effective_rules via ADR-008 formula: `(directives - suppressed) ∪ manual`.
   Set difference by exact string match; union preserves directive order
   with manuals appended.
9. Group by `topic:` field. Fail with a clear error if any source has no
   topic (after Phase 1 this should be impossible; legacy blocks fall
   back to the LLM one-shot in the markdown command).
10. For each topic, render tools/gov-compile/templates/topic.md.tmpl with:
    frontmatter paths (from config or sentinel), directives list (effective_rules
    merged across sources for this topic, de-duplicated by exact string match,
    first-occurrence source ref preserved), topic name, sources list.
11. Render tools/gov-compile/templates/index.md.tmpl with: invariant directives,
    routing table, aggregated reminders (cap 10), aggregated verification (cap 15).
12. Write output atomically: tmp + os.replace. Mkdir as needed.
13. Orphan detection: port the Python block from compile.md §12d into a
    method call. Pass in current orphan set, previous state, return block/write decisions.
14. Cross-ref validation: for every directive that names `INV-NNN` or
    `ADR-NNN`, confirm the reference exists in source. Strip fabricated refs.
15. `--check` mode: do everything except the final write; exit non-zero on
    contradictions or errors. Suitable for CI.
16. `--json` mode: emit only the JSON output per compile.md Reference.

Determinism: sort all iterations (dict keys, file globs, directive de-dup
ordering). No timestamps in hashed content.

When complete, output: HELPER READY
```

---

## Phase 3: Update `commands/gov/compile.md` to delegate

**Objective:** Replace the 500+ lines of inline procedure in `commands/gov/compile.md` with a thin shell that invokes the tier-2 helper. LLM re-enters only for the three retained cases.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `COMMAND REWIRED`
**Dependencies:** 2

**Prompt:**
```
Rewrite commands/gov/compile.md to a thin wrapper around `edikt gov-compile`
(the tier-2 Python helper from Phase 2). Preserve:

- --check and --json flag parsing (delegate to helper)
- Interactive sentinel generation for artifacts that have no sentinel
  block (LLM-only; helper returns "needs-sentinel: [list]")
- Hand-edit conflict interview (LLM-only; helper returns
  "hand-edit-conflict: ADR-NNN") — ask one question per detected edit,
  record resolution, re-run helper with `--strategy=<user-choice>`
- Contradiction warning wording (helper returns mechanical contradiction
  pairs; command composes human-readable warning)

Remove the inline procedure for everything the helper now handles:
YAML parse, sentinel extract, three-list merge, topic group, template
render, orphan detection, cross-ref validate.

Add a compatibility note: if `edikt gov-compile` is not installed, the
command falls through to the legacy LLM path (the old inline procedure,
preserved under `## Legacy Procedure (pre-tier-2)`). Users without the
tier-2 helper installed still get working compile (per ADR-015 tier-1/2
parity rule).

When complete, output: COMMAND REWIRED
```

---

## Phase 4: Determinism regression test

**Objective:** Lock in the byte-equal-output guarantee. A test that compiles twice and diffs the output. Runs in CI.
**Model:** `sonnet`
**Max Iterations:** 3
**Completion Promise:** `DETERMINISM PINNED`
**Dependencies:** 3

**Prompt:**
```
Add tools/gov-compile/tests/test_determinism.py:

1. Copy the repo's current docs/architecture/ and .edikt/config.yaml into
   a tmp dir.
2. Run `edikt gov-compile` twice against the tmp dir.
3. Assert byte-equal output (both the index and every topic file) via
   `diff -r`.
4. Also assert no-op recompile (both hashes match): the second run's
   output has not changed AND the helper reports "skipped: hash match".

Add a CI job (or extend test/run.sh) that invokes this test. Include the
test in test/security/run.sh if it fits there; otherwise keep in
tools/gov-compile/tests/.

Add also: time-budget assertion. First run of 40-source corpus must
complete in < 5s. No-op recompile < 500ms.

When complete, output: DETERMINISM PINNED
```

---

## Phase 5: Release pipeline + install wiring + CHANGELOG

**Objective:** Cross-compile the Go binary in CI, attach to the signed release, wire `edikt install gov-compile`, update docs. Per ADR-021.
**Model:** `haiku`
**Max Iterations:** 3
**Completion Promise:** `RELEASE READY`
**Dependencies:** 3,4

**Prompt:**
```
1. .github/workflows/release.yml: add a build-helpers job that runs after
   the build job. For each of {darwin-amd64, darwin-arm64, linux-amd64,
   linux-arm64}:
     - Cross-compile: GOOS=$OS GOARCH=$ARCH CGO_ENABLED=0 \
         go build -trimpath -ldflags='-s -w' -o gov-compile-$OS-$ARCH \
         ./tools/gov-compile/cmd/gov-compile
     - Verify statically linked (file command).
     - Emit SHA-256 to append to the pre-existing SHA256SUMS BEFORE
       the cosign sign-blob step (so the binary hashes are inside the
       signed manifest).
   Upload the four binaries as release assets alongside the payload
   tarball.
2. bin/edikt: add `install gov-compile` subcommand. Logic:
     - Detect host OS + ARCH (uname -s, uname -m mapped to GOOS/GOARCH).
     - Download the matching binary from the release asset URL.
     - Verify against the cosign-verified SHA256SUMS (reuse the existing
       _cosign_verify_release_checksums flow from install.sh).
     - chmod +x, atomic move into $EDIKT_ROOT/tools/gov-compile.
     - Symlink $EDIKT_ROOT/bin/gov-compile -> $EDIKT_ROOT/tools/gov-compile
       so the helper is invocable as `gov-compile` from $PATH.
   Idempotent uninstall: `edikt uninstall gov-compile` removes the binary
   and the symlink; exits 0 if already removed.
3. Update CHANGELOG.md: add to the v0.5.x section describing the new
   helper, time numbers from Phase 4 tests, and the one-liner
   `edikt install gov-compile`.
4. Update docs/guides/upgrade-v0.5.0.md with the opt-in install
   instruction for users who want the fast path.
5. Add the new helper to README.md's feature list (one bullet).

When complete, output: RELEASE READY
```

---

## Known risks

- **INV-002 + topic: field on accepted ADRs (Phase 1).** Adding `topic:` to an accepted ADR's sentinel block is a structural mutation. The existing precedent: compile already edits `source_hash`, `directives_hash`, `compiler_version` in those blocks — those are not considered content edits under INV-002. Adding `topic:` follows the same pattern. If we want to be strict, the first compile after this plan ships could emit `topic:` via LLM as part of the hand-edit path, avoiding manual backfill. Flag for review before Phase 1 executes.
- **Tier-2 install friction.** Zero: Go binary is static, no runtime, no pipx, no venv. Download + SHA verify + chmod + symlink. The legacy LLM path must stay functional for users who choose not to install the helper (per ADR-015 tier-1/2 parity). Phase 3 preserves it explicitly.
- **Go toolchain for maintainers.** Contributors need `go` installed to build the helper locally. Mitigation: `go` is one of the easiest toolchains to set up (single binary), and CI builds releases for users. Contributor docs in Phase 5 cover this.
- **Template engine choice.** `text/template` (Go stdlib) vs external libs like `html/template` or third-party options. Phase 2 uses `text/template` (stdlib) — no external template dep — as the default. If templates grow complex enough to need conditionals/loops beyond stdlib capability, revisit in a later phase.
