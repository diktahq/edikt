"""Pytest fixtures for the multi-version migration suite.

Phase 7a builds **synthetic** source layouts inline. Each `synth_v*`
helper lays out a directory tree that carries the detection signals
the launcher's M2/M3/M5/M4 functions look for, but the bytes are NOT
captured from a real historical install. Phase 7b will replace these
with byte-for-byte fixtures produced by `capture.sh`.

Conventions:

* Every fixture creates an isolated `$HOME` under a per-test tmp dir.
* `$EDIKT_HOME` and `$CLAUDE_HOME` are exported alongside `$HOME` so the
  launcher's `resolve_edikt_root` / `resolve_claude_root` resolution
  paths land inside the sandbox.
* Tests invoke `bin/edikt migrate --yes` via `subprocess.run` — no
  monkey-patching of the launcher.

Synthetic fixture matrix:

| version | VERSION | flat hooks/ | flat commands     | CLAUDE.md HTML | config keys missing | gov v2 sentinel |
|---------|---------|-------------|-------------------|----------------|---------------------|-----------------|
| 0.1.0   | 0.1.0   | yes         | plan,context,init | yes            | paths,stack,gates   | n/a (no gov)    |
| 0.1.4   | 0.1.4   | yes         | plan,context,init | yes            | paths,stack,gates   | n/a             |
| 0.2.0   | 0.2.0   | yes         | (namespaced)      | no             | gates only          | absent          |
| 0.3.0   | 0.3.0   | yes         | (namespaced)      | no             | gates only          | absent          |
| 0.4.3   | 0.4.3   | no          | (namespaced)      | no             | (none)              | absent          |

Detection consequences (which Mn fires per version):

| version | M1 | M2 | M3 | M5 | M4 |
|---------|----|----|----|----|----|
| 0.1.0   | ✓  | ✓  | ✓  | ✓  | -  |
| 0.1.4   | ✓  | ✓  | ✓  | ✓  | -  |
| 0.2.0   | ✓  | -  | -  | ✓  | ✓  |
| 0.3.0   | ✓  | -  | -  | ✓  | ✓  |
| 0.4.3   | -  | -  | -  | -  | ✓  |
"""

from __future__ import annotations

import os
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
EDIKT_BIN = REPO_ROOT / "bin" / "edikt"
PAYLOAD_VERSION = "0.5.0"


# ─── Helpers ────────────────────────────────────────────────────────────────


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def _make_payload(versions_root: Path, version: str = PAYLOAD_VERSION) -> Path:
    """Create a minimal v0.5.0 payload tree under versions/<version>/.

    Includes namespaced commands (sdlc/plan.md, etc.) so M3 can find
    same-basename replacements when the source had flat names.
    """
    payload = versions_root / version
    (payload / "hooks").mkdir(parents=True, exist_ok=True)
    (payload / "templates").mkdir(parents=True, exist_ok=True)
    cmds = payload / "commands" / "edikt"
    cmds.mkdir(parents=True, exist_ok=True)
    # Top-level (these are intentionally flat — they don't have namespaced
    # twins, so M3 must NOT touch them).
    _write(cmds / "context.md", "# context (v0.5.0)\n")
    _write(cmds / "init.md", "# init (v0.5.0)\n")
    # Namespaced versions of formerly-flat commands.
    _write(cmds / "sdlc" / "plan.md", "# sdlc/plan (v0.5.0)\n")
    # VERSION + manifest sentinel.
    _write(payload / "VERSION", version + "\n")
    return payload


# ─── Sandbox fixture ────────────────────────────────────────────────────────


@pytest.fixture
def sandbox_home(tmp_path: Path):
    """Yields a dict with HOME / EDIKT_HOME / CLAUDE_HOME paths.

    Sets up the env mapping but does NOT mutate os.environ — tests pass
    the env explicitly to subprocess.run so concurrent tests stay
    isolated.
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
        # Strip any inherited EDIKT_ROOT so resolution falls through to EDIKT_HOME.
        "EDIKT_ROOT": "",
    }
    # The launcher reads EDIKT_ROOT only if non-empty; remove the empty key
    # so resolve_edikt_root falls through to EDIKT_HOME.
    env.pop("EDIKT_ROOT", None)
    return {
        "home": home,
        "edikt_home": edikt_home,
        "claude_home": claude_home,
        "env": env,
    }


# ─── Synthetic fixture builders ─────────────────────────────────────────────


def _post_m1_layout(edikt_home: Path, source_version: str, *,
                    with_html_sentinels: bool,
                    missing_keys: tuple[str, ...]) -> None:
    """Lay down the **post-M1** state of a pre-v0.5.0 install.

    Phase 7a tests M2/M3/M5/M4 in isolation. The synthetic fixture
    represents the on-disk shape AFTER M1 has run (or AFTER install.sh
    cross-upgraded a flat install to the versioned layout). The
    `source_version` is preserved in `config.yaml` so M5's detection
    sees it; the active payload at `current` is always v0.5.0.

    Layout produced:
        edikt_home/
          versions/0.5.0/       # active payload (namespaced commands)
          current -> versions/0.5.0
          hooks -> current/hooks
          templates -> current/templates
          CLAUDE.md             # with HTML sentinels iff requested
          config.yaml           # selectively missing v0.5.0 keys
          lock.yaml             # active=0.5.0
    """
    _versioned_seed(edikt_home)

    # CLAUDE.md with HTML sentinels (M2 detection).
    if with_html_sentinels:
        _write(
            edikt_home / "CLAUDE.md",
            textwrap.dedent(
                """\
                # Project CLAUDE.md

                User-authored prose above the managed block.

                <!-- edikt:start -->
                ## edikt
                managed content goes here — must be preserved byte-for-byte
                <!-- edikt:end -->

                Trailing user content.
                """
            ),
        )

    # config.yaml with selectively-missing keys (M5 detection).
    cfg_lines = [
        "edikt_version: " + source_version,
        "base: docs",
    ]
    if "paths" not in missing_keys:
        cfg_lines += [
            "paths:",
            "  decisions: docs/architecture/decisions",
        ]
    if "stack" not in missing_keys:
        cfg_lines += ["stack: [go]"]
    if "gates" not in missing_keys:
        cfg_lines += ["gates:", "  quality-gates: true"]
    _write(edikt_home / "config.yaml", "\n".join(cfg_lines) + "\n")


def _flat_commands(claude_home: Path, names: tuple[str, ...]) -> None:
    """Drop top-level command .md files (M3 detection)."""
    cmd_dir = claude_home / "commands" / "edikt"
    cmd_dir.mkdir(parents=True, exist_ok=True)
    for n in names:
        # Match the v0.5.0 namespaced version's bytes so M3 treats it as
        # unmodified (deletes rather than preserves). Tests can override.
        _write(cmd_dir / f"{n}.md", f"# sdlc/{n} (v0.5.0)\n")


def _governance_v1(claude_home: Path) -> None:
    """Drop a v1-schema governance.md (M4 detection: lacks v2 sentinel)."""
    rules_dir = claude_home / "rules"
    rules_dir.mkdir(parents=True, exist_ok=True)
    _write(rules_dir / "governance.md", "# Governance (v1 schema)\n\n- rule\n")


def _versioned_seed(edikt_home: Path) -> None:
    """For v0.4.3 synthetic: already on the versioned layout (no M1)."""
    versions = edikt_home / "versions"
    payload = _make_payload(versions, PAYLOAD_VERSION)
    # current symlink
    cur = edikt_home / "current"
    if cur.exists() or cur.is_symlink():
        cur.unlink()
    cur.symlink_to(Path("versions") / PAYLOAD_VERSION)
    # external symlinks
    for name in ("hooks", "templates"):
        link = edikt_home / name
        if link.exists() or link.is_symlink():
            if link.is_symlink() or link.is_file():
                link.unlink()
            else:
                shutil.rmtree(link)
        link.symlink_to(Path("current") / name)
    # lock.yaml
    _write(
        edikt_home / "lock.yaml",
        f'active: "{PAYLOAD_VERSION}"\ninstalled_via: "fixture"\n',
    )


def build_synth_v010(sandbox: dict) -> None:
    _post_m1_layout(
        sandbox["edikt_home"], "0.1.0",
        with_html_sentinels=True,
        missing_keys=("paths", "stack", "gates"),
    )
    _flat_commands(sandbox["claude_home"], ("plan",))


def build_synth_v014(sandbox: dict) -> None:
    _post_m1_layout(
        sandbox["edikt_home"], "0.1.4",
        with_html_sentinels=True,
        missing_keys=("paths", "stack", "gates"),
    )
    _flat_commands(sandbox["claude_home"], ("plan",))


def build_synth_v020(sandbox: dict) -> None:
    _post_m1_layout(
        sandbox["edikt_home"], "0.2.0",
        with_html_sentinels=False,
        missing_keys=("gates",),
    )
    _governance_v1(sandbox["claude_home"])


def build_synth_v030(sandbox: dict) -> None:
    _post_m1_layout(
        sandbox["edikt_home"], "0.3.0",
        with_html_sentinels=False,
        missing_keys=("gates",),
    )
    _governance_v1(sandbox["claude_home"])


def build_synth_v043(sandbox: dict) -> None:
    """v0.4.3 — already on versioned layout. Only M4 should fire."""
    _versioned_seed(sandbox["edikt_home"])
    _governance_v1(sandbox["claude_home"])
    # config.yaml has all keys — no M5 needed.
    _write(
        sandbox["edikt_home"] / "config.yaml",
        textwrap.dedent(
            """\
            edikt_version: 0.4.3
            base: docs
            stack: [go]
            paths:
              decisions: docs/architecture/decisions
            gates:
              quality-gates: true
            """
        ),
    )


# ─── Real fixture loader ─────────────────────────────────────────────────────

FIXTURE_ROOT = REPO_ROOT / "test" / "integration" / "migration" / "fixtures"


def load_real_fixture(sandbox: dict, tag: str) -> bool:
    """Load a byte-for-byte captured fixture into sandbox.

    Copies fixtures/<tag>/edikt/ → sandbox EDIKT_HOME and
    fixtures/<tag>/commands/ → sandbox CLAUDE_HOME/commands/edikt/.
    Replaces the '${HOME}' placeholder written by capture.sh with the
    actual sandbox home path so hooks resolve correctly.

    Returns True if the fixture exists, False if it is absent (allows
    tests to skip cleanly when capture.sh hasn't been run yet).
    """
    fixture_dir = FIXTURE_ROOT / tag
    if not fixture_dir.exists():
        return False

    edikt_src = fixture_dir / "edikt"
    commands_src = fixture_dir / "commands"
    edikt_home: Path = sandbox["edikt_home"]
    claude_home: Path = sandbox["claude_home"]
    real_home: str = str(sandbox["home"])

    if edikt_src.exists():
        shutil.copytree(str(edikt_src), str(edikt_home), dirs_exist_ok=True)
    if commands_src.exists():
        cmd_dest = claude_home / "commands" / "edikt"
        cmd_dest.mkdir(parents=True, exist_ok=True)
        shutil.copytree(str(commands_src), str(cmd_dest), dirs_exist_ok=True)

    # Replace the ${HOME} placeholder with the real sandbox home.
    placeholder = "${HOME}"
    for f in edikt_home.rglob("*"):
        if f.is_file():
            try:
                text = f.read_text(errors="replace")
                if placeholder in text:
                    f.write_text(text.replace(placeholder, real_home))
            except (OSError, UnicodeDecodeError):
                pass
    for f in (claude_home / "commands").rglob("*"):
        if f.is_file():
            try:
                text = f.read_text(errors="replace")
                if placeholder in text:
                    f.write_text(text.replace(placeholder, real_home))
            except (OSError, UnicodeDecodeError):
                pass

    return True


# ─── Migrate runner ─────────────────────────────────────────────────────────


def run_migrate(sandbox: dict, *args: str) -> subprocess.CompletedProcess:
    cmd = [str(EDIKT_BIN), "migrate", *args]
    return subprocess.run(
        cmd,
        env=sandbox["env"],
        capture_output=True,
        text=True,
        timeout=60,
    )
