# edikt experiments

Pre-registered engineering validation experiments for v0.3.0. Tests whether loading an Invariant Record into Claude's context actually reduces the rate at which Claude violates the rule.

**Full methodology, pre-registration, and hypothesis:** see [`docs/architecture/proposals/PROPOSAL-001-spec/experiments/`](../../docs/architecture/proposals/PROPOSAL-001-spec/experiments/) in the source tree.

## Quick start

```bash
# Run a single experiment (N=10 per condition by default)
./test/experiments/run.sh 01-multi-tenancy

# Run all experiments sequentially
./test/experiments/run.sh all

# Dry run — show what would execute without actually calling Claude
./test/experiments/run.sh 01-multi-tenancy --dry-run
```

## Requirements

- `claude` CLI installed locally (Claude Code headless)
- Active Claude Code subscription (the experiments use the user's subscription — no API billing)
- `sha256sum` or `shasum` for deterministic run IDs
- `jq` for parsing Claude Code's output

## What this is (and isn't)

**What this is:**
- Engineering validation — "does this feature I built actually work?"
- Publication-grade discipline as a shield against self-deception (pre-registration, human-natural prompts, committed assertions, honest negative results)
- A one-time-ish measurement to ship with the v0.3.0 release notes
- Reusable infrastructure for v0.4.0+ if we decide to build a fuller Tier 2 suite later

**What this is NOT:**
- A formal research paper (methodology is publication-grade; presentation is informal notebook-style)
- A CI gate (experiments never block commits, pushes, or releases)
- An LLM-as-judge rubric system (we use grep-based assertions, not evaluator-scored rubrics)
- Automated (experiments are user-invoked, not nightly or pre-push)

## Cost model

**Zero API spending.** Experiments run locally via `claude -p` using the user's Claude Code subscription. Each experiment is N=10 runs per condition × 2 conditions = 20 Claude invocations. At ~20-60 seconds per invocation, a single experiment takes 10-20 minutes of wall time.

All three experiments together: ~30-60 minutes of wall time, still zero API cost.

## Three pre-registered experiments

| # | Invariant | Language | Fixture |
|---|---|---|---|
| [01](fixtures/01-multi-tenancy/) | INV-012 Tenant isolation | Go | Minimal HTTP server with order repository + existing tenant-aware handlers |
| [02](fixtures/02-money-precision/) | INV-008 Money precision | Python | Module with existing Decimal-based pricing functions |
| [03](fixtures/03-timezone-awareness/) | INV-016 Timezone awareness | Python | Module with existing `datetime.now(UTC)` usage and a naive-hostile mock DB |

Each fixture has:

- `project/` — the working source code Claude operates on
- `prompt.txt` — the innocent human-natural task prompt (committed before running)
- `invariant.md` — the Invariant Record loaded in condition B (committed before running)
- `assertion.sh` — pass/fail check on Claude's output (committed before running)

## Methodology summary

See the full methodology at [`docs/architecture/proposals/PROPOSAL-001-spec/experiments/README.md`](../../docs/architecture/proposals/PROPOSAL-001-spec/experiments/README.md). Key commitments:

1. **Pre-registration before running** — fixture + prompt + assertion committed to git before any run
2. **Human-natural prompts** — reviewed for contamination, no hints at the invariant
3. **Committed assertion logic** — written before running, not post-hoc
4. **N=10 per condition** — baseline vs invariant-loaded
5. **Model version pinned and recorded** — every run captures Claude Code version and model
6. **Full transcripts preserved** — committed alongside summaries
7. **No quiet deletions** — failed experiments stay in the results directory
8. **Negative results honestly reported** — if the hypothesis is wrong, we publish that

## Results

Results live in `results/{experiment-id}-{date}/`. See `runner-spec.md` in the PROPOSAL-001-spec for the reporting format.

After running, the results summary is human-readable markdown. The full transcripts and per-run verdicts are committed alongside the summary for auditability.

## Running an experiment

1. Verify `claude` is installed: `which claude`
2. Verify the fixture is clean (no uncommitted modifications)
3. Run: `./test/experiments/run.sh {experiment-id}`
4. Review the generated `results/{experiment-id}-{date}/summary.md`
5. Commit results: `git add test/experiments/results/{experiment-id}-{date}/`

## Interpreting results

See the "Possible outcomes and responses" table in the full methodology doc. Short version:

| Outcome | What it means |
|---|---|
| Strong effect (baseline ≥5/10, invariant-loaded ≤1/10) | Hypothesis confirmed, invariant dramatically reduces the failure rate |
| Weak effect (baseline ≥5/10, invariant-loaded < baseline but > 1/10) | Hypothesis partially confirmed, worth investigating further |
| No effect (baseline ≤2/10) | Modern Claude is already good at this — hypothesis not supported for this invariant on this model |
| Inverted effect (invariant-loaded > baseline) | Invariant made things worse — investigate, may indicate a problem with the invariant wording |

In all cases, v0.3.0 ships the feature regardless. Experiments inform the release notes framing, they don't gate the release.
