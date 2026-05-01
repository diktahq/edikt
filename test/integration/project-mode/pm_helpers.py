"""Shared fixtures for project-mode install parity tests (Phase 8).

Tests verify that:
- install.sh --project scopes everything under <project-root>/.edikt/
- settings.json hook paths are project-relative, not $HOME-relative
- resolve_edikt_root in bin/edikt prefers project .edikt/ over global ~/.edikt/
- edikt migrate --dry-run inside a project operates only on the project .edikt/

Sandbox model:
- Each test gets an isolated tmp_path.
- A synthetic "home" and "project" directory are created inside it.
- HOME, EDIKT_HOME, CLAUDE_HOME are redirected into the sandbox.
- Tests invoke bin/edikt and install.sh via subprocess.run — no monkey-patching.
"""

from __future__ import annotations

import os
import subprocess
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
EDIKT_BIN = REPO_ROOT / "bin" / "edikt"
INSTALL_SH = REPO_ROOT / "install.sh"
PAYLOAD_VERSION = "0.5.0"


# ─── Helpers ────────────────────────────────────────────────────────────────


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


# ─── Fixtures ───────────────────────────────────────────────────────────────


@pytest.fixture
def project_dir(tmp_path: Path) -> Path:
    """A scratch directory simulating a project repo."""
    d = tmp_path / "my-project"
    d.mkdir(parents=True)
    return d


@pytest.fixture
def global_edikt_dir(tmp_path: Path) -> Path:
    """A scratch directory simulating global ~/.edikt."""
    d = tmp_path / "global-edikt"
    d.mkdir(parents=True)
    return d


@pytest.fixture
def sandbox(tmp_path: Path):
    """Combined sandbox: isolated home + project dir.

    Yields a dict with:
      home        — synthetic $HOME
      edikt_home  — synthetic $HOME/.edikt  (global install location)
      claude_home — synthetic $HOME/.claude (global Claude config)
      project     — synthetic project directory
      env         — os.environ copy with HOME/EDIKT_HOME/CLAUDE_HOME redirected
                    and EDIKT_ROOT cleared so resolution falls through naturally
    """
    home = tmp_path / "home"
    edikt_home = home / ".edikt"
    claude_home = home / ".claude"
    project = tmp_path / "my-project"

    home.mkdir(parents=True)
    edikt_home.mkdir()
    claude_home.mkdir()
    project.mkdir()

    env = {
        **os.environ,
        "HOME": str(home),
        "EDIKT_HOME": str(edikt_home),
        "CLAUDE_HOME": str(claude_home),
    }
    # Remove EDIKT_ROOT so the launcher's resolve_edikt_root falls through to
    # the ancestor walk and then EDIKT_HOME, not any inherited value.
    env.pop("EDIKT_ROOT", None)

    return {
        "home": home,
        "edikt_home": edikt_home,
        "claude_home": claude_home,
        "project": project,
        "env": env,
    }


# ─── Synthetic layout helpers ─────────────────────────────────────────────────


def make_versioned_edikt(root: Path, version: str = PAYLOAD_VERSION) -> None:
    """Plant a minimal v0.5.0-style versioned layout at root/.

    Creates:
        root/
          bin/edikt           (copy of REPO_ROOT/bin/edikt)
          versions/<version>/ (minimal payload with hooks/, templates/)
          current -> versions/<version>
          hooks -> current/hooks
          templates -> current/templates
          lock.yaml
    """
    payload = root / "versions" / version
    (payload / "hooks").mkdir(parents=True, exist_ok=True)
    (payload / "templates").mkdir(parents=True, exist_ok=True)
    _write(payload / "VERSION", version + "\n")

    # current symlink
    cur = root / "current"
    if cur.exists() or cur.is_symlink():
        cur.unlink()
    cur.symlink_to(Path("versions") / version)

    # external symlinks
    for name in ("hooks", "templates"):
        link = root / name
        if link.exists() or link.is_symlink():
            link.unlink() if link.is_symlink() else link.rmdir()
        link.symlink_to(Path("current") / name)

    # lock.yaml
    _write(root / "lock.yaml", f'active: "{version}"\ninstalled_via: "fixture"\n')

    # bin/edikt (copy from repo for project-mode auto-detect)
    bin_dir = root / "bin"
    bin_dir.mkdir(parents=True, exist_ok=True)
    import shutil
    shutil.copy2(str(EDIKT_BIN), str(bin_dir / "edikt"))
    (bin_dir / "edikt").chmod(0o755)


def make_legacy_v043_edikt(root: Path) -> None:
    """Plant a flat (pre-v0.5.0) v0.4.3 layout at root/.

    Matches the M1-detection signal: hooks/ is a real directory + VERSION exists.
    """
    (root / "hooks").mkdir(parents=True, exist_ok=True)
    (root / "templates").mkdir(parents=True, exist_ok=True)
    _write(root / "VERSION", "0.4.3\n")
    # config.yaml with all v0.5.0 keys present (M5 is a no-op)
    _write(
        root / "config.yaml",
        textwrap.dedent(
            """\
            edikt_version: 0.4.3
            stack: []
            paths:
              decisions: docs/architecture/decisions
            gates:
              quality-gates: true
            """
        ),
    )


def run_launcher(cwd: Path, env: dict, *args: str) -> subprocess.CompletedProcess:
    """Run bin/edikt with the given args from cwd."""
    cmd = [str(EDIKT_BIN), *args]
    return subprocess.run(
        cmd,
        cwd=str(cwd),
        env=env,
        capture_output=True,
        text=True,
        timeout=60,
    )
