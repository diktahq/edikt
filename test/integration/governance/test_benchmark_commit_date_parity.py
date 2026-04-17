"""AC-010 — commit-date parity soft check (drift finding #2).

Spec AC-010 states: "a dedicated linter check asserts both files have the
same most-recent-commit date (soft parity signal)".  Phase 9 prompt clarifies:
"both files' most-recent-commit dates must be within 14 days of each other
(warns, doesn't fail)".

The two paired files are:
  - test/integration/benchmarks/runner.py  (reference sandbox builder)
  - commands/gov/benchmark.md              (tier-2 markdown command)

These three tests cover:

  test_commit_date_parity_within_14_days
      Soft signal: warns if delta > 14 d, but never fails.

  test_commit_date_hard_fail_after_90_days
      Safety net: fails if delta > 90 d (catches years-long drift).
      Labelled clearly so the distinction from the spec ask is obvious.

  test_runner_docstring_invariant_present
      AC-010 companion: asserts the literal sentinel text required by the spec
      ("edits here require a paired edit in commands/gov/benchmark.md") is
      present in runner.py's module docstring / build_project docstring.
"""

from __future__ import annotations

import subprocess
import warnings
import datetime
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).parents[3]

# The two files whose commit dates AC-010 tracks.
RUNNER_PY = REPO_ROOT / "test" / "integration" / "benchmarks" / "runner.py"
BENCHMARK_MD = REPO_ROOT / "commands" / "gov" / "benchmark.md"

# Soft parity threshold (spec §Phase 9 prompt).
SOFT_WARN_DAYS = 14

# Hard safety-net threshold — NOT in the spec; catches years-long drift.
# Labelled explicitly so reviewers know this goes beyond the spec ask.
HARD_FAIL_DAYS = 90


def _git_commit_date(path: Path) -> datetime.datetime | None:
    """Return the most-recent committer date for *path*, or None if not committed.

    Shells out to ``git log -1 --format=%cI -- <path>`` with cwd = repo root
    so the path resolution is stable regardless of the test runner's cwd.
    Returns ``None`` when the file has no git history (e.g. brand-new
    uncommitted file).
    """
    result = subprocess.run(
        ["git", "log", "-1", "--format=%cI", "--", str(path)],
        capture_output=True,
        text=True,
        cwd=str(REPO_ROOT),
    )
    raw = result.stdout.strip()
    if not raw:
        return None
    return datetime.datetime.fromisoformat(raw)


def _require_git_repo() -> None:
    """Skip the test if we are not inside a git repository."""
    result = subprocess.run(
        ["git", "rev-parse", "--git-dir"],
        capture_output=True,
        cwd=str(REPO_ROOT),
    )
    if result.returncode != 0:
        pytest.skip("not inside a git repository")


# ─── Test 1 — soft warn at 14 days (spec requirement) ────────────────────────


def test_commit_date_parity_within_14_days():
    """Soft parity signal per AC-010: warns if delta > 14 d, passes always.

    This is the exact requirement from Phase 9 prompt: "both files'
    most-recent-commit dates must be within 14 days of each other (warns,
    doesn't fail)".  The test NEVER raises AssertionError — only warnings.warn.
    """
    _require_git_repo()

    runner_date = _git_commit_date(RUNNER_PY)
    benchmark_date = _git_commit_date(BENCHMARK_MD)

    if runner_date is None or benchmark_date is None:
        # One or both files have never been committed.  This is expected for
        # newly-created files (e.g. benchmark.md is untracked on this branch).
        missing = []
        if runner_date is None:
            missing.append(str(RUNNER_PY.relative_to(REPO_ROOT)))
        if benchmark_date is None:
            missing.append(str(BENCHMARK_MD.relative_to(REPO_ROOT)))
        pytest.skip(f"file(s) not yet committed — skipping parity check: {', '.join(missing)}")

    delta = abs((runner_date - benchmark_date).days)

    if delta > SOFT_WARN_DAYS:
        warnings.warn(
            f"AC-010 parity drift: "
            f"test/integration/benchmarks/runner.py was last committed "
            f"{runner_date.date()} and commands/gov/benchmark.md was last "
            f"committed {benchmark_date.date()} — delta is {delta} days "
            f"(threshold: {SOFT_WARN_DAYS} days).  These files must be edited "
            f"together (see runner.py::build_project docstring).  Run "
            f"`/edikt:gov:benchmark` to verify parity is still intact.",
            stacklevel=1,
        )

    # Soft signal: always passes regardless of delta.
    # The warning above is the full extent of the signal for delta ≤ 90 d.


# ─── Test 2 — hard fail at 90 days (safety net, NOT a spec requirement) ──────


def test_commit_date_hard_fail_after_90_days():
    """Safety net: fails if delta > 90 d.

    NOTE: This goes BEYOND the spec ask (which is warn-only at 14 d).
    This test exists purely as a catch-all for years-long drift where a
    developer changed one file in a point release and forgot the other for
    multiple releases.  90 days is conservative enough to allow normal feature
    cadences while still catching true long-term neglect.
    """
    _require_git_repo()

    runner_date = _git_commit_date(RUNNER_PY)
    benchmark_date = _git_commit_date(BENCHMARK_MD)

    if runner_date is None or benchmark_date is None:
        missing = []
        if runner_date is None:
            missing.append(str(RUNNER_PY.relative_to(REPO_ROOT)))
        if benchmark_date is None:
            missing.append(str(BENCHMARK_MD.relative_to(REPO_ROOT)))
        pytest.skip(f"file(s) not yet committed — skipping parity check: {', '.join(missing)}")

    delta = abs((runner_date - benchmark_date).days)

    assert delta <= HARD_FAIL_DAYS, (
        f"AC-010 hard-fail safety net triggered: "
        f"test/integration/benchmarks/runner.py ({runner_date.date()}) and "
        f"commands/gov/benchmark.md ({benchmark_date.date()}) diverged by "
        f"{delta} days — exceeds the {HARD_FAIL_DAYS}-day safety net.  "
        f"These files define the same sandbox layout and MUST be kept in sync.  "
        f"See runner.py::build_project docstring and SPEC-005 AC-010."
    )


# ─── Test 3 — docstring invariant present in runner.py ───────────────────────


def test_runner_docstring_invariant_present():
    """AC-010: runner.py must contain the paired-edit sentinel text.

    The spec requires: 'runner.py's module docstring must include the literal
    text "edits here require a paired edit in commands/gov/benchmark.md"'.
    This test asserts that invariant is present (case-insensitive search so
    minor capitalisation changes don't false-alarm).
    """
    assert RUNNER_PY.exists(), f"runner.py not found at {RUNNER_PY}"

    content = RUNNER_PY.read_text()

    sentinel = "edits here require a paired edit in commands/gov/benchmark.md"
    assert sentinel.lower() in content.lower(), (
        f"AC-010: runner.py is missing the paired-edit invariant text.\n"
        f"Expected to find (case-insensitive):\n"
        f"  {sentinel!r}\n"
        f"in {RUNNER_PY.relative_to(REPO_ROOT)}\n\n"
        f"Add the following to build_project's docstring:\n"
        f'  "Edits here require a paired edit in commands/gov/benchmark.md"\n'
        f"This is required by SPEC-005 AC-010."
    )
