"""Layer 2 — /edikt:init on an empty project."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from helpers import with_retry


@pytest.mark.asyncio
async def test_init_greenfield(
    fresh_project: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(fresh_project),
        setting_sources=["project"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt="/edikt:init", options=options):
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
    assert not result_msg.is_error, f"init failed with error: {result_msg.result}"

    # init uses a mix of Bash (to run install/copy logic), Read, Write, Edit.
    # Any tool calls indicate edikt:init ran — we don't prescribe which tools.
    assert tool_calls, (
        "init must invoke at least one tool; "
        f"result: {result_msg.result!r}"
    )

    tool_names = {tc["tool_name"] for tc in tool_calls}
    assert tool_names & {"Bash", "Write", "Edit", "Read"}, (
        "init must use at least one of Bash/Write/Edit/Read; "
        f"saw: {tool_names}"
    )
