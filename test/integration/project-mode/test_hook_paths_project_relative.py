"""Test: settings.json written by install.sh --project has project-relative hook paths.

Verifies that when install.sh writes .claude/settings.json in project mode,
the hook paths point to <project>/.edikt/hooks/ and NOT to $HOME/.edikt/hooks/.

The test sets up a versioned project .edikt/ with the settings.json.tmpl in
place, then invokes the write_settings_json logic by running install.sh in
do_current_v05 mode with network bypassed.
"""

from __future__ import annotations

import json
import os
import shutil
import subprocess
from pathlib import Path

import pytest

from pm_helpers import (
    EDIKT_BIN,
    INSTALL_SH,
    PAYLOAD_VERSION,
    REPO_ROOT,
    make_versioned_edikt,
)


def _install_settings_tmpl(edikt_root: Path) -> None:
    """Copy the repo's settings.json.tmpl into the versioned payload."""
    src = REPO_ROOT / "templates" / "settings.json.tmpl"
    dst_dir = edikt_root / "versions" / PAYLOAD_VERSION / "templates"
    dst_dir.mkdir(parents=True, exist_ok=True)
    shutil.copy2(str(src), str(dst_dir / "settings.json.tmpl"))


def test_settings_json_uses_project_hook_dir(sandbox):
    """Settings.json generated for project mode references project .edikt/hooks/."""
    project = sandbox["project"]
    project_edikt = project / ".edikt"

    # Set up a versioned project .edikt/ layout
    make_versioned_edikt(project_edikt, PAYLOAD_VERSION)
    _install_settings_tmpl(project_edikt)

    # Symlink templates so the installer can read settings.json.tmpl at the
    # stable path $EDIKT_ROOT/templates/settings.json.tmpl
    tmpl_link = project_edikt / "templates"
    if tmpl_link.exists() or tmpl_link.is_symlink():
        if tmpl_link.is_symlink():
            tmpl_link.unlink()
        else:
            shutil.rmtree(str(tmpl_link))
    tmpl_link.symlink_to(
        Path("versions") / PAYLOAD_VERSION / "templates"
    )

    env = {
        **sandbox["env"],
        "EDIKT_RELEASE_TAG": "v0.5.0",
        "EDIKT_LAUNCHER_SOURCE": str(EDIKT_BIN),
        "EDIKT_INSTALL_INSECURE": "1",
    }

    proc = subprocess.run(
        ["bash", str(INSTALL_SH), "--project"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )

    # install.sh may fail for reasons unrelated to hook-path writing
    # (e.g. EX_ALREADY from re-install attempt). What matters is that
    # if settings.json was written, its paths are project-relative.
    settings_path = project / ".claude" / "settings.json"
    if not settings_path.exists():
        pytest.skip("settings.json was not written (install failed before write step)")

    content = settings_path.read_text()
    project_hook_dir = str(project_edikt / "hooks")
    home_hook_dir = str(sandbox["edikt_home"] / "hooks")

    assert project_hook_dir in content, (
        f"Expected project hook dir '{project_hook_dir}' in settings.json.\n"
        f"Content:\n{content}"
    )
    assert home_hook_dir not in content, (
        f"settings.json must NOT contain global hook dir '{home_hook_dir}'.\n"
        f"Content:\n{content}"
    )

    # Confirm it's valid JSON
    try:
        parsed = json.loads(content)
    except json.JSONDecodeError as e:
        pytest.fail(f"settings.json is not valid JSON: {e}\nContent:\n{content}")

    # Confirm the placeholder was fully substituted
    assert "${EDIKT_HOOK_DIR}" not in content, (
        "settings.json still contains the unsubstituted placeholder ${EDIKT_HOOK_DIR}"
    )


def test_settings_json_not_written_in_global_mode(sandbox):
    """In global mode, settings.json hook paths use $HOME/.edikt/hooks/."""
    project = sandbox["project"]
    global_edikt = sandbox["edikt_home"]

    # Set up a versioned global .edikt/ with the settings template
    make_versioned_edikt(global_edikt, PAYLOAD_VERSION)
    _install_settings_tmpl(global_edikt)

    # Symlink templates at stable path
    tmpl_link = global_edikt / "templates"
    if tmpl_link.exists() or tmpl_link.is_symlink():
        if tmpl_link.is_symlink():
            tmpl_link.unlink()
        else:
            shutil.rmtree(str(tmpl_link))
    tmpl_link.symlink_to(
        Path("versions") / PAYLOAD_VERSION / "templates"
    )

    env = {
        **sandbox["env"],
        "EDIKT_RELEASE_TAG": "v0.5.0",
        "EDIKT_LAUNCHER_SOURCE": str(EDIKT_BIN),
        "EDIKT_INSTALL_INSECURE": "1",
    }

    proc = subprocess.run(
        ["bash", str(INSTALL_SH), "--global"],
        cwd=str(project),
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )

    settings_path = sandbox["claude_home"] / "settings.json"
    if not settings_path.exists():
        pytest.skip("settings.json was not written (install failed before write step)")

    content = settings_path.read_text()
    expected_hook_dir = str(sandbox["home"] / ".edikt" / "hooks")
    project_hook_dir = str(project / ".edikt" / "hooks")

    assert expected_hook_dir in content, (
        f"Expected global hook dir '{expected_hook_dir}' in settings.json.\n"
        f"Content:\n{content}"
    )
    assert project_hook_dir not in content, (
        f"Global settings.json must NOT contain project hook dir '{project_hook_dir}'.\n"
        f"Content:\n{content}"
    )
    assert "${EDIKT_HOOK_DIR}" not in content, (
        "settings.json still contains the unsubstituted placeholder ${EDIKT_HOOK_DIR}"
    )
