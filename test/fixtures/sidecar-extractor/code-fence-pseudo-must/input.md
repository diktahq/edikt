---
type: adr
id: ADR-099
title: "ADR-099 — Hook JSON Emission"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Hook JSON Emission

**Status:** Accepted
**Date:** 2026-05-03

## Context

Hooks need to emit structured JSON to communicate with the Claude host.

## Decision

All hooks MUST emit structured JSON via `python3 -c 'import json,sys; print(json.dumps(...))'`.
Hook scripts MUST NOT build JSON by string concatenation.

The following is an illustrative example only — the MUST NOT rule applies to the pattern, not this code:

```bash
# MUST NOT do this:
echo '{"message": "'"$USER_INPUT"'"}'
```

## Consequences

### Good
- Guaranteed parse safety regardless of input content
