"""
REGRESSION TEST — DO NOT DELETE.

Reproduces: v0.4.3 preprocessor glob fails under zsh with cwd != project
root, outputting '(eval):1: no matches found: /architecture/decisions/
ADR-*.md' and falsely reporting 'Next ADR: ADR-001' when 13 ADRs exist.

Bug class:
  1. cwd assumption — preprocessor opens .edikt/config.yaml relative to $PWD
  2. zsh nomatch — bare glob expansion leaks shell errors
  3. broken fallback — `|| echo "docs"` binds to tr, not the pipeline

Invariant: The live-block preprocessor MUST resolve paths cwd-independently
and MUST NOT leak shell errors into its output.

Fix: ADR-014 Phase 18 — wrap each preprocessor in `bash -c` with upward
config walk, `find` instead of glob, proper `${VAR:-default}` fallback.

Removing this test reopens the bug. Only delete when the preprocessor
pattern is removed entirely.
"""
from __future__ import annotations

import re
import shutil
import subprocess
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
COMMANDS = [
    ("commands/adr/new.md", "Next ADR number: ADR-"),
    ("commands/invariant/new.md", "Next INV number: INV-"),
    ("commands/sdlc/prd.md", "Next PRD number: PRD-"),
    ("commands/sdlc/spec.md", "Next SPEC number: SPEC-"),
]

PREPROCESSOR_RE = re.compile(r"^!`(.*)`\s*$", re.MULTILINE)


def _extract_preprocessor(file_path: Path) -> str:
    """Extract the shell command inside the !`...` live block."""
    content = file_path.read_text()
    for line in content.splitlines():
        if line.startswith("!`") and line.rstrip().endswith("`"):
            return line[2:-1]
    raise AssertionError(f"No preprocessor block in {file_path}")


def _run(block: str, shell: str, cwd: Path) -> subprocess.CompletedProcess:
    """Execute a preprocessor block under a given shell and cwd."""
    if not Path(shell).exists():
        pytest.skip(f"{shell} not available on this system")
    return subprocess.run(
        [shell, "-c", block],
        capture_output=True,
        text=True,
        cwd=str(cwd),
        timeout=10,
    )


@pytest.mark.parametrize("file_rel,expected", COMMANDS)
def test_preprocessor_produces_expected_under_zsh_from_project_root(file_rel, expected):
    """Baseline: zsh + project root produces expected output."""
    block = _extract_preprocessor(REPO_ROOT / file_rel)
    result = _run(block, "/bin/zsh", REPO_ROOT)
    assert result.returncode == 0, f"non-zero exit: {result.stderr}"
    assert expected in result.stdout, f"missing '{expected}' in output: {result.stdout}"


@pytest.mark.parametrize("file_rel,expected", COMMANDS)
def test_preprocessor_does_not_leak_zsh_nomatch_errors(file_rel, expected):
    """The zsh bug we fixed: shell errors MUST NOT appear in output."""
    block = _extract_preprocessor(REPO_ROOT / file_rel)
    result = _run(block, "/bin/zsh", REPO_ROOT)
    assert "(eval):" not in result.stdout, (
        f"zsh shell error leaked into stdout:\n{result.stdout}"
    )
    assert "(eval):" not in result.stderr, (
        f"zsh shell error leaked into stderr:\n{result.stderr}"
    )
    assert "no matches found" not in (result.stdout + result.stderr), (
        f"zsh nomatch error surfaced:\n"
        f"stdout={result.stdout}\nstderr={result.stderr}"
    )


@pytest.mark.parametrize("file_rel,_expected", COMMANDS)
def test_preprocessor_cwd_agnostic_from_tmp(tmp_path, file_rel, _expected):
    """Running from /tmp (no config) produces graceful no-op, not a shell error."""
    block = _extract_preprocessor(REPO_ROOT / file_rel)
    result = _run(block, "/bin/zsh", tmp_path)
    assert result.returncode == 0, f"non-zero exit: {result.stderr}"
    assert "(eval):" not in (result.stdout + result.stderr)
    assert "no matches found" not in (result.stdout + result.stderr)
    # Graceful no-op should contain either "(none yet)" fallback or be empty (plan.md)
    output = result.stdout.strip()
    if output:  # non-empty
        assert "(none yet)" in output or "Next " in output, (
            f"expected graceful no-op output from /tmp, got: {output}"
        )


@pytest.mark.parametrize("file_rel,expected", COMMANDS)
def test_preprocessor_cwd_agnostic_from_subdirectory(file_rel, expected):
    """Running from a project subdirectory still resolves config via upward walk."""
    subdir = REPO_ROOT / "commands"
    block = _extract_preprocessor(REPO_ROOT / file_rel)
    result = _run(block, "/bin/zsh", subdir)
    assert result.returncode == 0
    assert expected in result.stdout, (
        f"preprocessor failed from subdirectory. cwd={subdir} "
        f"stdout={result.stdout} stderr={result.stderr}"
    )


@pytest.mark.parametrize("file_rel,expected", COMMANDS)
def test_preprocessor_works_under_bash(file_rel, expected):
    """Preprocessor must also work under bash (not zsh-only)."""
    block = _extract_preprocessor(REPO_ROOT / file_rel)
    result = _run(block, "/bin/bash", REPO_ROOT)
    assert result.returncode == 0
    assert expected in result.stdout


def test_preprocessor_base_fallback_defaults_to_docs(tmp_path):
    """Config lacking `base:` must default BASE to 'docs'.

    Regression: the old `|| echo "docs"` pattern didn't fire because `||`
    bound to `tr`, not the pipeline. BASE stayed empty and paths became
    absolute-from-root (e.g. `/architecture/decisions/`).
    """
    # Create a test project with config lacking base: and paths:
    edikt_dir = tmp_path / ".edikt"
    edikt_dir.mkdir()
    (edikt_dir / "config.yaml").write_text(
        'edikt_version: "0.5.0"\nstack: []\npaths: {}\n'
    )
    adr_dir = tmp_path / "docs" / "architecture" / "decisions"
    adr_dir.mkdir(parents=True)
    (adr_dir / "ADR-001-test.md").touch()
    (adr_dir / "ADR-002-test.md").touch()

    block = _extract_preprocessor(REPO_ROOT / "commands/adr/new.md")
    result = _run(block, "/bin/zsh", tmp_path)
    assert result.returncode == 0
    assert "Next ADR number: ADR-003" in result.stdout, (
        f"BASE fallback did not apply 'docs' default. Output: {result.stdout}"
    )


def test_all_preprocessors_use_bash_c_wrapper():
    """Shell-isolation invariant: every command's preprocessor wraps in `bash -c`."""
    files = [
        "commands/adr/new.md",
        "commands/invariant/new.md",
        "commands/sdlc/prd.md",
        "commands/sdlc/plan.md",
        "commands/sdlc/spec.md",
    ]
    for f in files:
        content = (REPO_ROOT / f).read_text()
        preprocessor_lines = [
            line for line in content.splitlines() if line.startswith("!`")
        ]
        assert preprocessor_lines, f"{f}: no preprocessor block"
        for line in preprocessor_lines:
            assert line.startswith("!`bash -c"), (
                f"{f}: preprocessor missing `bash -c` shell isolation. "
                f"Line: {line[:80]}..."
            )


def test_no_broken_fallback_pattern():
    """The legacy `|| echo "docs"` pipeline pattern must be gone.

    This pattern was broken because `||` binds to the last command in the
    pipeline (`tr`), not the pipeline as a whole. `tr` always exits 0 with
    empty input, so the fallback never fires.
    """
    files = [
        "commands/adr/new.md",
        "commands/invariant/new.md",
        "commands/sdlc/prd.md",
        "commands/sdlc/plan.md",
        "commands/sdlc/spec.md",
    ]
    for f in files:
        content = (REPO_ROOT / f).read_text()
        assert '|| echo "docs"' not in content, (
            f"{f}: still uses broken `|| echo \"docs\"` pipeline fallback"
        )
