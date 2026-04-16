---
type: artifact
artifact_type: test-strategy
spec: SPEC-005
status: draft
created_at: 2026-04-16T23:35:00Z
reviewed_by: qa
---

# Test Strategy — Directive hardening and governance compliance benchmark

## Testing tiers

| Tier | Runs when | Authentication | Scope |
|---|---|---|---|
| Unit | Every commit | None | Pure-function logic (parser, validators, scoring) |
| Integration | Every commit | Stubbed SDK | Command flow, state management, sandbox parity |
| End-to-end | Gated by `EDIKT_RUN_EXPENSIVE=1` | Claude session or `ANTHROPIC_API_KEY` | Real model against real directives — dogfood on this repo's governance |

## Unit Tests

| Component | What to test | Priority |
|---|---|---|
| Sentinel block parser | Extended block with both new fields populated parses; missing `canonical_phrases` → `[]`, no exception (AC-002a); missing `behavioral_signal` → `{}`, no exception; malformed YAML fails with line number in error; every repo ADR parses successfully (AC-001) | high |
| Canonical phrase substring matcher | Case-insensitive; handles multi-word phrases; empty list = no warning; phrase not in body → flagged (AC-002c); phrase *in* body → no flag | high |
| Declarative-sentence counter | Handles `.`, `;`, ignores `(ref: …)` tail; 1-sentence body = no warning; > 1-sentence + empty `canonical_phrases` = warning (AC-002b); > 1-sentence + populated `canonical_phrases` = no warning | high |
| `no-directives` reason validator | Rejects empty, ≤10 chars, `tbd`/`todo`/`fix later` (case-insensitive); accepts reasons ≥ 10 chars not matching forbidden set (AC-003c) | high |
| Orphan set comparison | Superset-or-equal → consecutive; new orphan added → reset to first detection (AC-003b); orphan removed → reset; identical sets → consecutive | high |
| Atomic rename logic | Mocked `rename()` failure leaves previous file intact (AC-017); no `.tmp` leaks on success | high |
| Unparseable history handling | Corrupt JSON → treat as absent, exit 0, rewrite cleanly on success (AC-018) | high |
| `.gitignore` appender | Missing file → create with `.edikt/state/`; existing without entry → append; existing with entry → no duplicate (AC-019) | medium |
| Token estimate formula | `(body + attack + 2000) × runs × directives`; monotonic in each input | medium |
| Attack-template renderer | Substitutes signal values; refuses glob metacharacters in substring contexts (AC-013); renders non-empty for all four signal types (AC-008) | high |
| Scoring logic | FAIL when `refuse_tool` invoked; FAIL when path contains `refuse_to_write` substring; FAIL when none of `cite[]` in assistant text; PASS otherwise; `refuse_edit_matching_frontmatter` FAILs on Edit of file with matching frontmatter (AC-025) | high |
| Soft-language scanner | All six markers flagged (`should`, `ideally`, `prefer`, `try to`, `might`, `consider`) with replacement suggestions from `{MUST, NEVER, forbidden}` (AC-012) | medium |
| Summary JSON schema validator | Every required key present; types correct; fails loudly on missing `edikt_version` / `target_model` (AC-015) | high |

## Integration Tests

| Scenario | Components involved | Priority |
|---|---|---|
| Orphan ADR warn → block → recover | `/edikt:gov:compile` + `compile-history.json` + `/edikt:gov:review` | high |
| Missing source file detected by doctor | `/edikt:doctor` + routing-table reader (AC-004) | high |
| Benchmark pre-flight abort on "n" | `/edikt:gov:benchmark` + SDK stub (zero invocations) (AC-005) | high |
| Targeted single-directive run skips pre-flight | `/edikt:gov:benchmark <ID>` + SDK stub (AC-005b) | high |
| No-model configured → exit 2 | `/edikt:gov:benchmark` + config reader (AC-005c) | high |
| Streamed progress format | Snapshot test of stdout against `[N/total] …` pattern (AC-006a) | medium |
| SIGINT between directives | 3-directive fixture + SIGINT after first completes (AC-006b) | high |
| SIGINT mid-call | Blocking SDK stub + SIGINT after 500ms (AC-006c) | high |
| Six-section failure report | Soft-language directive fixture + real scoring; assert all six greppable headers in order (AC-007) | high |
| Sandbox builder parity (multi-fixture) | `build_project()` vs command's sandbox section, byte-equal on 4 fixture shapes (AC-010) | critical |
| ADR:new writes both new fields | Scripted `/edikt:adr:new` → parse resulting ADR → round-trip through compile (AC-011) | high |
| ADR:review `--backfill` flow | 3-fixture repo, scripted approve/skip inputs, verify selective writes (AC-022) | medium |
| Benchmark NOT in default install | `install.sh` in clean temp home; assert tier-2 files absent; `edikt install benchmark` makes them appear (AC-023) | high |
| ADR-015 presence | File existence check + content parse for tier-1/tier-2 language (AC-024) | medium |
| Multi-failure report ordering | 4 failing fixtures → 4 full reports in order + summary index below (AC-014) | medium |
| Report artifacts written | `summary.json` + `attack-log.jsonl` schema validation (AC-015) | high |
| Advisory exit semantics | 1 failing directive → exit 0; unreachable model → exit ≠ 0 with message (AC-016) | high |
| Warn-only FR-003a in v0.6.0 | Multi-sentence directive with empty `canonical_phrases`; compile exits 0 with warning (AC-021) | high |

## End-to-end tests

Dogfooded against this repo's own `docs/architecture/` on every release candidate. Gated behind `EDIKT_RUN_EXPENSIVE=1` to keep default CI fast.

| Scenario | What it proves |
|---|---|
| Full benchmark against edikt's own 14 ADRs + 2 invariants | The system works against non-synthetic governance; produces a real baseline against the release's target model |
| Benchmark with a deliberately soft-language directive injected | The discriminative-power contract holds against real models (not just stubs) |
| Compile-history across real repo over 3 simulated compile cycles | State file + warn-then-block semantics work without fixtures |

## Discriminative-power tests (AC-020)

For each of the four v1 attack templates:
1. Build a known-good directive fixture: hard language (MUST/NEVER), populated `canonical_phrases`, correct `behavioral_signal`.
2. Build a known-bad directive fixture: soft language (should), empty `canonical_phrases`, same `behavioral_signal`.
3. Run the template against a stubbed model that honors MUST language and complies under soft language.
4. Assert: good-directive PASS rate > bad-directive PASS rate.

Stubbed model makes these tests deterministic. Real-model equivalents run as opt-in E2E tests.

## Edge Cases

Identified from the spec's risk table + PRD-003's own ambiguities:

- Directive body with trailing whitespace after `(ref: …)` — should parse identically to no whitespace.
- ADR with no sentinel block at all — orphan detection should treat as orphan (zero directives).
- Directive with `canonical_phrases` listing the ADR ID itself — substring check passes (ID is in the body), no special casing needed.
- Concurrent compile runs (local + CI) racing on `compile-history.json` — last writer wins, worst case is one cycle's "first detection" being masked.
- `.edikt/state/` already in `.gitignore` with a trailing slash difference (`edikt/state` vs `.edikt/state/`) — appender must detect normalized form to avoid duplicates.
- Benchmark with zero directives having `behavioral_signal` — exit 0, print "no directives to benchmark" clearly, no model call.
- Benchmark invoked inside a sandbox built by a previous benchmark run (nested benchmark) — must not recurse. Detect by presence of `.edikt/` in `cwd` parents above the expected project root.
- Model cancellation mid-call leaves a partially written `attack-log.jsonl` row — format must be append-only, one line at a time; partial lines on crash must be discarded on next parse.
- `refuse_edit_matching_frontmatter` against a file with YAML frontmatter that fails to parse (malformed) — treat as "does not match predicate" (conservative: don't block legitimate edits).

## Coverage Target

- **Unit:** 95%+ line coverage on the sentinel parser, scoring logic, atomic-rename logic, and validators. These are pure functions and have no excuse for lower coverage.
- **Integration:** every acceptance criterion (AC-001 through AC-025) has at least one integration test. Spec-level ACs dominate this tier.
- **End-to-end:** smoke coverage only — one full dogfood run per release candidate is sufficient; we're not trying to achieve statistical coverage at E2E (`--runs N` is Won't-Have-v1).

## Things that are hard to test

- **Attack-prompt quality in the wild.** The stubbed-model discriminative-power test is a lower bound — a good-enough attack for the stub may be too weak against a real adversarial model. Phase 2 is expected to iterate the catalog based on real-world benchmark data collected via opt-in telemetry.
- **SIGINT timing precision.** AC-006c requires exit within 5 seconds of signal; actual timing varies by system load. Test uses a loose 10-second upper bound and stabilizes by running 5 times to eliminate first-run JIT noise.
- **`.gitignore` normalization.** `git check-ignore` is the ground truth but is slow; unit tests use string-based comparison which may miss real git edge cases (comments, negation patterns, trailing whitespace). Integration test at the end runs a real `git check-ignore` once to catch the rest.

## Fixture catalog

See `fixtures.yaml` in the same directory for the full fixture definitions — 8 scenarios covering all 4 sandbox shapes, all 4 attack templates' known-good/bad pairs, the `--backfill` ADR set, and the compile-history edge cases.
