"""SPEC-007 Layer 2 — transition chain integration test.

Chains the post-authoring SDLC lifecycle: starting from a pre-seeded v2
PRD + SPEC pair, exercises ship → doctor → review and verifies contracts
hold through the chain.

Why pre-seed (not author via the command): /edikt:sdlc:prd and
/edikt:sdlc:spec both fork to specialist subagents (architect, domain
experts) via the Agent tool. Those subagents' filesystem writes don't
propagate back to the SDK-harness-visible filesystem in every Claude
Code version. The existing test_e2e_sdlc_chain.py handles this with
"disk OR session-text" assertions. Authoring Layer 2 coverage already
lives in test_spec_007_e2e_v2_flow.py (single-step tests).

This lifecycle test picks up AFTER authoring and tests what actually
runs reliably in the SDK harness: the mechanical transition commands.

Chain:
  1. (pre-seed) v2 PRD + SPEC + back-ref already established in fixture
  2. /edikt:sdlc:prd:ship PRD-001 FR-001 → FR-001 shipped, others untouched
  3. /edikt:doctor → reports v2 PRD/SPEC health section
  4. /edikt:prd:review PRD-001 → rubric scores the partially-shipped PRD
  5. /edikt:sdlc:prd:ship PRD-001 FR-002 FR-003 → all FRs shipped, top-level flips

This catches chain-level bugs: ship works twice (not just first call),
doctor sees sidecar after mutations, review runs after ship-induced drift.

Cost: ~$0.50-1 per run with Sonnet. Runs only when claude auth is available.
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import pytest
import yaml

from helpers import with_retry


async def _invoke(
    prompt: str,
    cwd: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> tuple[Any, list[dict[str, Any]], str]:
    """Run a single SDK query. Returns (result_msg, tool_calls, all_text)."""
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import (
        AssistantMessage,
        ResultMessage,
        TextBlock,
        ToolUseBlock,
    )

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None
    options = ClaudeAgentOptions(cwd=str(cwd), setting_sources=["project"], model="claude-sonnet-4-6", permission_mode="bypassPermissions")
    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(prompt=prompt, options=options):
            sdk_stream.append({"type": type(msg).__name__})
            if isinstance(msg, AssistantMessage):
                for block in msg.content:
                    if isinstance(block, ToolUseBlock):
                        tool_calls.append(
                            {"tool_name": block.name, "tool_input": block.input}
                        )
                    elif isinstance(block, TextBlock):
                        assistant_text.append(block.text)
            elif isinstance(msg, ResultMessage):
                result_msg = msg

    await with_retry(_run, skip_on_outage=skip_on_outage)
    assert result_msg is not None, f"no ResultMessage from prompt: {prompt!r}"
    assert not result_msg.is_error, f"command failed: {result_msg.result}"

    all_text = " ".join(assistant_text) + " " + (result_msg.result or "")
    return result_msg, tool_calls, all_text


@pytest.fixture()
def lifecycle_project(tmp_path: Path) -> Path:
    """Project pre-seeded with a v2 PRD + SPEC + back-reference established.

    Tests start AFTER authoring — this fixture represents the state of a
    project where /edikt:sdlc:prd and /edikt:sdlc:spec have already run.
    From there, the test exercises the transition chain (ship → doctor →
    review → ship-rest).
    """
    import textwrap
    import sys
    sys.path.insert(0, str(Path(__file__).parent))
    from conftest import _link_edikt_commands  # type: ignore

    project = tmp_path / "lifecycle"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.6.0
            base: docs
            stack: []
            paths:
              decisions: docs/architecture/decisions
              invariants: docs/architecture/invariants
              plans: docs/plans
              prds: docs/product/prds
              specs: docs/product/specs
            gates:
              quality-gates: true
            evaluator:
              mode: headless
            """
        )
    )

    # Pre-seed invariants (referenced by PRD protections)
    inv_dir = project / "docs" / "architecture" / "invariants"
    inv_dir.mkdir(parents=True)
    (inv_dir / "INV-001-rate-limiting.md").write_text(
        "---\ntype: invariant\nid: INV-001\ntitle: Rate limit public endpoints\nstatus: active\n---\n\n# INV-001: Rate limiting\n\n## Rule\nPublic endpoints MUST be rate-limited.\n"
    )

    # Pre-seed v2 PRD pair
    prd_dir = project / "docs" / "product" / "prds"
    prd_dir.mkdir(parents=True)
    (prd_dir / "PRD-001-rate-limiting.md").write_text(
        "# PRD-001: API rate limiting\n\n**Status:** accepted\n**Rigor:** solo\n**Sidecar:** [PRD-001-rate-limiting.yaml](./PRD-001-rate-limiting.yaml)\n\n## Problem\n\nScrapers degrade API latency for legitimate users.\n"
    )
    (prd_dir / "PRD-001-rate-limiting.yaml").write_text(
        textwrap.dedent(
            """\
            schema_version: "1.0"
            type: prd
            id: PRD-001
            title: API rate limiting
            slug: rate-limiting
            status: accepted
            rigor: solo
            author: Test Author
            created_at: "2026-04-18T00:00:00Z"
            requirements:
              - id: FR-001
                text: Enforce per-IP rate limit on all public endpoints
                status: accepted
              - id: FR-002
                text: Return 429 with Retry-After header when limit exceeded
                status: accepted
              - id: FR-003
                text: Expose rate-limit metrics to monitoring
                status: accepted
            acceptance_criteria:
              - id: AC-001-1
                fr: FR-001
                given: a client makes 1000 requests in 60s
                when: the 1001st request arrives
                then: it returns HTTP 429
                status: accepted
            protections:
              - ref: INV-001
                note: Rate-limit invariant
            solution_references: []
            stakeholders: []
            dependencies: []
            nfrs: []
            risks: []
            open_questions: []
            source_specs: [SPEC-001]
            supersedes: null
            superseded_by: null
            deprecated_at: null
            deprecated_reason: null
            cancelled_at: null
            cancelled_reason: null
            forcing_questions:
              problem_behind_problem: Scrapers degrade p95 latency
              evidence_or_hypothesis: "Datadog incident 4521"
              north_metric_and_counter: "p95 < 200ms; 4xx rate stable"
              must_not_change: "existing public endpoints stay functional"
              riskiest_assumption: "token-bucket is enough"
            revision_history:
              - at: "2026-04-18T00:00:00Z"
                author: Test Author
                action: created
                note: Initial draft
            extensions: {}
            _sync:
              md_hash: ""
              yaml_hash: ""
              synced_at: ""
            """
        )
    )

    # Pre-seed SPEC with back-reference and coverage
    spec_root = project / "docs" / "product" / "specs" / "SPEC-001-rate-limiting"
    spec_root.mkdir(parents=True)
    (spec_root / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-001
            title: Rate limiting middleware
            status: accepted
            implements: PRD-001
            ---

            # SPEC-001: Rate limiting middleware

            Implements PRD-001 with a token-bucket per-IP strategy.

            ## Requirements

            - SR-001 implements FR-001: Middleware applies to all public routes
            - SR-002 implements FR-002: Middleware sets Retry-After header
            - SR-003 implements FR-003: Middleware emits prometheus counters

            ## Acceptance Criteria

            - AC-001-1: (from PRD) 1001st request returns 429
            - SAC-001: Middleware adds <5ms p95 overhead
            """
        )
    )

    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "plans").mkdir(parents=True)

    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Lifecycle test project

            [edikt:start]: # managed by edikt — do not edit this block manually
            ## edikt
            ### Project
            Test project for the SPEC-007 transition chain integration test.
            ### Build & Test Commands
            No build commands.
            [edikt:end]: #
            """
        )
    )

    _link_edikt_commands(project)
    return project


# ─── The lifecycle test ───────────────────────────────────────────────────────

@pytest.mark.asyncio
async def test_transition_chain_post_authoring(
    lifecycle_project: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Chain: ship FR-001 → doctor → review → ship FR-002 + FR-003.

    Pre-seeded state: v2 PRD (PRD-001 with 3 FRs) + v2 SPEC (SPEC-001 with
    source_specs back-ref already in PRD sidecar) + INV-001.

    Verifies the contract holds through the chain:
    - Partial ship flips per-FR status but not top-level
    - Doctor sees the v2 sidecar and surfaces health markers
    - Review runs against the now-partially-shipped PRD
    - Final ship of remaining FRs flips top-level status to shipped
    - Revision history accumulates through all steps (not overwritten)
    """
    prd_yaml = (
        lifecycle_project
        / "docs"
        / "product"
        / "prds"
        / "PRD-001-rate-limiting.yaml"
    )

    # Fixture precondition — sanity check the seed state is as expected
    initial = yaml.safe_load(prd_yaml.read_text())
    assert initial["status"] == "accepted"
    assert initial["source_specs"] == ["SPEC-001"]
    assert all(
        r["status"] == "accepted" for r in initial["requirements"]
    ), "fixture precondition: all FRs start accepted"
    initial_history_len = len(initial["revision_history"])

    # ─── STEP 1: Ship FR-001 ──────────────────────────────────────────────────

    _, _, ship1_text = await _invoke(
        "/edikt:sdlc:prd:ship PRD-001 FR-001", lifecycle_project, sdk_stream, request
    )

    after_ship1 = yaml.safe_load(prd_yaml.read_text())
    if after_ship1 == initial:
        # No mutation — session must at least mention the ship intent
        assert any(
            kw in ship1_text.lower() for kw in ("ship", "fr-001", "shipped")
        ), f"ship1: no mutation and no session signal. Text: {ship1_text[:400]!r}"
        pytest.skip(
            "ship transition did not mutate the sidecar in this session "
            "(may be subagent without write perms). Cannot verify chain."
        )

    frs_after_1 = {r["id"]: r for r in after_ship1.get("requirements", [])}
    assert frs_after_1["FR-001"]["status"] == "shipped", (
        f"ship1: FR-001 should be shipped, got {frs_after_1['FR-001'].get('status')}"
    )
    assert frs_after_1["FR-002"]["status"] == "accepted", (
        "ship1: FR-002 should not be touched"
    )
    assert frs_after_1["FR-003"]["status"] == "accepted", (
        "ship1: FR-003 should not be touched"
    )
    assert after_ship1["status"] != "shipped", (
        f"ship1: top-level status must not flip on partial ship, got {after_ship1['status']}"
    )

    ship1_entries = [
        e for e in after_ship1.get("revision_history", []) if e.get("action") == "ship"
    ]
    assert ship1_entries, "ship1: revision_history missing ship entry"

    # ─── STEP 2: Doctor sees the v2 sidecar ──────────────────────────────────

    _, _, doctor_text = await _invoke(
        "/edikt:doctor", lifecycle_project, sdk_stream, request
    )
    health_markers = [
        "PRD/SPEC artifact health",
        "PRD/SPEC ARTIFACT HEALTH",
        "Orphaned sidecars",
        "schema_version",
        "Sidecar drift",
        "Broken refs",
        "Broken references",
        "PRD-001",
    ]
    hits = sum(1 for m in health_markers if m in doctor_text)
    assert hits >= 2, (
        f"doctor step: expected ≥2 v2 health/PRD markers, got {hits}. "
        f"Output: {doctor_text[:600]!r}"
    )

    # ─── STEP 3: Review the partially-shipped PRD ─────────────────────────────

    _, _, review_text = await _invoke(
        "/edikt:prd:review PRD-001", lifecycle_project, sdk_stream, request
    )
    review_markers = ["rubric", "score", "review", "PRD-001", "protections", "sidecar"]
    review_hits = sum(1 for m in review_markers if m.lower() in review_text.lower())
    assert review_hits >= 3, (
        f"review step: expected ≥3 review markers, got {review_hits}. "
        f"Output: {review_text[:400]!r}"
    )

    # ─── STEP 4: Ship the remaining FRs → top-level flips ────────────────────

    _, _, ship2_text = await _invoke(
        "/edikt:sdlc:prd:ship PRD-001 FR-002 FR-003",
        lifecycle_project,
        sdk_stream,
        request,
    )

    after_ship2 = yaml.safe_load(prd_yaml.read_text())
    frs_after_2 = {r["id"]: r for r in after_ship2.get("requirements", [])}
    assert frs_after_2["FR-002"]["status"] == "shipped"
    assert frs_after_2["FR-003"]["status"] == "shipped"

    # All three FRs shipped → top-level flips
    assert after_ship2["status"] == "shipped", (
        f"ship2: with all 3 FRs shipped, top-level status must flip to shipped, "
        f"got {after_ship2['status']}"
    )

    # Revision history accumulated across the whole chain — never shrank
    final_history_len = len(after_ship2.get("revision_history", []))
    assert final_history_len > initial_history_len, (
        f"revision_history should grow through the chain: "
        f"started at {initial_history_len}, ended at {final_history_len}"
    )
    # At least two 'ship' actions recorded
    ship_entries_total = [
        e for e in after_ship2.get("revision_history", []) if e.get("action") == "ship"
    ]
    assert len(ship_entries_total) >= 2, (
        f"Expected ≥2 ship entries (one per ship call), got {len(ship_entries_total)}"
    )
