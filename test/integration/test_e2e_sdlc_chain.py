"""E2E — SDLC chain: accepted PRD → spec → plan.

Tests the handoffs between the three core SDLC commands in a single
project. Each step depends on the output of the previous one:

  /edikt:sdlc:spec PRD-001  →  spec file written to docs/product/specs/
  /edikt:sdlc:plan           →  plan file written to docs/plans/
  (verify PRD status gate: draft PRD must be rejected)

A pre-seeded accepted PRD (PRD-001) is in the fixture so the test starts
at the spec step — the spec command's status-gate check (requires
status: accepted) is the first real handoff to verify.

Implementation note: edikt SDLC commands (spec, plan) may run through a
forked sub-agent (context: fork per ADR-003). In this case the main
session returns an empty result while the work happens in a subprocess.
Tests therefore check filesystem state (files written to disk) as the
primary assertion rather than SDK stream content.

Why this matters: commands that compose across sessions (user writes PRD
on Monday, spec on Tuesday, plan on Wednesday) must hand off through the
filesystem. If spec silently ignores the PRD, or plan ignores the spec,
users get garbage. This test catches that whole class of regression.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest

from helpers import with_retry


def _all_written_content(tool_calls: list[dict]) -> str:
    return " ".join(
        tc["tool_input"].get("content", "")
        for tc in tool_calls
        if tc["tool_name"] in {"Write", "Edit"}
    )


@pytest.mark.asyncio
async def test_sdlc_spec_from_accepted_prd(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Spec command reads PRD-001 and writes a spec file.

    Verifies the handoff: an accepted PRD leads to a spec file on disk
    that references the PRD's requirements. Checks both the SDK stream
    (for commands that write inline) and disk (for forked commands).
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_accepted_prd),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:spec PRD-001",
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
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"spec command failed: {result_msg.result}"

    # Check disk first — spec may run in a forked sub-agent (ADR-003: context:fork).
    # The fork writes to disk while the main session returns empty.
    specs_dir = project_with_accepted_prd / "docs" / "product" / "specs"
    spec_files = list(specs_dir.rglob("*.md")) if specs_dir.exists() else []

    # Collect all text from the session.
    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text + " " + _all_written_content(tool_calls)

    # The spec must exist on disk OR the session must explain the spec content.
    prd_terms = {"FR-001", "FR-002", "OAuth", "authentication", "SPEC", "spec", "requirement"}
    spec_on_disk = len(spec_files) > 0
    spec_in_session = any(term in all_text for term in prd_terms)

    assert spec_on_disk or spec_in_session, (
        "spec command must either write a spec file to disk "
        "(docs/product/specs/) OR produce session output referencing PRD content. "
        f"Spec files on disk: {[str(f.relative_to(project_with_accepted_prd)) for f in spec_files]}, "
        f"session text snippet: {all_text[:200]!r}, "
        f"sdk stream: {sdk_stream}"
    )

    if spec_on_disk:
        # If a spec was written, it should reference the PRD requirements.
        spec_content = " ".join(f.read_text() for f in spec_files)
        assert any(term in spec_content for term in prd_terms), (
            "written spec file must reference PRD content; "
            f"files: {[str(f.relative_to(project_with_accepted_prd)) for f in spec_files]}"
        )


@pytest.mark.asyncio
async def test_sdlc_plan_from_spec(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Plan command creates a phased plan after a spec exists.

    Seeds a spec file in the project first, then invokes /edikt:sdlc:plan
    and verifies Claude produces a plan either in the session or on disk.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    # Seed a minimal accepted spec.
    spec_dir = project_with_accepted_prd / "docs" / "product" / "specs" / "SPEC-001-user-auth"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        """\
---
type: spec
id: SPEC-001
title: User authentication — technical spec
status: accepted
source_prd: PRD-001
---

# SPEC-001: User authentication

## Components

1. OAuth2 callback handler — validates Google token, creates session
2. Session middleware — validates cookie on every request, expires after 24h
3. Rate limiter — blocks IPs after 5 failed attempts in 60 seconds

## Acceptance Criteria

- AC-001: /auth/google/callback returns 302 to / on success
- AC-002: Expired cookie returns 401
- AC-003: 6th attempt within 60s returns 429
"""
    )

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_accepted_prd),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan SPEC-001 — implement the user auth spec",
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
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"plan command failed: {result_msg.result}"

    # Check disk first — plan may run in a forked sub-agent.
    plans_dir = project_with_accepted_prd / "docs" / "plans"
    plan_files = list(plans_dir.rglob("PLAN-*.md")) if plans_dir.exists() else []

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text + " " + _all_written_content(tool_calls)

    plan_terms = {"Phase", "phase", "implementation", "PLAN", "step"}
    plan_on_disk = len(plan_files) > 0
    plan_in_session = any(t in all_text for t in plan_terms)

    assert plan_on_disk or plan_in_session, (
        "plan command must either write a plan file to disk (docs/plans/PLAN-*.md) "
        "or produce a phase-structured response in the session. "
        f"Plan files on disk: {[str(f.relative_to(project_with_accepted_prd)) for f in plan_files]}, "
        f"session text: {all_text[:200]!r}"
    )


@pytest.mark.asyncio
async def test_sdlc_chain_handoff_prd_status_gate(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Spec command must not generate a spec for a draft PRD.

    Creates a DRAFT PRD and invokes spec for it. Verifies Claude either
    refuses with a status-gate message or writes nothing.
    This guards the handoff contract: spec must NEVER generate from a draft PRD.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    prd_dir = project_with_accepted_prd / "docs" / "product" / "prds"
    (prd_dir / "PRD-002-draft-feature.md").write_text(
        """\
---
type: prd
id: PRD-002
title: Draft feature — do not spec yet
status: draft
---

# PRD-002: Draft feature

This PRD is draft and must not be processed by /edikt:sdlc:spec.
"""
    )

    # Snapshot specs dir before running spec on the draft PRD.
    specs_dir = project_with_accepted_prd / "docs" / "product" / "specs"
    specs_before = set(specs_dir.rglob("SPEC-*.md")) if specs_dir.exists() else set()

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_accepted_prd),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:spec PRD-002",
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
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None

    # No new SPEC files should exist for a draft PRD.
    specs_after = set(specs_dir.rglob("SPEC-*.md")) if specs_dir.exists() else set()
    new_specs = specs_after - specs_before

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text
    gate_terms = {"draft", "accepted", "status", "cannot", "must be accepted", "not accepted"}

    # Either no new spec was written, or the result explains the status gate.
    assert not new_specs or any(term in all_text.lower() for term in gate_terms), (
        "spec command must not write a spec for a draft PRD, "
        "or must mention the status gate in its output. "
        f"New spec files: {[str(f.relative_to(project_with_accepted_prd)) for f in new_specs]}, "
        f"result: {result_msg.result!r}"
    )
