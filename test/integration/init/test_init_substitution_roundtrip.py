"""Substitution round-trip validation for agent templates.

Verifies that:
  1. Every substitution entry in _substitutions.yaml has at least one
     agent template that contains the default path string. A stale entry
     (substitution registered for a path that no template uses) is a sign
     that templates drifted from the substitution table.

  2. After applying a non-default config, the default path string no longer
     appears in the output. An unsubstituted default path means the user's
     configured path was silently ignored.

  3. After applying substitutions, the configured (non-default) path DOES
     appear in the output.

No ANTHROPIC_API_KEY required — pure string/file validation.
"""

from __future__ import annotations

from pathlib import Path

import pytest
import yaml

from .conftest import apply_substitutions, load_substitutions

REPO_ROOT = Path(__file__).resolve().parents[3]
AGENTS_DIR = REPO_ROOT / "templates" / "agents"
REGISTRY_FILE = AGENTS_DIR / "_registry.yaml"


def _registered_slugs() -> set[str]:
    registry = yaml.safe_load(REGISTRY_FILE.read_text())
    slugs: set[str] = set()
    for entries in registry.values():
        if isinstance(entries, list):
            slugs.update(entries)
    return slugs


def _agent_contents() -> dict[str, str]:
    """Return {filename: content} for registered installable agents only.

    Excludes prompt-only files (e.g. evaluator-headless.md) that are in
    templates/agents/ but are not installed by /edikt:init.
    """
    slugs = _registered_slugs()
    return {
        f"{slug}.md": (AGENTS_DIR / f"{slug}.md").read_text()
        for slug in slugs
        if (AGENTS_DIR / f"{slug}.md").exists()
    }


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_every_substitution_entry_appears_in_at_least_one_template() -> None:
    """_substitutions.yaml must not have stale entries.

    A substitution registered for a default path that appears in zero
    templates is dead configuration — the substitution will never fire
    and the config key is silently ignored. This catches the scenario
    where a template path is renamed but _substitutions.yaml isn't updated.
    """
    sub_table = load_substitutions()
    contents = _agent_contents()

    for key, entry in sub_table.items():
        default = entry["default"]
        found = any(default in content for content in contents.values())
        assert found, (
            f"Substitution entry '{key}' has default path '{default}' "
            f"but it does not appear in any agent template under templates/agents/. "
            f"Either update the default or remove the stale entry from _substitutions.yaml."
        )


@pytest.mark.parametrize("sub_key,entry", load_substitutions().items())
def test_substitution_replaces_default_with_configured_path(
    sub_key: str,
    entry: dict,
) -> None:
    """Applying a non-default config must replace the default path in output.

    For every substitution entry, find an agent template that contains the
    default path, apply substitutions with a custom value, and verify:
      - The custom value appears in the output
      - The default value no longer appears in the output
    """
    default = entry["default"]
    leaf = entry["config_key"].split(".")[-1]
    custom = f"custom/org/{leaf}"

    contents = _agent_contents()

    # Find templates that actually contain this default path.
    candidates = {
        name: content
        for name, content in contents.items()
        if default in content
    }
    if not candidates:
        pytest.skip(f"No agent template contains '{default}' — covered by the stale-entry test")

    config_paths = {leaf: custom}

    for name, original in candidates.items():
        result = apply_substitutions(original, config_paths)

        assert custom in result, (
            f"{name}: after substituting '{leaf}' → '{custom}', "
            f"the configured path was not found in the output. "
            f"Substitution silently failed."
        )
        assert default not in result, (
            f"{name}: after substituting '{leaf}' → '{custom}', "
            f"the default path '{default}' still appears in the output. "
            f"Substitution was partial or missed an occurrence."
        )


def test_default_config_leaves_content_unchanged() -> None:
    """Applying substitutions with default values must not mutate the template.

    If a user's config.yaml uses all default paths (or is missing path keys),
    the installed file must be byte-for-byte identical to the source template
    body (modulo provenance frontmatter added separately). A substitution that
    fires on default → default is a no-op by design, but this test verifies
    that nothing accidentally changes when no customisation is requested.
    """
    sub_table = load_substitutions()
    # config_paths with every key set to its own default.
    default_config = {
        entry["config_key"].split(".")[-1]: entry["default"]
        for entry in sub_table.values()
    }

    contents = _agent_contents()
    for name, original in contents.items():
        result = apply_substitutions(original, default_config)
        assert result == original, (
            f"{name}: applying default-value substitutions must not change content, "
            f"but the file was modified. This means a default path is being "
            f"substituted for itself, which corrupts the template."
        )
