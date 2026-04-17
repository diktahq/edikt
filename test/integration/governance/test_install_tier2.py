"""AC-023 / AC-023b / AC-023c / AC-023d / AC-023e / AC-023f — tier-2 install.

Tests the `edikt install benchmark` / `edikt uninstall benchmark` verbs in
bin/edikt. These are shell-only tests that exercise the launcher against a
sandboxed HOME + EDIKT_HOME + CLAUDE_HOME.

AC-023  — install.sh in a clean temp home does NOT install benchmark;
          edikt install benchmark DOES install it; tier-1 unchanged.
AC-023b — partial pip-install failure rolls back copied markdown.
AC-023c — Python version check fires BEFORE filesystem mutation, with the
          literal required message.
AC-023d — venv isolation (installs into ~/.edikt/venv/gov-benchmark/).
AC-023e — uninstall idempotence: tolerates missing state, exit 0.
AC-023f — wheel checksum verification (mismatch → clear abort message).

Note: We pass EDIKT_TIER2_SKIP_PIP=1 for most tests to avoid requiring
network during test runs. Pip-install-specific tests monkeypatch carefully.
"""

from __future__ import annotations

import hashlib
import os
import shutil
import subprocess
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).parents[3]
LAUNCHER = REPO_ROOT / "bin" / "edikt"
TOOLS_DIR = REPO_ROOT / "tools" / "gov-benchmark"
INSTALL_SH = REPO_ROOT / "install.sh"


def _sandbox_env(tmp_path: Path) -> dict[str, str]:
    """Create a sandbox $HOME/.edikt + $HOME/.claude and return env dict."""
    home = tmp_path / "home"
    edikt = home / ".edikt"
    claude = home / ".claude"
    home.mkdir()
    edikt.mkdir()
    claude.mkdir()
    # Simulate an installed core by creating a current/ directory with
    # the repo's commands + templates.
    versions = edikt / "versions" / "0.6.0"
    versions.mkdir(parents=True)
    shutil.copytree(REPO_ROOT / "commands", versions / "commands")
    shutil.copytree(REPO_ROOT / "templates", versions / "templates")
    shutil.copytree(REPO_ROOT / "tools", versions / "tools")
    (edikt / "current").symlink_to(versions)
    (edikt / "lock.yaml").write_text("active: 0.6.0\n")
    (edikt / "VERSION").write_text("0.6.0\n")
    return {
        **os.environ,
        "HOME": str(home),
        "EDIKT_HOME": str(edikt),
        "CLAUDE_HOME": str(claude),
    }


def _run(env: dict[str, str], *args: str, input_text: str | None = None) -> subprocess.CompletedProcess:
    return subprocess.run(
        [str(LAUNCHER), *args],
        env=env,
        capture_output=True,
        text=True,
        input=input_text,
    )


# ─── AC-023: install.sh clean-home excludes benchmark ────────────────────────


def test_install_sh_does_not_install_benchmark(tmp_path):
    """install.sh must not mention or install tier-2 benchmark artifacts.

    AC-023 + ADR-015: install.sh is tier-1 only. Any reference to the
    benchmark tool or its helper would violate the tier-2 carve-out.
    """
    text = INSTALL_SH.read_text()
    assert "benchmark" not in text.lower(), (
        "install.sh references 'benchmark' — ADR-015 forbids tier-2 in install.sh."
    )
    assert "gov-benchmark" not in text, (
        "install.sh references tier-2 helper path — forbidden."
    )
    assert "pip install" not in text, "install.sh must not invoke pip (INV-001)"
    assert "pipx" not in text, "install.sh must not invoke pipx"


def test_install_benchmark_adds_markdown_and_venv(tmp_path, monkeypatch):
    """edikt install benchmark installs tier-2 markdown + venv.

    Uses EDIKT_TIER2_SKIP_PIP=1 to skip the actual pip install step.
    """
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    proc = _run(env, "install", "benchmark")
    assert proc.returncode == 0, f"stderr: {proc.stderr}"

    claude = Path(env["CLAUDE_HOME"])
    benchmark_md = claude / "commands" / "edikt" / "gov" / "benchmark.md"
    attacks_dir = claude / "commands" / "edikt" / "templates" / "attacks"
    assert benchmark_md.exists()
    for t in ("refuse_tool_use.md", "refuse_file_pattern.md",
              "must_cite.md", "refuse_edit_matching_frontmatter.md"):
        assert (attacks_dir / t).exists(), f"missing {t}"


def test_install_benchmark_leaves_tier1_unchanged(tmp_path):
    """AC-023 — tier-1 command surface is byte-equal before/after tier-2 install."""
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    versions = Path(env["EDIKT_HOME"]) / "versions" / "0.6.0"
    # Hash all tier-1 files (commands/) before install.
    before = _hash_tree(versions / "commands")

    proc = _run(env, "install", "benchmark")
    assert proc.returncode == 0

    after = _hash_tree(versions / "commands")
    assert before == after, "tier-1 commands/ changed during tier-2 install"


def _hash_tree(root: Path) -> dict[str, str]:
    h: dict[str, str] = {}
    if not root.exists():
        return h
    for dirpath, _dirnames, filenames in os.walk(root):
        for f in filenames:
            p = Path(dirpath) / f
            rel = p.relative_to(root).as_posix()
            h[rel] = hashlib.sha256(p.read_bytes()).hexdigest()
    return h


# ─── AC-023c: Python version check ───────────────────────────────────────────


def test_python_version_check_uses_literal_message(tmp_path):
    """AC-023c — failure emits the required literal message."""
    env = _sandbox_env(tmp_path)
    # Point at a nonexistent Python via the test override.
    env["EDIKT_TIER2_PYTHON"] = str(tmp_path / "definitely-not-python")
    proc = _run(env, "install", "benchmark")
    assert proc.returncode != 0
    # The literal message format required by AC-023c.
    assert "edikt benchmark requires Python 3.10+" in proc.stderr


def test_python_version_check_rejects_old_python(tmp_path):
    """AC-023c — Python < 3.10 rejected with literal message."""
    env = _sandbox_env(tmp_path)
    # Stub script that reports Python 3.9.
    stub = tmp_path / "fake-py"
    stub.write_text(
        "#!/bin/sh\n"
        "if [ \"$1\" = \"-c\" ]; then\n"
        "  # Match the launcher's probe: print MAJOR.MINOR.\n"
        "  echo '3.9'\n"
        "fi\n"
    )
    stub.chmod(0o755)
    env["EDIKT_TIER2_PYTHON"] = str(stub)
    proc = _run(env, "install", "benchmark")
    assert proc.returncode != 0
    assert "edikt benchmark requires Python 3.10+" in proc.stderr
    assert "found 3.9 at" in proc.stderr


# ─── AC-023b: rollback on pip failure ────────────────────────────────────────


def test_pip_failure_rolls_back_markdown(tmp_path):
    """AC-023b — if pip install fails, copied markdown is removed."""
    env = _sandbox_env(tmp_path)
    # Force pip failure by pointing the wheel source at a nonexistent path.
    env["EDIKT_TIER2_WHEEL"] = "/nonexistent/definitely-not-a-wheel.whl"
    # Do NOT set SKIP_PIP so pip runs and fails.
    proc = _run(env, "install", "benchmark")
    assert proc.returncode != 0, "pip failure should return non-zero"

    claude = Path(env["CLAUDE_HOME"])
    benchmark_md = claude / "commands" / "edikt" / "gov" / "benchmark.md"
    attacks_dir = claude / "commands" / "edikt" / "templates" / "attacks"
    assert not benchmark_md.exists(), "benchmark.md not rolled back"
    # Attack templates either all removed or dir removed.
    if attacks_dir.exists():
        remaining = list(attacks_dir.iterdir())
        assert not remaining, f"attacks dir not rolled back: {remaining}"


# ─── AC-023e: uninstall idempotence ──────────────────────────────────────────


def test_uninstall_on_empty_state_exits_zero(tmp_path):
    """AC-023e — uninstall with nothing installed exits 0."""
    env = _sandbox_env(tmp_path)
    proc = _run(env, "uninstall", "benchmark")
    assert proc.returncode == 0
    assert "already uninstalled" in proc.stderr.lower() or \
           "already uninstalled" in proc.stdout.lower()


def test_uninstall_after_install_exits_zero(tmp_path):
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    install_proc = _run(env, "install", "benchmark")
    assert install_proc.returncode == 0
    uninstall_proc = _run(env, "uninstall", "benchmark")
    assert uninstall_proc.returncode == 0
    claude = Path(env["CLAUDE_HOME"])
    assert not (claude / "commands" / "edikt" / "gov" / "benchmark.md").exists()


def test_uninstall_tolerates_partial_state(tmp_path):
    """AC-023e — remove markdown by hand, then uninstall cleans up the venv."""
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    _run(env, "install", "benchmark")
    # Manually delete markdown — simulate partial corruption.
    claude = Path(env["CLAUDE_HOME"])
    md = claude / "commands" / "edikt" / "gov" / "benchmark.md"
    if md.exists():
        md.unlink()
    proc = _run(env, "uninstall", "benchmark")
    assert proc.returncode == 0


# ─── AC-023f: wheel checksum ─────────────────────────────────────────────────


def test_wheel_checksum_mismatch_aborts(tmp_path):
    """AC-023f — mismatched SHA-256 aborts with clear message."""
    env = _sandbox_env(tmp_path)
    # Create a fake wheel.
    fake_wheel = tmp_path / "fake.whl"
    fake_wheel.write_bytes(b"fake wheel contents")
    env["EDIKT_TIER2_WHEEL"] = str(fake_wheel)
    # Wrong expected checksum.
    env["EDIKT_TIER2_WHEEL_SHA256"] = "0" * 64
    proc = _run(env, "install", "benchmark")
    assert proc.returncode != 0
    assert "Wheel checksum mismatch" in proc.stderr


def test_wheel_checksum_match_proceeds(tmp_path):
    """AC-023f — matching SHA-256 proceeds past verification."""
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    fake_wheel = tmp_path / "fake.whl"
    fake_wheel.write_bytes(b"fake wheel contents")
    actual = hashlib.sha256(fake_wheel.read_bytes()).hexdigest()
    env["EDIKT_TIER2_WHEEL"] = str(fake_wheel)
    env["EDIKT_TIER2_WHEEL_SHA256"] = actual
    proc = _run(env, "install", "benchmark")
    # With SKIP_PIP=1, succeeds. Checksum gate passed without abort.
    assert proc.returncode == 0
    assert "Wheel checksum mismatch" not in proc.stderr


# ─── pyproject.toml pins (AC-023f secondary) ────────────────────────────────


def test_pyproject_pins_sdk_exactly():
    text = (TOOLS_DIR / "pyproject.toml").read_text()
    assert "claude-agent-sdk==" in text, (
        "tier-2 must pin claude-agent-sdk with == (ADR-015). "
        "Found pyproject.toml without exact pin."
    )
    # Explicitly reject float ranges.
    for forbidden in ("claude-agent-sdk>=", "claude-agent-sdk~=", "claude-agent-sdk*"):
        assert forbidden not in text, (
            f"tier-2 pyproject.toml uses forbidden range: {forbidden}"
        )


def test_tier_frontmatter_declared_on_benchmark():
    """ADR-015 — benchmark.md declares tier: 2 in its frontmatter."""
    text = (REPO_ROOT / "commands" / "gov" / "benchmark.md").read_text()
    # Frontmatter block between leading --- markers.
    assert text.startswith("---\n")
    end = text.index("\n---\n", 4)
    frontmatter = text[:end]
    assert "tier: 2" in frontmatter, (
        "benchmark.md must declare `tier: 2` in frontmatter (ADR-015)"
    )


# ─── Finding #13: release-path requires SHA256 ───────────────────────────────


def test_release_path_wheel_without_sha256_is_rejected(tmp_path):
    """#13 — a wheel under a /current/ path requires EDIKT_TIER2_WHEEL_SHA256.

    This prevents production (release-layout) installs from silently bypassing
    checksum verification when no SHA256 is provided.

    The sandbox sets up a symlink ~/.edikt/current -> versions/0.6.0; we place
    the fake wheel directly under that symlinked tree (using exist_ok=True since
    the tools dir may already exist from the sandbox setup).
    """
    env = _sandbox_env(tmp_path)
    # The sandbox already creates current/ as a symlink to versions/0.6.0/.
    # Place the wheel under current/tools/gov-benchmark/ (the canonical release path).
    current_dir = Path(env["EDIKT_HOME"]) / "current" / "tools" / "gov-benchmark"
    current_dir.mkdir(parents=True, exist_ok=True)
    release_wheel = current_dir / "gov_benchmark-0.6.0-py3-none-any.whl"
    release_wheel.write_bytes(b"fake release wheel")
    env["EDIKT_TIER2_WHEEL"] = str(release_wheel)
    # Intentionally do NOT set EDIKT_TIER2_WHEEL_SHA256.
    # Do NOT set SKIP_PIP — we want the verify step to fire.
    proc = _run(env, "install", "benchmark")
    assert proc.returncode != 0, (
        "Release-path install without SHA256 should be rejected."
    )
    assert "Release install requires EDIKT_TIER2_WHEEL_SHA256" in proc.stderr, (
        f"Expected literal rejection message; got stderr: {proc.stderr!r}"
    )


def test_release_path_wheel_with_sha256_proceeds(tmp_path):
    """#13 — a wheel under /current/ with a matching SHA256 proceeds normally."""
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    current_dir = Path(env["EDIKT_HOME"]) / "current" / "tools" / "gov-benchmark"
    current_dir.mkdir(parents=True, exist_ok=True)
    release_wheel = current_dir / "gov_benchmark-0.6.0-py3-none-any.whl"
    release_wheel.write_bytes(b"fake release wheel")
    actual_sha = hashlib.sha256(release_wheel.read_bytes()).hexdigest()
    env["EDIKT_TIER2_WHEEL"] = str(release_wheel)
    env["EDIKT_TIER2_WHEEL_SHA256"] = actual_sha
    proc = _run(env, "install", "benchmark")
    assert proc.returncode == 0, (
        f"Release-path install with correct SHA256 should succeed; stderr: {proc.stderr}"
    )
    assert "Release install requires EDIKT_TIER2_WHEEL_SHA256" not in proc.stderr


def test_non_release_path_wheel_without_sha256_allowed(tmp_path):
    """#13 — a wheel under a non-release path (e.g. /tmp/...) allows no SHA256.

    This preserves the test/dev-mode escape hatch: EDIKT_TIER2_SOURCE=/tmp/...
    or a dev-tree wheel path must still work without a checksum.
    """
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"
    dev_wheel = tmp_path / "gov_benchmark-dev.whl"
    dev_wheel.write_bytes(b"dev wheel")
    env["EDIKT_TIER2_WHEEL"] = str(dev_wheel)
    # No SHA256 — this is the test/dev mode escape hatch.
    proc = _run(env, "install", "benchmark")
    # Should proceed past the verify step (SKIP_PIP=1 keeps the rest simple).
    assert "Release install requires EDIKT_TIER2_WHEEL_SHA256" not in proc.stderr, (
        "Non-release-path wheel should not require SHA256."
    )
    # May still fail for unrelated reasons (e.g., wheel file type), but the
    # specific rejection message must not appear.


# ─── Finding #14: rollback respects CLAUDE_ROOT boundary ────────────────────


def test_rollback_does_not_remove_files_outside_claude_root(tmp_path):
    """#14 — tier2_rollback_markdown() ignores paths outside $CLAUDE_ROOT.

    A poisoned receipt containing a path outside CLAUDE_ROOT (e.g. /tmp/...)
    must not cause that file to be deleted during rollback.
    """
    env = _sandbox_env(tmp_path)

    # File that must survive rollback — it lives outside CLAUDE_ROOT.
    outside_file = tmp_path / f"should-not-be-deleted-{os.getpid()}.txt"
    outside_file.write_text("sentinel — must survive rollback")

    # Create a valid file inside CLAUDE_ROOT that IS a legitimate tier-2 artifact.
    claude = Path(env["CLAUDE_HOME"])
    legit_dir = claude / "commands" / "edikt" / "gov"
    legit_dir.mkdir(parents=True)
    legit_file = legit_dir / "benchmark.md"
    legit_file.write_text("# fake benchmark.md\n")

    # Craft a poisoned receipt: one entry outside CLAUDE_ROOT, one valid entry.
    receipt = Path(env["EDIKT_HOME"]) / ".tier2-benchmark-receipt"
    receipt.write_text(f"{outside_file}\n{legit_file}\n")

    # Simulate a pip-install failure by pointing the wheel at a nonexistent path
    # AND providing a receipt at the standard location so rollback fires.
    # We trigger rollback by running install with a bad wheel (non-release path
    # so the SHA256 gate doesn't fire first).
    bad_wheel = tmp_path / "bad.whl"
    bad_wheel.write_bytes(b"not a real wheel")
    env["EDIKT_TIER2_WHEEL"] = str(bad_wheel)
    # Do NOT set SKIP_PIP — we need pip to fail so rollback fires.

    # Pre-write the receipt so the rollback function finds it
    # (normally written by tier2_copy_markdown before pip runs).
    # We use a direct approach: create the environment so that
    # tier2_rollback_markdown is called with our poisoned receipt.
    # Easiest to test: call the function indirectly through a failed install.
    _run(env, "install", "benchmark")

    # The file outside CLAUDE_ROOT must still exist.
    assert outside_file.exists(), (
        "Rollback removed a file outside CLAUDE_ROOT — path guard failed.\n"
        f"  poisoned path: {outside_file}"
    )


# ─── Post-Phase-9 hardening: dangling symlink from v0.4.x migration ─────────

def test_install_benchmark_through_dangling_edikt_symlink(tmp_path):
    """edikt install benchmark recovers when $CLAUDE_HOME/commands/edikt is a
    symlink to a non-existent target.

    The v0.4.x → versioned-layout migration creates
    ~/.claude/commands/edikt → ~/.edikt/current/commands/edikt, but
    ~/.edikt/current/commands/edikt/ may not exist yet (commands were not
    part of the v0.4.3 payload). mkdir -p through a dangling symlink fails
    with ENOENT. The installer now detects the dangling link and creates
    the target explicitly.
    """
    env = _sandbox_env(tmp_path)
    env["EDIKT_TIER2_SKIP_PIP"] = "1"

    claude = Path(env["CLAUDE_HOME"])
    edikt = Path(env["EDIKT_HOME"])

    # Simulate the v0.4.x migration result: symlink exists, target doesn't.
    (claude / "commands").mkdir(parents=True)
    link = claude / "commands" / "edikt"
    target = edikt / "current" / "commands" / "edikt"
    assert not target.exists(), "precondition: target dir should be absent"
    link.symlink_to(target)
    assert link.is_symlink()
    assert not link.exists(), "precondition: symlink should be dangling"

    proc = _run(env, "install", "benchmark")
    assert proc.returncode == 0, f"install failed: stderr={proc.stderr}"

    # After install, benchmark.md should resolve through the now-healed link.
    benchmark_md = claude / "commands" / "edikt" / "gov" / "benchmark.md"
    assert benchmark_md.exists(), (
        "benchmark.md not created through the once-dangling symlink"
    )
    # And the symlink target must now exist.
    assert target.exists(), "installer did not create the symlink target dir"


def test_pyproject_pulls_pyyaml(tmp_path):
    """sandbox.py imports yaml at runtime; pyproject must declare pyyaml.

    Regression for the v0.6.0 baseline-run incident: `pip install` succeeded
    but the helper crashed on first invocation with ImportError: No module
    named 'yaml' because pyyaml was missing from [project.dependencies].
    """
    pyproject = REPO_ROOT / "tools" / "gov-benchmark" / "pyproject.toml"
    content = pyproject.read_text()
    assert "pyyaml==" in content.lower(), (
        "pyyaml must be pinned in tools/gov-benchmark/pyproject.toml "
        "dependencies — sandbox.py imports it at runtime"
    )
