"""test_init_paths_substituted.py — Phase 9: path substitution tests.

Verifies that apply_substitutions() replaces default path strings with
project-configured values, and leaves content unchanged when paths match
the defaults.
"""

from .conftest import apply_substitutions


# Only includes paths that have active entries in _substitutions.yaml.
# specs, prds, plans, guidelines were removed from the table in v0.5.0 because
# no agent template referenced them — apply_substitutions() reads the table
# dynamically, so those keys no longer trigger substitution.
TEMPLATE_WITH_PATHS = """\
---
name: architect
description: Architect specialist
---

Read ADRs in docs/architecture/decisions before starting.
Store invariants in docs/architecture/invariants.
"""


def test_decisions_path_substituted():
    result = apply_substitutions(
        TEMPLATE_WITH_PATHS,
        {"decisions": "architecture/adr"},
    )
    assert "architecture/adr" in result
    assert "docs/architecture/decisions" not in result


def test_invariants_path_substituted():
    result = apply_substitutions(
        TEMPLATE_WITH_PATHS,
        {"invariants": "arch/invariants"},
    )
    assert "arch/invariants" in result
    assert "docs/architecture/invariants" not in result


def test_multiple_paths_substituted_independently():
    result = apply_substitutions(
        TEMPLATE_WITH_PATHS,
        {"decisions": "adr", "invariants": "arch/inv"},
    )
    assert "adr" in result
    assert "arch/inv" in result
    assert "docs/architecture/decisions" not in result
    assert "docs/architecture/invariants" not in result


def test_default_path_unchanged():
    """If configured path equals the default, content must not change."""
    original = TEMPLATE_WITH_PATHS
    result = apply_substitutions(
        original,
        {"decisions": "docs/architecture/decisions"},
    )
    assert result == original


def test_empty_config_paths_unchanged():
    """No substitutions when config_paths is empty."""
    original = TEMPLATE_WITH_PATHS
    result = apply_substitutions(original, {})
    assert result == original


def test_substitution_applied_to_real_backend_template(backend_template_content):
    """Backend template has docs/architecture/decisions — custom path replaces it."""
    result = apply_substitutions(
        backend_template_content,
        {"decisions": "custom/adr"},
    )
    # Only assert if the default was present to begin with.
    if "docs/architecture/decisions" in backend_template_content:
        assert "custom/adr" in result
        assert "docs/architecture/decisions" not in result
    else:
        # Default not present in this template — substitution is a no-op.
        assert result == backend_template_content
