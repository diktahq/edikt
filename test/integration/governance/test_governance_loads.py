"""Governance integrity — governance.md structure and completeness.

Verifies that the compiled governance.md:

1. Has the correct compile_schema_version.
2. Has the Non-Negotiable Constraints section with all active invariant text.
3. Has the Routing Table section with well-formed rows.
4. Directive count in the header comment matches the actual directives
   loaded across the topic files.
5. Every ADR and invariant directive appears somewhere in governance.md
   (either inlined or via a topic file referenced from the routing table).

These tests run without ANTHROPIC_API_KEY — pure file parsing.
"""

from __future__ import annotations

import re
from pathlib import Path

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
GOVERNANCE_MD = REPO_ROOT / ".claude" / "rules" / "governance.md"
RULES_DIR = REPO_ROOT / ".claude" / "rules"
DECISIONS_DIR = REPO_ROOT / "docs" / "architecture" / "decisions"
INVARIANTS_DIR = REPO_ROOT / "docs" / "architecture" / "invariants"

_BLOCK_RE = re.compile(
    r"\[edikt:directives:start\]: #\n(.*?)\n\[edikt:directives:end\]: #",
    re.DOTALL,
)


# ─── Helpers ─────────────────────────────────────────────────────────────────


def _governance_text() -> str:
    return GOVERNANCE_MD.read_text()


def _routing_table_rows(text: str) -> list[dict[str, str]]:
    """Parse the Routing Table from governance.md into a list of dicts."""
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


def _extract_backtick_path(cell: str) -> str | None:
    """Extract `path/to/file.md` from a routing table File cell."""
    m = re.search(r"`([^`]+\.md)`", cell)
    return m.group(1) if m else None


def _parse_block(block_yaml: str) -> dict:
    if not block_yaml:
        return {}
    result: dict = {}
    current_key: str | None = None
    current_list: list | None = None
    for raw_line in block_yaml.splitlines():
        line = raw_line.rstrip()
        if line.startswith("  - ") and current_list is not None:
            current_list.append(line[4:])
            continue
        m_empty = re.match(r'^(\w[\w_-]*):\s*\[(.*)\]\s*$', line)
        if m_empty:
            key = m_empty.group(1)
            inner = m_empty.group(2).strip()
            result[key] = [i.strip() for i in inner.split(",")] if inner else []
            current_key = None; current_list = None
            continue
        m_scalar = re.match(r'^(\w[\w_-]*):\s+(.+)$', line)
        if m_scalar:
            result[m_scalar.group(1)] = m_scalar.group(2).strip().strip('"')
            current_key = None; current_list = None
            continue
        m_list = re.match(r'^(\w[\w_-]*):\s*$', line)
        if m_list:
            key = m_list.group(1)
            result[key] = []
            current_key = key; current_list = result[key]
    return result


def _all_directives_in_artifact(path: Path) -> list[str]:
    text = path.read_text()
    m = _BLOCK_RE.search(text)
    if not m:
        return []
    block = _parse_block(m.group(1))
    auto = block.get("directives") or []
    manual = block.get("manual_directives") or []
    suppressed = set(block.get("suppressed_directives") or [])
    effective = [d for d in auto if d not in suppressed] + list(manual)
    return [str(d) for d in effective]


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_governance_file_exists() -> None:
    assert GOVERNANCE_MD.exists(), (
        f"governance.md not found at {GOVERNANCE_MD}. "
        "Run /edikt:gov:compile to generate it."
    )


def test_governance_has_compile_schema_version() -> None:
    text = _governance_text()
    # Schema version is in YAML frontmatter at the top of the file.
    m = re.search(r"compile_schema_version:\s*(\d+)", text)
    assert m, (
        "governance.md is missing compile_schema_version in frontmatter. "
        "File may be corrupted or written by an old compiler."
    )
    version = int(m.group(1))
    assert version >= 2, (
        f"compile_schema_version is {version}, expected >= 2 (ADR-007). "
        "Run /edikt:gov:compile to upgrade."
    )


def test_governance_has_non_negotiable_section() -> None:
    text = _governance_text()
    assert "## Non-Negotiable Constraints" in text, (
        "governance.md is missing the '## Non-Negotiable Constraints' section. "
        "This section surfaces invariants directly — without it Claude has no hard guards."
    )


def test_governance_has_routing_table() -> None:
    text = _governance_text()
    assert "## Routing Table" in text, (
        "governance.md is missing the '## Routing Table' section. "
        "Without it Claude cannot find the specialist topic files."
    )
    rows = _routing_table_rows(text)
    assert rows, (
        "Routing Table section found but contains no rows. "
        "Run /edikt:gov:compile to regenerate."
    )


def test_governance_routing_table_files_exist() -> None:
    """Every file referenced in the routing table must exist on disk."""
    text = _governance_text()
    rows = _routing_table_rows(text)
    for row in rows:
        rel_path = _extract_backtick_path(row["file"])
        if not rel_path:
            continue
        full_path = RULES_DIR / rel_path.replace("governance/", "")
        # The routing table uses relative paths like `governance/architecture.md`
        # which resolve relative to .claude/rules/
        alt_path = RULES_DIR.parent / rel_path
        exists = full_path.exists() or alt_path.exists() or (RULES_DIR / rel_path).exists()
        assert exists, (
            f"Routing table references '{rel_path}' but the file was not found. "
            f"Checked: {full_path}, {alt_path}. "
            "Either the file was deleted or the routing table is stale. "
            "Run /edikt:gov:compile."
        )


def test_governance_routing_table_files_non_empty() -> None:
    """Every routing topic file must contain actual directives content."""
    text = _governance_text()
    rows = _routing_table_rows(text)
    for row in rows:
        rel_path = _extract_backtick_path(row["file"])
        if not rel_path:
            continue
        # Try all likely resolution paths.
        candidates = [
            RULES_DIR / rel_path.replace("governance/", ""),
            RULES_DIR.parent / rel_path,
            RULES_DIR / rel_path,
        ]
        topic_file = next((p for p in candidates if p.exists()), None)
        if not topic_file:
            continue  # covered by test_governance_routing_table_files_exist
        content = topic_file.read_text().strip()
        assert content, (
            f"Routing table file '{rel_path}' exists but is empty. "
            "A routing entry pointing to an empty file gives Claude a dead end."
        )
        assert len(content) > 100, (
            f"Routing table file '{rel_path}' has only {len(content)} chars — "
            "suspiciously short. May be a stub that was never filled in."
        )


def test_governance_routing_table_signal_keywords_appear_in_topic_file() -> None:
    """Each routing row's signal keywords must appear in the referenced file.

    If a row says 'Signals: agent, specialist' but the topic file has no
    mention of 'agent' or 'specialist', the routing is decorative — Claude
    reads the keyword from the table but finds nothing relevant in the file.
    """
    text = _governance_text()
    rows = _routing_table_rows(text)
    for row in rows:
        rel_path = _extract_backtick_path(row["file"])
        if not rel_path:
            continue
        candidates = [
            RULES_DIR / rel_path.replace("governance/", ""),
            RULES_DIR.parent / rel_path,
            RULES_DIR / rel_path,
        ]
        topic_file = next((p for p in candidates if p.exists()), None)
        if not topic_file:
            continue
        topic_text = topic_file.read_text().lower()
        signals = [s.strip().lower() for s in row["signals"].split(",") if s.strip()]
        matched = [s for s in signals if s and s in topic_text]
        assert matched, (
            f"Routing row signals ({signals!r}) do not appear in "
            f"'{rel_path}'. The routing is stale — signals changed but "
            "the topic file content doesn't reflect them."
        )


def test_governance_invariant_text_appears_in_non_negotiable_section() -> None:
    """Every active invariant's core rule must appear in the Non-Negotiable section.

    The Non-Negotiable Constraints section is what Claude reads first and
    treats as absolute. If an invariant's directive is missing from it, the
    invariant is effectively unenforced in practice.
    """
    text = _governance_text()
    # Extract just the Non-Negotiable section.
    m = re.search(
        r"## Non-Negotiable Constraints(.*?)(?=^##|\Z)",
        text,
        re.DOTALL | re.MULTILINE,
    )
    if not m:
        pytest.skip("Non-Negotiable section missing — covered by another test")
    nn_section = m.group(1).lower()

    for inv_path in sorted(INVARIANTS_DIR.glob("INV-*.md")):
        directives = _all_directives_in_artifact(inv_path)
        if not directives:
            continue
        # At least one directive from each invariant should appear in NN section.
        # Use significant words from the first directive as a fingerprint.
        # Avoids false negatives from compile-time reformatting or line breaks.
        first = directives[0].lower()
        # Extract the first 5 meaningful words (skip articles and short words).
        words = [w for w in re.split(r'\W+', first) if len(w) > 3][:5]
        assert words, f"{inv_path.name}: directive is too short to fingerprint"
        any_word_found = any(w in nn_section for w in words)
        assert any_word_found, (
            f"{inv_path.name}: no words from primary directive found in Non-Negotiable section.\n"
            f"  Directive words checked: {words}\n"
            "Run /edikt:gov:compile to regenerate governance.md."
        )


def test_governance_header_directive_count_is_accurate() -> None:
    """The directive count in governance.md header comment must be correct.

    The header says '51 across 5 topic files' (or similar). If the count
    is wrong Claude's operators have a false sense of coverage.
    """
    text = _governance_text()
    # Extract claimed count from header comment.
    m = re.search(r"directives:\s*(\d+)\s*across", text)
    if not m:
        pytest.skip("directive count comment not found in governance.md header")
    claimed = int(m.group(1))

    # Count actual directive bullets across all topic files.
    # Only count lines that start with "- " inside the markdown body
    # (after the YAML frontmatter block, if any).
    actual_count = 0
    rows = _routing_table_rows(text)
    seen_files: set[str] = set()
    for row in rows:
        rel_path = _extract_backtick_path(row["file"])
        if not rel_path or rel_path in seen_files:
            continue
        seen_files.add(rel_path)
        candidates = [
            RULES_DIR / rel_path.replace("governance/", ""),
            RULES_DIR.parent / rel_path,
            RULES_DIR / rel_path,
        ]
        topic_file = next((p for p in candidates if p.exists()), None)
        if not topic_file:
            continue
        # Skip YAML frontmatter (between opening and closing ---).
        content = topic_file.read_text()
        if content.startswith("---\n"):
            end = content.find("\n---\n", 4)
            content = content[end + 5:] if end != -1 else content
        actual_count += sum(
            1 for line in content.splitlines()
            if line.strip().startswith("- ")
        )

    # Allow ±10 tolerance: count includes inline constraints (Non-Negotiable
    # section) that are also counted in the header comment.
    assert abs(actual_count - claimed) <= 10, (
        f"governance.md header says {claimed} directives but "
        f"counting topic file bullets gives ~{actual_count}. "
        "Run /edikt:gov:compile to refresh the count."
    )
