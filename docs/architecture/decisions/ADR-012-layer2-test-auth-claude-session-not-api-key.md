---
type: adr
id: ADR-012
title: Layer 2 integration tests authenticate via claude CLI session, not ANTHROPIC_API_KEY
status: accepted
decision-makers: [Daniel Gomes]
created_at: 2026-04-16T00:00:00Z
references:
  adrs: [ADR-011]
  invariants: []
  prds: [PRD-002]
  specs: [SPEC-004]
---

# ADR-012: Layer 2 integration tests authenticate via claude CLI session, not ANTHROPIC_API_KEY

**Status:** Accepted
**Date:** 2026-04-16
**Decision-makers:** Daniel Gomes

---

## Context and Problem Statement

SPEC-004 §9.2 described Layer 2 integration tests as "gated on `ANTHROPIC_API_KEY`" and the
Phase 14 CI workflow snippet used `ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}`. Phase 12
implemented this literally — `pytest_sessionstart` called `pytest.exit()` when `ANTHROPIC_API_KEY`
was unset.

During implementation, inspection of the `claude-agent-sdk` package (v0.1.59) revealed that
it never reads `ANTHROPIC_API_KEY`. It spawns a bundled `claude` binary via subprocess and
communicates over JSON streaming. Authentication is the user's Claude subscription session
stored in `~/.claude/`. This is the same credential used interactively — no separate API key.

SPEC-004's own decisions-locked section (§16 item 4) states: "No cost gating — maintainer
subscription covers it." This contradicts the `ANTHROPIC_API_KEY` requirement and confirms
the intent was always subscription-based, not API-key-based.

A second problem surfaced alongside the auth mechanism: the original gate called
`pytest_sessionstart`, which blocked the entire pytest session — including regression museum
tests that are pure Python reference implementations and never call claude at all.

How should Layer 2 integration tests verify that claude is available and authenticated?

## Decision Drivers

- `claude-agent-sdk` spawns the bundled `claude` CLI — `ANTHROPIC_API_KEY` is never read by the SDK
- Developer ergonomics: a logged-in Claude Code user should be able to run tests without obtaining a separate API key
- CI compatibility: headless/non-interactive environments need a credential mechanism
- Regression museum tests use Python reference implementations and MUST NOT be gated on claude auth
- No silent skips — unauthenticated runs must fail loudly for SDK tests

## Considered Options

1. **ANTHROPIC_API_KEY only** — as written in SPEC-004 §9.2 and the Phase 12 prompt
2. **claude session only** — check `~/.claude/` for credential files
3. **Either: claude session OR ANTHROPIC_API_KEY** — accept whichever is present

## Decision

Layer 2 SDK tests gate on claude authentication using `pytest_collection_finish`, which checks
that either a claude CLI session exists (`~/.claude/credentials` or equivalent) OR
`ANTHROPIC_API_KEY` is set in the environment. If neither is present when SDK tests are
collected, `pytest.exit()` fires with a clear message listing all three remediation paths:
`claude auth login`, set `ANTHROPIC_API_KEY`, or `SKIP_INTEGRATION=1`.

Regression museum tests (`test/integration/regression/`) MUST be runnable without any
authentication. The gate MUST NOT fire when only regression tests are collected.

The gate uses `pytest_collection_finish` (post-collection) rather than `pytest_sessionstart`
(pre-collection) so the scope of what was collected is known before gating.

`ANTHROPIC_API_KEY` remains supported as the CI/headless fallback. It is not the primary
mechanism — it is an escape hatch for environments where interactive login is not possible.

## Alternatives Considered

### ANTHROPIC_API_KEY only (original spec)
- **Pros:** matches the CI YAML snippet in SPEC-004 §9 verbatim; familiar pattern
- **Cons:** factually wrong — the SDK never reads it; breaks every developer who is logged in
  to Claude Code but has not obtained a separate API key; contradicts SPEC-004 §16 item 4
- **Rejected because:** incorrect description of the SDK's actual auth mechanism

### claude session only
- **Pros:** pure subscription model; no API key management
- **Cons:** breaks CI — non-interactive environments cannot do `claude auth login`; no
  escape hatch for automated pipelines
- **Rejected because:** CI needs a headless credential path

## Consequences

- **Good:** Developers logged in to Claude Code can run Layer 2 tests immediately without
  additional setup; consistent with "subscription covers it" policy
- **Good:** Regression museum tests run freely in any environment, including offline, making
  them useful as a fast local sanity check
- **Bad:** CI setup requires either a persisted claude session or `ANTHROPIC_API_KEY`; the
  Phase 14 CI workflow YAML must use a claude auth mechanism, not a raw API key secret
  (or use `ANTHROPIC_API_KEY` as the documented fallback — both are valid)
- **Neutral:** `ANTHROPIC_API_KEY` is still present in `conftest.py` as a recognized
  credential; it just no longer has exclusive responsibility for auth

## Confirmation

- `python3 -m pytest test/integration/regression/ --no-header -q` passes without any
  environment variables set (no auth required)
- `python3 -m pytest test/integration/test_init_greenfield.py --collect-only` exits with
  "claude CLI not authenticated" when neither `~/.claude/credentials` nor
  `ANTHROPIC_API_KEY` is present
- Both `ANTHROPIC_API_KEY` and `pytest.exit` appear in `test/integration/conftest.py`
  (AC-12.9 verify command remains satisfied)
- CI workflow (Phase 14) uses either `ANTHROPIC_API_KEY` secret or a persisted claude
  session — both paths must be documented in `website/guides/` (Phase 14 deliverable)

## Directives

[edikt:directives:start]: #
source_hash: ""
directives_hash: ""
compiler_version: "0.4.3"
topic: agent-rules
paths:
  - "test/integration/**/*.py"
  - "test/integration/conftest.py"
scope:
  - implementation
  - review
directives:
  - Layer 2 SDK tests MUST gate on claude authentication using pytest_collection_finish, not pytest_sessionstart — collection-time gating lets the scope of collected tests determine whether auth is needed. (ref: ADR-012)
  - The auth gate MUST accept either a claude CLI session (credentials present in $CLAUDE_HOME) OR ANTHROPIC_API_KEY — never require both, never require only one. (ref: ADR-012)
  - Tests in test/integration/regression/ MUST run without any claude authentication — they are pure Python reference implementations. The gate MUST NOT fire when only regression tests are collected. (ref: ADR-012)
  - The claude-agent-sdk spawns a subprocess claude CLI; ANTHROPIC_API_KEY is a CI/headless fallback, NOT the primary auth mechanism. Never document it as the only option. (ref: ADR-012)
  - When the auth gate fires, the error message MUST list all three remediation paths: claude auth login, ANTHROPIC_API_KEY, and SKIP_INTEGRATION=1. Silent exits are forbidden. (ref: ADR-012)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #

---

*Captured by edikt:adr — 2026-04-16*
