"""test_init_version_locked.py — Phase 9: provenance frontmatter contract.

Verifies that update_frontmatter() correctly writes edikt_template_hash and
edikt_template_version into YAML frontmatter, and that the version field is
treated as a locked string (written once at install, not auto-bumped).

SPEC-004 §7: "version is written once at install and never bumped on
upgrade-preserved files. This means 'version answers when was this installed',
not 'was this touched by a recent upgrade'."
"""

import re

import pytest

from .conftest import update_frontmatter

MINIMAL_FM = """\
---
name: backend
description: Backend specialist
tools:
  - Read
---

Body content here.
"""

EXISTING_HASH_FM = """\
---
name: backend
description: Backend specialist
edikt_template_hash: "old_hash_value"
edikt_template_version: "0.4.3"
---

Body content here.
"""


def test_hash_written_to_frontmatter():
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    assert 'edikt_template_hash: "abc123def456"' in result


def test_version_written_to_frontmatter():
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    assert 'edikt_template_version: "0.5.0"' in result


def test_version_is_string_not_float():
    """'0.5.0' must be stored as a quoted string, never as a bare number."""
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    # Must appear quoted — bare 0.5.0 would be invalid YAML and could parse as float
    assert 'edikt_template_version: "0.5.0"' in result
    assert "edikt_template_version: 0.5.0" not in result


def test_body_content_preserved():
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    assert "Body content here." in result


def test_existing_fields_preserved():
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    assert "name: backend" in result
    assert "description: Backend specialist" in result


def test_frontmatter_delimiters_preserved():
    result = update_frontmatter(MINIMAL_FM, "abc123def456", "0.5.0")
    assert result.startswith("---\n")
    assert "\n---\n" in result


def test_reinstall_updates_hash_in_place():
    """On reinstall, hash is updated — not duplicated."""
    result = update_frontmatter(EXISTING_HASH_FM, "new_hash_value", "0.5.0")
    assert 'edikt_template_hash: "new_hash_value"' in result
    assert "old_hash_value" not in result
    assert result.count("edikt_template_hash:") == 1


def test_reinstall_updates_version_in_place():
    """On reinstall, version is updated — not duplicated."""
    result = update_frontmatter(EXISTING_HASH_FM, "new_hash_value", "0.5.0")
    assert 'edikt_template_version: "0.5.0"' in result
    assert '"0.4.3"' not in result
    assert result.count("edikt_template_version:") == 1


def test_hash_value_is_full_md5():
    """A 32-char hex string passes through unchanged."""
    full_md5 = "a" * 32
    result = update_frontmatter(MINIMAL_FM, full_md5, "0.5.0")
    assert f'edikt_template_hash: "{full_md5}"' in result


def test_no_frontmatter_raises():
    with pytest.raises((ValueError, Exception)):
        update_frontmatter("no frontmatter here\n", "abc", "0.5.0")
