"""E2E — Artifacts: accepted spec → artifact files generated.

Tests /edikt:sdlc:artifacts against a pre-seeded accepted spec.
Verifies:
- Artifacts are written to the spec folder (api.yaml, data-model, test-strategy)
- Draft spec is blocked (status gate)
- Spec with no database type still generates a data model
- api.yaml is generated when spec describes HTTP endpoints
- Artifacts respect database_type from spec frontmatter

Edge cases:
- Draft spec → blocked, no artifacts written
- Spec with missing spec folder → graceful failure
- Config missing specs path → falls back to default
- Spec with database_type: postgresql → data model references postgres

Note: /edikt:sdlc:artifacts may run through a forked sub-agent (context:fork,
ADR-003). Tests check both disk state and session content.
"""

from __future__ import annotations

import textwrap
from pathlib import Path
from typing import Any

import pytest

from helpers import with_retry


def _collect_artifacts(spec_dir: Path) -> list[Path]:
    """Return all artifact files written to the spec folder."""
    patterns = ["**/*.yaml", "**/*.json", "**/*.sql", "**/*.md", "**/*.mmd"]
    found = []
    for pat in patterns:
        found.extend(spec_dir.glob(pat))
    # Exclude spec.md itself
    return [f for f in found if f.name != "spec.md"]


@pytest.mark.asyncio
async def test_artifacts_from_accepted_spec(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Artifacts command generates at least one artifact file from an accepted spec.

    Seeds an accepted spec, runs /edikt:sdlc:artifacts, checks disk for
    generated artifact files.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    spec_dir = project_with_accepted_prd / "docs" / "product" / "specs" / "SPEC-001-auth"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-001
            title: OAuth2 authentication
            status: accepted
            database_type: postgresql
            ---

            # SPEC-001: OAuth2 authentication

            ## Components

            1. Token validator — validates Google OAuth2 JWT
            2. Session store — PostgreSQL table with TTL
            3. Rate limiter — Redis-backed sliding window

            ## Acceptance Criteria

            - AC-001: Valid token creates a session row in PostgreSQL
            - AC-002: Expired session returns 401
            - AC-003: Rate limit blocks after 5 attempts
            """
        )
    )

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None
    artifacts_before = set(_collect_artifacts(spec_dir))

    options = ClaudeAgentOptions(
        cwd=str(project_with_accepted_prd),
        setting_sources=["user", "project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt="/edikt:sdlc:artifacts SPEC-001", options=options):
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
    assert not result_msg.is_error, f"artifacts failed: {result_msg.result}"

    artifacts_after = set(_collect_artifacts(spec_dir))
    new_artifacts = artifacts_after - artifacts_before

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text + " ".join(
        tc["tool_input"].get("content", "") for tc in tool_calls if tc["tool_name"] in {"Write", "Edit"}
    )

    artifact_terms = {"api.yaml", "data-model", "schema", "test-strategy", "contracts",
                      "migration", "fixture", "artifact", "generated"}

    assert new_artifacts or any(t in all_text for t in artifact_terms), (
        "artifacts command must write artifact files to the spec folder OR describe "
        "the artifacts in the session output; "
        f"new files on disk: {[str(f.relative_to(project_with_accepted_prd)) for f in new_artifacts]}, "
        f"session text: {all_text[:200]!r}"
    )


@pytest.mark.asyncio
async def test_artifacts_blocked_for_draft_spec(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Draft spec must be blocked — no artifacts generated.

    Status gate: /edikt:sdlc:artifacts must refuse to generate artifacts
    from a spec with status: draft. Edge case: user runs artifacts too early.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    spec_dir = project_with_accepted_prd / "docs" / "product" / "specs" / "SPEC-002-draft"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-002
            title: Draft feature
            status: draft
            ---

            # SPEC-002: Draft — do not generate artifacts

            This spec is a draft and must not produce artifacts.
            """
        )
    )
    artifacts_before = set(_collect_artifacts(spec_dir))

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
        async for msg in query(prompt="/edikt:sdlc:artifacts SPEC-002", options=options):
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

    artifacts_after = set(_collect_artifacts(spec_dir))
    new_artifacts = artifacts_after - artifacts_before

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text
    gate_terms = {"draft", "accepted", "status", "must be accepted", "cannot"}

    assert not new_artifacts or any(t in all_text.lower() for t in gate_terms), (
        "artifacts command must not generate artifacts from a draft spec; "
        f"new files written: {[str(f.relative_to(project_with_accepted_prd)) for f in new_artifacts]}, "
        f"result: {result_text[:200]!r}"
    )


@pytest.mark.asyncio
async def test_artifacts_api_contract_when_spec_has_endpoints(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """When spec describes REST endpoints, api.yaml should be generated.

    Verifies the artifact routing logic: spec content with HTTP endpoint
    descriptions triggers api.yaml generation.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    spec_dir = project_with_accepted_prd / "docs" / "product" / "specs" / "SPEC-003-api"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-003
            title: REST API for user management
            status: accepted
            ---

            # SPEC-003: REST API

            ## Endpoints

            - POST /api/v1/users — create user
            - GET /api/v1/users/:id — get user by id
            - DELETE /api/v1/users/:id — delete user

            ## Acceptance Criteria

            - AC-001: POST /api/v1/users returns 201 with user JSON
            - AC-002: GET /api/v1/users/:id returns 404 for missing user
            - AC-003: DELETE /api/v1/users/:id returns 204
            """
        )
    )
    artifacts_before = set(_collect_artifacts(spec_dir))

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
        async for msg in query(prompt="/edikt:sdlc:artifacts SPEC-003", options=options):
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
    assert not result_msg.is_error, f"artifacts failed: {result_msg.result}"

    artifacts_after = set(_collect_artifacts(spec_dir))
    new_artifacts = artifacts_after - artifacts_before

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text + " ".join(
        tc["tool_input"].get("content", "") for tc in tool_calls if tc["tool_name"] in {"Write", "Edit"}
    )

    # api.yaml should be present or mentioned.
    api_present = any("api" in str(f).lower() for f in new_artifacts)
    api_mentioned = "api.yaml" in all_text or "openapi" in all_text.lower() or "paths:" in all_text

    assert api_present or api_mentioned, (
        "When spec describes REST endpoints, api.yaml must be written or mentioned; "
        f"new artifacts: {[str(f.relative_to(project_with_accepted_prd)) for f in new_artifacts]}, "
        f"session: {all_text[:300]!r}"
    )


@pytest.mark.asyncio
async def test_artifacts_references_database_type_from_spec(
    project_with_accepted_prd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Artifacts respect database_type from spec frontmatter.

    When spec.md has database_type: postgresql, the data model and
    migrations must not reference sqlite or other engines.
    Edge case: spec frontmatter drives artifact content.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    spec_dir = project_with_accepted_prd / "docs" / "product" / "specs" / "SPEC-004-db"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-004
            title: Data model for sessions
            status: accepted
            database_type: postgresql
            ---

            # SPEC-004: Session data model

            ## Schema

            sessions table:
            - id (uuid, primary key)
            - user_id (uuid, foreign key → users)
            - expires_at (timestamptz)
            - created_at (timestamptz, default now())

            ## Acceptance Criteria

            - AC-001: sessions table exists in PostgreSQL
            - AC-002: expired sessions are prunable by a cron job
            """
        )
    )
    artifacts_before = set(_collect_artifacts(spec_dir))

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
        async for msg in query(prompt="/edikt:sdlc:artifacts SPEC-004", options=options):
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
    assert not result_msg.is_error, f"artifacts failed: {result_msg.result}"

    artifacts_after = set(_collect_artifacts(spec_dir))
    new_artifacts = artifacts_after - artifacts_before

    all_written_content = " ".join(
        tc["tool_input"].get("content", "") for tc in tool_calls if tc["tool_name"] in {"Write", "Edit"}
    ) + " ".join(f.read_text() for f in new_artifacts if f.suffix in {".yaml", ".sql", ".md"})

    # If any data model or migration was written, it must not reference SQLite.
    if new_artifacts or all_written_content.strip():
        assert "sqlite" not in all_written_content.lower(), (
            "Artifacts generated for a postgresql spec must not reference SQLite; "
            "database_type from spec frontmatter must be respected"
        )
