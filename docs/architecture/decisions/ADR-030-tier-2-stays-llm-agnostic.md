---
type: adr
id: ADR-030
title: Tier-2 binary stays LLM-agnostic — agent dispatch lives in tier-1 markdown
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-05-02T21:30:00Z
references:
  adrs: [ADR-021, ADR-022, ADR-028, ADR-029]
  invariants: [INV-001]
  prds: []
  specs: []
amends: ADR-028
---

# ADR-030 — Tier-2 binary stays LLM-agnostic — agent dispatch lives in tier-1 markdown

## Status

**Accepted**

## Context

The tier-2 Go binary (`tools/edikt/`, distributed as `bin/edikt`) currently
shells out to the `claude` CLI in three places:

1. `cmd/migrate_sidecars.go` — partial-v0.5.x and v0.4.3 lift paths
   dispatch `claude -p /edikt:<kind>:compile <ID>` to fill in
   `topic` + `signals` from the parent prose.
2. `cmd/migrate.go` — the pre-v0.5.0 layout migration tries
   `claude -p /edikt:gov:compile` to regenerate compiled rules.
3. `internal/phasea/runner.go` — Phase A of the two-phase compile
   spawns `claude` as a parallel subagent dispatcher (the canonical
   path for sidecar resync per ADR-028).

Each of these hardcodes a specific LLM CLI (Anthropic's `claude`).
v0.7.0 is on the roadmap to support additional host agents — Codex,
Cursor, and others — and the existing tier-2 LLM dispatch breaks for
every project that runs edikt under a non-Claude agent. The user
discovers this only at migration time, when their tier-1 markdown
commands are happily orchestrated by their chosen agent but the tier-2
binary silently fails because `claude` is not on `PATH`.

The architectural separation already implied elsewhere in the codebase:

- ADR-021 establishes tier-2 binaries as deterministic helpers; LLM
  round-trips are the responsibility of tier-1 markdown.
- ADR-028 §"Phase B" makes the merge step pure (no `os/exec`, no
  `net/http`, no LLM); enforced by `tools/edikt/check/no-llm-in-merge.sh`
  and `internal/phaseb/purity_test.go`.
- ADR-029 lets tier-1 markdown invoke tier-2 binaries by exit code
  only, never by parsing output shape.

But ADR-028 §"Phase A" and the migration paths above currently sit on
the wrong side of that line. They run from inside Go and hard-code an
LLM CLI choice the user never made.

## Decision

**Tier-2 Go binaries MUST NOT spawn, invoke, or shell out to any LLM
CLI.** The `claude` binary is one option among many; the choice belongs
to the host agent (Claude Code, Codex, Cursor, whichever the user
runs), not to edikt's tier-2 layer.

When a tier-2 path needs an LLM round-trip (resync a stale sidecar,
extract topic/signals from prose, generate compiled rules from a body),
the tier-2 binary MUST instead:

1. Perform the mechanical work it can do unaided (read the file, parse
   the structured fields, validate the schema).
2. For the parts it cannot do mechanically, write a partial sidecar
   with `topic: needs-review` (and any other recoverable fields) so
   the structural state is consistent on disk.
3. Emit a machine-readable manifest of artifacts that need LLM resync,
   on stderr or via a `--json` document on stdout.
4. Exit cleanly. The user's subsequent `/edikt:upgrade` /
   `/edikt:gov:compile` slash command (tier-1 markdown, executed by
   the host agent) is responsible for reading the manifest and
   dispatching the locked extractor agent via whatever subagent
   mechanism the host agent natively supports.

The locked extractor agent itself stays as a single
`templates/agents/sidecar-extractor.md` artifact. What changes is
*who* dispatches it: previously the Go binary via `exec.Command(claude,
-p, slash)`, going forward the tier-1 markdown via the host agent's
subagent primitive.

This ADR amends ADR-028's Phase A description. Phase B's purity gate
extends to apply project-wide: every tier-2 source file is held to the
same `os/exec`/`net/http`/no-LLM-import discipline, not just `phaseb`.

### Roll-out

- **v0.6.0-rc3** lands the migrate refactor: `migrate sidecars
  --apply` writes mechanical-only sidecars (partial-needs-review for
  v0.4.3 and partial-v0.5.x). `commands/upgrade.md` (tier-1) handles
  the LLM resync via the host agent's subagent dispatch. `cmd/
  migrate.go`'s pre-v0.5.0 `/edikt:gov:compile` shell-out is also
  removed (tier-1 already triggers compile naturally after migration).
- **v0.7.0** lands the Phase A refactor: `internal/phasea/runner.go`
  no longer shells out to `claude`. The hot-path resync moves to
  tier-1 markdown. The two-phase split per ADR-028 stays; the dispatch
  layer changes hands.
- A new static-analysis gate (`tools/edikt/check/no-llm-in-tier-2.sh`)
  greps every non-test `.go` file under `tools/edikt/` for
  `exec.Command.*claude`, `exec.LookPath.*claude`, and the literal
  string `"claude"`. CI fails on any match. Phase A's exception is
  carved out by an exemption file with an explicit v0.7.0 deadline;
  the exemption is removed when Phase A is refactored.

## Consequences

**Positive:**

- edikt becomes agent-agnostic by construction — running it under
  Codex or Cursor "just works" once the host agent supports the same
  slash-command + subagent surface.
- The dispatch path consolidates in tier-1 markdown, where the user's
  chosen agent already executes commands. One control point for
  observability (the agent's transcript), one error surface (the
  agent's error rendering), one place to evolve.
- The Go binary's purity story extends from Phase B alone to all of
  tier-2. The static-analysis gate makes regressions fail-fast.
- `bin/edikt migrate sidecars --apply` becomes deterministic and
  fast (no LLM latency in the hot path of a stable migration).

**Negative:**

- Migration apply now ALWAYS produces partial-needs-review sidecars
  for v0.4.3 / partial-v0.5.x corpora; the user must run
  `/edikt:upgrade` (tier-1) to complete the resync. This is a
  two-step flow rather than one-shot. Mitigated by the upgrade
  command orchestrating both steps.
- Phase A's exception until v0.7.0 means the compile hot path retains
  its `claude` dependency for one more minor. Acceptable because
  Phase A's failure mode (claude missing) only fires on resync, not
  on every compile — most compiles after the v0.6.0 stable settle
  hit Phase B's no-op short-circuit.
- Adding a new host agent in the future still requires a small port
  of the slash-command surface to that agent's primitives. The
  tier-2 binary stays unchanged.

## Alternatives Considered

- **Make the LLM CLI configurable via `EDIKT_LLM_BIN`.** Rejected:
  papers over the architectural smell. Even with the config, the Go
  binary still owns the dispatch — a poor fit for users running edikt
  through Cursor's IDE-native agent (no CLI surface to configure).
- **Move to a generic "agent dispatch" abstraction inside Go that
  pluggably calls Claude / Codex / etc.** Rejected: every additional
  agent integration requires a Go release; tier-1 markdown evolves
  faster and is the natural seam for agent-specific code paths.
- **Keep the LLM dispatch in Go but skip on missing-binary failure.**
  Rejected: silent degradation to "topic: needs-review" without
  clear user-facing remediation is exactly the bug we hit on the
  ddd-workbench corpus. Better to make the architectural separation
  visible and have tier-1 own the dispatch by design.

## Directives

[edikt:directives:start]: #
source_hash: pending
directives_hash: pending
compiler_version: "0.6.0-rc3"
topic: tooling
paths:
  - "tools/edikt/**/*.go"
  - "tools/edikt/check/no-llm-*.sh"
scope:
  - implementation
  - design
  - review
directives:
  - "Tier-2 Go binaries (tools/edikt/, tools/<name>/) MUST NOT spawn, invoke, or shell out to any LLM CLI. NEVER call exec.Command(\"claude\", ...) or exec.LookPath(\"claude\") in tier-2 source. (ref: ADR-030)"
  - "Tier-2 paths needing an LLM round-trip MUST write partial sidecars with topic: needs-review for the structural fields, then exit. The host-agent-driven tier-1 markdown is responsible for the LLM resync via the agent's native subagent mechanism. (ref: ADR-030)"
  - "CI MUST grep every non-test .go file under tools/edikt/ for exec.Command-claude, exec.LookPath-claude, and the literal string \"claude\" and fail on any unexempted match. (ref: ADR-030)"
  - "Phase A's claude dispatch in internal/phasea/runner.go is exempt until v0.7.0; the exemption file MUST cite this ADR and a removal deadline. NEVER add new exemptions outside of phasea without amending this ADR. (ref: ADR-030)"
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
