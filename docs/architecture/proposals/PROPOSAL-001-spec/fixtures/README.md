# Fixtures — PROPOSAL-001 Spec

Concrete example files showing every block state an implementation must handle. Future Claude can read these to know exactly what valid input and output look like.

## Block state fixtures

| File | State | Use case |
|---|---|---|
| [`adr-fresh.md`](adr-fresh.md) | Just created by `/edikt:adr:new` before compile auto-chains. Empty stub block. | First-compile input. |
| [`adr-populated.md`](adr-populated.md) | Compiled once, has auto directives and valid hashes. Empty manual/suppressed lists. | Fast-path input. |
| [`adr-with-manual.md`](adr-with-manual.md) | Has both auto directives and user-added manual directives. | `gov:compile` merge input. |
| [`adr-with-suppressed.md`](adr-with-suppressed.md) | Has auto directives and suppressed entries. | `gov:compile` filter input. |
| [`adr-with-both.md`](adr-with-both.md) | All three lists populated. Real-world post-iteration example. | `gov:compile` full merge input. |
| [`adr-hand-edited.md`](adr-hand-edited.md) | User added a line to `directives:` by hand. `directives_hash` stale. | Interview trigger input. |
| [`adr-legacy-v02.md`](adr-legacy-v02.md) | v0.2.x format, no hashes, no manual/suppressed. | Backward-compat input. |

## Artifact-type variants

The same state patterns apply to invariants and guidelines. These variants show each type's body shape but use the same schema for the directive block.

| File | Type |
|---|---|
| [`invariant-populated.md`](invariant-populated.md) | Invariant with populated directives |
| [`guideline-populated.md`](guideline-populated.md) | Guideline with populated directives (uses new `/edikt:guideline:compile`) |

## Hash test vector fixtures

| File | Purpose |
|---|---|
| [`hashes/test-vector-3.md`](hashes/test-vector-3.md) | Matches test vector 3 from `../hash-reference.md` |

(Only test vector 3 is fleshed out — the others can be derived from the reference implementation when tests are implemented. Each fixture is a tiny self-contained file.)

## Usage

When implementing `/edikt:<artifact>:compile`:
- Use `adr-fresh.md` as input for testing the "first compile" path
- Use `adr-populated.md` for the fast-path test
- Use `adr-hand-edited.md` for the interview trigger test (in headless: assert error)
- Use `adr-legacy-v02.md` for the backward-compat path

When implementing `/edikt:gov:compile`:
- Use `adr-with-both.md` to test the full merge formula
- Use `adr-with-manual.md` to test that manual directives are included
- Use `adr-with-suppressed.md` to test that suppressed directives are filtered

**Don't modify these files** unless the schema itself changes via a new ADR superseding ADR-008.
