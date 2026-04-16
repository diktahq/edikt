"""test_init_unterminated_marker_warns.py — Phase 9: unterminated marker fail-safe.

Per SPEC-004 §6 (as corrected): when a stack opening marker has no matching
closing marker, filtering is skipped for the entire file and a warning is emitted.
This prevents partial corruption of the agent content.
"""

import pytest
from .conftest import apply_stack_filter

GOOD_TEMPLATE = """\
<!-- edikt:stack:go -->
- Go: `gofmt -w <file>`
<!-- /edikt:stack -->
"""

UNTERMINATED_OPEN = """\
Normal content before.

<!-- edikt:stack:go -->
- Go: `gofmt -w <file>`

No closing marker here.

More content after.
"""

UNTERMINATED_CLOSE = """\
No opening marker here.

- Go: `gofmt -w <file>`
<!-- /edikt:stack -->

More content.
"""

EXTRA_OPEN = """\
<!-- edikt:stack:go -->
- Go formatter
<!-- /edikt:stack -->

<!-- edikt:stack:python -->
- Python formatter (unterminated)
"""


def test_well_formed_template_has_no_warning():
    _result, warns = apply_stack_filter(GOOD_TEMPLATE, ["go"])
    assert warns == []


def test_unterminated_open_emits_warning():
    _result, warns = apply_stack_filter(UNTERMINATED_OPEN, ["go"])
    assert len(warns) == 1
    assert "unterminated" in warns[0].lower()


def test_unterminated_open_returns_content_verbatim():
    result, _warns = apply_stack_filter(UNTERMINATED_OPEN, ["go"])
    assert result == UNTERMINATED_OPEN


def test_extra_close_emits_warning():
    result, warns = apply_stack_filter(UNTERMINATED_CLOSE, ["go"])
    assert len(warns) == 1


def test_extra_close_returns_content_verbatim():
    result, _warns = apply_stack_filter(UNTERMINATED_CLOSE, ["go"])
    assert result == UNTERMINATED_CLOSE


def test_extra_open_emits_warning():
    result, warns = apply_stack_filter(EXTRA_OPEN, ["go"])
    assert len(warns) == 1


def test_extra_open_returns_content_verbatim():
    result, _warns = apply_stack_filter(EXTRA_OPEN, ["go"])
    assert result == EXTRA_OPEN


def test_empty_stack_skips_filter_no_warning():
    """Empty stack skips filtering entirely — no warning even with bad markers."""
    result, warns = apply_stack_filter(UNTERMINATED_OPEN, [])
    assert result == UNTERMINATED_OPEN
    assert warns == []


def test_real_backend_template_is_well_formed(backend_template_content):
    """backend.md must have balanced markers — no warnings on any stack."""
    _result, warns = apply_stack_filter(backend_template_content, ["go"])
    assert warns == [], f"Unexpected warnings in backend.md: {warns}"
