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


def _safe_copytree(src: Path, dst: Path) -> None:
    """Copy a fixture directory into a sandbox, enforcing INV-007.

    Rules (INV-007):
    - symlinks=True: preserve symlinks rather than following them, so a symlink
      in the fixture doesn't escape the sandbox.
    - realpath guard: refuse if src's resolved path escapes REPO_ROOT — prevents
      a fixture entry like ``../../~/.claude/settings.json`` from silently
      copying host secrets into the sandbox.
    """
    src_real = src.resolve()
    repo_real = REPO_ROOT.resolve()
    try:
        src_real.relative_to(repo_real)
    except ValueError:
        raise RuntimeError(
            f"[INV-007] copytree source {src_real} escapes repo root {repo_real}"
        )
    shutil.copytree(src, dst, symlinks=True)


def _load_dotenv() -> None:
    """Load test/integration/.env into os.environ if the file exists.

    Supports KEY=value and KEY="value" formats. Does NOT override variables
    that are already set in the environment — explicit env vars always win.
    Create test/integration/.env with ANTHROPIC_API_KEY=sk-... and it will
    be picked up automatically. Never commit that file (.gitignore covers it).

    Security (INV-006, audit MED-2): keys are allowlisted. A .env line setting
    LD_PRELOAD, DYLD_*, PATH, PYTHONPATH, or similar would otherwise be forwarded
    to every subprocess test fixture. Forbidden prefixes cause a loud error so
    the policy is not silently suppressed.
    """
    env_file = _HERE / ".env"
    if not env_file.exists():
        env_file = REPO_ROOT / ".env"
    if not env_file.exists():
        return

    # Allowlist: ANTHROPIC_*, CLAUDE_*, EDIKT_*. Explicit deny-list for known
    # dangerous prefixes so typos produce a clear error instead of a silent skip.
    import re as _re_env
    allow_pat = _re_env.compile(r'^(ANTHROPIC_|CLAUDE_|EDIKT_)[A-Z0-9_]*$')
    deny_prefixes = (
        'LD_', 'DYLD_', 'PATH', 'PYTHONPATH', 'PYTHONSTARTUP',
        'PYTHONHOME', 'PYTHONDONTWRITEBYTECODE',
    )

    for line in env_file.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        key = key.strip()
        value = value.strip().strip('"').strip("'")
        if not key:
            continue
        # Loud failure on known-dangerous prefixes.
        if any(key == p or key.startswith(p) for p in deny_prefixes):
            raise RuntimeError(
                f"[edikt tests] .env key {key!r} is in the deny-list "
                f"({', '.join(deny_prefixes)}). Refusing to load — "
                "remove the line or rename the variable."
            )
        if not allow_pat.match(key):
            # Unknown key shape — skip silently (forward-compat).
            continue
        if key not in os.environ:
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
    Security (INV-006, audit MED-3): session files must parse as JSON and
    contain at least one credential-carrying field — an empty file dropped
    by an attacker (who controls CLAUDE_HOME) would otherwise satisfy the
    gate and mask real auth failures as upstream outages under
    --skip-on-outage.
    """
    claude_home = Path(os.environ.get("CLAUDE_HOME", str(Path.home() / ".claude")))
    required_fields = {"access_token", "refresh_token", "expiresAt", "token"}
    sessions_dir = claude_home / "sessions"
    if sessions_dir.is_dir():
        for p in sessions_dir.glob("*.json"):
            try:
                data = json.loads(p.read_text())
            except (OSError, json.JSONDecodeError):
                continue
            if isinstance(data, dict) and any(f in data for f in required_fields):
                return True
    # Legacy credential file locations — accept presence only if non-empty.
    for candidate in ("credentials", "auth.json", ".credentials", "session.json"):
        cand = claude_home / candidate
        if cand.exists() and cand.stat().st_size > 0:
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


def _is_retryable_upstream(exc: Exception) -> bool:
    """Return True if exc is a retryable upstream error class.

    Security (INV-006, audit LOW-9): whitelist by class, not by error text.
    An auth error (401) would otherwise match no substring and fall through
    to the "retry anyway" path — and with --skip-on-outage, an invalid
    credential would be silently reported as an upstream outage.
    """
    # Prefer class-based matching.
    name = exc.__class__.__name__
    retryable_names = {
        "OverloadedError", "InternalServerError", "APITimeoutError",
        "APIConnectionError", "RateLimitError",
    }
    if name in retryable_names:
        return True
    # Text-based fallback — only for 5xx status codes, never auth/4xx.
    msg = str(exc).lower()
    if any(code in msg for code in ("500", "502", "503", "504")) and not any(
        auth in msg for auth in ("401", "403", "invalid_api_key", "authentication")
    ):
        return True
    if "overloaded" in msg:
        return True
    return False


# Back-compat alias — some tests may import _is_5xx directly.
_is_5xx = _is_retryable_upstream


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
            # LOW-9: only retry if the error class is known-retryable.
            # Auth errors and other 4xxs re-raise immediately — they are not
            # transient and should not be counted against the retry budget
            # or classified as upstream outages.
            if not _is_retryable_upstream(exc):
                raise
            last_exc = exc
            is_upstream = True

            if i < attempts - 1:
                base = base_delays[min(i + 1, len(base_delays) - 1)]
                jitter = random.uniform(0, base)
                await asyncio.sleep(base + jitter)
            elif is_upstream and skip_on_outage:
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
        # LOW-10: never auto-write a missing snapshot. A silent "first run"
        # bake-in made it trivial for a drive-by contributor to lock in a
        # regression as the expected output. Require the explicit flag.
        raise AssertionError(
            f"Snapshot {snapshot!r} missing. "
            "Run tests with --update-snapshots or EDIKT_UPDATE_SNAPSHOTS=1 to generate it."
        )
        # Unreachable code preserved for reference only (never executes).
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
    _safe_copytree(src, project)

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
    _safe_copytree(src, project)

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


@pytest.fixture()
def project_with_accepted_prd(tmp_path: Path) -> Path:
    """Project containing an accepted PRD ready for spec generation.

    The PRD has ``status: accepted`` so /edikt:sdlc:spec can proceed without
    a status-gate prompt. Used by the SDLC chain E2E test.
    """
    project = tmp_path / "project-sdlc"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.5.0
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
            """
        )
    )
    # Pre-seed an accepted PRD so spec can proceed immediately.
    prd_dir = project / "docs" / "product" / "prds"
    prd_dir.mkdir(parents=True)
    (prd_dir / "PRD-001-user-auth.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: prd
            id: PRD-001
            title: User authentication with OAuth2
            status: accepted
            created_at: 2026-04-16T00:00:00Z
            ---

            # PRD-001: User authentication with OAuth2

            **Status:** Accepted

            ## Problem

            Users cannot log in. The application has no authentication layer.

            ## Requirements

            - FR-001: Users must be able to log in with Google OAuth2
            - FR-002: Sessions must expire after 24 hours
            - FR-003: Failed login attempts must be rate-limited

            ## Acceptance Criteria

            - AC-001: Login page renders with "Sign in with Google" button
            - AC-002: Successful OAuth callback creates a session cookie
            - AC-003: Expired sessions redirect to login page
            """
        )
    )
    # Create empty decision + invariant dirs so compile has valid paths.
    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "architecture" / "invariants").mkdir(parents=True)
    (project / "docs" / "product" / "specs").mkdir(parents=True)
    (project / "docs" / "plans").mkdir(parents=True)
    # CLAUDE.md with edikt sentinel so Claude knows this is an edikt project.
    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Project

            Test project for edikt SDLC chain integration tests.

            [edikt:start]: # managed by edikt — do not edit this block manually
            ## edikt

            ### Project
            A test project for the SDLC chain E2E test.

            ### Build & Test Commands
            No build commands — this is a test fixture.
            [edikt:end]: #
            """
        )
    )
    return project


@pytest.fixture()
def project_with_spec_and_artifacts(tmp_path: Path) -> Path:
    """Project with an accepted spec AND pre-generated artifact files.

    Used by the plan command E2E test — plan reads both spec and artifacts
    to produce a phase-covered plan with criteria sidecar.
    """
    project = tmp_path / "project-spec-artifacts"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent(
            """\
            edikt_version: 0.5.0
            base: docs
            stack: []
            paths:
              decisions: docs/architecture/decisions
              plans: docs/plans
              specs: docs/product/specs
              prds: docs/product/prds
            gates:
              quality-gates: true
            """
        )
    )
    spec_dir = project / "docs" / "product" / "specs" / "SPEC-001-user-auth"
    spec_dir.mkdir(parents=True)
    (spec_dir / "spec.md").write_text(
        textwrap.dedent(
            """\
            ---
            type: spec
            id: SPEC-001
            title: User authentication — OAuth2
            status: accepted
            database_type: postgresql
            source_prd: PRD-001
            ---

            # SPEC-001: User authentication

            ## Components

            1. OAuth2 callback handler — validates token, creates session
            2. Session middleware — validates cookie, expires after 24h
            3. Rate limiter — 5 attempts per 60 seconds per IP

            ## Acceptance Criteria

            - AC-001: POST /auth/google returns 302 on valid token
            - AC-002: Expired session cookie returns 401
            - AC-003: 6th attempt in 60s returns 429
            """
        )
    )
    # Pre-seed artifacts so plan can reference them.
    contracts_dir = spec_dir / "contracts"
    contracts_dir.mkdir()
    (contracts_dir / "api.yaml").write_text(
        textwrap.dedent(
            """\
            openapi: "3.1.0"
            info:
              title: User Auth API
              version: "1.0"
            paths:
              /auth/google:
                post:
                  summary: OAuth2 callback
                  responses:
                    "302": {description: Redirect on success}
                    "401": {description: Invalid token}
              /auth/logout:
                post:
                  summary: Logout
                  responses:
                    "200": {description: Logged out}
            """
        )
    )
    (spec_dir / "data-model.schema.yaml").write_text(
        textwrap.dedent(
            """\
            $schema: "https://json-schema.org/draft/2020-12/schema"
            title: UserSession
            type: object
            properties:
              id: {type: string}
              user_id: {type: string}
              expires_at: {type: string, format: date-time}
            required: [id, user_id, expires_at]
            """
        )
    )
    (spec_dir / "test-strategy.md").write_text(
        textwrap.dedent(
            """\
            # Test Strategy — SPEC-001

            ## Unit tests
            - OAuth token validation logic
            - Session expiry logic
            - Rate limit counter

            ## Integration tests
            - Full OAuth2 callback flow
            - Session middleware with real cookies
            """
        )
    )
    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "plans").mkdir(parents=True)
    (project / "CLAUDE.md").write_text(
        textwrap.dedent(
            """\
            # Project

            [edikt:start]: # managed by edikt
            ## edikt

            ### Project
            Test project for SDLC artifacts + plan E2E tests.

            ### Build & Test Commands
            No build commands — test fixture.
            [edikt:end]: #
            """
        )
    )
    return project


@pytest.fixture()
def project_for_governance_chain(tmp_path: Path) -> Path:
    """Empty project ready for an ADR → compile → governance chain test.

    Has the required directory structure so /edikt:adr:new and
    /edikt:gov:compile can write to the correct locations, but starts with
    no existing ADRs or compiled governance so the test can verify the
    full chain from scratch.
    """
    project = tmp_path / "project-gov"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
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
            features:
              signal-detection: true
            """
        )
    )
    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "architecture" / "invariants").mkdir(parents=True)
    (project / ".claude" / "rules").mkdir(parents=True)
    return project
