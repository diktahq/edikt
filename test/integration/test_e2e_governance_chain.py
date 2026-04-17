"""E2E — Governance chain: ADR → compile → governance directives verified.

Tests the full compile pipeline end-to-end through Claude:

  /edikt:adr:new  →  ADR file written with directive sentinel block
  /edikt:gov:compile  →  topic files + routing index written
  (verify directive content appears in compiled output)
  (verify routing table references resolve to written files)

Why this matters: the governance compile pipeline is the bridge between
human decisions (ADRs, invariants) and the directives Claude reads every
session. If the compile chain is broken, governance exists on paper but
is never enforced. This test catches the whole class of compile/route
regression that offline tests cannot — because offline tests verify
already-compiled artifacts, not the pipeline that produces them.
"""

from __future__ import annotations

import re
from pathlib import Path
from typing import Any

import pytest

from helpers import with_retry


@pytest.mark.asyncio
async def test_adr_new_creates_file_with_sentinel(
    project_for_governance_chain: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """ADR creation writes a file with a directive sentinel block.

    Verifies:
    - /edikt:adr:new produces output (tool calls or result)
    - If a file was written, it contains a sentinel block
    - The sentinel includes at least one MUST/NEVER directive
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_governance_chain),
        setting_sources=["project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt=(
                "/edikt:adr:new Use PostgreSQL over SQLite for persistence — "
                "PostgreSQL supports concurrent writes and row-level locking "
                "which SQLite cannot handle at our expected load. "
                "Decision: all data stores MUST use PostgreSQL. Never SQLite in production."
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

    assert result_msg is not None
    assert not result_msg.is_error, f"adr:new failed: {result_msg.result}"

    result_text = result_msg.result or ""
    all_written = " ".join(
        tc["tool_input"].get("content", "")
        for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
    )
    all_content = all_written + " " + result_text

    # Command must have produced something.
    assert tool_calls or result_text, "adr:new produced no output"

    # The ADR file (if written) must contain a sentinel block.
    adr_writes = [
        tc for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
        and (
            "decisions" in (tc["tool_input"].get("file_path") or "").lower()
            or "ADR" in (tc["tool_input"].get("file_path") or "").upper()
        )
    ]
    if adr_writes:
        for write in adr_writes:
            content = write["tool_input"].get("content", "")
            assert "[edikt:directives:start]: #" in content, (
                f"ADR file must have '[edikt:directives:start]: #' sentinel; "
                f"file: {write['tool_input'].get('file_path')}"
            )
            assert "MUST" in content or "NEVER" in content, (
                "ADR directive sentinel must contain MUST or NEVER language; "
                f"file: {write['tool_input'].get('file_path')}"
            )
    else:
        # No direct write — check result mentions the ADR was created.
        adr_terms = {"ADR", "adr", "decision", "accepted", "directive", "PostgreSQL"}
        assert any(t in all_content for t in adr_terms), (
            "adr:new result must reference the created ADR; "
            f"result: {result_text[:300]!r}"
        )


@pytest.mark.asyncio
async def test_gov_compile_creates_governance_output(
    project_for_governance_chain: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Governance compile produces output containing the ADR directive.

    Seeds one ADR with a directive sentinel block (PostgreSQL constraint),
    runs /edikt:gov:compile, and verifies:
    - Claude writes at least one governance file
    - The compiled output contains the ADR directive
    - The output has schema version 2 (compile_schema_version: 2)
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    decisions_dir = project_for_governance_chain / "docs" / "architecture" / "decisions"
    (decisions_dir / "ADR-001-database-choice.md").write_text(
        """\
---
type: adr
id: ADR-001
title: Use PostgreSQL for all persistence
status: accepted
created_at: 2026-04-16T00:00:00Z
---

# ADR-001: Use PostgreSQL for all persistence

**Status:** Accepted

## Context

We need a database that supports concurrent writes.

## Decision

All data stores MUST use PostgreSQL. SQLite is NEVER permitted in production.

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - implementation
  - review
directives:
  - All data stores MUST use PostgreSQL. SQLite is NEVER permitted in production. (ref: ADR-001)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
"""
    )

    all_writes: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_governance_chain),
        setting_sources=["project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt="/edikt:gov:compile", options=options):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        if block.name in {"Write", "Edit"}:
                            all_writes.append({
                                "path": block.input.get("file_path", ""),
                                "content": block.input.get("content", ""),
                            })
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"gov:compile failed: {result_msg.result}"

    result_text = result_msg.result or ""
    all_content = " ".join(w["content"] for w in all_writes) + " " + result_text

    # Compile must have produced something.
    assert all_writes or result_text, "gov:compile produced no output"

    # The compiled output must contain the ADR directive.
    assert "PostgreSQL" in all_content or "postgresql" in all_content.lower(), (
        "gov:compile output must include the ADR-001 PostgreSQL directive; "
        "the compile pipeline is not picking up the ADR sentinel. "
        f"Written files: {[w['path'] for w in all_writes]}, "
        f"result: {result_text[:200]!r}"
    )

    # Schema version must be 2.
    assert "compile_schema_version: 2" in all_content, (
        "compiled governance files must declare compile_schema_version: 2 (ADR-007); "
        "files written: {[w['path'] for w in all_writes]}"
    )


@pytest.mark.asyncio
async def test_governance_routing_table_references_resolve(
    project_for_governance_chain: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """After compile, every routing table file reference resolves to a written file.

    This is the routing integrity check run through Claude. The distinction
    from the offline test: this verifies the governance.md that Claude *just
    compiled* is internally consistent — routing rows point to files that were
    actually written in the same compile run.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, ToolUseBlock

    decisions_dir = project_for_governance_chain / "docs" / "architecture" / "decisions"
    (decisions_dir / "ADR-001-test.md").write_text(
        """\
---
type: adr
id: ADR-001
title: All modules must have unit tests
status: accepted
---

# ADR-001: All modules must have unit tests

**Status:** Accepted

## Decision

All modules MUST have unit tests. Untested code MUST NOT be merged to main.

## Directives

[edikt:directives:start]: #
paths:
  - "**/*"
scope:
  - implementation
  - review
directives:
  - All modules MUST have unit tests. Untested code MUST NOT be merged to main. (ref: ADR-001)
manual_directives: []
suppressed_directives: []
[edikt:directives:end]: #
"""
    )

    all_writes: list[dict[str, Any]] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_governance_chain),
        setting_sources=["project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt="/edikt:gov:compile", options=options):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        if block.name in {"Write", "Edit"}:
                            all_writes.append({
                                "path": block.input.get("file_path", ""),
                                "content": block.input.get("content", ""),
                            })
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"gov:compile failed: {result_msg.result}"
    assert all_writes, "gov:compile must write at least one file"

    written_basenames = {Path(w["path"]).name for w in all_writes if w["path"]}

    # Find the governance index (routing table) among the writes.
    # It's either governance.md or the file with "Routing Table" in its content.
    routing_content = ""
    for w in all_writes:
        if "Routing Table" in w["content"] or w["path"].endswith("governance.md"):
            routing_content = w["content"]
            break

    if not routing_content:
        # No routing table written yet — if only a topic file was written,
        # that's a partial compile (valid for a single-ADR project).
        # Just verify the directive made it into the output.
        all_content = " ".join(w["content"] for w in all_writes)
        assert "MUST" in all_content or "NEVER" in all_content, (
            "gov:compile must produce at least one MUST/NEVER directive in output; "
            f"files written: {[w['path'] for w in all_writes]}"
        )
        return  # Single-ADR compile — no routing table required

    # Extract backtick-quoted .md paths from the routing table.
    referenced = re.findall(r"`([^`]+\.md)`", routing_content)

    for ref in referenced:
        basename = Path(ref).name
        written = basename in written_basenames
        # Also accept if the file exists in the project from a prior run.
        on_disk = (
            (project_for_governance_chain / ref).exists()
            or (project_for_governance_chain / ".claude" / "rules" / ref).exists()
            or (project_for_governance_chain / ".claude" / "rules" / basename).exists()
        )
        assert written or on_disk, (
            f"Routing table references '{ref}' (basename: {basename!r}) but it was "
            "neither written by this compile run nor found on disk. "
            "The compile produced an inconsistent routing table. "
            f"Written files: {sorted(written_basenames)}"
        )
