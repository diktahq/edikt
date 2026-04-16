"""
REGRESSION TEST — DO NOT DELETE.
Reproduces: Upgrade silently overwrites a customized agent when the
            stored template has moved to a different path, bypassing
            the threeway_prompt branch entirely.
Bug commit: d81f6e3
Fix commit: (Phase 10 — provenance-first upgrade)
Invariant:  When edikt_template_hash is present but stored template cannot
            be reconstructed AND the installed file differs from any known
            re-synthesis, the upgrade MUST take threeway_prompt — never
            silent overwrite.
Removing this test reopens the bug.
"""

from __future__ import annotations

import hashlib
import os
import sys
import textwrap
from pathlib import Path

import pytest

# Reach the upgrade reference implementation.
_HERE = Path(__file__).resolve().parent
_UPGRADE_DIR = _HERE.parent / "upgrade"
sys.path.insert(0, str(_UPGRADE_DIR.parent))
from upgrade.conftest import (  # type: ignore[import]  # noqa: E402
    AGENTS_DIR,
    assert_path_covered,
    emit_event,
    install_agent,
    read_events,
    upgrade_agent,
)


@pytest.fixture(autouse=True)
def _isolate_events(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    """Redirect events.jsonl to a per-test tmp dir so tests don't bleed."""
    edikt_home = tmp_path / ".edikt"
    edikt_home.mkdir()
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setenv("EDIKT_HOME", str(edikt_home))


def test_v040_silent_overwrite_prevented(tmp_path: Path) -> None:
    """Customized agent + template moved → must NOT silently overwrite.

    Scenario:
      1. Install backend.md from template version A (hash X).
      2. User adds custom content after install.
      3. Template moves to a new path (stored_template_path=None).
      4. upgrade_agent() runs.

    Expected: threeway_prompt path (stored template unrecoverable, installed
    file diverged from last known synthesis → user must decide).

    The v0.4.0 bug: without provenance hash checks the upgrade would see
    "template changed" and overwrite unconditionally (the legacy classifier
    path). With provenance, stored_template_path=None + divergence forces
    threeway_prompt.
    """
    agents_dir = tmp_path / ".claude" / "agents"
    agents_dir.mkdir(parents=True)
    templates_dir = tmp_path / ".edikt" / "templates" / "agents"
    templates_dir.mkdir(parents=True)

    slug = "backend"
    source_template = AGENTS_DIR / f"{slug}.md"
    template_path = templates_dir / f"{slug}.md"
    template_path.write_bytes(source_template.read_bytes())

    installed_path = agents_dir / f"{slug}.md"
    install_agent(installed_path, template_path)

    # Simulate user edits after install.
    current_content = installed_path.read_text()
    installed_path.write_text(
        current_content
        + textwrap.dedent(
            """

            ## User Custom Section (added post-install)

            - Always prefer structured logging (zerolog).
            - Never use fmt.Printf in production paths.
            """
        )
    )

    # Simulate template move: the "current" template is now at a different
    # path (template_v2.md). stored_template_path is None because the original
    # path is gone — the upgrade cannot reconstruct the previous synthesis.
    new_template_path = templates_dir / f"{slug}_v2.md"
    new_template_path.write_bytes(source_template.read_bytes())
    new_template_path.write_text(
        source_template.read_text() + "\n## New Section (v2 addition)\n\n- New rule.\n"
    )

    path_taken = upgrade_agent(
        installed_path,
        current_template_path=new_template_path,
        stored_template_path=None,   # template path has moved — cannot reconstruct
        user_choice="s",             # "skip for now" — don't mutate in the test
    )

    assert path_taken == "threeway_prompt", (
        f"expected threeway_prompt, got {path_taken!r}. "
        "Regression: v0.4.0 would have taken legacy_classifier or silently overwritten."
    )

    assert_path_covered("threeway_prompt")
