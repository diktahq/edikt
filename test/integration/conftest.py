"""Top-level conftest for test/integration/ — Layer 2 Agent SDK integration tests.

Provides:
  - claude auth gate (session start, loud failure when SDK tests run without auth)
  - Fixture projects: sandbox_home, fresh_project, project_with_plan,
    project_post_compact, project_with_customized_agents
  - assert_tool_sequence: fuzzy-match snapshot helper (tool type + path pattern)
  - with_retry: jittered exponential backoff around SDK query() calls
  - failure_logger: pytest_runtest_makereport hook persists SDK streams on failure
  - --skip-on-outage: marks tests skipped (not failed) on upstream errors

Auth model
----------
The claude-agent-sdk spawns the bundled ``claude`` CLI binary and communicates
via JSON streaming. Authentication is the user's Claude subscription session
(stored in ``~/.claude/``), NOT an ANTHROPIC_API_KEY — the SDK never reads that
env var. In CI/headless environments set ANTHROPIC_API_KEY so the claude CLI can
authenticate without an interactive login.

Tests in ``regression/`` use pure Python reference implementations and never
call claude. They run without any authentication. The auth gate only fires when
SDK-marked tests are collected.
"""

from __future__ import annotations

import asyncio
import json
import os
import re
import random
import shutil
import textwrap
import time
from datetime import datetime, timezone
from pathlib import Path
from typing import Any, Callable, Coroutine

import pytest

_HERE = Path(__file__).parent
REPO_ROOT = _HERE.parents[1]
SNAPSHOTS_DIR = _HERE / "snapshots"
FAILURES_DIR = _HERE / "failures"
_FIXTURES_DIR = REPO_ROOT / "test" / "fixtures" / "projects"


def _load_dotenv() -> None:
    """Load test/integration/.env into os.environ if the file exists.

    Supports KEY=value and KEY="value" formats. Does NOT override variables
    that are already set in the environment — explicit env vars always win.
    Create test/integration/.env with ANTHROPIC_API_KEY=sk-... and it will
    be picked up automatically. Never commit that file (.gitignore covers it).
    """
    env_file = _HERE / ".env"
    if not env_file.exists():
        env_file = REPO_ROOT / ".env"
    if not env_file.exists():
        return
    for line in env_file.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if key and key not in os.environ:
            os.environ[key] = value


_load_dotenv()


# ─── Pytest options ──────────────────────────────────────────────────────────


def pytest_addoption(parser: pytest.Parser) -> None:
    parser.addoption(
        "--skip-on-outage",
        action="store_true",
        default=False,
        help=(
            "After retry budget exhausted on upstream errors, mark the test "
            "as skipped rather than failed and write an event to "
            "failures/outages.jsonl. Allows CI to surface 'run partial' "
            "rather than blocking the entire gate on transient issues."
        ),
    )


# ─── Claude auth gate ──────────────────────────────��─────────────────────────


def _claude_session_exists() -> bool:
    """Return True if the claude CLI has a stored subscription session.

    Claude Code stores sessions in ~/.claude/sessions/*.json (OAuth flow).
    Also checks legacy credential file locations for older installs.
    """
    claude_home = Path(os.environ.get("CLAUDE_HOME", str(Path.home() / ".claude")))
    # OAuth session files (current Claude Code auth model)
    sessions_dir = claude_home / "sessions"
    if sessions_dir.is_dir() and any(sessions_dir.glob("*.json")):
        return True
    # Legacy credential file locations
    for candidate in ("credentials", "auth.json", ".credentials", "session.json"):
        if (claude_home / candidate).exists():
            return True
    return False


def _sdk_tests_collected(items: list[pytest.Item]) -> bool:
    """Return True if any collected test requires the claude CLI.

    Tests in regression/ and governance/ use pure Python — no SDK calls.
    Everything else (the core test_*.py SDK tests) calls claude.
    """
    no_auth_dirs = {str(_HERE / "regression"), str(_HERE / "governance")}
    return any(
        not any(str(item.fspath).startswith(d) for d in no_auth_dirs)
        and not str(item.fspath).endswith("conftest.py")
        for item in items
        if hasattr(item, "fspath")
    )


def pytest_collection_finish(session: pytest.Session) -> None:
    """Gate on claude authentication when SDK tests are collected.

    Regression museum tests (regression/) use Python reference implementations
    and never call claude — they are always allowed through.

    SDK tests require the user's subscription session OR ANTHROPIC_API_KEY
    for CI/headless environments. Neither present → loud failure, no silent skip.
    """
    if not _sdk_tests_collected(session.items):
        return

    has_session = _claude_session_exists()
    has_api_key = bool(os.environ.get("ANTHROPIC_API_KEY"))

    if not has_session and not has_api_key:
        pytest.exit(
            "claude CLI not authenticated for Layer 2 SDK tests.\n\n"
            "Options:\n"
            "  1. Log in interactively:  claude auth login\n"
            "     (uses your Claude subscription — no separate API key needed)\n"
            "  2. Set ANTHROPIC_API_KEY  (for CI/headless environments)\n"
            "  3. Skip SDK tests:        SKIP_INTEGRATION=1 ./test/run.sh\n\n"
            "Regression museum tests never call claude and run without auth:\n"
            "  pytest test/integration/regression/",
            returncode=1,
        )


# ─── SDK stream capture / failure persistence ────────────────────────────────

# Maps test nodeid → message list populated during the test.
_sdk_streams: dict[str, list[Any]] = {}


@pytest.fixture()
def sdk_stream(request: pytest.FixtureRequest) -> list[Any]:
    """Yields a list that tests append Agent SDK messages to.

    Captured on failure by pytest_runtest_makereport and written to
    test/integration/failures/<name>-<ts>.jsonl for claude-replay.
    """
    stream: list[Any] = []
    _sdk_streams[request.node.nodeid] = stream
    yield stream
    _sdk_streams.pop(request.node.nodeid, None)


@pytest.hookimpl(hookwrapper=True)
def pytest_runtest_makereport(
    item: pytest.Item, call: pytest.CallInfo  # type: ignore[type-arg]
) -> None:
    outcome = yield
    report: pytest.TestReport = outcome.get_result()

    if report.when != "call" or not report.failed:
        return

    stream = _sdk_streams.get(item.nodeid, [])
    if not stream:
        return

    FAILURES_DIR.mkdir(parents=True, exist_ok=True)
    ts = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%SZ")
    safe_name = re.sub(r"[^\w.-]", "_", item.name)
    out_path = FAILURES_DIR / f"{safe_name}-{ts}.jsonl"
    with out_path.open("w") as fh:
        for msg in stream:
            record = msg if isinstance(msg, dict) else {"message": str(msg)}
            fh.write(json.dumps(record) + "\n")


# ─── Retry / backoff ─────────────────────────────────────────────────────────


def _is_5xx(exc: Exception) -> bool:
    """Return True if exc looks like an Anthropic API 5xx error."""
    msg = str(exc).lower()
    return "500" in msg or "502" in msg or "503" in msg or "504" in msg or "overloaded" in msg


def _log_outage(exc: Exception) -> None:
    FAILURES_DIR.mkdir(parents=True, exist_ok=True)
    log_path = FAILURES_DIR / "outages.jsonl"
    record = {
        "at": datetime.now(timezone.utc).isoformat(),
        "error": str(exc),
    }
    with log_path.open("a") as fh:
        fh.write(json.dumps(record) + "\n")


async def with_retry(
    func: Callable[[], Coroutine[Any, Any, Any]],
    attempts: int = 3,
    *,
    skip_on_outage: bool = False,
) -> Any:
    """Jittered exponential backoff wrapper for SDK query() calls.

    Attempt 1: immediate.
    Attempt 2: 1s base + U(0, 1)s jitter.
    Attempt 3: 2s base + U(0, 2)s jitter.

    If ``skip_on_outage`` is True and the final attempt raises a 5xx-like
    error, writes an outage event and calls pytest.skip() instead of re-raising.
    """
    base_delays = [0.0, 1.0, 2.0]
    last_exc: Exception | None = None

    for i in range(attempts):
        try:
            return await func()
        except Exception as exc:  # noqa: BLE001
            last_exc = exc
            is_5xx = _is_5xx(exc)

            if i < attempts - 1:
                base = base_delays[min(i + 1, len(base_delays) - 1)]
                jitter = random.uniform(0, base)
                await asyncio.sleep(base + jitter)
            elif is_5xx and skip_on_outage:
                _log_outage(exc)
                pytest.skip(f"Upstream outage after {attempts} retries — {exc}")

    assert last_exc is not None
    raise last_exc


# ─── Snapshot / fuzzy-match helper ──────────────────────────────────────────


def assert_tool_sequence(
    tool_calls: list[dict[str, Any]],
    snapshot: str | None = None,
    *,
    update: bool = False,
) -> None:
    """Fuzzy-match tool_calls against a stored snapshot.

    Compares by *tool type + path pattern*, ignoring exact argument wording,
    exact content values, and ordering of parallel tool calls.

    Snapshot format (test/integration/snapshots/<name>.json)::

        [
          {"tool_type": "Write", "path_pattern": ".*\\.go$"},
          {"tool_type": "Edit",  "path_pattern": ".*\\.md$"}
        ]

    If ``snapshot`` is None, skips the snapshot assertion (useful for
    tests that only need behavioral assertions, not structural replay).

    If ``update=True`` (never set by CI), regenerates the snapshot file
    from the current tool_calls. Only available locally.
    """
    if snapshot is None:
        return

    snapshot_path = SNAPSHOTS_DIR / f"{snapshot}.json"

    # Normalise tool_calls into (type, path) tuples.
    def _normalize(tc: dict[str, Any]) -> tuple[str, str]:
        tool_type = tc.get("tool_name") or tc.get("type") or ""
        path = (
            tc.get("tool_input", {}).get("file_path")
            or tc.get("tool_input", {}).get("path")
            or ""
        )
        return (str(tool_type), str(path))

    actual = [_normalize(tc) for tc in tool_calls]

    if update:
        SNAPSHOTS_DIR.mkdir(parents=True, exist_ok=True)
        snapshot_path.write_text(
            json.dumps(
                [{"tool_type": t, "path_pattern": re.escape(p)} for t, p in actual],
                indent=2,
            )
            + "\n"
        )
        return

    if not snapshot_path.exists():
        # First run — write the snapshot automatically only in local mode.
        if os.environ.get("CI"):
            raise AssertionError(
                f"Snapshot {snapshot!r} missing. "
                "Run tests locally with --update-snapshots to generate it."
            )
        SNAPSHOTS_DIR.mkdir(parents=True, exist_ok=True)
        snapshot_path.write_text(
            json.dumps(
                [{"tool_type": t, "path_pattern": re.escape(p)} for t, p in actual],
                indent=2,
            )
            + "\n"
        )
        return

    expected: list[dict[str, str]] = json.loads(snapshot_path.read_text())

    for exp in expected:
        exp_type = exp.get("tool_type", "")
        exp_pattern = exp.get("path_pattern", ".*")
        matched = any(
            t == exp_type and re.search(exp_pattern, p) for t, p in actual
        )
        assert matched, (
            f"Expected tool call {exp_type!r} matching path {exp_pattern!r} "
            f"not found in actual calls: {actual}"
        )


# ─── Sandbox / project fixtures ──────────────────────────────────────────────


@pytest.fixture()
def sandbox_home(tmp_path: Path) -> dict[str, Path]:
    """Isolated $HOME with empty .edikt/ and .claude/ subtrees.

    Returns a dict with keys: home, edikt_home, claude_home.
    Does NOT mutate os.environ — pass ``sb["env"]`` to subprocess.run
    so concurrent tests stay isolated.
    """
    home = tmp_path / "home"
    edikt_home = home / ".edikt"
    claude_home = home / ".claude"
    home.mkdir(parents=True)
    edikt_home.mkdir()
    claude_home.mkdir()
    env = {
        **os.environ,
        "HOME": str(home),
        "EDIKT_HOME": str(edikt_home),
        "CLAUDE_HOME": str(claude_home),
    }
    env.pop("EDIKT_ROOT", None)
    return {
        "home": home,
        "edikt_home": edikt_home,
        "claude_home": claude_home,
        "env": env,
    }


@pytest.fixture()
def fresh_project(sandbox_home: dict[str, Path], tmp_path: Path) -> Path:
    """Minimal project directory with a bare .edikt/config.yaml.

    No existing plans, no hooks registration, no compile output.
    Represents the 'empty' baseline that /edikt:init operates against.
    """
    project = tmp_path / "project"
    project.mkdir()
    edikt_dir = project / ".edikt"
    edikt_dir.mkdir()
    (edikt_dir / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.5.0
            base: docs
            stack: []
            paths:
              decisions: docs/architecture/decisions
              invariants: docs/architecture/invariants
              plans: docs/plans
            gates:
              quality-gates: true
            """
        )
    )
    return project


@pytest.fixture()
def project_with_plan(tmp_path: Path) -> Path:
    """Project with an active plan, Phase 1 done and Phase 2 in-progress.

    Includes a CLAUDE.md with the edikt sentinel block so Claude has project
    context about the plan structure. The plan file has enough content for
    Claude to read and understand the current state.
    """
    src = _FIXTURES_DIR / "mid-plan"
    project = tmp_path / "project-with-plan"
    shutil.copytree(src, project)

    # Add CLAUDE.md so edikt context is present in the project root.
    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Project

            Test project for edikt integration tests.

            [edikt:start]: # managed by edikt — do not edit this block manually
            ## edikt

            ### Active Plan
            There is an active plan at docs/plans/PLAN-feature-x.md.
            Phase 1 is done. Phase 2 (Implementation) is in-progress.

            ### Build & Test Commands
            No build commands — this is a test fixture.
            [edikt:end]: #
            """
        )
    )
    return project


@pytest.fixture()
def project_post_compact(tmp_path: Path) -> Path:
    """Project simulating post-compaction state.

    The active plan file exists on disk. Includes a CLAUDE.md edikt block
    that references the plan so Claude can recover context after compaction.
    """
    src = _FIXTURES_DIR / "post-compact"
    project = tmp_path / "project-post-compact"
    shutil.copytree(src, project)

    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Project

            Test project for edikt integration tests.

            [edikt:start]: # managed by edikt — do not edit this block manually
            ## edikt

            ### Active Plan
            There is an active plan at docs/plans/PLAN-feature-x.md.
            Phase 1 is done. Phase 2 (Implementation) is in-progress.
            Phase 3 (Testing) is pending.

            ### Build & Test Commands
            No build commands — this is a test fixture.
            [edikt:end]: #
            """
        )
    )
    return project


@pytest.fixture()
def project_with_customized_agents(tmp_path: Path) -> Path:
    """Project with one agent file that carries provenance frontmatter AND user edits.

    ``backend.md`` has ``edikt_template_hash`` set (from install) plus a
    custom section added by the user after install. No
    ``<!-- edikt:custom -->`` marker, so the upgrade provenance-first path
    must detect the divergence and take ``threeway_prompt``.
    """
    project = tmp_path / "project-customized"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.5.0
            base: docs
            stack: [go]
            paths:
              decisions: docs/architecture/decisions
            gates:
              quality-gates: true
            """
        )
    )
    agents_dir = project / ".claude" / "agents"
    agents_dir.mkdir(parents=True)
    (agents_dir / "backend.md").write_text(
        textwrap.dedent(
            """\
            ---
            description: Backend specialist agent
            edikt_template_hash: aaabbbccc000111222333444555666777888999000
            edikt_template_version: 0.4.3
            ---

            # Backend Agent

            You are a backend specialist. Focus on Go services.

            ## Custom Rules (added by user)

            - Always use context.Context as first parameter.
            - Prefer table-driven tests in *_test.go files.
            """
        )
    )
    return project
