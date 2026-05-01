"""test_init_stack_multi.py — Phase 9: stack filter with multiple languages.

Verifies that with a multi-language stack, all matching blocks are retained
and non-matching blocks are removed.
"""

from .conftest import apply_stack_filter

MULTI_LANG_TEMPLATE = """\
<!-- edikt:stack:go -->
## File Formatting

- Go (*.go): `gofmt -w <file>`

Run the formatter immediately.
<!-- /edikt:stack -->

<!-- edikt:stack:typescript,javascript -->
## File Formatting

- TypeScript/JavaScript: `prettier --write <file>`

Run the formatter immediately.
<!-- /edikt:stack -->

<!-- edikt:stack:python -->
## File Formatting

- Python: `black <file>`

Run the formatter immediately.
<!-- /edikt:stack -->

<!-- edikt:stack:rust -->
## File Formatting

- Rust: `rustfmt <file>`

Run the formatter immediately.
<!-- /edikt:stack -->
"""


def test_go_and_typescript_both_retained():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go", "typescript"])
    assert "gofmt -w" in result
    assert "prettier --write" in result
    assert not warns


def test_non_matching_blocks_removed():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go", "typescript"])
    assert "black <file>" not in result
    assert "rustfmt" not in result


def test_typescript_in_combined_marker_matches():
    """typescript matches the 'typescript,javascript' marker."""
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["typescript"])
    assert "prettier --write" in result


def test_javascript_in_combined_marker_matches():
    """javascript also matches the 'typescript,javascript' marker."""
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["javascript"])
    assert "prettier --write" in result


def test_empty_stack_returns_unchanged():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, [])
    assert result == MULTI_LANG_TEMPLATE
    assert not warns


def test_all_blocks_retained_when_all_langs_match():
    result, warns = apply_stack_filter(
        MULTI_LANG_TEMPLATE, ["go", "typescript", "python", "rust"]
    )
    assert "gofmt -w" in result
    assert "prettier --write" in result
    assert "black <file>" in result
    assert "rustfmt" in result
    assert not warns


def test_mobile_template_typescript_only(mobile_template_content):
    """React Native project: only TypeScript/JavaScript block kept."""
    result, warns = apply_stack_filter(mobile_template_content, ["typescript"])
    assert not warns
    assert "prettier --write" in result
    assert "dart format" not in result
    assert "swiftformat" not in result
    assert "ktlint" not in result


def test_mobile_template_dart_and_kotlin(mobile_template_content):
    """Flutter + Android project: Dart and Kotlin blocks kept."""
    result, warns = apply_stack_filter(mobile_template_content, ["dart", "kotlin"])
    assert not warns
    assert "dart format" in result
    assert "ktlint" in result
    assert "prettier --write" not in result
    assert "swiftformat" not in result
