"""Test: edikt migrate --dry-run inside a project operates only on the project .edikt/.

Verifies that:
1. migrate --dry-run output mentions the project path, not $HOME/.edikt/
2. The global ~/.edikt/ is completely untouched after migrate runs on a project
3. A real migrate run on the project doesn't touch global state
"""

from __future__ import annotations

import os
import subprocess
import textwrap
from pathlib import Path

import pytest

from pm_helpers import (
    EDIKT_BIN,
    PAYLOAD_VERSION,
    make_legacy_v043_edikt,
    make_versioned_edikt,
    run_launcher,
)


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def test_project_migrate_dry_run_mentions_project_path(sandbox):
    """migrate --dry-run from inside a project references the project .edikt/ path."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"

    # Set up project with a legacy (flat) v0.4.3 layout so M1 fires
    make_legacy_v043_edikt(project_edikt)

    env = {
        **sandbox["env"],
        # Point EDIKT_ROOT at the project's .edikt/ explicitly
        "EDIKT_ROOT": str(project_edikt),
    }

    proc = run_launcher(project, env, "migrate", "--dry-run")
    output = proc.stdout + proc.stderr

    project_edikt_str = str(project_edikt)
    home_edikt_str = str(sandbox["edikt_home"])

    assert project_edikt_str in output, (
        f"Expected project .edikt path '{project_edikt_str}' in migrate --dry-run output.\n"
        f"Output:\n{output}"
    )
    # The plan lines should not reference the home-level edikt
    # (A note about $HOME might appear in environment lines, but the
    # migration target directories should all be under the project.)
    migration_target_lines = [
        l for l in output.splitlines()
        if "→" in l or "Will move" in l or "Will create" in l or "versions/" in l
    ]
    for line in migration_target_lines:
        assert home_edikt_str not in line, (
            f"Migration target line references global ~/.edikt/:\n  {line}\n"
            f"Full output:\n{output}"
        )


def test_global_edikt_untouched_after_project_migrate(sandbox):
    """Running migrate on a project leaves global ~/.edikt/ completely unchanged."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"
    global_edikt = sandbox["edikt_home"]

    # Set up a versioned global install
    make_versioned_edikt(global_edikt, PAYLOAD_VERSION)

    # Record global state snapshot before migration
    global_snapshot_before = {
        p.relative_to(global_edikt): p.read_text() if p.is_file() else None
        for p in global_edikt.rglob("*")
        if not p.is_symlink()
    }

    # Set up project with a legacy layout so M1 fires
    make_legacy_v043_edikt(project_edikt)

    env = {
        **sandbox["env"],
        "EDIKT_ROOT": str(project_edikt),
        "CLAUDE_HOME": str(project / ".claude"),
    }

    # Run the actual migration (with --yes) on the project
    proc = run_launcher(project, env, "migrate", "--yes")
    # Don't assert returncode — migration may succeed or fail for various reasons
    # in the test sandbox; what matters is global state is unchanged.

    # Snapshot global state after
    global_snapshot_after = {
        p.relative_to(global_edikt): p.read_text() if p.is_file() else None
        for p in global_edikt.rglob("*")
        if not p.is_symlink()
    }

    assert global_snapshot_before == global_snapshot_after, (
        "Global ~/.edikt/ was modified by a project-scoped migrate!\n"
        f"Before: {set(global_snapshot_before.keys())}\n"
        f"After:  {set(global_snapshot_after.keys())}"
    )


def test_project_edikt_list_shows_project_versions(sandbox):
    """edikt list inside a project dir shows project versions, not global ones."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"
    global_edikt = sandbox["edikt_home"]

    # Install different versions in global vs project
    make_versioned_edikt(global_edikt, "0.5.0")
    make_versioned_edikt(project_edikt, "0.5.0")
    # Add a second version only in the project
    proj_extra = project_edikt / "versions" / "0.5.1"
    (proj_extra / "hooks").mkdir(parents=True, exist_ok=True)
    (proj_extra / "templates").mkdir(parents=True, exist_ok=True)
    _write(proj_extra / "VERSION", "0.5.1\n")

    env = {
        **sandbox["env"],
        "EDIKT_ROOT": str(project_edikt),
    }

    proc = run_launcher(project, env, "list")
    assert proc.returncode == 0, proc.stderr
    output = proc.stdout

    # Project has 0.5.0 and 0.5.1; both should appear
    assert "0.5.0" in output, f"Expected 0.5.0 in list output: {output}"
    assert "0.5.1" in output, f"Expected 0.5.1 in list output: {output}"


def test_edikt_list_global_flag_uses_home_edikt(sandbox):
    """edikt list --global shows global versions even from inside a project."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"
    global_edikt = sandbox["edikt_home"]

    # Global has 0.5.0 only; project has 0.5.0 + 0.5.1
    make_versioned_edikt(global_edikt, "0.5.0")
    make_versioned_edikt(project_edikt, "0.5.0")
    proj_extra = project_edikt / "versions" / "0.5.1"
    (proj_extra / "hooks").mkdir(parents=True, exist_ok=True)
    (proj_extra / "templates").mkdir(parents=True, exist_ok=True)
    _write(proj_extra / "VERSION", "0.5.1\n")

    env = {
        **sandbox["env"],
        "EDIKT_ROOT": str(project_edikt),
        # Pass EDIKT_HOME so --global knows where to look
        "EDIKT_HOME": str(global_edikt),
    }

    proc = run_launcher(project, env, "list", "--global")
    assert proc.returncode == 0, proc.stderr
    output = proc.stdout

    # --global should list from global_edikt which only has 0.5.0
    assert "0.5.0" in output, f"Expected 0.5.0 in --global list output: {output}"
    # 0.5.1 is only in project — must not appear in --global output
    assert "0.5.1" not in output, (
        f"0.5.1 is only in project .edikt/, should not appear in --global list.\n"
        f"Output: {output}"
    )
