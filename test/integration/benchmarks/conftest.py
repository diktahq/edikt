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
    parser.addoption(
        "--runs",
        action="store",
        type=int,
        default=1,
        help="Number of runs per case (default 1; recommended 5+ for reportable results — see METHODOLOGY.md)",
    )


@pytest.fixture(scope="session")
def model_under_test(request: pytest.FixtureRequest) -> str:
    return request.config.getoption("--model")


@pytest.fixture(scope="session")
def n_runs(request: pytest.FixtureRequest) -> int:
    return int(request.config.getoption("--runs"))


# Session-wide result accumulator — holds full CaseResult objects.
_results: list[Any] = []


@pytest.fixture()
def benchmark_result():
    """Tests call this fixture to record their CaseResult."""
    def _record(case_result):
        _results.append(case_result)
    return _record


def _env_fingerprint() -> dict[str, Any]:
    """Capture the execution environment for reproducibility."""
    import platform
    import subprocess
    import sys
    fp = {
        "python": sys.version.split()[0],
        "platform": f"{platform.system()} {platform.machine()}",
    }
    try:
        import claude_agent_sdk
        fp["sdk_version"] = claude_agent_sdk.__version__
    except Exception:
        fp["sdk_version"] = "unknown"
    try:
        from claude_agent_sdk._internal.transport.subprocess_cli import (
            SubprocessCLITransport,
        )
        # Best-effort: find the bundled claude binary and read version.
        import importlib.resources as _r
        # Simple approach: try invoking `claude --version` from PATH.
        cli_ver = subprocess.run(
            ["claude", "--version"],
            capture_output=True, text=True, timeout=5,
        )
        fp["cli_version"] = cli_ver.stdout.strip() or cli_ver.stderr.strip()
    except Exception:
        fp["cli_version"] = "unknown"
    return fp


def pytest_sessionfinish(session: pytest.Session, exitstatus: int) -> None:
    """Write both the aggregate summary JSON and per-run JSONL files."""
    if not _results:
        return

    model = session.config.getoption("--model")
    n_runs = int(session.config.getoption("--runs"))
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    model_slug = model.replace(".", "-").replace("/", "-")

    # Per-model directory for raw JSONL + summary.
    model_dir = _RESULTS_DIR / model_slug
    model_dir.mkdir(parents=True, exist_ok=True)

    # 1. Raw JSONL — every run of every case.
    # INV-007 / audit HI-11: redact `tool_calls[*].tool_input.content`, length-cap
    # `response`, and abort the write if credential-pattern regexes appear in any
    # serialized field. Prevents benchmark results from committing verbatim file
    # contents or leaked secrets into the repo.
    import re as _re_cred
    _CRED_PATTERNS = [
        _re_cred.compile(r"sk-ant-[A-Za-z0-9_\-]{20,}"),
        _re_cred.compile(r"Bearer\s+[A-Za-z0-9_\-\.]{20,}"),
        _re_cred.compile(r"-----BEGIN [A-Z ]+-----"),
        _re_cred.compile(r"AKIA[0-9A-Z]{16}"),  # AWS access key ID
        _re_cred.compile(r"ghp_[A-Za-z0-9]{36}"),  # GitHub PAT
    ]
    _RESPONSE_CAP = 4096

    def _redact_run(run, case_id: str) -> dict:
        # Length-cap response.
        resp = run.response
        if resp and len(resp) > _RESPONSE_CAP:
            resp = resp[:_RESPONSE_CAP] + f"\n...<truncated: {len(run.response) - _RESPONSE_CAP} more chars>"
        # Redact tool_input.content.
        redacted_calls = []
        for call in run.tool_calls:
            call_copy = dict(call)
            tool_input = call_copy.get("tool_input")
            if isinstance(tool_input, dict) and "content" in tool_input:
                content = tool_input["content"]
                tool_input = dict(tool_input)
                tool_input["content"] = f"<redacted:len={len(content) if isinstance(content, str) else 'n/a'}>"
                call_copy["tool_input"] = tool_input
            redacted_calls.append(call_copy)
        return {
            "case_id": case_id,
            "run_index": run.run_index,
            "verdict": run.verdict,
            "reasons": run.reasons,
            "response": resp,
            "tool_calls": redacted_calls,
            "api_ms": run.api_ms,
            "written_paths": run.written_paths,
        }

    def _credential_check(blob: str, where: str) -> None:
        for pat in _CRED_PATTERNS:
            if pat.search(blob):
                raise RuntimeError(
                    f"[edikt benchmark] credential pattern matched in {where}: "
                    f"{pat.pattern}. Refusing to write benchmark results to disk. "
                    "Review the run output before retrying."
                )

    jsonl_path = model_dir / f"{ts}-runs.jsonl"
    with jsonl_path.open("w") as fh:
        for cr in _results:
            for run in cr.runs:
                payload = _redact_run(run, cr.case_id)
                payload.update({
                    "dimension": cr.dimension,
                    "severity": cr.severity,
                    "targets": cr.targets,
                })
                serialized = json.dumps(payload)
                _credential_check(serialized, f"case {cr.case_id} run {run.run_index}")
                fh.write(serialized + "\n")

    # 2. Aggregate summary JSON.
    summary_path = model_dir / f"{ts}-summary.json"
    dimensions: dict[str, dict[str, int]] = {}
    severities: dict[str, dict[str, int]] = {}
    for cr in _results:
        # Per-dimension
        d = cr.dimension
        dimensions.setdefault(d, {"PASS": 0, "FAIL": 0, "UNCLEAR": 0, "total": 0})
        dimensions[d][cr.final_verdict] += 1
        dimensions[d]["total"] += 1
        # Per-severity
        s = cr.severity
        severities.setdefault(s, {"PASS": 0, "FAIL": 0, "UNCLEAR": 0, "total": 0})
        severities[s][cr.final_verdict] += 1
        severities[s]["total"] += 1

    overall_pass = sum(1 for cr in _results if cr.final_verdict == "PASS")
    overall_total = len(_results)
    overall_score = overall_pass / overall_total if overall_total else 0.0

    # Total API cost proxy (sum api_ms across all runs).
    total_api_ms = sum(run.api_ms for cr in _results for run in cr.runs)

    summary = {
        "model": model,
        "timestamp": ts,
        "methodology_version": "0.1",
        "n_runs_per_case": n_runs,
        "environment": _env_fingerprint(),
        "overall": {
            "pass": overall_pass,
            "total": overall_total,
            "score": overall_score,
            "total_api_ms": total_api_ms,
        },
        "by_dimension": dimensions,
        "by_severity": severities,
        "cases": [cr.to_summary() for cr in _results],
        "raw_jsonl": str(jsonl_path.relative_to(_HERE.parent)),
    }
    summary_path.write_text(json.dumps(summary, indent=2))

    # Print summary.
    print("\n")
    print("=" * 68)
    print(f"GOVERNANCE COMPLIANCE BENCHMARK — {model}")
    print("=" * 68)
    print(f"Methodology:   v0.1   Runs per case: {n_runs}")
    print(f"Overall:       {overall_pass}/{overall_total} "
          f"({100 * overall_score:.1f}%)   Total API ms: {total_api_ms}")
    print()
    print("By dimension:")
    for dim, counts in dimensions.items():
        score = counts["PASS"] / counts["total"] * 100 if counts["total"] else 0
        print(f"  {dim:20s} {counts['PASS']}/{counts['total']}  ({score:5.1f}%)")
    print()
    print("By severity:")
    for sev, counts in severities.items():
        score = counts["PASS"] / counts["total"] * 100 if counts["total"] else 0
        print(f"  {sev:20s} {counts['PASS']}/{counts['total']}  ({score:5.1f}%)")
    print()
    print(f"Summary:  {summary_path.relative_to(_HERE.parent)}")
    print(f"Raw runs: {jsonl_path.relative_to(_HERE.parent)}")
    print("=" * 68)
