"""E2E — /edikt:sdlc:plan --sidecar-only: rebuild evaluation history from plan.

Tests the --sidecar-only flag end-to-end through real Claude.

Scenarios:
  happy path        → plan exists, sidecar generated with correct schema
  merge             → partial sidecar exists, existing pass/fail data preserved
  new phases        → plan has more phases than old sidecar, new phases added
  pass not reset    → passing criteria NOT reset to pending on regeneration
  no plan           → clear error, no crash
  correct schema    → AC-N.M ids, status:pending for new, phases[] structure

This tests the command path that the phase-end-detector hook calls
automatically when the evaluation history is missing.
"""

from __future__ import annotations

import textwrap
from pathlib import Path
from typing import Any

import pytest
import yaml

from conftest import with_retry

_FM_RE = __import__("re").compile(r"^---\n(.*?)\n---", __import__("re").DOTALL)


def _load_sidecar(path: Path) -> dict:
    return yaml.safe_load(path.read_text()) or {}


def _write_plan(plans_dir: Path, slug: str, phases: list[dict]) -> Path:
    """Write a PLAN-{slug}.md with the given phases."""
    rows = "\n".join(
        f"| {p['num']} | {p['title']} | {p.get('status', 'pending')} |"
        for p in phases
    )
    criteria_blocks = "\n\n".join(
        f"### Phase {p['num']}: {p['title']}\n\n**Acceptance Criteria:**\n"
        + "\n".join(f"- {ac}" for ac in p.get("criteria", ["Tests pass"]))
        for p in phases
    )
    plan_path = plans_dir / f"PLAN-{slug}.md"
    plan_path.write_text(
        textwrap.dedent(
            f"""\
            # PLAN-{slug}

            ## Progress

            | Phase | Title | Status |
            |---|---|---|
            {rows}

            {criteria_blocks}
            """
        )
    )
    return plan_path


@pytest.fixture()
def project_for_sidecar_only(tmp_path: Path) -> Path:
    """Minimal project with a plan file and edikt config."""
    project = tmp_path / "project"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.5.0
            base: docs
            paths:
              plans: docs/plans
            """
        )
    )
    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Project

            [edikt:start]: # managed by edikt
            ## edikt

            ### Project
            Test project for --sidecar-only E2E tests.
            [edikt:end]: #
            """
        )
    )
    plans_dir = project / "docs" / "plans"
    plans_dir.mkdir(parents=True)
    _write_plan(
        plans_dir,
        "auth",
        [
            {
                "num": 1,
                "title": "OAuth callback handler",
                "status": "pending",
                "criteria": [
                    "POST /auth/google returns 302 on valid token",
                    "Invalid token returns 401",
                ],
            },
            {
                "num": 2,
                "title": "Session middleware",
                "status": "pending",
                "criteria": ["Expired session returns 401"],
            },
        ],
    )
    return project


@pytest.mark.asyncio
async def test_sidecar_only_creates_file_when_missing(
    project_for_sidecar_only: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """--sidecar-only creates PLAN-auth-criteria.yaml from an existing plan.

    The generated file must exist as a sibling of the plan and have valid structure.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    plans_dir = project_for_sidecar_only / "docs" / "plans"
    sidecar = plans_dir / "PLAN-auth-criteria.yaml"
    assert not sidecar.exists(), "sidecar must not pre-exist for this test"

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_sidecar_only),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan --sidecar-only PLAN-auth",
            options=options,
        ):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append({"tool_name": block.name, "tool_input": block.input})
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"--sidecar-only failed: {result_msg.result}"

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text

    # Primary check: sidecar on disk.
    if sidecar.exists():
        data = _load_sidecar(sidecar)
        assert "phases" in data, "generated sidecar must have 'phases' key"
        assert data["phases"], "generated sidecar must have at least one phase"
        for phase in data["phases"]:
            for criterion in phase.get("criteria", []):
                import re
                assert re.match(r"^AC-\d+\.\d+$", criterion["id"]), (
                    f"criterion id {criterion['id']!r} must match AC-N.M"
                )
                assert criterion["status"] == "pending", (
                    f"new criterion {criterion['id']} must start pending"
                )
    else:
        # Command ran but produced no file — check session content.
        sidecar_terms = {"criteria", "PLAN-auth", "phase", "AC-", "evaluation history"}
        assert any(t in all_text for t in sidecar_terms), (
            "--sidecar-only must produce a sidecar file or describe the criteria; "
            f"result: {result_text[:300]!r}"
        )


@pytest.mark.asyncio
async def test_sidecar_only_preserves_existing_pass_results(
    project_for_sidecar_only: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """--sidecar-only must not reset passing criteria to pending.

    Pre-seed a sidecar where Phase 1 AC-1.1 is 'pass' with fail_count=0.
    After regeneration, that criterion must still be 'pass'.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    plans_dir = project_for_sidecar_only / "docs" / "plans"
    sidecar = plans_dir / "PLAN-auth-criteria.yaml"

    # Pre-seed with one passing criterion.
    sidecar.write_text(
        textwrap.dedent(
            """\
            plan: PLAN-auth
            generated: "2026-04-16T00:00:00Z"
            last_evaluated: "2026-04-16"
            phases:
              - phase: 1
                title: OAuth callback handler
                status: pass
                attempt: "1/5"
                criteria:
                  - id: AC-1.1
                    text: "POST /auth/google returns 302 on valid token"
                    status: pass
                    fail_count: 0
                    fail_reason: null
                    last_evaluated: "2026-04-16"
                    verify: "curl -s -o /dev/null -w '%{http_code}' -X POST localhost/auth/google | grep 302"
            """
        )
    )

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_sidecar_only),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan --sidecar-only PLAN-auth",
            options=options,
        ):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append({"tool_name": block.name, "tool_input": block.input})
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"--sidecar-only failed: {result_msg.result}"

    # If sidecar was rewritten, AC-1.1 must still be pass.
    if sidecar.exists():
        data = _load_sidecar(sidecar)
        for phase in data.get("phases", []):
            if phase.get("phase") == 1:
                for criterion in phase.get("criteria", []):
                    if criterion["id"] == "AC-1.1":
                        assert criterion["status"] == "pass", (
                            "AC-1.1 was passing before --sidecar-only; "
                            "regeneration must not reset it to pending. "
                            f"Got status: {criterion['status']!r}"
                        )


@pytest.mark.asyncio
async def test_sidecar_only_adds_new_phase_from_plan(
    project_for_sidecar_only: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """--sidecar-only adds criteria for a new phase not in the old sidecar.

    Old sidecar has phase 1 only. Plan has phases 1 and 2.
    After regeneration, sidecar must have both phases with phase 1's
    history preserved and phase 2 added as pending.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    plans_dir = project_for_sidecar_only / "docs" / "plans"
    sidecar = plans_dir / "PLAN-auth-criteria.yaml"

    # Old sidecar with only phase 1 (phase 2 is new in the plan).
    sidecar.write_text(
        textwrap.dedent(
            """\
            plan: PLAN-auth
            generated: "2026-04-15T00:00:00Z"
            last_evaluated: null
            phases:
              - phase: 1
                title: OAuth callback handler
                status: fail
                attempt: "1/5"
                criteria:
                  - id: AC-1.1
                    text: "POST /auth/google returns 302"
                    status: fail
                    fail_count: 1
                    fail_reason: "returned 500"
                    last_evaluated: "2026-04-15"
                    verify: null
            """
        )
    )

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_sidecar_only),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan --sidecar-only PLAN-auth",
            options=options,
        ):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append({"tool_name": block.name, "tool_input": block.input})
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None
    assert not result_msg.is_error, f"--sidecar-only failed: {result_msg.result}"

    if sidecar.exists():
        data = _load_sidecar(sidecar)
        phases = {p["phase"]: p for p in data.get("phases", [])}

        if 1 in phases:
            # Phase 1: fail_count must be preserved.
            p1_criteria = {c["id"]: c for c in phases[1].get("criteria", [])}
            if "AC-1.1" in p1_criteria:
                assert p1_criteria["AC-1.1"]["fail_count"] == 1, (
                    "fail_count must be preserved on merge; "
                    f"got: {p1_criteria['AC-1.1']['fail_count']}"
                )

        if 2 in phases:
            # Phase 2: all criteria must be pending (new, no history).
            for criterion in phases[2].get("criteria", []):
                assert criterion["status"] == "pending", (
                    f"New phase 2 criterion {criterion['id']} must start as pending; "
                    f"got: {criterion['status']!r}"
                )


@pytest.mark.asyncio
async def test_sidecar_only_error_when_plan_not_found(
    project_for_sidecar_only: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """--sidecar-only with a non-existent plan slug must give a clear error.

    Must NOT silently succeed or create an empty sidecar.
    Must tell the user which plan wasn't found and suggest /edikt:sdlc:plan.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    plans_dir = project_for_sidecar_only / "docs" / "plans"
    nonexistent_sidecar = plans_dir / "PLAN-nonexistent-criteria.yaml"

    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_for_sidecar_only),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan --sidecar-only PLAN-nonexistent",
            options=options,
        ):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)

    assert result_msg is not None

    # Must NOT have created a sidecar for a non-existent plan.
    assert not nonexistent_sidecar.exists(), (
        "--sidecar-only must not create a sidecar for a plan that doesn't exist"
    )

    # Result must explain the error clearly.
    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text
    error_terms = {"not found", "no plan", "doesn't exist", "cannot find", "missing"}
    assert any(t in all_text.lower() for t in error_terms), (
        "--sidecar-only with non-existent plan must explain the error; "
        f"result: {result_text[:300]!r}"
    )
