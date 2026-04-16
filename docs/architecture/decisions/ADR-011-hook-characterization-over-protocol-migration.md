---
type: adr
id: ADR-011
title: Hook test suite uses characterization fixtures, not aspirational protocol fixtures
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-15T00:00:00Z
references:
  adrs: [ADR-008]
  invariants: []
  prds: []
  specs: [SPEC-004]
---

# ADR-011: Hook test suite uses characterization fixtures, not aspirational protocol fixtures

**Status:** Accepted
**Date:** 2026-04-15
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

edikt's 9 lifecycle hooks (`stop-hook.sh`, `subagent-stop.sh`, `post-compact.sh`, etc.) emit
a mix of JSON and plaintext via `echo`/`printf`. The Claude Code hook protocol specifies a
structured JSON output contract (`{continue, systemMessage, additionalContext, decision}`).

Phase 2 (v0.5.0) created 21 fixture pairs — payload JSON piped to each hook, expected output
JSON to diff against — and gated them behind `EDIKT_ENABLE_HOOK_JSON_TESTS=1`. The expected
outputs were written by hand against the desired future JSON protocol, not against what the
hooks actually emit today. As a result, every fixture was aspirational: if the gate were
flipped on, all 21 would fail immediately.

Two options existed for v0.5.0:

1. **Migrate hooks** — rewrite each hook to emit JSON per the protocol, then enable the tests.
2. **Characterize hooks** — rewrite the expected outputs to match what hooks actually emit today,
   enable the tests immediately, defer the hook rewrites to v0.6.0.

How should edikt's hook unit test suite relate to the current vs. desired hook behavior?

## Decision Drivers

- Test suite must be a regression net, not a CI liability. Aspirational fixtures that always
  fail provide zero regression signal and actively harm CI confidence.
- Hook behavior changes carry risk. Rewriting 9 hooks mid-v0.5.0, which is a stability release,
  introduces behavior risk that conflicts with v0.5.0's stated goal.
- Fixture expected outputs must be verifiable by running the code. Hand-authored aspirational
  JSON is an unverified contract — it says what we want, not what we ship.
- The gap between aspirational and actual needs to be visible, named, and tracked — not hidden
  behind a gate.

## Considered Options

1. **Characterization** — rewrite expected outputs to match actual hook emissions; gate becomes
   opt-out; 3 non-characterizable fixture pairs removed with documented rationale.
2. **Protocol migration** — rewrite hooks to emit the JSON protocol; expected outputs stay;
   gate flips on once hooks are migrated.
3. **Delete the test suite** — remove hook tests entirely until hooks emit JSON. Deferred the
   problem indefinitely with no regression coverage.

## Decision

Hook fixture expected outputs encode what v0.5.0 hooks **actually emit**, verified by running
`cat payload.json | bash hook.sh` and capturing output. The test suite is a characterization
regression net — it catches behavior changes, not protocol non-compliance.

Hook protocol migration (rewriting hooks to emit structured JSON) is deferred to v0.6.0
(Phase 2b.ii) as an independent, separately-scoped deliverable. The two concerns — "do hooks
emit what they used to emit" and "do hooks emit the desired protocol" — are decoupled.

Specific consequences locked in by this decision:

- `EDIKT_ENABLE_HOOK_JSON_TESTS=1` opt-in gate replaced with `EDIKT_SKIP_HOOK_TESTS=1`
  opt-out. Tests run by default; skipping is the exception.
- 3 fixture pairs removed rather than weakened: `pre-compact` (plaintext output), 
  `session-start-with-edikt` (plaintext output), `subagent-stop-critical` (embeds git
  user identity and timestamps — nondeterministic by design). Each removal is documented in
  `fixtures.yaml` §9.1 with a `target_phase` pointer.
- `_runner.sh` is immutable. CWD-dependent hook staging is added via a separate
  `_staged_runner.sh` extension file, keeping the base runner unmodified.
- `fixtures.yaml` §9.1 expected-output records carry a characterization rationale in each
  `_note` field explaining **why** the output is what it is, not what it aspires to be.

## Alternatives Considered

### Protocol migration in v0.5.0

- **Pros:** Closes the gap between spec and implementation immediately; all 21 fixture pairs
  characterizable once hooks emit JSON.
- **Cons:** v0.5.0 is a stability release — rewriting 9 hooks is a significant behavioral
  change scope. Each rewritten hook is a regression surface. Hooks that emit plaintext today
  (`pre-compact`, `session-start`) serve users who read that plaintext; silent migration to
  JSON changes observable behavior. Risk/reward is poor for a stability cycle.
- **Rejected because:** behavior risk incompatible with v0.5.0 stability mandate.

### Delete the test suite

- **Pros:** No maintenance burden, no aspirational fixtures.
- **Cons:** Zero regression coverage on hook behavior. Any future hook change silently breaks
  user-facing behavior with no safety net.
- **Rejected because:** a characterization suite with 18 passing fixtures is strictly better
  than no suite.

## Consequences

- **Good:** Test suite provides immediate regression coverage for 18 fixture pairs. Any hook
  behavior change is caught before shipping. Gate polarity is honest — tests run by default.
- **Good:** Fixture expected outputs are machine-verifiable. `verified_by` commands in
  `fixtures.yaml` §9.1 document exactly how each output was produced.
- **Bad:** 3 fixture pairs remain absent until v0.6.0 Phase 2b.ii. `pre-compact`,
  `session-start-with-edikt`, and `subagent-stop-critical` blocking path have no regression
  coverage until hooks are migrated.
- **Bad:** Aspirational vs. characterized distinction lives only in `fixtures.yaml` `_note`
  fields and `target_phase` pointers — there is no machine-readable `status:` field per
  fixture record yet. That schema addition is tracked in ROADMAP §5.2 for v0.6.0.
- **Neutral:** Two test infrastructure files instead of one (`_runner.sh` +
  `_staged_runner.sh`). The split is intentional (immutable base + extension layer per
  ADR-005) and self-documenting.

## Confirmation

- `EDIKT_SKIP_HOOK_TESTS` MUST NOT appear in `test/run.sh` as a required env var. If it does,
  the gate polarity has regressed to opt-in.
  ```bash
  ! grep -q 'EDIKT_ENABLE_HOOK_JSON_TESTS' test/run.sh test/unit/hooks/test_*.sh
  ```
- All 9 hook test suites MUST pass without `EDIKT_SKIP_HOOK_TESTS=1` set:
  ```bash
  for t in test/unit/hooks/test_*.sh; do bash "$t"; done
  ```
- Every removed fixture pair MUST have a `_note` in `fixtures.yaml` §9.1 explaining the
  removal and a `target_phase` or `target_contract` field identifying when it will return.
- `_runner.sh` MUST NOT contain `run_staged_fixture`. That function lives only in
  `_staged_runner.sh`.

## Directives

[edikt:directives:start]: #
source_hash: 98ae95c994d78bd66729026cf81dd0eaaf7ea4bcce5a9f9abc778e38c7ce02ca
directives_hash: e007fa6fac5dd6ad482dd43cf43ff9ae29996594d06022849a2b01b90ee94b77
compiler_version: "0.4.3"
paths:
  - "test/unit/hooks/**"
  - "test/fixtures/hook-payloads/**"
  - "test/expected/hook-outputs/**"
  - "docs/product/specs/SPEC-004-v050-stability/fixtures.yaml"
scope:
  - implementation
  - review
directives:
  - Hook fixture expected outputs MUST encode what hooks actually emit today, verified by running the hook against the payload. NEVER write expected outputs by hand against a desired future protocol. (ref: ADR-011)
  - The hook test gate MUST be opt-out (`EDIKT_SKIP_HOOK_TESTS=1`), not opt-in. Hook tests run by default. (ref: ADR-011)
  - `test/unit/hooks/_runner.sh` MUST NOT be modified to add CWD-staging or other hook-specific behavior. CWD-dependent extensions go in `_staged_runner.sh`. (ref: ADR-011)
  - Fixture pairs that cannot be characterized deterministically MUST be removed with a `_note` explaining why and a `target_phase` identifying when they will return. NEVER weaken the runner diff to tolerate nondeterministic output. (ref: ADR-011)
  - Hook protocol migration (rewriting hooks to emit structured JSON) is v0.6.0 scope. NEVER rewrite hook behavior in a stability release. (ref: ADR-011)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-15*
