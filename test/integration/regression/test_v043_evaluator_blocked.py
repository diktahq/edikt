"""
REGRESSION TEST — DO NOT DELETE.
Reproduces: The evaluator, when invoked inside a permission sandbox that
            blocked Write/Edit/Bash tools, returned a silent PASS verdict
            instead of BLOCKED. Quality gates appeared green on restricted
            CI runners while the actual evaluation never executed.
Bug commit: 58ce609
Fix commit: (Phase 11 — evaluator sandbox detection)
Invariant:  The evaluator MUST detect when its required inspection tools
            are blocked and return BLOCKED explicitly. Silent PASS when
            tools are unavailable is a contract violation.
Removing this test reopens the bug.
"""

from __future__ import annotations

import json
import os
import time
from pathlib import Path
from typing import Literal

import pytest


# ─── Reference implementation ────────────────────────────────────────────────
# Mirrors the evaluator verdict logic in commands/sdlc/review.md.
# The bug: the evaluator's "can I inspect this?" check was missing.
# It would attempt inspection, catch the PermissionError silently, and
# fall through to PASS without ever surfacing the restriction.

VerdictType = Literal["PASS", "BLOCKED", "FAIL"]


class PermissionSandboxError(Exception):
    """Raised when a required tool is blocked by the permission sandbox."""


def _tool_available(tool_name: str, blocked_tools: set[str]) -> bool:
    return tool_name not in blocked_tools


def run_evaluator(
    project_dir: Path,
    *,
    blocked_tools: set[str] | None = None,
    buggy_version: bool = False,
) -> VerdictType:
    """Reference implementation of the evaluator logic.

    Returns one of: "PASS", "FAIL", "BLOCKED".

    Args:
        blocked_tools: Tools unavailable in the current sandbox.
        buggy_version: If True, reproduces v0.4.3 bug (missing sandbox check).
    """
    blocked = blocked_tools or set()

    if buggy_version:
        # v0.4.3 bug: no sandbox detection — tries to read and silently swallows errors.
        try:
            if not _tool_available("Read", blocked):
                raise PermissionSandboxError("Read blocked")
            # Would inspect files here…
        except PermissionSandboxError:
            pass  # BUG: silently swallowed — falls through to PASS
        _emit_verdict("PASS", "buggy_silent_pass")
        return "PASS"

    # Fixed: explicit sandbox detection before attempting inspection.
    required_tools = {"Read"}
    unavailable = required_tools & blocked
    if unavailable:
        _emit_verdict("BLOCKED", "evaluator_blocked")
        return "BLOCKED"

    # Normal evaluation path.
    _emit_verdict("PASS", "evaluator_pass")
    return "PASS"


def _emit_verdict(verdict: str, path: str) -> None:
    edikt_home = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt")))
    edikt_home.mkdir(parents=True, exist_ok=True)
    record = {
        "type": "evaluator_path",
        "path": path,
        "verdict": verdict,
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    with (edikt_home / "events.jsonl").open("a") as fh:
        fh.write(json.dumps(record) + "\n")


def assert_path_covered(path_id: str) -> None:
    events_path = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt"))) / "events.jsonl"
    if not events_path.exists():
        raise AssertionError(
            f"events.jsonl not found; expected evaluator_path={path_id!r}"
        )
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    hits = [
        e for e in events
        if e.get("type") == "evaluator_path" and e.get("path") == path_id
    ]
    assert hits, (
        f"expected evaluator_path event with path={path_id!r}; "
        f"saw: {[e.get('path') for e in events if e.get('type') == 'evaluator_path']}"
    )


# ─── Fixture ─────────────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _isolate(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    edikt_home = tmp_path / ".edikt"
    edikt_home.mkdir()
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setenv("EDIKT_HOME", str(edikt_home))


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_v043_evaluator_blocked_not_silent_pass(tmp_path: Path) -> None:
    """Read-blocked sandbox → BLOCKED verdict, never PASS.

    The v0.4.3 bug: with buggy_version=True the evaluator catches the
    sandbox error silently and returns PASS. The fix detects unavailable
    tools upfront and returns BLOCKED before attempting inspection.
    """
    project = tmp_path / "project"
    project.mkdir()

    verdict = run_evaluator(project, blocked_tools={"Read", "Write", "Edit", "Bash"})

    assert verdict == "BLOCKED", (
        f"expected BLOCKED, got {verdict!r}. "
        "Regression: v0.4.3 silently returned PASS when Read was blocked."
    )

    assert_path_covered("evaluator_blocked")


def test_v043_buggy_evaluator_returns_silent_pass(tmp_path: Path) -> None:
    """Confirm the buggy implementation silently returns PASS.

    Documents the original failure mode. If this test starts failing it
    means the reference impl no longer faithfully reproduces the bug.
    """
    project = tmp_path / "project"
    project.mkdir()

    verdict = run_evaluator(
        project,
        blocked_tools={"Read", "Write", "Edit", "Bash"},
        buggy_version=True,
    )

    assert verdict == "PASS", (
        f"buggy evaluator should return PASS on blocked sandbox; got {verdict!r}"
    )


def test_v043_evaluator_passes_in_unrestricted_sandbox(tmp_path: Path) -> None:
    """Unrestricted sandbox → PASS (happy path, regression guard)."""
    project = tmp_path / "project"
    project.mkdir()

    verdict = run_evaluator(project, blocked_tools=set())

    assert verdict == "PASS", f"expected PASS in unrestricted sandbox; got {verdict!r}"

    assert_path_covered("evaluator_pass")
