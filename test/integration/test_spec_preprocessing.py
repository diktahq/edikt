"""Layer 2 — Claude reads a spec file and preserves its structure."""

from __future__ import annotations

import textwrap
from pathlib import Path
from typing import Any

import pytest

from conftest import with_retry


@pytest.mark.asyncio
async def test_spec_preprocessing_blank_line(
    fresh_project: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    # Create a spec file with a blank line before a section heading —
    # the byte pattern that triggered corruption in v0.4.2 (commit c3df32c).
    spec_dir = fresh_project / "docs" / "product" / "specs"
    spec_dir.mkdir(parents=True)
    spec_file = spec_dir / "TEST-SPEC.md"
    spec_file.write_text(
        textwrap.dedent(
            """\
            # Test Feature Spec

            ## Requirements

            - FR-001: system must handle requests
            - FR-002: system must respond within 200ms

            ## Acceptance Criteria

            - AC-001: all requests return HTTP 200
            """
        )
    )

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(fresh_project),
        setting_sources=["user", "project"],
    )

    skip_on_outage: bool = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt=f"Read {spec_file} and list all the requirements (FR-*) you find in it.",
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
    assert not result_msg.is_error, f"spec read failed: {result_msg.result}"

    read_calls = [tc for tc in tool_calls if tc["tool_name"] == "Read"]
    assert read_calls, (
        "must Read the spec file; "
        f"tool calls: {[tc['tool_name'] for tc in tool_calls]}"
    )

    result_text = result_msg.result or ""
    # Both requirements must be present in the output — none were dropped by preprocessing.
    assert "FR-001" in result_text, (
        f"FR-001 missing from result — section may have been dropped. Got: {result_text!r}"
    )
    assert "FR-002" in result_text, (
        f"FR-002 missing from result — section may have been dropped. Got: {result_text!r}"
    )
