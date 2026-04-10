# PROPOSAL-001 Specification Artifacts

This directory contains the formal specification for v0.3.0's three-list directive schema and hash-based caching. It exists alongside [`PROPOSAL-001-adapt-and-measure.md`](../PROPOSAL-001-adapt-and-measure.md) to provide machine-readable and example-based references that eliminate ambiguity during implementation.

**Future Claude (or human) implementing v0.3.0 — read this directory before writing code.** The proposal doc describes *why*. These files describe *exactly what*.

## Contents

| File | Purpose |
|---|---|
| [`schema.yaml`](schema.yaml) | Formal YAML schema for the directives block. Single source of truth for field names, types, and requiredness. |
| [`glossary.md`](glossary.md) | Terminology reference. Prevents confusion between `directives` / `manual_directives` / `suppressed_directives`, between `<artifact>:compile` and `gov:compile`, etc. |
| [`hash-reference.md`](hash-reference.md) | Exact hash algorithm with reference implementation and test vectors. |
| [`fixtures/`](fixtures/) | Concrete example artifacts in every block state (fresh, populated, hand-edited, legacy, etc.). Claude can read these to know what valid output looks like. |
| [`anti-patterns.md`](anti-patterns.md) | Explicit "don't do this" list. Catches common mistakes before they happen. |
| [`file-changes.md`](file-changes.md) | Per-phase file-by-file change checklist for v0.3.0 implementation. |

## How to use this spec

**When implementing a phase:**
1. Read the phase's file-changes.md entry to know which files to touch.
2. Read schema.yaml to know the exact block structure to write.
3. Read hash-reference.md to know the exact hash algorithm.
4. Read relevant fixtures to see before/after examples.
5. Check anti-patterns.md before committing to avoid known mistakes.

**When writing tests:**
1. Use fixtures as test inputs.
2. Use hash-reference.md test vectors as assertion values.
3. Use glossary.md to keep test names and error messages consistent.

**When reviewing an implementation:**
1. Verify the code matches schema.yaml literally (not approximately).
2. Verify hashes match the test vectors in hash-reference.md.
3. Verify no anti-patterns from anti-patterns.md are present.
4. Verify behavior matches ADR-008's directives block.

## Status

- **2026-04-09** — Created during Part 1 design lockdown. Captures Q1–Q6 decisions + Q5 deep-dive design.
- **Updated by**: never (by humans). Updated via ADR supersession when contracts change.
