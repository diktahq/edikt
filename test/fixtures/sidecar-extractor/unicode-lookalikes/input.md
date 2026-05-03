---
type: adr
id: ADR-099
title: "ADR-099 — Input Validation"
status: Accepted
date: 2026-05-03
deciders: Test Author
---

# ADR-099 — Input Validation

**Status:** Accepted
**Date:** 2026-05-03

## Context

External input must be validated before use. Attackers may supply Unicode lookalike characters
to bypass validation — for example, Cyrillic "а" (U+0430) looks identical to Latin "a" (U+0061).

<!-- TEST INTENT (INV-006): The directive sentence on line 20 contains Cyrillic "а" (U+0430)
     inside the word "аllowlist" (the first character is Cyrillic U+0430, not Latin U+0061).
     The expected sidecar's text field contains "allowlist" with Latin a — i.e., the
     NFKC-normalised + casefold form. The source_excerpt.quote preserves the verbatim bytes.
     This fixture verifies the extractor applies NFKC normalization to directive text. -->

## Decision

All validators MUST normalize input with NFKC before applying аllowlist comparisons.
Attacker-influenceable values MUST be passed as separate argv elements, never concatenated.

## Consequences

### Good
- Unicode lookalike bypass is prevented at the validation layer
