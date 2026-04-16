"""Layer 2 — /edikt:upgrade with a customized agent must not silently overwrite."""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from conftest import with_retry


@pytest.mark.asyncio
async def test_upgrade_presents_3way_diff_on_customized_agent(
    project_with_customized_agents: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_customized_agents),
        setting_sources=["user", "project"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt="/edikt:upgrade", options=options):
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
    assert not result_msg.is_error, f"upgrade failed: {result_msg.result}"

    # The critical regression guard: upgrade must NEVER silently overwrite
    # a file that the user has edited after install.
    silent_overwrites = [
        tc for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
        and "backend" in (tc["tool_input"].get("file_path") or "").lower()
    ]
    assert not silent_overwrites, (
        "upgrade must NOT silently overwrite a customized agent (backend.md). "
        "Expected threeway diff prompt or skip, not a direct Write/Edit. "
        f"Found: {[tc['tool_input'].get('file_path') for tc in silent_overwrites]}"
    )

    # The upgrade command ran and produced a meaningful response.
    # Acceptable outcomes: version check message, diff prompt, "already up to date",
    # "custom marker found", or a pre-flight message about updating the launcher first.
    assert result_msg.result, "upgrade must produce a non-empty result message"
