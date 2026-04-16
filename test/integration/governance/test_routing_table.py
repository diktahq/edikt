"""Governance integrity — routing table completeness and signal coverage.

The routing table tells Claude which specialist file to read for a given
task. Tests verify:

1. Every domain that has governance files is covered by at least one row.
2. Signal keywords in governance.md routing table are specific enough to
   be actionable (no row with only generic words like 'code' or 'file').
3. Each topic file is referenced by exactly one routing row — no orphan
   files and no duplicate rows.
4. The scope column contains only recognized values.
5. All topic files have at least one directive of the form recognized by
   the compile system (starts with '-', contains MUST/NEVER/MUST NOT).

No ANTHROPIC_API_KEY required.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
GOVERNANCE_MD = REPO_ROOT / ".claude" / "rules" / "governance.md"
RULES_DIR = REPO_ROOT / ".claude" / "rules"
GOVERNANCE_SUBDIR = RULES_DIR / "governance"

RECOGNIZED_SCOPES = {
    "planning", "design", "review", "implementation",
    "planning, design, review, implementation",
    "implementation, design",
}

# Words too generic to be useful routing signals.
GENERIC_WORDS = {"code", "file", "project", "task", "work", "change", "update"}


# ─── Helpers ─────────────────────────────────────────────────────────────────


def _governance_text() -> str:
    return GOVERNANCE_MD.read_text()


def _routing_rows(text: str) -> list[dict[str, str]]:
    rows = []
    in_table = False
    for line in text.splitlines():
        if "| Signals |" in line and "| Scope |" in line:
            in_table = True
            continue
        if in_table:
            if not line.startswith("|"):
                in_table = False
                continue
            cells = [c.strip() for c in line.strip().strip("|").split("|")]
            if len(cells) >= 3 and not set(cells[0]) <= {"-", " "}:
                rows.append({
                    "signals": cells[0],
                    "scope": cells[1] if len(cells) > 1 else "",
                    "file": cells[2] if len(cells) > 2 else "",
                })
    return rows


def _extract_path(cell: str) -> str | None:
    m = re.search(r"`([^`]+\.md)`", cell)
    return m.group(1) if m else None


def _resolve_topic(rel: str) -> Path | None:
    candidates = [
        RULES_DIR / rel.replace("governance/", ""),
        GOVERNANCE_SUBDIR / Path(rel).name,
        RULES_DIR.parent / rel,
    ]
    return next((p for p in candidates if p.exists()), None)


def _topic_files_on_disk() -> list[Path]:
    return list(GOVERNANCE_SUBDIR.glob("*.md"))


# ─── Routing table completeness ───────────────────────────────────────────────


def test_routing_table_has_at_least_one_row() -> None:
    rows = _routing_rows(_governance_text())
    assert rows, "routing table has no rows — governance is empty"


def test_every_topic_file_is_in_routing_table() -> None:
    """No orphan topic files — every .md in governance/ must have a routing row."""
    text = _governance_text()
    rows = _routing_rows(text)
    referenced = {Path(_extract_path(r["file"])).name for r in rows if _extract_path(r["file"])}
    for topic in _topic_files_on_disk():
        assert topic.name in referenced, (
            f"governance/{topic.name} exists but is not referenced in the routing table. "
            "Claude will never read it. Either add a routing row or delete the file."
        )


def test_no_duplicate_routing_rows() -> None:
    """Each topic file should appear in at most one routing row.

    Duplicate rows create ambiguity and inflate the context Claude loads.
    """
    rows = _routing_rows(_governance_text())
    paths = [_extract_path(r["file"]) for r in rows if _extract_path(r["file"])]
    seen: dict[str, int] = {}
    for p in paths:
        seen[p] = seen.get(p, 0) + 1
    dupes = {k: v for k, v in seen.items() if v > 1}
    assert not dupes, (
        f"Duplicate routing rows for: {list(dupes.keys())}. "
        "Merge into one row or keep only the most specific signals."
    )


def test_routing_scopes_are_recognized() -> None:
    """Scope column must contain only recognized lifecycle stage values."""
    rows = _routing_rows(_governance_text())
    for row in rows:
        scope = row["scope"].strip()
        normalized = {s.strip() for s in scope.split(",")}
        for s in normalized:
            assert s in RECOGNIZED_SCOPES or s in {
                "planning", "design", "review", "implementation"
            }, (
                f"Routing row for '{row['file']}' has unrecognized scope value: {s!r}. "
                f"Recognized values: {RECOGNIZED_SCOPES}"
            )


# ─── Signal keyword quality ───────────────────────────────────────────────────


def test_routing_signals_are_specific() -> None:
    """No routing row should rely solely on generic signal words.

    A row with only 'code' or 'file' as signals is too broad — it would
    match almost any task and force Claude to load context it doesn't need.
    At least one signal per row must be domain-specific.
    """
    rows = _routing_rows(_governance_text())
    for row in rows:
        signals = {s.strip().lower() for s in row["signals"].split(",")}
        specific = signals - GENERIC_WORDS
        assert specific, (
            f"Routing row for '{row['file']}' has only generic signal words: {signals}. "
            "Add at least one domain-specific keyword."
        )


def test_routing_signals_are_non_empty() -> None:
    rows = _routing_rows(_governance_text())
    for row in rows:
        assert row["signals"].strip(), (
            f"Routing row for '{row['file']}' has an empty signals column."
        )


# ─── Topic file directive quality ─────────────────────────────────────────────


@pytest.fixture(
    params=[p.name for p in _topic_files_on_disk()],
    ids=lambda n: n,
)
def topic_file(request: pytest.FixtureRequest) -> Path:
    return GOVERNANCE_SUBDIR / request.param


def test_topic_file_has_hard_constraint_directives(topic_file: Path) -> None:
    """Every topic file must contain at least one MUST or NEVER directive.

    A topic file with only soft language ('consider', 'prefer') is a
    governance dead zone — Claude reads it but has nothing to enforce.
    """
    text = topic_file.read_text()
    directives = [
        line.strip()
        for line in text.splitlines()
        if line.strip().startswith("- ")
    ]
    assert directives, (
        f"governance/{topic_file.name}: no bullet-point directives found. "
        "The file must contain at least one '- ' directive line."
    )
    hard = [d for d in directives if "MUST" in d or "NEVER" in d]
    assert hard, (
        f"governance/{topic_file.name}: no MUST or NEVER directives found. "
        f"All {len(directives)} directives use soft language. "
        "At least one directive must be a hard constraint."
    )


def test_topic_file_ref_tags_are_parseable(topic_file: Path) -> None:
    """All (ref: ADR-NNN) or (ref: INV-NNN) tags in the topic must be valid IDs."""
    text = topic_file.read_text()
    for ref in re.findall(r"\(ref: ([A-Z]+-\d+)\)", text):
        assert re.match(r"^(ADR|INV)-\d+$", ref), (
            f"governance/{topic_file.name}: malformed ref tag '{ref}'. "
            "Expected format: (ref: ADR-NNN) or (ref: INV-NNN)."
        )


def test_topic_file_has_markdown_heading(topic_file: Path) -> None:
    """Topic files must contain at least one markdown heading (# ...).

    Compiled topic files have YAML frontmatter followed by markdown.
    If no heading exists the file was never properly compiled.
    """
    text = topic_file.read_text()
    assert re.search(r'^#{1,3} \w', text, re.MULTILINE), (
        f"governance/{topic_file.name}: no markdown heading found. "
        "Compiled topic files must have at least one '# Section' heading."
    )
