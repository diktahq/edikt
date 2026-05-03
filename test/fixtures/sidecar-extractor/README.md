# Sidecar Extractor — Golden Corpus

Hand-authored deterministic fixtures for the `bin/edikt sidecar diff` comparator (Phase 6 / Phase 9 of PLAN-v060-governance-accuracy). No LLM in the loop for `input.md` or `expected.edikt.yaml` — every fixture is a precise spec for what the extractor SHOULD produce per Phase 2 Rules A–D and the v1 schema.

## Bug Taxonomy

Each fixture targets a specific failure mode. When a regression surfaces in the field, add a fixture for its exact class before fixing the extractor — the fixture is the regression test.

| Fixture | Bug Class | What It Catches |
|---|---|---|
| `adr-001/` | Baseline | Full ADR with paths, scope, prohibitions, reminders, verification |
| `modality-drift-fallback/` | Modality drift — Fallback | `Fallback:` prefix must stay `MAY`, never promoted to `MUST` (Rule D) |
| `modality-drift-alternatively/` | Modality drift — Alternatively | `Alternatively:` prefix must stay `MAY`, never promoted to `MUST` (Rule D) |
| `rejected-options-1/` | Prohibitions — 1 option | Single considered option (chosen) → `prohibitions: []` |
| `rejected-options-3/` | Prohibitions — 3 options | 3 options, 1 chosen, 2 rejected → 2 prohibition entries with distinct `derived_from` |
| `rejected-options-7/` | Prohibitions — 7 options | 7 options, 1 chosen, 6 rejected → 6 prohibition entries |
| `glob-character-classes/` | Glob fidelity | `**/*.{ts,tsx}` style globs must be preserved verbatim in `paths[]` |
| `glob-negation/` | Glob fidelity | `!**/_test.go` exclusion globs must be preserved verbatim in `paths[]` |
| `empty-decision/` | Empty section | ADR with empty `## Decision` → `directives: []` |
| `code-fence-pseudo-must/` | Code-block exclusion | `MUST NOT` inside a fenced code block is NOT extracted as a directive |
| `unicode-lookalikes/` | Unicode normalization | Cyrillic lookalike in prose → `text` uses NFKC-normalized Latin form; `quote` preserves verbatim bytes (INV-006) |
| `multi-paragraph-decision/` | Multi-paragraph | 4 decision paragraphs → 4 distinct directives, each with its own `source_excerpt` |
| `single-line-directive/` | Single directive | ADR with one-sentence Decision → exactly 1 directive |
| `sparse-inv/` | Invariant — sparse | Minimal INV (Statement only, no Enforcement) → 1 directive, empty `reminders`/`verification` |
| `verbose-inv/` | Invariant — verbose | Full INV (Statement + Rationale + Enforcement + Examples + Anti-patterns) → Statement directives + Enforcement reminders/verification; Rationale and Anti-patterns NOT extracted |
| `guideline-no-modals/` | Guideline verb normalization | Guideline with imperative prose but no MUST/SHOULD modals → directives use `MUST` via verb-normalization rule |

## Fixture Structure

Each fixture directory contains four files:

```
<fixture-name>/
  input.md            — the governance artifact (ADR / INV / guideline)
  expected.edikt.yaml — hand-authored spec: what the extractor SHOULD produce
  actual.edikt.yaml   — initially a copy of expected; replaced by `make regen-fixtures`
  fixture.yaml        — comparator config: model, temperature, seed, thresholds, hash_baseline
```

## Running the Comparator

```bash
# Single fixture
bin/edikt sidecar diff test/fixtures/sidecar-extractor/<name>

# All fixtures (same command CI runs)
for fixture in test/fixtures/sidecar-extractor/*/; do
  echo "→ $fixture"
  bin/edikt sidecar diff "$fixture" || exit 1
done
```

## Adding a Fixture for a New Failure Mode

When a new extractor regression surfaces in the field:

1. Create a new directory under `test/fixtures/sidecar-extractor/<bug-class>/`.
2. Write `input.md` — small (10–30 lines), focused on the single failure mode. Use ADR-099 / INV-099 / guideline-099 IDs to avoid collision with the dogfood corpus.
3. Write `expected.edikt.yaml` — the spec for what the extractor SHOULD produce. Validate it:
   - `schema_version: 1`
   - `topic` matches `^[a-z][a-z0-9-]{0,39}$`
   - `signals` each match `^[a-z0-9][a-z0-9 _.-]*$` (no slashes, no caps)
   - `directives[].text` ≤ 500 chars, includes `(ref: <ID>)` tail
   - `scope` from closed enum: `planning | design | implementation | review`
4. Copy `expected.edikt.yaml` to `actual.edikt.yaml` (initial seed for CI).
5. Compute the sha256: `shasum -a 256 expected.edikt.yaml | awk '{print $1}'`
6. Write `fixture.yaml` with `model: claude-sonnet-4-6`, `temperature: 0`, `seed: 42`, default thresholds, and the computed `hash_baseline`.
7. Verify: `bin/edikt sidecar diff test/fixtures/sidecar-extractor/<bug-class>` → exit 0.
8. Add a row to the Bug Taxonomy table above.
9. Fix the extractor, then run `make regen-fixtures` to regenerate `actual.edikt.yaml` and confirm the comparator still passes.
