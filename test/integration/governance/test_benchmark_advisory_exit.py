"""AC-014 / AC-016 — multi-failure handling + advisory exit semantics.

AC-014 — multi-failure output prints each six-section report AND a summary
         index table row per failing directive. (Documented in benchmark.md,
         enforced by the schema tests in test_benchmark_report_schema.py.)
AC-016 — benchmark exits 0 on directive failures; exits != 0 only on
         infrastructure failure. Tested by simulating multiple helper runs
         with mixed PASS/FAIL verdicts and confirming the helper's per-run
         exit code is always 0.
"""

from __future__ import annotations

import json
import sys
import io
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).parents[3]
TOOLS_DIR = REPO_ROOT / "tools" / "gov-benchmark"
sys.path.insert(0, str(TOOLS_DIR))

from gov_benchmark import run as helper_run  # noqa: E402


def _mk_stub(assistant_text: str, tool_calls: list):
    async def _stub(prompt, project_dir, model, timeout_s, cancel):
        return {
            "assistant_text": assistant_text,
            "tool_calls": tool_calls,
            "written_paths": [
                (tc.get("tool_input") or {}).get("file_path", "")
                for tc in tool_calls
                if tc.get("tool_name") in {"Write", "Edit"}
            ],
            "api_ms": 5,
        }

    return _stub


def test_multiple_directives_mixed_verdicts_all_exit_zero(tmp_path, monkeypatch):
    """AC-016 — multi-directive run with mixed PASS/FAIL always exits 0 per helper call."""
    runs = [
        # PASS — refuses
        (
            {
                "directive_id": "ADR-001",
                "signal_type": "refuse_tool_use",
                "behavioral_signal": {"refuse_tool": ["Write"]},
                "attack_prompt": "x",
                "target_model": "stub",
                "project_dir": str(tmp_path),
                "timeout_s": 5,
            },
            _mk_stub("refused per ADR-001", []),
            "PASS",
        ),
        # FAIL — writes forbidden path
        (
            {
                "directive_id": "ADR-002",
                "signal_type": "refuse_file_pattern",
                "behavioral_signal": {"refuse_to_write": ["users.sql"]},
                "attack_prompt": "x",
                "target_model": "stub",
                "project_dir": str(tmp_path),
                "timeout_s": 5,
            },
            _mk_stub(
                "here",
                [{"tool_name": "Write", "tool_input": {"file_path": "apps/users.sql"}}],
            ),
            "FAIL",
        ),
        # FAIL — missing citation
        (
            {
                "directive_id": "ADR-003",
                "signal_type": "must_cite",
                "behavioral_signal": {"cite": ["ADR-003"]},
                "attack_prompt": "x",
                "target_model": "stub",
                "project_dir": str(tmp_path),
                "timeout_s": 5,
            },
            _mk_stub("I refuse", []),
            "FAIL",
        ),
    ]
    outs = []
    for payload, stub, expected in runs:
        monkeypatch.setattr(helper_run, "_invoke_sdk", stub)
        out = helper_run.run_one(payload)
        outs.append((out, expected))

    # Every helper call exited with status=ok (FAIL is a directive verdict,
    # not an infra failure).
    for out, expected in outs:
        assert out["status"] == "ok"
        assert out["verdict"] == expected

    # AC-014: we have ≥2 failing directives available for the summary index.
    fails = [o for o, _ in outs if o["verdict"] == "FAIL"]
    assert len(fails) == 2


def test_infra_failure_does_not_silently_pass(tmp_path, monkeypatch):
    """AC-016 — infrastructure failure returns non-ok status with actionable msg."""

    async def _break(prompt, project_dir, model, timeout_s, cancel):
        raise RuntimeError("not logged in - auth failed")

    monkeypatch.setattr(helper_run, "_invoke_sdk", _break)
    out = helper_run.run_one(
        {
            "directive_id": "ADR-001",
            "signal_type": "refuse_tool_use",
            "behavioral_signal": {"refuse_tool": ["Write"]},
            "attack_prompt": "x",
            "target_model": "stub",
            "project_dir": str(tmp_path),
            "timeout_s": 5,
        }
    )
    assert out["status"] == "auth_error"
    # The command handles this by aborting; the helper itself returns 0
    # and lets the caller interpret status.
