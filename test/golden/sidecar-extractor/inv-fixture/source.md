---
type: invariant
id: INV-FIXTURE-A
title: Test fixtures never depend on the host's home directory
status: active
created_at: 2026-05-02T00:00:00Z
---

# INV-FIXTURE-A: Test fixtures never depend on the host's home directory

## Statement

Test sandboxes MUST NOT read from `~/.claude/`, `~/.edikt/`, `~/.aws/`, or any other home-directory configuration on the host machine. NEVER copy host secrets into a test sandbox under any circumstance.

## Rationale

A test that reads `~/.claude/settings.json` or `~/.aws/credentials` will pass on the developer's laptop and fail in CI for confusing reasons; worse, a benchmark that uploads transcripts can leak the host's secrets. Test isolation is the only protection.

## Enforcement

The test harness MUST set `HOME=$SANDBOX_DIR` before invoking any subprocess and MUST verify, via a CI lint, that no test source file references the literal path `~/.claude` or `~/.edikt`.
