"""Shared fixtures and helper implementations for Phase 9 init-provenance tests.

These tests validate the logic described in commands/init.md §4 "Specialist agents"
without invoking a live Claude Code session. The helper functions below are
reference implementations of the three algorithms Claude executes at install time:

  apply_substitutions  — §4c path substitution
  apply_stack_filter   — §4d stack-aware section filtering
  compute_hash         — §4b md5 of raw template before substitution

Tests treat these as the specification: if the test passes, the algorithm is correct.
"""

from __future__ import annotations

import hashlib
import re
import warnings
from pathlib import Path
from typing import Any

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
AGENTS_DIR = REPO_ROOT / "templates" / "agents"
SUBSTITUTIONS_FILE = AGENTS_DIR / "_substitutions.yaml"


# ─── Reference implementations ──────────────────────────────────────────────


def load_substitutions() -> dict[str, dict[str, str]]:
    """Return the substitution table from _substitutions.yaml."""
    data = yaml.safe_load(SUBSTITUTIONS_FILE.read_text())
    return data["substitutions"]


def apply_substitutions(content: str, config_paths: dict[str, str]) -> str:
    """Apply path substitutions to template content.

    For each entry in _substitutions.yaml, if config_paths[config_key_leaf]
    is set and differs from the default, replace every occurrence of the
    default string in content with the configured path.

    config_paths maps the leaf key (e.g. "decisions") to the configured value.
    """
    sub_table = load_substitutions()
    result = content
    for _key, entry in sub_table.items():
        default = entry["default"]
        # config_key is "paths.decisions" — leaf is "decisions"
        leaf = entry["config_key"].split(".")[-1]
        configured = config_paths.get(leaf)
        if configured and configured != default:
            result = result.replace(default, configured)
    return result


def apply_stack_filter(content: str, stack: list[str]) -> tuple[str, list[str]]:
    """Apply stack-aware section filtering.

    Returns (filtered_content, warnings_list).

    If stack is empty, returns content unchanged.
    If an unterminated opening marker is detected, returns content unchanged
    with a warning — the file is left verbatim (whole-file fail-safe).
    For each well-formed block: keep if langs intersects stack, drop otherwise.
    """
    if not stack:
        return content, []

    warns: list[str] = []

    # Check for unterminated markers (whole-file fail-safe).
    open_count = len(re.findall(r"<!-- edikt:stack:[^>]+ -->", content))
    close_count = len(re.findall(r"<!-- /edikt:stack -->", content))
    if open_count != close_count:
        warns.append(
            f"unterminated stack marker(s): {open_count} open, {close_count} close — "
            "skipping stack filtering for this file"
        )
        return content, warns

    # Process each block.
    # Pattern: <!-- edikt:stack:LANGS --> ... <!-- /edikt:stack -->
    # DOTALL so block body can span multiple lines.
    pattern = re.compile(
        r"<!-- edikt:stack:([^>]+) -->\n(.*?)<!-- /edikt:stack -->",
        re.DOTALL,
    )

    def _replace_block(m: re.Match) -> str:
        langs_str = m.group(1).strip()
        body = m.group(2)
        block_langs = {l.strip() for l in langs_str.split(",")}
        if block_langs & set(stack):
            # Keep body (strip marker lines only — trailing newline from open marker
            # is already consumed by the \n after -->).
            return body.rstrip("\n")
        # No intersection — drop the entire block.
        return ""

    filtered = pattern.sub(_replace_block, content)

    # Collapse runs of more than two consecutive blank lines left by dropped blocks.
    filtered = re.sub(r"\n{3,}", "\n\n", filtered)

    return filtered, warns


def compute_hash(path: Path) -> str:
    """Return the md5 hex digest of the file at path (raw bytes)."""
    data = path.read_bytes()
    return hashlib.md5(data).hexdigest()


def update_frontmatter(content: str, hash_value: str, version: str) -> str:
    """Prepend or update edikt_template_hash and edikt_template_version in YAML frontmatter.

    Assumes content starts with '---\\n'.  If the fields already exist they
    are updated in place; otherwise they are appended before the closing '---'.
    """
    if not content.startswith("---\n"):
        raise ValueError("Content does not start with YAML frontmatter")

    # Find end of frontmatter (second '---' line).
    end_marker = content.index("\n---\n", 4)
    fm_body = content[4:end_marker]
    after_fm = content[end_marker + 1 :]  # includes the closing '---\n...'

    lines = fm_body.splitlines()

    # Update or remove existing provenance lines.
    lines = [l for l in lines if not l.startswith(("edikt_template_hash:", "edikt_template_version:"))]

    # Append provenance fields.
    lines.append(f'edikt_template_hash: "{hash_value}"')
    lines.append(f'edikt_template_version: "{version}"')

    new_fm = "\n".join(lines)
    return f"---\n{new_fm}\n{after_fm}"


# ─── Fixtures ────────────────────────────────────────────────────────────────


@pytest.fixture
def backend_template_content() -> str:
    return (AGENTS_DIR / "backend.md").read_text()


@pytest.fixture
def mobile_template_content() -> str:
    return (AGENTS_DIR / "mobile.md").read_text()
