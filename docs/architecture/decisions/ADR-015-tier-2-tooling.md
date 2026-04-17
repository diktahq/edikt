---
type: adr
id: ADR-015
title: Tier-2 optional tools may depend on packages; core stays markdown-only
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-001, ADR-005, ADR-013]
  invariants: [INV-001]
  prds: [PRD-003]
  specs: [SPEC-005]
---

# ADR-015: Tier-2 optional tools may depend on packages; core stays markdown-only

**Status:** accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

INV-001 (plain markdown only) states that "every command and template MUST be a `.md` or `.yaml` file. No TypeScript, no compiled binaries, no build step. Installation is copy files only — no npm, no package managers." This invariant is load-bearing for edikt's install model and is the reason `install.sh` is a 100-line shell script rather than a package-management front-end.

SPEC-005 proposes `/edikt:gov:benchmark` — a new tool that runs adversarial prompts against the user's configured Claude model to test whether governance directives hold under pressure. The benchmark's execution path requires the Claude Agent SDK (a Python package) to invoke models, handle SIGINT cleanly, and stream progress. There is no markdown-only path that meets SPEC-005's acceptance criteria (AC-006c SIGINT-mid-call cancellation, AC-009 no-signal-skip semantics, AC-016b actionable error messages). Candidate alternatives were evaluated in SPEC-005 §Alternatives and rejected:

- **A1 — Bundle benchmark with core install and add a Python dependency to `install.sh`.** Violates INV-001 for every user, not just benchmark consumers. Users who only want static governance would be forced to install a package manager.
- **A2 — Shared Python module between command and test harness.** Does not solve the install-path dependency question; pushes the same tension deeper.
- **A3 — Shell out to `claude -p` from a pure-markdown command.** Cannot satisfy SIGINT cancellation (process-kill leaks state) or streaming progress (shell-escaping brittleness). PRD-002 also standardized on the Agent SDK for `claude`-invoking code.

A clean decision is needed: either weaken INV-001, create a structural carve-out, or ship SPEC-005 without the benchmark tool. The first is unacceptable; the third is the wrong priority. This ADR establishes the carve-out.

## Decision Drivers

- **INV-001 must hold verbatim for core governance commands.** Users installing edikt for static governance (`/edikt:gov:compile`, `/edikt:gov:review`, `/edikt:doctor`, ADR authoring) must not need Python, npm, or any package manager.
- **The benchmark tool is genuinely valuable but optional.** A user who never runs `/edikt:gov:benchmark` should never notice its dependencies exist.
- **Dependency isolation matters.** If a user installs the benchmark and later regrets it, uninstalling must leave tier-1 commands untouched.
- **Future tier-2 tools are likely.** Model-specific directive packs, live governance dashboards, and telemetry aggregators are plausible follow-ons; each should face the same tier-2 bar rather than negotiating a new exception.
- **Tiers must be visible and stable.** A user should know at install time which tier a tool is and trust that bar won't drift later.

## Considered Options

1. Weaken INV-001 to "core commands are markdown-only; supporting tools may depend on packages". Rejected — INV-001's strength is that it has no asterisks. Adding an exception at the invariant level erodes the core guarantee.
2. Keep INV-001 unchanged and defer the benchmark to a separate downstream project. Rejected — the benchmark is central to SPEC-005's "governance holds under literal execution" claim; splitting it weakens the release.
3. **Establish a tier-2 carve-out via this ADR (accepted).** INV-001 holds for tier-1. Tier-2 tools are explicit, opt-in, and isolated.

## Decision

edikt distinguishes two tiers of shipped tools. INV-001 applies to tier-1 without modification. Tier-2 tools may depend on packages provided three conditions hold.

### Tier-1 — core governance commands

- `install.sh` and the global installer install **only** tier-1 artifacts.
- Every tier-1 command file is a `.md` or `.yaml` file; no tier-1 code is compiled; tier-1 install is file-copy only.
- Tier-1 commands MUST NOT read from, write to, or depend on any tier-2 file or helper at runtime.
- INV-001 applies verbatim to every tier-1 artifact with no exceptions.

### Tier-2 — optional opt-in tools

Tier-2 tools may depend on packages provided:

1. **Install is explicit.** Tier-2 tools are installed via `edikt install <tool>`, never bundled in `install.sh`, never added to a user's environment without an explicit command.
2. **Uninstall is isolated.** `edikt uninstall <tool>` removes tier-2 files and any package-managed dependencies for that tool. After uninstall, tier-1 command surface — files on disk, their content, their behavior — is byte-equal to its pre-install state. Uninstall MUST be idempotent: repeated runs exit 0 without error.
3. **Tier is frozen at install time.** A tool's tier is declared in its command frontmatter (`tier: 1` or `tier: 2`). Promoting a tool from tier-2 to tier-1 requires a major-version bump of edikt. Demoting tier-1 to tier-2 requires a major-version bump of edikt. This prevents silent install-profile drift.

Tier-2 tools MUST:

- Declare `tier: 2` in their command file frontmatter.
- Verify their package-install artifacts (e.g., vendored wheels) against the release checksum manifest established by ADR-013 before installing.
- Pin exact versions (`==`) for their package dependencies in any `pyproject.toml` / equivalent manifest — no `>=` or floating ranges.
- Use isolated environment for any pip-installed helpers: dedicated venv under `~/.edikt/venv/<tool>/` or `pipx`. Never install into ambient system Python.
- Fail fast with an actionable error message when prerequisites (Python version, venv creation, disk space) are unmet, before touching the filesystem.
- Roll back partial installs: if install fails after some files were copied, those files are removed before exit.

### Parity between tier-1 markdown and tier-2 supporting code

For tier-2 tools that ship both a markdown command surface and supporting code (e.g., `/edikt:gov:benchmark` + `tools/gov-benchmark/`), parity between the two is enforced by **tests**, not by **code reuse**. A single shared module is acceptable within the tier-2 boundary (the Python helper may share code with itself) but MUST NOT cross into tier-1. Tier-1 files depend on tier-2 at runtime is forbidden.

## Alternatives Considered

See "Considered Options" above. The detailed alternatives evaluation lives in `docs/product/specs/SPEC-005-directive-hardening-and-gov-benchmark/spec.md` §Alternatives (A1, A2, A3).

## Consequences

### Good

- INV-001 survives unchanged and remains a hard guarantee for the install surface users care most about.
- SPEC-005's `/edikt:gov:benchmark` tool can ship with the capability-level guarantees it needs (SIGINT handling, streaming progress, structured SDK invocation) without forcing every edikt user into a Python runtime.
- The tier system gives edikt a coherent answer for future tools that want to depend on packages: they go into tier-2, with the same install contract and the same isolation guarantees.
- The parity-by-tests rule reinforces INV-001's "no compiled code" posture — the markdown command is the authoritative contract; the supporting code is one valid implementation of that contract.

### Neutral

- Users of `/edikt:gov:benchmark` see a two-step install (`install.sh` then `edikt install benchmark`). This is slight friction versus a one-step install, but the friction is proportional to the feature's scope — users who don't want the tool never pay it.

### Negative

- Introduces a new concept (tier) that users and maintainers must understand. Mitigation: declare it in command frontmatter and surface it in `/edikt:doctor` output.
- Tier-2 tools will accumulate bespoke install logic over time. Mitigation: a shared `bin/edikt install <tool>` dispatcher with tool-specific hooks, rather than per-tool install scripts.

## Confirmation

To verify this ADR is in effect, check that:

- `install.sh` contains no package-manager invocations and references only tier-1 artifacts.
- `bin/edikt install benchmark` — the first tier-2 install verb (SPEC-005, Phase 9) — is the only `pip`-invoking code in the repo.
- Every command file in `commands/` that ships with a package-dependent helper declares `tier: 2` in frontmatter. Every other command declares `tier: 1` (or omits the field as shorthand for tier-1 at v0.6.0; the field becomes required in v0.7.0).
- Uninstalling any tier-2 tool leaves tier-1 file checksums unchanged, verified by `test/integration/test_install_tier2.py::test_tier1_checksums_unchanged`.
- `/edikt:doctor` surfaces the tier-2 install state (which tools are installed, which are not).

## Directives

[edikt:directives:start]: #
source_hash: 6846bed4ed6ccdf818545a9c78ce5ab9f86c57c1d0bbebaa6968ff877f688bfd
directives_hash: de2a16e7f0032d06217e3eb44b40f16daa55e8dfecb13264a71bda75ae124330
compiler_version: "0.6.0"
paths:
  - "install.sh"
  - "bin/edikt"
  - "commands/**/*.md"
  - "tools/**"
scope:
  - implementation
  - design
directives:
  - Tier-2 optional tools MUST be installed via `edikt install <tool>` and MUST NOT be bundled in `install.sh`. (ref: ADR-015)
  - Tier-1 command files MUST remain pure markdown with no compiled code, no build step, and no runtime dependency on tier-2 artifacts. INV-001 applies verbatim to tier-1. (ref: ADR-015)
  - Tier-2 tools MUST NOT modify any tier-1 command file, config, or state at install or uninstall time. After `edikt uninstall <tool>`, tier-1 file checksums MUST be byte-equal to their pre-install state. (ref: ADR-015)
  - A tool's tier MUST be declared in its command frontmatter as `tier: 1` or `tier: 2`. A tool's tier is frozen at install time; promoting or demoting a tool across tiers requires a major-version bump of edikt. (ref: ADR-015)
  - Tier-2 install MUST verify any vendored package artifacts against the release checksum manifest from ADR-013 before executing the package install. Mismatch MUST abort with a clear message. (ref: ADR-015)
  - Tier-2 install MUST pin exact versions (`==`) for package dependencies. Floating ranges (`>=`, `~=`, `*`) are forbidden in tier-2 manifests. (ref: ADR-015)
  - Tier-2 package helpers MUST install into an isolated environment (`~/.edikt/venv/<tool>/` venv or `pipx`), NEVER into the ambient system Python. (ref: ADR-015)
  - Tier-2 install MUST fail fast with an actionable error before touching the filesystem when prerequisites (Python version, venv creation, disk space) are unmet. Partial installs MUST roll back copied files on failure. (ref: ADR-015)
  - `edikt uninstall <tool>` MUST be idempotent — repeated runs exit 0 and tolerate missing state (already-removed files, already-uninstalled helpers) without error. (ref: ADR-015)
  - Parity between a tier-2 markdown command and its supporting code MUST be enforced by tests, not by shared modules crossing the tier-1/tier-2 boundary. (ref: ADR-015)
manual_directives: []
suppressed_directives: []
canonical_phrases:
  - "tier-2"
  - "edikt install"
  - "isolated environment"
behavioral_signal: {}
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
