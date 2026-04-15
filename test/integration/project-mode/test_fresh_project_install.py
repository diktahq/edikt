"""Test: install.sh --project --dry-run mentions project-relative paths.

Verifies that dry-run output refers to paths under the project root, not
under $HOME. The install never writes files in dry-run mode, so we inspect
stdout/stderr text only.
"""

from __future__ import annotations

import os
import subprocess
from pathlib import Path

import pytest

from pm_helpers import INSTALL_SH


def test_dry_run_mentions_project_relative_paths(sandbox):
    """install.sh --project --dry-run output contains project-scoped paths."""
    project = sandbox["project"]
    env = {
        **sandbox["env"],
        # Disable network: use a placeholder tag so the script prints the plan
        # before attempting any real fetch. We override EDIKT_RELEASE_TAG so
        # the tag-resolution function returns immediately.
        "EDIKT_RELEASE_TAG": "v0.5.0",
        # No launcher source — dry-run should still print the would-run lines
        # without actually fetching.
        "EDIKT_LAUNCHER_SOURCE": str(INSTALL_SH.parent / "bin" / "edikt"),
        "EDIKT_INSTALL_INSECURE": "1",
    }

    proc = subprocess.run(
        ["bash", str(INSTALL_SH), "--project", "--dry-run"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )

    output = proc.stdout + proc.stderr

    project_edikt = str(project / ".edikt")
    project_claude = str(project / ".claude")
    home_edikt = str(sandbox["edikt_home"])

    # EDIKT_ROOT in the banner must point to the project, not $HOME/.edikt
    assert project_edikt in output, (
        f"Expected project .edikt path '{project_edikt}' in dry-run output.\n"
        f"Output was:\n{output}"
    )

    # The dry-run "would-run" lines reference the project launcher path
    assert f"{project_edikt}/bin/edikt" in output, (
        f"Expected project launcher path in dry-run output.\n"
        f"Output was:\n{output}"
    )

    # settings.json would be written to .claude/ under the project, not $HOME
    assert project_claude in output, (
        f"Expected project .claude path '{project_claude}' in dry-run output "
        f"(settings.json must be project-relative, not $HOME-relative).\n"
        f"Output was:\n{output}"
    )

    # The home-level .edikt must NOT appear as the install target
    # (it may appear in warnings about coexistence, but not as EDIKT_ROOT)
    assert f"EDIKT_ROOT = {project_edikt}" in output, (
        f"Expected 'EDIKT_ROOT = {project_edikt}' in output.\n"
        f"Output was:\n{output}"
    )


def test_dry_run_does_not_create_files(sandbox):
    """install.sh --project --dry-run creates no files in the project dir."""
    project = sandbox["project"]
    env = {
        **sandbox["env"],
        "EDIKT_RELEASE_TAG": "v0.5.0",
        "EDIKT_LAUNCHER_SOURCE": str(INSTALL_SH.parent / "bin" / "edikt"),
        "EDIKT_INSTALL_INSECURE": "1",
    }

    before = set(project.rglob("*"))

    subprocess.run(
        ["bash", str(INSTALL_SH), "--project", "--dry-run"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )

    after = set(project.rglob("*"))
    new_files = after - before
    assert new_files == set(), (
        f"Dry-run created unexpected files: {new_files}"
    )
