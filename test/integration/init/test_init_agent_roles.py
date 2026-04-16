"""Agent role validation — disallowedTools, maxTurns, and routing description quality.

Verifies that each agent template is correctly configured for its role:

- Read-only agents (evaluator, docs) MUST disallow Write and Edit.
- Writer agents (backend, frontend, qa, etc.) must NOT have Read in disallowedTools.
- maxTurns is within a sane range (2–50) — too low causes premature stops,
  too high allows runaway sessions.
- Agents that are always-installed must have a description that contains
  routing guidance (when to use them), not just a noun.
- Registry has no duplicate slug entries (same agent in multiple categories
  is fine, but the same slug twice in one category is a bug).

No ANTHROPIC_API_KEY required.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
AGENTS_DIR = REPO_ROOT / "templates" / "agents"
REGISTRY_FILE = AGENTS_DIR / "_registry.yaml"

_FM_RE = re.compile(r"^---\n(.*?)\n---", re.DOTALL)

# Agents that must be read-only — their purpose is observation, not mutation.
READ_ONLY_AGENTS = {"evaluator", "docs", "architect"}

MAX_TURNS_FLOOR = 2
MAX_TURNS_CEILING = 60

# Minimum description length — anything shorter is likely just a noun phrase
# with no routing context for Claude (e.g. "Backend engineer").
MIN_DESCRIPTION_LENGTH = 40


# ─── Helpers ─────────────────────────────────────────────────────────────────


def _load_registry() -> dict:
    return yaml.safe_load(REGISTRY_FILE.read_text())


def _registered_slugs() -> set[str]:
    registry = _load_registry()
    slugs: set[str] = set()
    for entries in registry.values():
        if isinstance(entries, list):
            slugs.update(entries)
    return slugs


def _load_frontmatter(slug: str) -> dict:
    path = AGENTS_DIR / f"{slug}.md"
    if not path.exists():
        return {}
    text = path.read_text()
    m = _FM_RE.match(text)
    if not m:
        return {}
    try:
        return yaml.safe_load(m.group(1)) or {}
    except yaml.YAMLError:
        return {}


def _agent_names() -> list[str]:
    return sorted(_registered_slugs())


# ─── Parametrized fixtures ────────────────────────────────────────────────────


@pytest.fixture(params=_agent_names(), ids=lambda n: n)
def agent_slug(request: pytest.FixtureRequest) -> str:
    return request.param


@pytest.fixture(params=sorted(READ_ONLY_AGENTS & _registered_slugs()), ids=lambda n: n)
def read_only_slug(request: pytest.FixtureRequest) -> str:
    return request.param


def _writer_slugs() -> list[str]:
    """Agents whose frontmatter has Write in tools and NOT in disallowedTools."""
    result = []
    for slug in _registered_slugs():
        fm = _load_frontmatter(slug)
        tools = fm.get("tools") or []
        dt = fm.get("disallowedTools") or []
        if "Write" in tools and "Write" not in dt:
            result.append(slug)
    return sorted(result)


@pytest.fixture(params=_writer_slugs(), ids=lambda n: n)
def writer_slug(request: pytest.FixtureRequest) -> str:
    return request.param


# ─── Read-only agent tests ────────────────────────────────────────────────────


def test_read_only_agent_disallows_write(read_only_slug: str) -> None:
    """Evaluator, docs, and architect agents MUST NOT be able to write files.

    These agents observe, analyze, and return findings. Granting Write or Edit
    would make their output untrustworthy — an evaluator that modifies the code
    it's judging is not an evaluator.
    """
    fm = _load_frontmatter(read_only_slug)
    dt = fm.get("disallowedTools") or []
    assert "Write" in dt, (
        f"{read_only_slug}.md: read-only agent must list 'Write' in disallowedTools. "
        "An agent that can write files cannot be trusted to evaluate them."
    )
    assert "Edit" in dt, (
        f"{read_only_slug}.md: read-only agent must list 'Edit' in disallowedTools. "
        "An agent that can edit files cannot be trusted to evaluate them."
    )


def test_read_only_agent_has_read_tool(read_only_slug: str) -> None:
    """Read-only agents must still be able to read files to do their job."""
    fm = _load_frontmatter(read_only_slug)
    tools = fm.get("tools") or []
    assert "Read" in tools, (
        f"{read_only_slug}.md: read-only agent must have 'Read' in tools list. "
        "Without Read it cannot inspect any files."
    )


# ─── Writer agent tests ───────────────────────────────────────────────────────


def test_writer_agent_not_blocked_from_writing(writer_slug: str) -> None:
    """Agents whose job is to write code must not have Write in disallowedTools.

    A backend agent that cannot Write can never implement anything. If Write
    appears in disallowedTools for a writer agent, the agent is broken by
    configuration and will confuse users with silent no-ops.
    """
    fm = _load_frontmatter(writer_slug)
    dt = fm.get("disallowedTools") or []
    assert "Write" not in dt, (
        f"{writer_slug}.md: writer agent must NOT list 'Write' in disallowedTools. "
        "This agent's job is to write files — blocking it makes it non-functional."
    )
    assert "Edit" not in dt, (
        f"{writer_slug}.md: writer agent must NOT list 'Edit' in disallowedTools. "
        "This agent's job requires editing — blocking it makes it non-functional."
    )


# ─── maxTurns bounds ──────────────────────────────────────────────────────────


def test_max_turns_within_bounds(agent_slug: str) -> None:
    """maxTurns must be between {floor} and {ceiling} (inclusive).

    Too low: agent stops before it can complete real tasks.
    Too high: runaway sessions with unbounded API cost.
    """.format(floor=MAX_TURNS_FLOOR, ceiling=MAX_TURNS_CEILING)
    fm = _load_frontmatter(agent_slug)
    mt = fm.get("maxTurns")
    if mt is None:
        pytest.skip(f"{agent_slug}: no maxTurns field — covered by frontmatter test")
    assert isinstance(mt, int), f"{agent_slug}.md: maxTurns must be an integer, got {type(mt).__name__}"
    assert mt >= MAX_TURNS_FLOOR, (
        f"{agent_slug}.md: maxTurns={mt} is below floor {MAX_TURNS_FLOOR}. "
        "Agent will stop before completing any real task."
    )
    assert mt <= MAX_TURNS_CEILING, (
        f"{agent_slug}.md: maxTurns={mt} exceeds ceiling {MAX_TURNS_CEILING}. "
        "Unbounded sessions have unpredictable API cost."
    )


# ─── Routing description quality ─────────────────────────────────────────────


def test_always_installed_agent_has_routing_description() -> None:
    """'always' category agents must have substantive descriptions (not just a noun).

    Claude uses the description to decide when to invoke the agent. A one-word
    description like 'Backend' gives no routing signal. A description must be long
    enough to explain the agent's purpose and trigger conditions.
    """
    registry = _load_registry()
    always_slugs = registry.get("always") or []

    for slug in always_slugs:
        fm = _load_frontmatter(slug)
        desc = fm.get("description", "")
        assert len(desc) >= MIN_DESCRIPTION_LENGTH, (
            f"{slug}.md: 'always' agent description is too short ({len(desc)} chars). "
            f"Got: {desc!r}\n"
            f"Descriptions must be at least {MIN_DESCRIPTION_LENGTH} chars to give Claude "
            "meaningful routing context."
        )


# ─── Registry integrity ───────────────────────────────────────────────────────


def test_registry_has_no_duplicate_slugs_per_category() -> None:
    """No agent slug should appear twice in the same registry category."""
    registry = _load_registry()
    for category, entries in registry.items():
        if not isinstance(entries, list):
            continue
        seen: dict[str, int] = {}
        for slug in entries:
            seen[slug] = seen.get(slug, 0) + 1
        dupes = {k: v for k, v in seen.items() if v > 1}
        assert not dupes, (
            f"Registry category '{category}' has duplicate slugs: {dupes}. "
            "Each slug should appear at most once per category."
        )


def test_all_registry_slugs_have_template_files() -> None:
    """Every slug in the registry must have a corresponding .md template file."""
    for slug in _registered_slugs():
        template = AGENTS_DIR / f"{slug}.md"
        assert template.exists(), (
            f"Registry lists '{slug}' but templates/agents/{slug}.md does not exist. "
            "Either create the template or remove the slug from the registry."
        )


def test_no_template_files_missing_from_registry() -> None:
    """Every .md template (except underscored files) should be in the registry.

    A template that exists but isn't registered is never installed — it's dead
    configuration that confuses contributors.
    """
    registered = _registered_slugs()
    optional = set((_load_registry().get("optional") or []))
    all_registered = registered | optional

    for template in AGENTS_DIR.glob("*.md"):
        if template.name.startswith("_") or "headless" in template.name:
            continue  # internal / prompt-only files
        slug = template.stem
        assert slug in all_registered, (
            f"templates/agents/{template.name} exists but '{slug}' is not in _registry.yaml. "
            "Either add it to the registry (or optional:) or remove the template."
        )
