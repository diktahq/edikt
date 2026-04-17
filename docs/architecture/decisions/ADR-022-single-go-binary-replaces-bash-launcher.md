---
type: adr
id: ADR-022
title: Single Go binary replaces the bash launcher — strangler-fig migration
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-17T00:00:00Z
references:
  adrs: [ADR-015, ADR-021, ADR-020]
  invariants: [INV-001, INV-008]
  prds: []
  specs: []
---

# ADR-022: Single Go binary replaces the bash launcher — strangler-fig migration

**Status:** Accepted
**Date:** 2026-04-17
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

After ADR-020 (migrate gov:compile to tier-2) and ADR-021 (Go for tier-2 helpers),
the natural outcome was a separate `gov-compile` binary that users install alongside
the existing `bin/edikt` bash launcher. This creates a two-binary situation:

```
install.sh → bin/edikt (bash, ~3000 lines)   # the launcher users type daily
             + gov-compile (Go)               # the fast compile helper
```

For a governance tool, two binaries is too much surface to explain. Users ask
"which binary do I run?", "do I need both?", "why is compile separate?". The
correct answer is one binary that does everything.

The bash launcher (`bin/edikt`) was never protected by INV-001. INV-001 says
commands under `commands/` must be markdown. The launcher binary is the runtime
dispatcher — it has no reason to stay in bash now that we are already shipping a
Go binary for deterministic compilation.

Additionally: `bin/edikt` is a 3000-line bash script that has accumulated significant
complexity and is hard to test. Go's unit tests, static analysis, and cross-compilation
make the codebase more maintainable and reliable over time.

## Decision Drivers

- Single user-facing binary. `edikt version`, `edikt gov compile`, `edikt upgrade`
  all from one `edikt`. No installation mental overhead.
- INV-001 does not protect the launcher from being Go.
- The Go binary for `gov compile` already exists and works at 20ms.
- Bash launchers cannot be unit tested cleanly. Go can.
- Strangler-fig allows incremental migration: no big-bang rewrite, no risk of
  missing a bash edge case.
- `install.sh` is preserved as a small shell bootstrap (chicken-and-egg: it runs
  before any binary exists). It stays bash. It gets simpler over time.

## Decision

We will migrate `bin/edikt` (bash) to a single Go binary (`edikt`) using the
strangler-fig pattern:

### Phase 1 (v0.5.0 — current)

The Go binary is the user-facing `edikt` entry point. It handles the subcommands
that have been migrated so far:

- `edikt version` — native Go
- `edikt gov compile` — native Go (ADR-020, 20ms deterministic compile)
- `edikt gov check` — native Go (--check mode)

For ALL other subcommands, the binary execs `edikt-shell` (the renamed bash
launcher) with full stdin/stdout/stderr passthrough and `EDIKT_SHELL_CALLER=1`
to prevent re-entry loops. Users see no difference: `edikt upgrade` works,
`edikt doctor` works, `edikt migrate` works — they all delegate to the bash shell
transparently.

`install.sh` places BOTH artifacts from the release tarball:
- `bin/edikt` — the Go binary (user-facing)
- `bin/edikt-shell` — the renamed bash launcher (internal)

### Phase 2 (v0.5.x)

Migrate subcommands one by one into Go. Simple ones first:
`list`, `use`, `version`, `prune`, `doctor`. Medium: `upgrade`, `rollback`. 
Complex last: `install`, `migrate`, `dev`. Each migration shrinks `edikt-shell`.

### Phase 3 (v0.6.x target)

`edikt-shell` is empty or deleted. Pure Go binary. `install.sh` shrinks to
~40 lines: download + cosign verify + chmod + place. One artifact, one install.

## Alternatives Considered

### Keep two binaries permanently

- **Pros:** Less migration work.
- **Cons:** Confusing for users ("is `gov-compile` the same as `edikt`?").
  Governance tools should have minimal surface area. Two binaries is one too many.
- **Rejected because:** the strangler-fig migration costs minimal extra work and
  solves the confusion permanently.

### Rewrite everything in Go at once

- **Pros:** Clean slate, no bash shim.
- **Cons:** 3000 lines of bash handle many edge cases (NFS locks, symlink chains,
  platform detection, migration logic). A big-bang rewrite risks regressions for
  months.
- **Rejected because:** the strangler-fig approach gives us the user-facing win
  (one binary) immediately without the big-bang risk.

### Keep bash launcher, never migrate

- **Pros:** No work required.
- **Cons:** Bash is untestable, bash is not cross-platform (no Windows), bash
  accumulates tech debt faster than Go. Every new feature in the launcher gets
  harder to maintain.
- **Rejected because:** ADR-021 already committed us to Go for the deterministic
  path. Extending that commitment to the full launcher is the natural next step.

## Consequences

- **Good:** Users see and run one binary. `edikt gov compile` lives next to
  `edikt upgrade`. Explained in a sentence: "Install edikt, run edikt."
- **Good:** `install.sh` gets simpler with each Phase 2 migration; Phase 3
  reduces it to ~40 lines.
- **Good:** Windows support becomes possible once enough subcommands are in Go.
- **Bad:** `bin/edikt-shell` is a confusing internal artifact. Documented in
  architecture docs; never exposed in user-facing help or error messages.
- **Bad:** Phase 2 migration work — each subcommand ported requires understanding
  the bash logic and testing the Go replacement.
- **Neutral:** `install.sh` places two files from the release tarball in Phase 1.
  Phase 3 reduces this to one.

## Confirmation

- `edikt version` runs natively in Go. No `edikt-shell` invoked.
- `edikt gov compile [root]` runs natively in Go in ≤ 20ms. No `edikt-shell` invoked.
- `edikt upgrade` execs `edikt-shell` with full stdio passthrough. Output is
  byte-identical to running the bash launcher directly.
- `edikt-shell not found` produces a clear error message (not a hang, not a panic)
  when `bin/edikt-shell` is absent.
- No user-visible command (`edikt --help`, any subcommand) surfaces the word
  "edikt-shell" or any bash internals.
- `tools/gov-compile/.gitignore` excludes compiled binary artifacts; the binary
  is shipped only via release assets (ADR-016), never committed to the repo.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.4.3"
paths:
  - "tools/gov-compile/**"
  - "bin/edikt"
  - "bin/edikt-shell"
  - "install.sh"
scope:
  - implementation
  - design
  - review
directives:
  - The user-facing `edikt` binary MUST be the Go binary in `tools/gov-compile/`. The bash launcher (`bin/edikt-shell`) is an internal shim; it MUST NOT appear in any user-facing help text, error messages, or documentation. (ref: ADR-022)
  - Subcommands not yet migrated to Go MUST be delegated to `bin/edikt-shell` via `os/exec` with full stdio passthrough and `EDIKT_SHELL_CALLER=1` set to prevent re-entry loops. (ref: ADR-022)
  - Compiled binary artifacts (the `edikt` binary) MUST NOT be committed to the git repository. They are shipped only via release assets (ADR-016) and are excluded by `.gitignore`. (ref: ADR-022)
  - When `bin/edikt-shell` is absent, the Go binary MUST emit a clear actionable error message and exit non-zero. NEVER hang, never panic. (ref: ADR-022)
  - Phase 1 release tarballs contain BOTH `bin/edikt` (Go) and `bin/edikt-shell` (bash). `install.sh` places both. (ref: ADR-022)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-17*
