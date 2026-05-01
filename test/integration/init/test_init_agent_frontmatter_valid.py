"""Structural validation of all agent templates in templates/agents/.

Verifies that every agent has the required YAML frontmatter fields, correct
types, and field values that match the schema expected by Claude Code and
edikt's upgrade provenance system.

No ANTHROPIC_API_KEY required — pure file parsing, no SDK calls.

Required fields (from Claude Code agent spec + edikt SPEC-004 §7):
  name         str   — agent identifier (no spaces, lowercase)
  description  str   — non-empty, used for routing
  tools        list  — at least one tool listed
  maxTurns     int   — positive integer
  effort       str   — one of: low, medium, high

Optional but validated when present:
  disallowedTools  list  — if present, must be a list of strings
  initialPrompt    str   — if present, must be non-empty string
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
AGENTS_DIR = REPO_ROOT / "templates" / "agents"
REGISTRY_FILE = AGENTS_DIR / "_registry.yaml"
SUBSTITUTIONS_FILE = AGENTS_DIR / "_substitutions.yaml"

REQUIRED_FIELDS = {"name", "description", "tools", "maxTurns", "effort"}
VALID_EFFORT_VALUES = {"low", "medium", "high"}
_FM_RE = re.compile(r"^---\n(.*?)\n---", re.DOTALL)

# ─── Helpers ─────────────────────────────────────────────────────────────────


def _load_frontmatter(path: Path) -> dict:
    text = path.read_text()
    m = _FM_RE.match(text)
    if not m:
        return {}
    try:
        return yaml.safe_load(m.group(1)) or {}
    except yaml.YAMLError:
        return {}


def _registered_agent_slugs() -> set[str]:
    """Return all agent slugs listed in _registry.yaml (any category).

    Only registered agents are installed by /edikt:init. Files in
    templates/agents/ that are NOT in the registry (e.g. evaluator-headless.md,
    which is a prompt-only file for `claude -p`) are excluded.
    """
    data = yaml.safe_load(REGISTRY_FILE.read_text())
    slugs: set[str] = set()
    for entries in data.values():
        if isinstance(entries, list):
            slugs.update(entries)
    return slugs


def _agent_files() -> list[Path]:
    slugs = _registered_agent_slugs()
    return [
        AGENTS_DIR / f"{slug}.md"
        for slug in sorted(slugs)
        if (AGENTS_DIR / f"{slug}.md").exists()
    ]


# ─── Parametrized tests ───────────────────────────────────────────────────────


@pytest.fixture(params=[p.name for p in _agent_files()], ids=lambda n: n)
def agent_template(request: pytest.FixtureRequest) -> tuple[str, dict]:
    path = AGENTS_DIR / request.param
    fm = _load_frontmatter(path)
    return request.param, fm


def test_frontmatter_is_present(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    assert fm, (
        f"{name}: missing YAML frontmatter. "
        "Every agent template must start with a --- ... --- block."
    )


def test_required_fields_present(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    missing = REQUIRED_FIELDS - fm.keys()
    assert not missing, (
        f"{name}: missing required frontmatter fields: {sorted(missing)}. "
        f"Present: {sorted(fm.keys())}"
    )


def test_description_is_non_empty_string(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    desc = fm.get("description", "")
    assert isinstance(desc, str) and desc.strip(), (
        f"{name}: 'description' must be a non-empty string; got {desc!r}"
    )


def test_tools_is_non_empty_list(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    tools = fm.get("tools")
    assert isinstance(tools, list) and tools, (
        f"{name}: 'tools' must be a non-empty list; got {tools!r}"
    )


def test_max_turns_is_positive_int(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    mt = fm.get("maxTurns")
    assert isinstance(mt, int) and mt > 0, (
        f"{name}: 'maxTurns' must be a positive integer; got {mt!r}"
    )


def test_effort_is_valid(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    effort = fm.get("effort")
    assert effort in VALID_EFFORT_VALUES, (
        f"{name}: 'effort' must be one of {VALID_EFFORT_VALUES}; got {effort!r}"
    )


def test_disallowed_tools_is_list_when_present(agent_template: tuple[str, dict]) -> None:
    name, fm = agent_template
    dt = fm.get("disallowedTools")
    if dt is None:
        return
    assert isinstance(dt, list), (
        f"{name}: 'disallowedTools' must be a list when present; got {type(dt).__name__}"
    )
    assert all(isinstance(t, str) for t in dt), (
        f"{name}: all entries in 'disallowedTools' must be strings; got {dt!r}"
    )


def test_evaluator_agent_disallows_write_edit(agent_template: tuple[str, dict]) -> None:
    """Evaluator agents MUST disallow Write and Edit.

    Per ADR-010: the evaluator runs in a read-only capacity — it judges
    output but must not modify files. Accidentally granting Write/Edit
    makes the evaluator's verdict untrustworthy (it could fix what it's
    judging). This is the production equivalent of the v0.4.3 regression
    (test_v043_evaluator_blocked.py).
    """
    name, fm = agent_template
    if "evaluator" not in name:
        return
    dt = fm.get("disallowedTools") or []
    assert "Write" in dt, (
        f"{name}: evaluator agent MUST list 'Write' in disallowedTools. "
        "Evaluators must be read-only — ADR-010."
    )
    assert "Edit" in dt, (
        f"{name}: evaluator agent MUST list 'Edit' in disallowedTools. "
        "Evaluators must be read-only — ADR-010."
    )
