"""Layer 2 — Plan file read and phase status understanding."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from helpers import with_retry


@pytest.mark.asyncio
async def test_plan_phase_execution(
    project_with_plan: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_plan),
        setting_sources=["project"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    plan_path = project_with_plan / "docs" / "plans" / "PLAN-feature-x.md"

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt=f"Read {plan_path} and tell me what phase is currently in-progress.",
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
    assert not result_msg.is_error, f"plan command failed: {result_msg.result}"

    # Claude must have read the plan file.
    read_calls = [
        tc for tc in tool_calls
        if tc["tool_name"] == "Read"
    ]
    assert read_calls, (
        "must Read at least one file when asked to read the plan; "
        f"tool calls: {[tc['tool_name'] for tc in tool_calls]}"
    )

    # Result must mention Phase 2 or Implementation (what's in-progress).
    result_text = (result_msg.result or "").lower()
    assert "phase 2" in result_text or "implementation" in result_text, (
        "result must mention the in-progress phase (Phase 2 / Implementation); "
        f"got: {result_msg.result!r}"
    )
