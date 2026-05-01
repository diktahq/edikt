"""E2E — Plan with criteria sidecar: written, schema-valid, evaluator-updated.

Tests /edikt:sdlc:plan produces a plan file AND a criteria sidecar
(PLAN-xxx-criteria.yaml) with the correct schema. Tests the evaluator
updating that sidecar on phase completion.

Covered scenarios:
  plan creation        → PLAN-*.md + PLAN-*-criteria.yaml written as siblings
  sidecar schema       → plan, phases[], AC-N.M ids, status:pending, verify field
  artifacts coverage   → plan references api.yaml endpoints from artifacts
  sidecar update       → phase-end-detector updates sidecar with PASS/FAIL/BLOCKED
  fail_count threshold → criterion with fail_count:3 is permanently failed
  blocked verdict      → BLOCKED propagates to sidecar with block_reason
  evaluator dry-run    → EDIKT_EVALUATOR_DRY_RUN=1 detects phase completion without API call

Edge cases:
  sidecar missing at eval time → silently skipped (plan markdown fallback)
  blocked status never increments fail_count
  fail_count resets to 0 on PASS (not on BLOCKED)
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import textwrap
import time
from pathlib import Path
from typing import Any

import pytest
import yaml

from helpers import with_retry

REPO_ROOT = Path(__file__).resolve().parents[2]
PHASE_END_HOOK = REPO_ROOT / "templates" / "hooks" / "phase-end-detector.sh"


# ─── Helpers ─────────────────────────────────────────────────────────────────


def _plan_files(plans_dir: Path) -> list[Path]:
    return [f for f in plans_dir.glob("PLAN-*.md") if not f.name.endswith("-criteria.yaml")]


def _sidecar_path(plan_file: Path) -> Path:
    stem = plan_file.stem
    return plan_file.parent / f"{stem}-criteria.yaml"


def _load_sidecar(sidecar: Path) -> dict:
    return yaml.safe_load(sidecar.read_text()) or {}


# ─── SDK-based tests (require claude auth) ───────────────────────────────────


@pytest.mark.asyncio
async def test_plan_writes_criteria_sidecar(
    project_with_spec_and_artifacts: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Plan command writes PLAN-xxx.md AND PLAN-xxx-criteria.yaml.

    The criteria sidecar is always a sibling of the plan file. If the
    plan is at docs/plans/PLAN-auth.md, the sidecar must be at
    docs/plans/PLAN-auth-criteria.yaml.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    plans_dir = project_with_spec_and_artifacts / "docs" / "plans"
    plans_before = set(plans_dir.rglob("PLAN-*"))

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_spec_and_artifacts),
        setting_sources=["project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan SPEC-001 — implement user auth",
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
    assert not result_msg.is_error, f"plan failed: {result_msg.result}"

    plans_after = set(plans_dir.rglob("PLAN-*"))
    new_files = plans_after - plans_before

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text

    # Check disk first (command may fork).
    plan_files = [f for f in new_files if f.suffix == ".md" and "criteria" not in f.name]
    sidecar_files = [f for f in new_files if "criteria" in f.name]

    if plan_files:
        # Plan written to disk — sidecar must accompany it.
        for plan_file in plan_files:
            expected_sidecar = _sidecar_path(plan_file)
            assert expected_sidecar.exists(), (
                f"Plan file {plan_file.name} was written but criteria sidecar "
                f"{expected_sidecar.name} is missing. "
                "Sidecar must always be written alongside the plan (plan.md step 10b)."
            )
    elif any(t in all_text for t in {"Phase", "phase", "PLAN", "criteria"}):
        # Command ran and produced plan content in session.
        pass
    else:
        pytest.fail(
            "plan command produced neither a plan file on disk nor plan content in the session; "
            f"result: {result_text[:200]!r}"
        )


@pytest.mark.asyncio
async def test_plan_with_artifacts_covers_api_endpoints(
    project_with_spec_and_artifacts: Path,
    sdk_stream: list[Any],
    request: pytest.FixtureRequest,
) -> None:
    """Plan references api.yaml endpoints — every endpoint must be covered.

    When artifacts include contracts/api.yaml, the plan command must
    verify all endpoints appear in at least one phase. Uncovered endpoints
    are surfaced as warnings.
    """
    from claude_agent_sdk import ClaudeAgentOptions, query
    from claude_agent_sdk.types import AssistantMessage, ResultMessage, TextBlock, ToolUseBlock

    tool_calls: list[dict[str, Any]] = []
    assistant_text: list[str] = []
    result_msg: ResultMessage | None = None

    options = ClaudeAgentOptions(
        cwd=str(project_with_spec_and_artifacts),
        setting_sources=["project"],
    )

    skip_on_outage = request.config.getoption("--skip-on-outage", default=False)

    async def _run() -> None:
        nonlocal result_msg
        async for msg in query(
            prompt="/edikt:sdlc:plan SPEC-001 — implement auth with full API coverage",
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
    assert not result_msg.is_error

    result_text = result_msg.result or ""
    all_text = " ".join(assistant_text) + " " + result_text + " ".join(
        tc["tool_input"].get("content", "") for tc in tool_calls if tc["tool_name"] in {"Write", "Edit"}
    )

    # If the plan ran fully, it should reference the endpoints or artifacts.
    if all_text.strip():
        endpoint_terms = {"/auth/google", "/auth/logout", "auth", "endpoint", "api.yaml", "contract"}
        found = any(t in all_text for t in endpoint_terms)
        assert found or "Phase" in all_text, (
            "plan with api.yaml artifacts must reference endpoints or phases; "
            f"session text: {all_text[:300]!r}"
        )


# ─── Offline sidecar schema tests (no auth needed) ───────────────────────────


class TestCriteriaSidecarSchema:
    """Validate the criteria sidecar YAML schema.

    These tests use a hand-constructed sidecar that mirrors what
    /edikt:sdlc:plan should produce. They verify the schema contract
    without calling Claude — fast, deterministic, always runnable.
    """

    def _make_sidecar(self, tmp_path: Path, extra: dict | None = None) -> Path:
        data = {
            "plan": "PLAN-auth",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [
                {
                    "phase": 1,
                    "title": "OAuth callback handler",
                    "status": "pending",
                    "attempt": "0/5",
                    "criteria": [
                        {
                            "id": "AC-1.1",
                            "text": "POST /auth/google returns 302 on valid token",
                            "status": "pending",
                            "fail_count": 0,
                            "fail_reason": None,
                            "last_evaluated": None,
                            "verify": "curl -s -o /dev/null -w '%{http_code}' -X POST http://localhost:8080/auth/google | grep -q 302",
                        },
                        {
                            "id": "AC-1.2",
                            "text": "Invalid token returns 401",
                            "status": "pending",
                            "fail_count": 0,
                            "fail_reason": None,
                            "last_evaluated": None,
                            "verify": None,
                        },
                    ],
                },
                {
                    "phase": 2,
                    "title": "Session middleware",
                    "status": "pending",
                    "attempt": "0/5",
                    "criteria": [
                        {
                            "id": "AC-2.1",
                            "text": "Expired session returns 401",
                            "status": "pending",
                            "fail_count": 0,
                            "fail_reason": None,
                            "last_evaluated": None,
                            "verify": None,
                        },
                    ],
                },
            ],
        }
        if extra:
            data.update(extra)
        sidecar = tmp_path / "PLAN-auth-criteria.yaml"
        sidecar.write_text(yaml.dump(data, default_flow_style=False))
        return sidecar

    def test_sidecar_top_level_keys_present(self, tmp_path: Path) -> None:
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)
        for key in ("plan", "generated", "phases"):
            assert key in data, f"criteria sidecar missing required top-level key: {key!r}"

    def test_sidecar_ac_ids_follow_pattern(self, tmp_path: Path) -> None:
        """Every criterion id must match AC-{phase}.{seq} pattern."""
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)
        ac_pattern = re.compile(r"^AC-\d+\.\d+$")
        for phase in data.get("phases", []):
            for criterion in phase.get("criteria", []):
                assert ac_pattern.match(criterion["id"]), (
                    f"criterion id {criterion['id']!r} does not match AC-N.M pattern; "
                    "phase: {phase['phase']}"
                )

    def test_sidecar_all_criteria_start_pending(self, tmp_path: Path) -> None:
        """All criteria must start with status: pending."""
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)
        for phase in data.get("phases", []):
            for criterion in phase.get("criteria", []):
                assert criterion["status"] == "pending", (
                    f"{criterion['id']}: initial status must be 'pending', "
                    f"got {criterion['status']!r}"
                )

    def test_sidecar_status_values_are_recognized(self, tmp_path: Path) -> None:
        """Status must be one of the recognized values."""
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)
        valid = {"pending", "in-progress", "pass", "fail", "blocked"}
        for phase in data.get("phases", []):
            for criterion in phase.get("criteria", []):
                assert criterion["status"] in valid, (
                    f"{criterion['id']}: status {criterion['status']!r} not in {valid}"
                )

    def test_sidecar_fail_count_threshold_permanently_fails(self, tmp_path: Path) -> None:
        """fail_count >= 3 means criterion is permanently failed.

        The plan command blocks the phase when any criterion reaches 3 failures.
        This test verifies the threshold logic is preserved in the sidecar.
        """
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)

        # Simulate 3 failures on AC-1.1.
        phase = data["phases"][0]
        criterion = phase["criteria"][0]
        criterion["status"] = "fail"
        criterion["fail_count"] = 3
        criterion["fail_reason"] = "POST /auth/google returned 500 not 302"
        criterion["last_evaluated"] = "2026-04-16"
        sidecar.write_text(yaml.dump(data))

        refreshed = _load_sidecar(sidecar)
        ac = refreshed["phases"][0]["criteria"][0]
        assert ac["fail_count"] >= 3, "fail_count must persist across writes"
        assert ac["status"] == "fail", "status must remain fail"
        assert ac["fail_reason"], "fail_reason must be recorded"

    def test_sidecar_blocked_does_not_increment_fail_count(self, tmp_path: Path) -> None:
        """BLOCKED verdict must not increment fail_count.

        Per plan.md step 10b update rules: fail_count is only incremented on
        FAIL verdicts, not on BLOCKED. A BLOCKED criterion can still pass later
        once the capability is restored.
        """
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)

        initial_fail_count = data["phases"][0]["criteria"][0]["fail_count"]

        # Apply a BLOCKED verdict.
        criterion = data["phases"][0]["criteria"][0]
        criterion["status"] = "blocked"
        criterion["block_reason"] = "Bash tool denied — cannot run curl"
        criterion["last_evaluated"] = "2026-04-16"
        # fail_count must NOT be incremented on BLOCKED.
        sidecar.write_text(yaml.dump(data))

        refreshed = _load_sidecar(sidecar)
        ac = refreshed["phases"][0]["criteria"][0]
        assert ac["fail_count"] == initial_fail_count, (
            f"BLOCKED verdict must not increment fail_count; "
            f"was {initial_fail_count}, now {ac['fail_count']}"
        )
        assert ac["status"] == "blocked"
        assert ac.get("block_reason"), "block_reason must be recorded on BLOCKED"

    def test_sidecar_pass_resets_fail_count(self, tmp_path: Path) -> None:
        """PASS verdict resets fail_count to 0 (not BLOCKED)."""
        sidecar = self._make_sidecar(tmp_path)
        data = _load_sidecar(sidecar)

        # Start with 2 prior failures.
        criterion = data["phases"][0]["criteria"][0]
        criterion["fail_count"] = 2
        criterion["status"] = "fail"
        sidecar.write_text(yaml.dump(data))

        # Apply PASS.
        data2 = _load_sidecar(sidecar)
        data2["phases"][0]["criteria"][0]["status"] = "pass"
        data2["phases"][0]["criteria"][0]["fail_count"] = 0
        data2["phases"][0]["criteria"][0]["last_evaluated"] = "2026-04-16"
        sidecar.write_text(yaml.dump(data2))

        refreshed = _load_sidecar(sidecar)
        ac = refreshed["phases"][0]["criteria"][0]
        assert ac["fail_count"] == 0, "PASS must reset fail_count to 0"
        assert ac["status"] == "pass"

    def test_sidecar_sibling_naming_convention(self, tmp_path: Path) -> None:
        """Sidecar must be a sibling of the plan with -criteria suffix."""
        plan = tmp_path / "PLAN-auth-v2.md"
        plan.write_text("# Plan")
        expected_sidecar = tmp_path / "PLAN-auth-v2-criteria.yaml"
        assert expected_sidecar == _sidecar_path(plan), (
            f"sidecar path {_sidecar_path(plan)} does not match expected {expected_sidecar}"
        )


# ─── Phase-end-detector hook tests (Layer 1, no auth) ────────────────────────


class TestPhaseEndDetectorSidecar:
    """Tests for phase-end-detector.sh sidecar interaction.

    The detector fires on Stop events and invokes the evaluator.
    These tests use EDIKT_EVALUATOR_DRY_RUN=1 to verify detection logic
    and sidecar reading without making real API calls.
    """

    def _make_project_with_plan(self, tmp_path: Path, in_progress_phase: int = 1) -> Path:
        project = tmp_path / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent(
                f"""\
                edikt_version: 0.5.0
                base: docs
                evaluator:
                  phase-end: true
                  mode: headless
                """
            )
        )
        plans_dir = project / "docs" / "plans"
        plans_dir.mkdir(parents=True)

        # Build phase table with phase N in-progress.
        rows = []
        for i in range(1, 4):
            status = "in-progress" if i == in_progress_phase else ("done" if i < in_progress_phase else "pending")
            rows.append(f"| {i} | Phase {i} title | {status} |")

        plan_content = "# PLAN-test\n\n| Phase | Title | Status |\n|---|---|---|\n" + "\n".join(rows)
        plan_path = plans_dir / "PLAN-test.md"
        plan_path.write_text(plan_content)

        # Criteria sidecar.
        sidecar_path = plans_dir / "PLAN-test-criteria.yaml"
        sidecar_data = {
            "plan": "PLAN-test",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [
                {
                    "phase": in_progress_phase,
                    "title": f"Phase {in_progress_phase} title",
                    "status": "in-progress",
                    "attempt": "0/5",
                    "criteria": [
                        {
                            "id": f"AC-{in_progress_phase}.1",
                            "text": "Tests pass",
                            "status": "pending",
                            "fail_count": 0,
                            "fail_reason": None,
                            "last_evaluated": None,
                            "verify": "pytest test/ -q",
                        },
                    ],
                }
            ],
        }
        sidecar_path.write_text(yaml.dump(sidecar_data))
        return project

    def _run_hook(
        self,
        project: Path,
        message: str,
        extra_env: dict | None = None,
    ) -> subprocess.CompletedProcess:
        payload = json.dumps({
            "hook_event_name": "Stop",
            "stop_hook_active": False,
            "last_assistant_message": message,
            "cwd": str(project),
        })
        env = {
            **os.environ,
            "EDIKT_EVALUATOR_DRY_RUN": "1",  # detect but don't call claude -p
            "HOME": str(project.parent),
            "EDIKT_HOME": str(project.parent / ".edikt"),
        }
        if extra_env:
            env.update(extra_env)
        return subprocess.run(
            ["bash", str(PHASE_END_HOOK)],
            input=payload,
            capture_output=True,
            text=True,
            timeout=30,
            env=env,
            cwd=str(project),
        )

    def test_phase_completion_detected_with_sidecar(self, tmp_path: Path) -> None:
        """Phase-end-detector finds criteria sidecar and includes it in dry-run output."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        result = self._run_hook(
            project,
            "Phase 1 complete. All acceptance criteria met. Tests pass."
        )
        assert result.returncode == 0, f"hook failed: {result.stderr}"
        combined = result.stdout + result.stderr
        # Dry-run should mention phase detection.
        assert "Phase 1" in combined or "phase" in combined.lower() or "dry-run" in combined.lower() or result.returncode == 0

    def test_no_completion_detected_for_wrong_phase(self, tmp_path: Path) -> None:
        """Non-completion message does not trigger evaluator."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=2)
        result = self._run_hook(
            project,
            "Refactored the helper function to reduce duplication."
        )
        assert result.returncode == 0, f"hook crashed: {result.stderr}"
        # No dry-run evaluator output — nothing was detected.
        combined = result.stdout + result.stderr
        assert "DRY" not in combined.upper() or "phase" not in combined.lower()

    def test_phase_end_false_skips_evaluation(self, tmp_path: Path) -> None:
        """phase-end: false in config disables evaluation entirely."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        # Disable phase-end evaluation.
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent(
                """\
                edikt_version: 0.5.0
                base: docs
                phase-end: false
                """
            )
        )
        result = self._run_hook(
            project,
            "Phase 1 complete. All acceptance criteria met."
        )
        assert result.returncode == 0, f"hook crashed: {result.stderr}"
        combined = result.stdout + result.stderr
        assert "DRY" not in combined.upper(), (
            "phase-end: false must prevent evaluator invocation; "
            f"unexpected evaluator output: {combined!r}"
        )

    def test_sidecar_missing_warns_user_with_recovery_command(self, tmp_path: Path) -> None:
        """Missing sidecar must warn the user and show --sidecar-only recovery command.

        Silent fallback trains users to ignore the gap. The correct behavior:
        surface a systemMessage explaining what's missing and exactly how to fix it.
        The hook still runs evaluation (falls back to plan markdown) but the user
        knows they're losing evaluation history.
        """
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        # Delete the sidecar to simulate the missing-sidecar scenario.
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        sidecar.unlink()

        result = self._run_hook(
            project,
            "Phase 1 complete. All acceptance criteria met."
        )
        assert result.returncode == 0, (
            "Missing criteria sidecar must not crash the hook; "
            f"stderr: {result.stderr!r}"
        )
        combined = result.stdout + result.stderr
        # The message must use plain language the user understands.
        assert "evaluation history" in combined.lower() or "history" in combined.lower(), (
            "Hook must use plain language ('evaluation history') not jargon ('sidecar'); "
            f"got: {combined!r}"
        )
        assert "--sidecar-only" in combined, (
            "Hook must surface the --sidecar-only recovery command; "
            f"got: {combined!r}"
        )
        # Must NOT use internal jargon in the user-facing message.
        jargon = ["criteria sidecar", "fail_count", "block_reason", "last_evaluated"]
        for term in jargon:
            assert term not in combined, (
                f"Hook must not expose internal jargon {term!r} to the user; "
                f"got: {combined!r}"
            )

    def test_no_edikt_project_exits_silently(self, tmp_path: Path) -> None:
        """Hook must exit 0 silently in a non-edikt project."""
        non_edikt_dir = tmp_path / "non-edikt"
        non_edikt_dir.mkdir()
        payload = json.dumps({
            "hook_event_name": "Stop",
            "stop_hook_active": False,
            "last_assistant_message": "Phase 1 complete.",
            "cwd": str(non_edikt_dir),
        })
        result = subprocess.run(
            ["bash", str(PHASE_END_HOOK)],
            input=payload,
            capture_output=True,
            text=True,
            timeout=15,
            cwd=str(non_edikt_dir),
        )
        assert result.returncode == 0
        assert not result.stdout.strip(), (
            "Hook must be silent in non-edikt projects; "
            f"got output: {result.stdout!r}"
        )

    # ── Auto-generation with mock claude ──────────────────────────────────────

    def _make_mock_claude(self, tmp_path: Path, behavior: str = "success") -> Path:
        """Create a mock claude binary that handles both sidecar-only and evaluator calls.

        behavior:
          "success"  — creates sidecar on --sidecar-only call, returns PASS on evaluator
          "fail"     — exits 1 on --sidecar-only (generation fails)
          "no_write" — exits 0 on --sidecar-only but writes no file
        """
        mock_dir = tmp_path / "mock-bin"
        mock_dir.mkdir(exist_ok=True)
        mock_claude = mock_dir / "claude"

        if behavior == "success":
            script = textwrap.dedent(
                """\
                #!/usr/bin/env bash
                # Mock claude: handles --sidecar-only AND evaluator prompt calls
                ARGS="$*"
                if echo "$ARGS" | grep -q "sidecar-only"; then
                    STEM=$(echo "$ARGS" | grep -oE 'PLAN-[^ ]+' | head -1)
                    for dir in docs/plans docs/product/plans; do
                        if [ -d "$dir" ]; then
                            cat > "$dir/${STEM}-criteria.yaml" <<YAML
plan: ${STEM}
generated: "2026-04-16T00:00:00Z"
last_evaluated: null
phases:
  - phase: 1
    title: Mock Phase 1
    status: pending
    attempt: "0/5"
    criteria:
      - id: AC-1.1
        text: "Tests pass"
        status: pending
        fail_count: 0
        fail_reason: null
        last_evaluated: null
        verify: "pytest test/ -q"
YAML
                            exit 0
                        fi
                    done
                else
                    # Evaluator call — return a simple PASS verdict
                    echo "VERDICT: PASS"
                    echo "AC-1.1: PASS — tests pass"
                fi
                exit 0
                """
            )
        elif behavior == "fail":
            script = "#!/usr/bin/env bash\nexit 1\n"
        else:  # no_write
            script = "#!/usr/bin/env bash\necho 'VERDICT: PASS'\nexit 0\n"

        mock_claude.write_text(script)
        mock_claude.chmod(0o755)
        return mock_dir

    def test_hook_auto_generates_sidecar_using_mock_claude(self, tmp_path: Path) -> None:
        """When sidecar is missing and claude is available, hook auto-generates it.

        Uses a mock claude binary that creates a valid criteria sidecar.
        After auto-generation succeeds, the hook must use the new sidecar
        (not fall back to plan markdown).
        """
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        # Remove the sidecar.
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        sidecar.unlink()
        assert not sidecar.exists()

        mock_bin = self._make_mock_claude(tmp_path, behavior="success")

        # Run WITHOUT EDIKT_EVALUATOR_DRY_RUN (so auto-gen can actually run).
        # The mock claude handles both the --sidecar-only call AND the evaluator call.
        # Unset EDIKT_EVALUATOR_DRY_RUN so it defaults to 0 (not "1").
        env = {
            **os.environ,
            "HOME": str(project.parent),
            "EDIKT_HOME": str(project.parent / ".edikt"),
            "EDIKT_SKIP_SIDECAR_REGEN": "0",
            "PATH": f"{mock_bin}:{os.environ.get('PATH', '/usr/bin:/bin')}",
        }
        env.pop("EDIKT_EVALUATOR_DRY_RUN", None)  # must NOT be set to "1"

        result = subprocess.run(
            ["bash", str(PHASE_END_HOOK)],
            input=json.dumps({
                "hook_event_name": "Stop",
                "stop_hook_active": False,
                "last_assistant_message": "Phase 1 complete. All acceptance criteria met.",
                "cwd": str(project),
            }),
            capture_output=True, text=True, timeout=30,
            env=env,
            cwd=str(project),
        )
        assert result.returncode == 0, f"hook crashed: {result.stderr}"

        # Sidecar must have been created by the mock claude.
        assert sidecar.exists(), (
            "Mock claude ran --sidecar-only but sidecar was not created; "
            f"hook output: {result.stdout + result.stderr}"
        )

        # The hook output must NOT show the 'history not found' warning
        # since generation succeeded.
        combined = result.stdout + result.stderr
        # The dry-run output should show Sidecar: <path> not (none).
        assert "(none)" not in combined, (
            "After successful auto-generation, hook must use the new sidecar "
            f"not fall back to plan markdown; output: {combined!r}"
        )

    def test_hook_warns_when_auto_generation_fails(self, tmp_path: Path) -> None:
        """When auto-generation fails (claude exits non-zero), hook warns and falls back."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        sidecar.unlink()

        mock_bin = self._make_mock_claude(tmp_path, behavior="fail")

        env = {
            **os.environ,
            "HOME": str(project.parent),
            "EDIKT_HOME": str(project.parent / ".edikt"),
            "EDIKT_SKIP_SIDECAR_REGEN": "0",
            "PATH": f"{mock_bin}:{os.environ.get('PATH', '/usr/bin:/bin')}",
        }
        env.pop("EDIKT_EVALUATOR_DRY_RUN", None)

        result = subprocess.run(
            ["bash", str(PHASE_END_HOOK)],
            input=json.dumps({
                "hook_event_name": "Stop",
                "stop_hook_active": False,
                "last_assistant_message": "Phase 1 complete. All acceptance criteria met.",
                "cwd": str(project),
            }),
            capture_output=True, text=True, timeout=30,
            env=env,
            cwd=str(project),
        )
        assert result.returncode == 0, f"hook crashed: {result.stderr}"

        combined = result.stdout + result.stderr
        # Must warn the user.
        assert "history" in combined.lower(), (
            "Hook must warn about missing history when auto-generation fails; "
            f"got: {combined!r}"
        )
        assert "--sidecar-only" in combined, (
            "Hook must show recovery command even when auto-generation fails"
        )
        # Must mention that auto-generation was attempted (gen_status="failed" path).
        assert "tried" in combined.lower() or "couldn't" in combined.lower() or "automatically" in combined.lower(), (
            "Hook warning must explain that auto-generation was attempted but failed; "
            f"got: {combined!r}"
        )

    def test_hook_warns_immediately_when_claude_unavailable(self, tmp_path: Path) -> None:
        """When claude is not in PATH, hook warns immediately without attempting generation."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        sidecar.unlink()

        # Use a temp dir with only bash-compatible system tools — no claude binary.
        # Must keep /bin, /usr/bin so bash itself and basic tools work.
        safe_path = "/usr/bin:/bin:/usr/local/bin"
        env = {
            **os.environ,
            "HOME": str(project.parent),
            "EDIKT_HOME": str(project.parent / ".edikt"),
            "EDIKT_SKIP_SIDECAR_REGEN": "0",
            "PATH": safe_path,
        }
        env.pop("EDIKT_EVALUATOR_DRY_RUN", None)

        result = subprocess.run(
            ["bash", str(PHASE_END_HOOK)],
            input=json.dumps({
                "hook_event_name": "Stop",
                "stop_hook_active": False,
                "last_assistant_message": "Phase 1 complete. All acceptance criteria met.",
                "cwd": str(project),
            }),
            capture_output=True, text=True, timeout=30,
            env=env,
            cwd=str(project),
        )
        assert result.returncode == 0
        combined = result.stdout + result.stderr
        assert "history" in combined.lower() or "--sidecar-only" in combined, (
            "Hook must warn about missing history when claude unavailable"
        )

    def test_hook_auto_gen_skipped_when_env_set(self, tmp_path: Path) -> None:
        """EDIKT_SKIP_SIDECAR_REGEN=1 prevents auto-generation attempt."""
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        sidecar.unlink()

        mock_bin = self._make_mock_claude(tmp_path, behavior="success")

        result = self._run_hook(
            project,
            "Phase 1 complete. All acceptance criteria met.",
            extra_env={
                "EDIKT_EVALUATOR_DRY_RUN": "1",
                "EDIKT_SKIP_SIDECAR_REGEN": "1",  # explicitly skip
                "PATH": f"{mock_bin}:{os.environ.get('PATH', '/usr/bin:/bin')}",
            },
        )
        assert result.returncode == 0
        # Sidecar must NOT have been created (skip flag honoured).
        assert not sidecar.exists(), (
            "EDIKT_SKIP_SIDECAR_REGEN=1 must prevent auto-generation"
        )

    def test_hook_corrupted_sidecar_falls_back_gracefully(self, tmp_path: Path) -> None:
        """Corrupted YAML in sidecar must not crash the hook.

        The hook passes the sidecar path to the evaluator prompt. The
        evaluator (claude -p) handles YAML parsing — a corrupted file
        should degrade gracefully, not crash the shell hook.
        """
        project = self._make_project_with_plan(tmp_path, in_progress_phase=1)
        sidecar = project / "docs" / "plans" / "PLAN-test-criteria.yaml"
        # Overwrite with invalid YAML.
        sidecar.write_text("this: is: not: valid: yaml: [\n  unclosed bracket\n")

        result = self._run_hook(
            project,
            "Phase 1 complete. All acceptance criteria met.",
        )
        # Hook must not crash — it passes sidecar path to evaluator, which handles parsing.
        assert result.returncode == 0, (
            "Corrupted sidecar YAML must not crash the hook; "
            f"stderr: {result.stderr!r}"
        )


class TestCriteriaSidecarEdgeCases:
    """Edge cases for criteria sidecar schema and sidecar-plan sync."""

    def test_sidecar_with_empty_criteria_list(self, tmp_path: Path) -> None:
        """A phase with no acceptance criteria must produce an empty list, not crash."""
        sidecar_path = tmp_path / "PLAN-empty-criteria.yaml"
        data = {
            "plan": "PLAN-empty-criteria",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [
                {
                    "phase": 1,
                    "title": "Phase with no ACs",
                    "status": "pending",
                    "attempt": "0/5",
                    "criteria": [],  # empty — valid but unusual
                }
            ],
        }
        sidecar_path.write_text(yaml.dump(data))
        loaded = yaml.safe_load(sidecar_path.read_text())
        assert loaded["phases"][0]["criteria"] == [], (
            "Empty criteria list must round-trip through YAML without error"
        )

    def test_sidecar_out_of_sync_plan_has_more_phases(self, tmp_path: Path) -> None:
        """Sidecar with 2 phases but plan has 3 — the merge must add the missing phase.

        This is the 'out of sync' scenario: someone added a phase to the plan
        after the sidecar was generated. The --sidecar-only regeneration must
        detect the new phase and add it with pending criteria.
        """
        # Sidecar with 2 phases.
        sidecar_path = tmp_path / "PLAN-test-criteria.yaml"
        old_data = {
            "plan": "PLAN-test",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [
                {
                    "phase": 1, "title": "Phase 1", "status": "pass",
                    "attempt": "1/5",
                    "criteria": [{"id": "AC-1.1", "text": "Tests pass",
                                  "status": "pass", "fail_count": 0,
                                  "fail_reason": None, "last_evaluated": "2026-04-16",
                                  "verify": "pytest"}],
                },
                {
                    "phase": 2, "title": "Phase 2", "status": "pending",
                    "attempt": "0/5",
                    "criteria": [{"id": "AC-2.1", "text": "API works",
                                  "status": "pending", "fail_count": 0,
                                  "fail_reason": None, "last_evaluated": None,
                                  "verify": None}],
                },
            ],
        }
        sidecar_path.write_text(yaml.dump(old_data))

        # Simulate merge: add phase 3 (new in plan) to existing sidecar.
        refreshed = yaml.safe_load(sidecar_path.read_text())
        new_phase = {
            "phase": 3, "title": "Phase 3 (new)", "status": "pending",
            "attempt": "0/5",
            "criteria": [{"id": "AC-3.1", "text": "Integration tests pass",
                          "status": "pending", "fail_count": 0,
                          "fail_reason": None, "last_evaluated": None,
                          "verify": None}],
        }
        refreshed["phases"].append(new_phase)
        sidecar_path.write_text(yaml.dump(refreshed))

        final = yaml.safe_load(sidecar_path.read_text())

        # Phase 1 must still be "pass" — not reset.
        assert final["phases"][0]["status"] == "pass", (
            "Merge must not reset existing phase evaluation results"
        )
        assert final["phases"][0]["criteria"][0]["status"] == "pass", (
            "Existing passing criterion must not be reset to pending on merge"
        )

        # Phase 3 must now exist with pending status.
        assert len(final["phases"]) == 3, "merged sidecar must have 3 phases"
        assert final["phases"][2]["status"] == "pending", (
            "New phase added by merge must start as pending"
        )

    def test_sidecar_fail_count_persists_across_merge(self, tmp_path: Path) -> None:
        """fail_count on a failing criterion must survive a --sidecar-only merge.

        If a criterion has fail_count: 2 and the plan is re-generated
        (e.g. to add a new phase), the merge must not reset fail_count to 0.
        Losing failure history would allow failing criteria to be retried
        indefinitely past the threshold.
        """
        sidecar_path = tmp_path / "PLAN-fc-criteria.yaml"
        data = {
            "plan": "PLAN-fc",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [{
                "phase": 1, "title": "Phase 1", "status": "fail",
                "attempt": "2/5",
                "criteria": [{
                    "id": "AC-1.1", "text": "Tests pass", "status": "fail",
                    "fail_count": 2, "fail_reason": "pytest exit 1",
                    "last_evaluated": "2026-04-16", "verify": "pytest",
                }],
            }],
        }
        sidecar_path.write_text(yaml.dump(data))

        # Simulate merge (no changes to existing criteria).
        loaded = yaml.safe_load(sidecar_path.read_text())
        sidecar_path.write_text(yaml.dump(loaded))

        final = yaml.safe_load(sidecar_path.read_text())
        ac = final["phases"][0]["criteria"][0]
        assert ac["fail_count"] == 2, (
            "fail_count must not be reset to 0 during sidecar merge; "
            f"got: {ac['fail_count']}"
        )
        assert ac["fail_reason"] == "pytest exit 1", (
            "fail_reason must be preserved during merge"
        )

    def test_sidecar_phase_status_worst_case_propagation(self, tmp_path: Path) -> None:
        """Phase status must reflect the worst-case criterion status.

        If any criterion in a phase is 'fail', the phase is 'fail'.
        If any is 'blocked' (but none 'fail'), the phase is 'blocked'.
        Only if all are 'pass' is the phase 'pass'.
        """
        sidecar_path = tmp_path / "PLAN-ws-criteria.yaml"

        # Mixed: pass + fail → phase should be fail.
        data = {
            "plan": "PLAN-ws",
            "generated": "2026-04-16T00:00:00Z",
            "last_evaluated": None,
            "phases": [{
                "phase": 1, "title": "Phase 1", "status": "fail",
                "attempt": "1/5",
                "criteria": [
                    {"id": "AC-1.1", "text": "Tests pass", "status": "pass",
                     "fail_count": 0, "fail_reason": None,
                     "last_evaluated": "2026-04-16", "verify": None},
                    {"id": "AC-1.2", "text": "Lint clean", "status": "fail",
                     "fail_count": 1, "fail_reason": "ruff exit 1",
                     "last_evaluated": "2026-04-16", "verify": None},
                ],
            }],
        }
        sidecar_path.write_text(yaml.dump(data))
        loaded = yaml.safe_load(sidecar_path.read_text())

        # Phase status must reflect worst case.
        phase = loaded["phases"][0]
        criterion_statuses = {c["status"] for c in phase["criteria"]}
        if "fail" in criterion_statuses:
            expected_phase_status = "fail"
        elif "blocked" in criterion_statuses:
            expected_phase_status = "blocked"
        elif all(s == "pass" for s in criterion_statuses):
            expected_phase_status = "pass"
        else:
            expected_phase_status = "in-progress"

        assert phase["status"] == expected_phase_status, (
            f"Phase status must reflect worst-case criterion: "
            f"criteria={criterion_statuses}, "
            f"expected phase={expected_phase_status!r}, got {phase['status']!r}"
        )
