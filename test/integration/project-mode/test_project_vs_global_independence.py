"""Test: resolve_edikt_root prefers project .edikt/ over global ~/.edikt/.

Verifies that when both a project .edikt/bin/edikt and a global ~/.edikt/bin/edikt
exist, the launcher running inside the project directory resolves to the project
root, not the global root.

Uses subprocess to invoke a shell snippet that sources the relevant logic from
bin/edikt (by running `edikt doctor` and checking which EDIKT_ROOT it reports).
"""

from __future__ import annotations

import os
import shutil
import subprocess
from pathlib import Path

import pytest

from pm_helpers import (
    EDIKT_BIN,
    PAYLOAD_VERSION,
    make_versioned_edikt,
)


def test_project_edikt_takes_precedence_over_global(sandbox):
    """When inside a project that has .edikt/bin/edikt, it wins over global."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"
    global_edikt = sandbox["edikt_home"]

    # Plant BOTH a project and a global versioned .edikt/
    make_versioned_edikt(project_edikt, PAYLOAD_VERSION)
    make_versioned_edikt(global_edikt, PAYLOAD_VERSION)

    env = {
        **sandbox["env"],
        # No EDIKT_ROOT override — let resolve_edikt_root walk ancestors
    }

    proc = subprocess.run(
        [str(project_edikt / "bin" / "edikt"), "doctor", "--quick"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )

    output = proc.stdout + proc.stderr
    # doctor prints "EDIKT_ROOT: <path>" — confirm it resolved to the project
    project_edikt_str = str(project_edikt)
    global_edikt_str = str(global_edikt)

    assert project_edikt_str in output, (
        f"Expected project EDIKT_ROOT '{project_edikt_str}' in doctor output.\n"
        f"Output:\n{output}"
    )
    # The doctor line with EDIKT_ROOT must NOT mention the global path
    for line in output.splitlines():
        if "EDIKT_ROOT:" in line:
            assert global_edikt_str not in line, (
                f"EDIKT_ROOT line incorrectly references global path.\n"
                f"Line: {line}"
            )
            break


def test_global_edikt_used_outside_project(sandbox):
    """When NOT inside a project, the global ~/.edikt/ is used."""
    home = sandbox["home"]
    global_edikt = sandbox["edikt_home"]

    # Plant only the global edikt — no project directory with .edikt/bin/edikt
    make_versioned_edikt(global_edikt, PAYLOAD_VERSION)

    env = {**sandbox["env"]}

    proc = subprocess.run(
        [str(EDIKT_BIN), "doctor", "--quick"],
        # Run from $HOME — there's no project .edikt/ here
        cwd=str(home),
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )

    output = proc.stdout + proc.stderr
    global_edikt_str = str(global_edikt)

    assert global_edikt_str in output, (
        f"Expected global EDIKT_ROOT '{global_edikt_str}' in doctor output "
        f"when running outside a project.\nOutput:\n{output}"
    )


def test_edikt_root_env_overrides_project_walk(sandbox):
    """EDIKT_ROOT env var takes priority over ancestor walk."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"
    custom_root = sandbox["home"] / "custom-edikt"

    # Plant a project .edikt/ with bin/edikt
    make_versioned_edikt(project_edikt, PAYLOAD_VERSION)
    # Plant a custom root
    make_versioned_edikt(custom_root, PAYLOAD_VERSION)

    env = {
        **sandbox["env"],
        "EDIKT_ROOT": str(custom_root),
    }

    proc = subprocess.run(
        [str(EDIKT_BIN), "doctor", "--quick"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=30,
    )

    output = proc.stdout + proc.stderr
    custom_str = str(custom_root)
    project_str = str(project_edikt)

    assert custom_str in output, (
        f"Expected custom EDIKT_ROOT '{custom_str}' in doctor output.\n"
        f"Output:\n{output}"
    )
    for line in output.splitlines():
        if "EDIKT_ROOT:" in line:
            assert project_str not in line, (
                f"EDIKT_ROOT env override was ignored — doctor used project path.\n"
                f"Line: {line}"
            )
            break
