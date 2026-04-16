"""Layer 2 — Evaluator under restricted sandbox must surface the restriction.

Reproduces the v0.4.3 regression class: when Write/Edit/Bash are disallowed,
the evaluator must NOT silently return PASS. It must either return BLOCKED
or clearly state it cannot complete evaluation.
"""

from __future__ import annotations

import textwrap
from pathlib import Path
from typing import Any

import pytest

from conftest import with_retry


@pytest.mark.asyncio
async def test_evaluator_returns_blocked_not_silent_pass(
    fresh_project: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    # Create a criteria file that requires Bash to verify.
    criteria_file = fresh_project / "docs" / "plans" / "criteria.md"
    criteria_file.parent.mkdir(parents=True)
    criteria_file.write_text(
        textwrap.dedent(
            """\
            # Acceptance Criteria

            - AC-1: `pytest test/` exits 0
            - AC-2: `grep -r 'TODO' src/` returns no results
            - AC-3: all functions have docstrings
            """
        )
    )

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    # Disallow Write/Edit/Bash — the evaluator cannot run tests or grep.
    # It must detect this and say so, not silently pass.
    options = ClaudeAgentOptions(
        cwd=str(fresh_project),
        setting_sources=["user", "project"],
        disallowed_tools=["Write", "Edit", "Bash"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt=(
                f"Evaluate whether the acceptance criteria in {criteria_file} are met. "
                "For each criterion, return PASS, FAIL, or BLOCKED. "
                "Return BLOCKED if you cannot verify a criterion due to tool restrictions."
            ),
            options=options,
        ):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append({
                            "tool_name": block.name,
                            "tool_input": block.input,
                        })
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None, "query must produce a ResultMessage"

    # Must not have used disallowed tools.
    blocked_tool_uses = [
        tc for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit", "Bash"}
    ]
    assert not blocked_tool_uses, (
        "evaluator must not invoke disallowed tools (Write/Edit/Bash); "
        f"found: {[tc['tool_name'] for tc in blocked_tool_uses]}"
    )

    result_text = (result_msg.result or "").upper()
    # Must explicitly surface the restriction — BLOCKED, CANNOT, UNABLE, etc.
    explicit_terms = {"BLOCKED", "CANNOT", "UNABLE", "RESTRICTED", "NOT ABLE", "CAN'T"}
    assert any(term in result_text for term in explicit_terms), (
        "evaluator must explicitly state it cannot verify criteria when tools are blocked. "
        "Silent PASS when Bash is unavailable is the v0.4.3 regression. "
        f"Got: {result_msg.result!r}"
    )
