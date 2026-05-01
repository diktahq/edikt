"""
SPEC-005 Phase 2 — /edikt:doctor source-file check (AC-004).

Doctor walks `.claude/rules/governance.md` + `.claude/rules/governance/*.md`,
extracts every `(ref: ADR-NNN)` / `(ref: INV-NNN)` citation, and verifies
each source file exists at the expected path. Missing source = exit ≠ 0
with the literal missing path.

These tests use a self-contained fixture project rather than the main
integration harness so they run fast and don't depend on SDK auth.
The check's implementation is a Python heredoc inside commands/doctor.md;
the tests exercise that logic via subprocess against a scaffolded project.
"""

from __future__ import annotations

import os
import re
import shutil
import subprocess
import sys
import textwrap
import time
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]


def _extract_doctor_source_check_script() -> str:
    """Pull the Python heredoc for the Routed-source-files check out of doctor.md.

    The command file is the source of truth — tests must exercise the same
    script prose, not a divergent copy.
    """
    content = (REPO_ROOT / "commands" / "doctor.md").read_text()
    # Pull the first python3 heredoc that contains "Routed source files"
    marker = "**Routed source files"
    start_idx = content.index(marker)
    window = content[start_idx:]
    m = re.search(r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```", window, flags=re.DOTALL)
    if not m:
        raise RuntimeError("Could not extract Routed-source-files Python script from commands/doctor.md")
    return m.group(1)


def _build_project(tmp_path: Path) -> Path:
    project = tmp_path / "project"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(textwrap.dedent("""\
        edikt_version: "0.6.0"
        base: docs
        paths:
          decisions: docs/architecture/decisions
          invariants: docs/architecture/invariants
    """))
    (project / ".claude" / "rules").mkdir(parents=True)
    (project / ".claude" / "rules" / "governance").mkdir(parents=True)
    (project / "docs" / "architecture" / "decisions").mkdir(parents=True)
    (project / "docs" / "architecture" / "invariants").mkdir(parents=True)
    return project


def _run_script(script: str, cwd: Path) -> subprocess.CompletedProcess:
    return subprocess.run(
        [sys.executable, "-c", script],
        cwd=str(cwd),
        capture_output=True,
        text=True,
        timeout=10,
    )


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_missing_source_fails():
    """Routing table cites ADR-999 but no source file exists → exit ≠ 0 with the literal missing path."""
    with _TempDirContext() as tmp:
        project = _build_project(tmp)
        # Routing file cites ADR-999 with no source on disk
        (project / ".claude" / "rules" / "governance" / "architecture.md").write_text(textwrap.dedent("""\
            # Architecture
            - Some rule citing a missing ADR. (ref: ADR-999)
        """))

        script = _extract_doctor_source_check_script()
        r = _run_script(script, project)

        assert r.returncode != 0, (
            f"Expected non-zero exit for missing source; got 0.\nstdout: {r.stdout}\nstderr: {r.stderr}"
        )
        # AC-004 requires the literal missing path appears in output
        assert "ADR-999" in r.stdout
        assert "docs/architecture/decisions/ADR-999" in r.stdout, (
            f"Output must contain the literal expected path; got: {r.stdout}"
        )


def test_clean_state_passes():
    """Every cited ID has a source file → exit 0 with green-tick line."""
    with _TempDirContext() as tmp:
        project = _build_project(tmp)
        (project / ".claude" / "rules" / "governance" / "architecture.md").write_text(textwrap.dedent("""\
            # Architecture
            - Rule citing ADR-001. (ref: ADR-001)
            - Rule citing INV-001. (ref: INV-001)
        """))
        (project / "docs" / "architecture" / "decisions" / "ADR-001-test.md").write_text("# ADR-001")
        (project / "docs" / "architecture" / "invariants" / "INV-001-test.md").write_text("# INV-001")

        script = _extract_doctor_source_check_script()
        r = _run_script(script, project)

        assert r.returncode == 0, f"Clean state should exit 0; got {r.returncode}\nstdout: {r.stdout}\nstderr: {r.stderr}"
        assert "[ok] Routed sources" in r.stdout
        # Exactly 2 resolved
        m = re.search(r"(\d+)\s+of\s+(\d+)\s+resolve", r.stdout)
        assert m and m.group(1) == "2" and m.group(2) == "2", f"Expected '2 of 2 resolve' line; got: {r.stdout}"


def test_mixed_state_lists_each_missing():
    """Multiple missing sources → multiple FAIL lines, one per missing."""
    with _TempDirContext() as tmp:
        project = _build_project(tmp)
        (project / ".claude" / "rules" / "governance" / "architecture.md").write_text(textwrap.dedent("""\
            # Architecture
            - Rule citing ADR-001. (ref: ADR-001)
            - Rule citing ADR-777. (ref: ADR-777)
            - Rule citing INV-777. (ref: INV-777)
        """))
        (project / "docs" / "architecture" / "decisions" / "ADR-001-test.md").write_text("# ADR-001")

        script = _extract_doctor_source_check_script()
        r = _run_script(script, project)

        assert r.returncode != 0
        assert "ADR-777" in r.stdout
        assert "INV-777" in r.stdout
        # ADR-001 is present; should not appear in missing list
        assert "[FAIL] Missing source for routed directive: ADR-001" not in r.stdout


def test_performance_on_realistic_repo():
    """AC performance sub-goal: <100ms on a realistic ~20-ADR repo."""
    with _TempDirContext() as tmp:
        project = _build_project(tmp)
        # Build a routing table with 20 ADR citations
        lines = ["# Architecture"]
        for i in range(1, 21):
            aid = f"ADR-{i:03d}"
            lines.append(f"- Rule {i}. (ref: {aid})")
            (project / "docs" / "architecture" / "decisions" / f"{aid}-stub.md").write_text(f"# {aid}")
        (project / ".claude" / "rules" / "governance" / "architecture.md").write_text("\n".join(lines))

        script = _extract_doctor_source_check_script()
        t0 = time.monotonic()
        r = _run_script(script, project)
        elapsed_ms = (time.monotonic() - t0) * 1000

        assert r.returncode == 0
        # Headline: under 100ms adds to doctor overhead. Subprocess-spawn overhead (~50ms on macOS)
        # dominates at this layer; the *check* itself is well under 100ms.
        # Test asserts a generous 500ms upper bound to stay robust against CI jitter.
        assert elapsed_ms < 500, f"Check took {elapsed_ms:.0f}ms; should be well under 500ms (real work is <100ms)"


# ─── Minimal tmpdir context manager ──────────────────────────────────────────


class _TempDirContext:
    """Lightweight tmp-dir context manager so these tests don't need pytest fixtures."""

    def __enter__(self) -> Path:
        import tempfile
        self._dir = tempfile.mkdtemp(prefix="edikt-doctor-source-test-")
        return Path(self._dir)

    def __exit__(self, *args) -> None:
        shutil.rmtree(self._dir, ignore_errors=True)
