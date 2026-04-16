"""Benchmark conftest — adds --model flag and result collector."""

from __future__ import annotations

import json
from datetime import datetime, timezone
from pathlib import Path
from typing import Any

import pytest

_HERE = Path(__file__).parent
_RESULTS_DIR = _HERE / "results"


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--model",
        action="store",
        default="claude-sonnet-4-6",
        help="Model to benchmark (e.g. claude-opus-4-7, claude-sonnet-4-6)",
    )


@pytest.fixture(scope="session")
def model_under_test(request: pytest.FixtureRequest) -> str:
    return request.config.getoption("--model")


# Session-wide result accumulator.
_results: list[dict[str, Any]] = []


@pytest.fixture()
def benchmark_result():
    """Tests call this fixture to record their CaseResult."""
    def _record(case_result):
        _results.append({
            "case_id": case_result.case_id,
            "dimension": case_result.dimension,
            "verdict": case_result.verdict,
            "reasons": case_result.reasons,
            "model": case_result.model,
            "response_excerpt": case_result.response_excerpt,
            "tool_calls": case_result.tool_calls,
            "api_ms": case_result.api_ms,
        })
    return _record


def pytest_sessionfinish(session: pytest.Session, exitstatus: int) -> None:
    """Write the full run's results to a JSON file and print a summary."""
    if not _results:
        return
    _RESULTS_DIR.mkdir(exist_ok=True)
    model = session.config.getoption("--model")
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    out_path = _RESULTS_DIR / f"{model.replace('.', '-')}-{ts}.json"

    dimensions: dict[str, dict[str, int]] = {}
    for r in _results:
        d = r["dimension"]
        dimensions.setdefault(d, {"PASS": 0, "FAIL": 0, "UNCLEAR": 0, "total": 0})
        dimensions[d][r["verdict"]] += 1
        dimensions[d]["total"] += 1

    overall_pass = sum(1 for r in _results if r["verdict"] == "PASS")
    overall_total = len(_results)

    report = {
        "model": model,
        "timestamp": ts,
        "overall": {
            "pass": overall_pass,
            "total": overall_total,
            "score": overall_pass / overall_total if overall_total else 0.0,
        },
        "by_dimension": dimensions,
        "cases": _results,
    }
    out_path.write_text(json.dumps(report, indent=2))

    # Print summary to terminal.
    print("\n")
    print("=" * 60)
    print(f"GOVERNANCE COMPLIANCE BENCHMARK — {model}")
    print("=" * 60)
    print(f"Overall: {overall_pass}/{overall_total} "
          f"({100 * overall_pass / overall_total:.1f}%)" if overall_total else "no cases")
    print()
    for dim, counts in dimensions.items():
        score = counts["PASS"] / counts["total"] * 100 if counts["total"] else 0
        print(f"  {dim:20s} {counts['PASS']}/{counts['total']}  ({score:.0f}%)")
    print()
    print(f"Full report: {out_path}")
    print("=" * 60)
