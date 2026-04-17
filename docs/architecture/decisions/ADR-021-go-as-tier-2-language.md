---
type: adr
id: ADR-021
title: Go is the language for tier-2 deterministic helpers
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-015, ADR-016, ADR-020]
  invariants: [INV-001]
  prds: []
  specs: []
---

# ADR-021: Go is the language for tier-2 deterministic helpers

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

ADR-020 locked in the decision to migrate `/edikt:gov:compile`'s deterministic work out of the LLM path and into a tier-2 helper per ADR-015. The original draft of ADR-020 proposed Python for the helper. Python works, but it drags user-side toolchain requirements into what should be a drop-in binary: Python 3.10+, pipx, isolated venvs, pinned deps resolved at install time. For a governance tool whose first-install experience is "copy a file, done" (INV-001 spirit), that's unnecessary friction.

This ADR picks the implementation language for tier-2 helpers so the next one (gov-compile) and every subsequent helper share a contract: single static binary, no runtime install, cosign-verified per ADR-016, drops in under `tools/<name>/`.

## Decision Drivers

- No user-side runtime dependency. A user who runs `edikt install gov-compile` should get a working binary without installing or managing a Python/Node/Ruby environment.
- Single-file binaries, small enough to ship as release assets without bloating the release footprint (target: < 15 MB per helper).
- Cross-compilable for at least darwin-amd64, darwin-arm64, linux-amd64, linux-arm64 from the release CI.
- Determinism-friendly: standard library covers YAML/JSON/templating/hashing/regex so edikt isn't hostage to third-party library drift.
- Familiar toolchain for edikt's target audience (infra/DevX engineers, Claude Code users) so contributor onboarding is low-friction.
- Compile times short enough that "edit helper → test → iterate" loops in local dev stay under 30 seconds.
- Good Sigstore/cosign integration story for signed releases (ADR-016).
- Stable language with a long backward-compat horizon (5+ years of "this binary still works").

## Considered Options

1. **Go.** Single binary, static by default, `GOOS`/`GOARCH` cross-compile, excellent stdlib for text/file work, large contributor pool, cosign is itself written in Go.
2. **Rust.** Smaller binaries (~2 MB) and stronger correctness guarantees. Cargo + crates.io is mature for YAML / templating / hashing.
3. **Deno + `deno compile`.** TypeScript syntax, single-executable output, rich stdlib. But bundles ~70 MB runtime per binary.
4. **Bun.** Same as Deno; bundles runtime; binary sizes similar.
5. **Zig.** Small, fast, great for embedding. Ecosystem immature for YAML / JSON schema / templating.
6. **Crystal / Nim.** Ruby-like / Python-like syntax, compiles to native. Communities are small; fewer prospective edikt contributors.
7. **Keep Python (per ADR-020 draft).** Requires Python 3.10+ and pipx on the host; isolated-venv install contract from ADR-015.

## Decision

We will adopt **Go** as the language for tier-2 deterministic helpers.

- All future tier-2 helpers under `tools/<name>/` are Go modules producing single static binaries.
- The release workflow cross-compiles each helper for darwin-amd64, darwin-arm64, linux-amd64, linux-arm64. Each binary's SHA-256 goes into the existing `SHA256SUMS` (cosign-signed per ADR-016).
- `bin/edikt install <helper>` downloads the matching binary for the host platform, verifies the signed SHA-256, chmods +x, and symlinks into `$EDIKT_ROOT/tools/<name>`.
- The bootstrap installer (`install.sh`) and the launcher (`bin/edikt`) stay bash. Chicken-and-egg for the installer (runs before any binary exists); small-and-stable-enough for the launcher (no build step needed for first install).
- Go dependency pinning uses `go.mod` + `go.sum` at build time. No user-side pinning needed because the binary is static. This satisfies ADR-015's "tier-2 deps MUST be pinned" directive via a different mechanism than `==` for Python; ADR-015 is clarified (not superseded) to acknowledge the Go path.

## Alternatives Considered

### Rust

- **Pros:** Smaller binaries (~2 MB with release + LTO). Strong type system catches whole classes of bugs at compile time. Cargo + crates.io is polished. Excellent cosign/sigstore tooling.
- **Cons:** Steeper learning curve for prospective edikt contributors. Longer compile times (tens of seconds for iterative dev). Smaller contributor pool in the infra/DevX adjacency where edikt's users live.
- **Rejected because:** the tooling win of Rust doesn't outweigh the contributor-onboarding tax. Go is the lingua franca of the user base we're serving; Rust would be the right call if edikt were a systems-level tool where microsecond latency or memory safety were dominant constraints, neither of which applies to gov-compile.

### Deno / Bun

- **Pros:** TypeScript-native, rich stdlib, `deno compile` produces a single executable. Familiar to JS/TS developers.
- **Cons:** Binaries bundle the full JS runtime (~60-100 MB). Cold start is slower. Still effectively shipping a runtime, just baked in. Release footprint grows fast across 4+ platform targets.
- **Rejected because:** binary size and cold-start latency clash with gov-compile's "under 500 ms no-op recompile" target.

### Zig / Crystal / Nim

- **Pros:** Each produces small native binaries. Zig and Nim have syntax approachable to Python and C users respectively.
- **Cons:** Ecosystem maturity for YAML parsing + templating + JSON schema is uneven. Contributor pool is small. Risk of a critical library going unmaintained.
- **Rejected because:** for a governance tool we want boring, stable choices. Each of these is a technology bet; Go is not.

### Keep Python

- **Pros:** Already widely installed. Familiar to most edikt users. Rich ecosystem.
- **Cons:** Users-side install friction (Python version, pipx, isolated venv). Non-static: helper breaks if a transitive dep changes on the host. Even with pinned deps, the install path is heavier than a single binary download.
- **Rejected because:** the whole point of tier-2 per ADR-015 is that the helper is opt-in and lightweight. Python undermines the "drop a binary in $EDIKT_ROOT/tools/" shape.

## Consequences

- **Good:** `edikt install gov-compile` becomes a download + SHA verify + chmod + symlink. No venv, no pip, no language runtime install. Minutes of friction collapse to seconds.
- **Good:** Binary is cosign-signed as part of the existing `SHA256SUMS` (ADR-016). No new signing pipeline to build.
- **Good:** Go's stdlib covers every dependency gov-compile needs (yaml.v3 is the one external dep; everything else is stdlib). This is a bounded surface for supply-chain review.
- **Good:** Same binary format for every future helper. `edikt install <name>` has a single install contract.
- **Good:** Cross-compilation from a single Linux runner in CI produces all four platform targets. No per-platform build matrix complication.
- **Bad:** Adds Go to the contributor toolchain. Mitigated by Go being one of the least-finicky languages to install and use. `go build` is the whole dev story.
- **Bad:** Slightly larger binaries than Rust (~8 MB vs ~2 MB per helper). For four platforms that's ~32 MB of release assets per helper. Acceptable; well under GitHub's release-asset budget.
- **Neutral:** ADR-015 gains a clarifying sentence about Go's static-binary approach satisfying its "tier-2 deps pinned" requirement via `go.mod` + build-time resolution rather than `==` at install time. Not a supersession.

## Confirmation

- `tools/gov-compile/` exists as a Go module (`go.mod` + `go.sum` committed) producing a binary named `gov-compile`.
- The binary links statically and runs on darwin-arm64, darwin-amd64, linux-amd64, linux-arm64 with no host-side Go install needed.
- `.github/workflows/release.yml` cross-compiles the binary for all four target platforms, adds per-binary SHA-256 entries to `SHA256SUMS`, uploads the binaries as release assets.
- `bin/edikt install gov-compile` downloads the platform-matching binary, verifies against the cosign-signed `SHA256SUMS`, symlinks into `$EDIKT_ROOT/tools/gov-compile`.
- Any future tier-2 helper ships under the same contract: `tools/<name>/` as a Go module, cross-compiled in CI, installed via `edikt install <name>`.
- ADR-015 is edited (in a draft ADR-NNN supersession if needed, or as prose clarification if strictly an addition) to note that Go binaries satisfy the "pinned deps" contract via `go.sum` at build time.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "tools/**"
  - ".github/workflows/release.yml"
  - "bin/edikt"
scope:
  - implementation
  - design
  - review
directives:
  - Tier-2 deterministic helpers under `tools/<name>/` MUST be Go modules producing single static binaries. Python, Node, Ruby, or other runtime-dependent languages are forbidden for this class of helper. (ref: ADR-021)
  - Tier-2 binaries MUST be cross-compiled in the release workflow for at least darwin-amd64, darwin-arm64, linux-amd64, linux-arm64. Each binary's SHA-256 MUST appear in the cosign-signed `SHA256SUMS` per ADR-016. (ref: ADR-021, ADR-016)
  - `bin/edikt install <helper>` MUST download the platform-matching binary, verify the SHA-256 against cosign-verified `SHA256SUMS`, chmod +x, and symlink into `$EDIKT_ROOT/tools/<helper>`. NEVER install via `pipx`, `npm`, or other package managers. (ref: ADR-021, ADR-015)
  - The bootstrap installer `install.sh` and the launcher `bin/edikt` MUST remain POSIX shell. NEVER rewrite these in Go — install.sh runs before any binary exists and the launcher must work on any POSIX system without a build step. (ref: ADR-021, INV-001)
  - Tier-2 Go dependencies MUST be pinned via `go.mod` and `go.sum` at build time. This satisfies ADR-015's "tier-2 deps pinned" contract via Go's native resolution, not via `==` at install time. (ref: ADR-021, ADR-015)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
