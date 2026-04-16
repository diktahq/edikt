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
    benchmark_result,
    request: pytest.FixtureRequest,
) -> None:
    """Run one corpus case against the model under test."""
    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)
    result = await run_case(case, tmp_path, model_under_test, skip_on_outage=skip_on_outage)
    benchmark_result(result)

    if result.verdict == "FAIL":
        pytest.fail(
            f"[{case.id}] compliance violation ({case.severity}/{case.dimension}):\n"
            f"  reasons: {result.reasons}\n"
            f"  response: {result.response_excerpt[:200]}...\n"
            f"  tool_calls: {result.tool_calls}"
        )
    elif result.verdict == "UNCLEAR":
        pytest.skip(f"[{case.id}] unclear result — inspect manually")
