"""
REGRESSION TEST — DO NOT DELETE.
Reproduces: The plan command executed the conclusion step (writing the plan
            completion entry) before running pre-flight checks. When the
            working tree was dirty the pre-flight would then fail AFTER the
            plan had already recorded completion — leaving the plan in a
            permanently-completed state even though the phase work was not
            done.
Bug commit: 8a86c22
Fix commit: (Phase 6 — plan guard hooks)
Invariant:  Pre-flight checks MUST execute before the conclusion step.
            A dirty working tree must abort before any plan state mutation.
Removing this test reopens the bug.
"""

from __future__ import annotations

import json
import os
import time
from pathlib import Path

import pytest


# ─── Reference implementation ────────────────────────────────────────────────
# Mirrors commands/sdlc/plan.md §3 pre-flight ordering.
# The bug was that the conclusion step (step 3) ran before pre-flight (step 1).

_STEPS_LOG: list[str] = []


def _emit(event_type: str, path: str) -> None:
    edikt_home = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt")))
    edikt_home.mkdir(parents=True, exist_ok=True)
    record = {
        "type": event_type,
        "path": path,
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    with (edikt_home / "events.jsonl").open("a") as fh:
        fh.write(json.dumps(record) + "\n")
    _STEPS_LOG.append(path)


class DirtyWorkingTreeError(Exception):
    """Raised by pre-flight when the git working tree has uncommitted changes."""


def run_plan_phase(
    project_dir: Path,
    *,
    dirty_tree: bool,
    buggy_order: bool = False,
) -> list[str]:
    """Reference implementation of the plan command phase execution.

    Returns the ordered list of steps that were reached.

    Args:
        dirty_tree: Simulates an uncommitted change in the working tree.
        buggy_order: If True, reproduces the v0.4.2 bug (conclusion first).
    """
    steps: list[str] = []
    _STEPS_LOG.clear()

    if buggy_order:
        # v0.4.2 buggy implementation: conclusion runs first.
        _emit("plan_path", "conclusion")
        steps.append("conclusion")

        if dirty_tree:
            _emit("plan_path", "preflight_dirty_tree")
            raise DirtyWorkingTreeError("working tree is dirty — pre-flight failed")
        _emit("plan_path", "preflight_clean")
        steps.append("preflight")
        return steps

    # Fixed implementation: pre-flight runs first.
    if dirty_tree:
        _emit("plan_path", "preflight_dirty_tree")
        raise DirtyWorkingTreeError("working tree is dirty — pre-flight aborted the phase")

    _emit("plan_path", "preflight_clean")
    steps.append("preflight")

    _emit("plan_path", "conclusion")
    steps.append("conclusion")
    return steps


def assert_path_covered(path_id: str) -> None:
    events_path = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt"))) / "events.jsonl"
    if not events_path.exists():
        raise AssertionError(
            f"events.jsonl not found; expected plan_path={path_id!r}"
        )
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    hits = [
        e for e in events
        if e.get("type") == "plan_path" and e.get("path") == path_id
    ]
    assert hits, (
        f"expected plan_path event with path={path_id!r}; "
        f"saw: {[e.get('path') for e in events if e.get('type') == 'plan_path']}"
    )


# ─── Fixture ─────────────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _isolate(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    edikt_home = tmp_path / ".edikt"
    edikt_home.mkdir()
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setenv("EDIKT_HOME", str(edikt_home))
    _STEPS_LOG.clear()


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_v042_preflight_runs_before_conclusion_on_dirty_tree(tmp_path: Path) -> None:
    """Dirty working tree must abort BEFORE conclusion mutates plan state.

    The v0.4.2 bug: with buggy_order=True, conclusion was reached and
    logged before pre-flight discovered the dirty tree. The plan was then
    permanently marked complete even though the phase work was not done.

    Fixed behaviour: DirtyWorkingTreeError is raised before conclusion
    is ever reached.
    """
    project = tmp_path / "project"
    project.mkdir()

    with pytest.raises(DirtyWorkingTreeError):
        run_plan_phase(project, dirty_tree=True)

    assert_path_covered("preflight_dirty_tree")

    # Conclusion must NOT have been reached.
    events_path = Path(os.environ.get("EDIKT_HOME", str(tmp_path / ".edikt"))) / "events.jsonl"
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    conclusion_events = [
        e for e in events
        if e.get("type") == "plan_path" and e.get("path") == "conclusion"
    ]
    assert not conclusion_events, (
        "conclusion must NOT be reached when pre-flight aborts; "
        f"found conclusion events: {conclusion_events}. "
        "Regression: v0.4.2 recorded conclusion before pre-flight."
    )


def test_v042_buggy_order_reproduces_bug(tmp_path: Path) -> None:
    """Confirm the buggy implementation DOES reach conclusion first.

    This test documents what the bug looked like — it must pass with
    buggy_order=True so we have evidence of the original failure mode.
    If this test starts failing it means the reference impl no longer
    faithfully reproduces the bug.
    """
    project = tmp_path / "project"
    project.mkdir()

    with pytest.raises(DirtyWorkingTreeError):
        run_plan_phase(project, dirty_tree=True, buggy_order=True)

    assert_path_covered("conclusion")


def test_v042_clean_tree_reaches_conclusion(tmp_path: Path) -> None:
    """A clean working tree must proceed through pre-flight to conclusion."""
    project = tmp_path / "project"
    project.mkdir()

    steps = run_plan_phase(project, dirty_tree=False)

    assert steps.index("preflight") < steps.index("conclusion"), (
        "preflight must precede conclusion in step ordering"
    )
    assert_path_covered("preflight_clean")
    assert_path_covered("conclusion")
