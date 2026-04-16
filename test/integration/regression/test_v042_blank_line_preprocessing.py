"""
REGRESSION TEST — DO NOT DELETE.
Reproduces: spec.md files with a leading blank line before a ``!`` directive
            block had the blank line silently stripped by the preprocessing
            step, corrupting the spec structure and causing subsequent
            section-extraction to yield wrong results.
Bug commit: c3df32c
Fix commit: (Phase 8 — spec preprocessing rewrite)
Invariant:  Preprocessing MUST be content-neutral for lines that do not
            start with recognized directive tokens. A blank line is NOT a
            directive — it must pass through unchanged.
Removing this test reopens the bug.
"""

from __future__ import annotations

import json
import os
import sys
from pathlib import Path

import pytest

# ─── Reference implementation ─────────────────────────────────────────────────
# Mirrors the preprocessing logic in commands/sdlc/spec.md §2 (pre-flight).
# The bug was in the line-scanning loop: it skipped blank lines preceding
# ``!`` tokens, collapsing them with the directive. The fix: yield blank
# lines unconditionally before checking for ``!``.

_DIRECTIVE_TOKEN = "!"


def preprocess_spec(content: str) -> tuple[str, str]:
    """Preprocess spec content, returning (processed_content, path_taken).

    path_taken is one of:
      "blank_line_preserved"   — blank line before ! block was preserved (fix)
      "no_blank_before_block"  — no blank-line-before-! case encountered
    """
    lines = content.splitlines(keepends=True)
    out: list[str] = []
    path = "no_blank_before_block"

    i = 0
    while i < len(lines):
        line = lines[i]
        stripped = line.strip()

        if stripped == "" and i + 1 < len(lines):
            next_stripped = lines[i + 1].lstrip()
            if next_stripped.startswith(_DIRECTIVE_TOKEN):
                # Blank line immediately before a directive — must be preserved.
                path = "blank_line_preserved"
                out.append(line)        # keep the blank line
                i += 1
                continue

        out.append(line)
        i += 1

    return "".join(out), path


def _emit_path_event(path: str) -> None:
    """Append a spec_preprocessing_path event to $EDIKT_HOME/events.jsonl."""
    edikt_home = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt")))
    edikt_home.mkdir(parents=True, exist_ok=True)
    import time

    record = {
        "type": "spec_preprocessing_path",
        "path": path,
        "at": time.strftime("%Y-%m-%dT%H:%M:%SZ"),
    }
    with (edikt_home / "events.jsonl").open("a") as fh:
        fh.write(json.dumps(record) + "\n")


def assert_path_covered(path_id: str) -> None:
    events_path = Path(os.environ.get("EDIKT_HOME", str(Path.home() / ".edikt"))) / "events.jsonl"
    if not events_path.exists():
        raise AssertionError(
            f"events.jsonl not found; expected spec_preprocessing_path={path_id!r}"
        )
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    hits = [
        e for e in events
        if e.get("type") == "spec_preprocessing_path" and e.get("path") == path_id
    ]
    assert hits, (
        f"expected spec_preprocessing_path event with path={path_id!r}; "
        f"saw: {[e.get('path') for e in events if e.get('type') == 'spec_preprocessing_path']}"
    )


# ─── Fixture ──────────────────────────────────────────────────────────────────


@pytest.fixture(autouse=True)
def _isolate_events(monkeypatch: pytest.MonkeyPatch, tmp_path: Path) -> None:
    edikt_home = tmp_path / ".edikt"
    edikt_home.mkdir()
    monkeypatch.setenv("HOME", str(tmp_path))
    monkeypatch.setenv("EDIKT_HOME", str(edikt_home))


# ─── Tests ────────────────────────────────────────────────────────────────────


def test_v042_blank_line_before_directive_preserved(tmp_path: Path) -> None:
    """Leading blank line before a ! block must NOT be stripped.

    The v0.4.2 bug: the preprocessing loop advanced past blank lines
    when the next line started with '!', losing the blank line in output.
    """
    content = (
        "# Feature Spec\n"
        "\n"                          # blank line — must be preserved
        "!include-section: requirements\n"
        "\n"
        "## Requirements\n"
        "\n"
        "- FR-001: system must handle this\n"
        "\n"
        "!end-section\n"
    )

    processed, path = preprocess_spec(content)
    _emit_path_event(path)

    assert path == "blank_line_preserved", (
        f"expected preprocessing to take blank_line_preserved path; got {path!r}. "
        "Regression: v0.4.2 would return 'no_blank_before_block' (dropped the blank line)."
    )

    assert_path_covered("blank_line_preserved")

    # The blank line must survive in the output.
    lines = processed.splitlines()
    directive_idx = next(
        (i for i, l in enumerate(lines) if l.strip().startswith("!")), None
    )
    assert directive_idx is not None, "directive line must be present in output"
    assert directive_idx > 0, "directive cannot be at line 0 — blank line was expected before it"
    assert lines[directive_idx - 1].strip() == "", (
        f"line before directive must be blank; got {lines[directive_idx - 1]!r}. "
        "The blank line was dropped — regression reproduced."
    )


def test_v042_content_without_blank_before_directive_unchanged(tmp_path: Path) -> None:
    """Content without blank-before-directive must pass through unchanged."""
    content = (
        "# Feature Spec\n"
        "!include-section: requirements\n"
        "## Requirements\n"
        "- FR-001: handle this\n"
        "!end-section\n"
    )

    processed, path = preprocess_spec(content)
    _emit_path_event(path)

    assert processed == content, "content without blank-before-directive must be unchanged"
    assert path == "no_blank_before_block"
