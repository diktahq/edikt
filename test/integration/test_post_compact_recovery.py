"""Layer 2 — Context recovery: reading active plan after context loss."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from conftest import with_retry


@pytest.mark.asyncio
async def test_post_compact_recovery(
    project_post_compact: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_post_compact),
        setting_sources=["user", "project"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    plan_path = project_post_compact / "docs" / "plans" / "PLAN-feature-x.md"

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt=f"Read {plan_path} and summarise which phases are done and which is next.",
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
    assert not result_msg.is_error, f"post-compact recovery failed: {result_msg.result}"

    # Must have read the plan file.
    read_calls = [tc for tc in tool_calls if tc["tool_name"] == "Read"]
    assert read_calls, (
        "must Read the plan file to recover context; "
        f"tool calls: {[tc['tool_name'] for tc in tool_calls]}"
    )

    result_text = (result_msg.result or "").lower()
    assert any(
        term in result_text for term in ("phase", "done", "in-progress", "pending", "complete")
    ), (
        "result must describe phase status; "
        f"got: {result_msg.result!r}"
    )
