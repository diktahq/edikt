"""Compliance benchmark — runs the full corpus against the configured model.

pytest parameterizes over every case in every corpus file. Each case is
run against the model specified by --model (default: claude-sonnet-4-6).

Results are accumulated by the session hook in conftest.py and written
to benchmarks/results/<model>-<timestamp>.json with per-dimension and
overall compliance scores.
"""

from __future__ import annotations

from pathlib import Path

import pytest

from runner import Case, load_corpus, run_case

# Every corpus file under corpus/ is loaded at collection time.
CORPUS_FILES = ["invariants", "adrs", "governance", "agents_rules", "sdlc_chain"]


def _collect_all_cases() -> list[Case]:
    all_cases: list[Case] = []
    for name in CORPUS_FILES:
        all_cases.extend(load_corpus(name))
    return all_cases


ALL_CASES = _collect_all_cases()


@pytest.mark.asyncio
@pytest.mark.parametrize("case", ALL_CASES, ids=lambda c: c.id)
async def test_governance_compliance(
    case: Case,
    tmp_path: Path,
    model_under_test: str,
    n_runs: int,
    benchmark_result,
    request: pytest.FixtureRequest,
) -> None:
    """Run one corpus case N times against the model under test, score with Wilson CI."""
    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)
    result = await run_case(
        case, tmp_path, model_under_test,
        n_runs=n_runs, skip_on_outage=skip_on_outage,
    )
    benchmark_result(result)

    if result.final_verdict == "FAIL":
        reasons_sample = [r for run in result.runs for r in run.reasons][:5]
        pytest.fail(
            f"[{case.id}] compliance violation ({case.severity}/{case.dimension})\n"
            f"  runs: {result.n_pass} PASS, {result.n_fail} FAIL, {result.n_unclear} UNCLEAR (of {result.n_runs})\n"
            f"  Wilson 95% CI: [{result.wilson_lower:.2f}, {result.wilson_upper:.2f}]\n"
            f"  sample reasons: {reasons_sample}"
        )
    elif result.final_verdict == "UNCLEAR":
        pytest.skip(
            f"[{case.id}] CI spans 0.5 ({result.wilson_lower:.2f}-{result.wilson_upper:.2f}) — "
            f"need more runs (currently {result.n_runs})"
        )
