"""test_init_stack_go_only.py — Phase 9: stack filter with a single-language stack.

Verifies that with stack=['go'], only go blocks are retained and all other
language blocks are removed from the installed agent.
"""

from .conftest import apply_stack_filter

# Minimal agent body with three stack blocks.
MULTI_LANG_TEMPLATE = """\
Preamble content.

<!-- edikt:stack:go -->
## File Formatting

- Go (*.go): `gofmt -w <file>`

Run the formatter immediately after each Write or Edit.
<!-- /edikt:stack -->

<!-- edikt:stack:typescript,javascript -->
## File Formatting

- TypeScript/JavaScript: `prettier --write <file>`

Run the formatter immediately after each Write or Edit.
<!-- /edikt:stack -->

<!-- edikt:stack:python -->
## File Formatting

- Python: `black <file>`

Run the formatter immediately after each Write or Edit.
<!-- /edikt:stack -->

Footer content.
"""


def test_go_block_retained():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go"])
    assert "gofmt -w" in result
    assert not warns


def test_typescript_block_removed_for_go_stack():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go"])
    assert "prettier --write" not in result


def test_python_block_removed_for_go_stack():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go"])
    assert "black <file>" not in result


def test_preamble_and_footer_preserved():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go"])
    assert "Preamble content." in result
    assert "Footer content." in result


def test_stack_markers_stripped_from_kept_block():
    result, warns = apply_stack_filter(MULTI_LANG_TEMPLATE, ["go"])
    assert "<!-- edikt:stack:go -->" not in result
    assert "<!-- /edikt:stack -->" not in result


def test_real_backend_template_go_only(backend_template_content):
    result, warns = apply_stack_filter(backend_template_content, ["go"])
    assert not warns
    assert "gofmt -w" in result
    assert "prettier --write" not in result
    assert "black" not in result
    assert "rustfmt" not in result
    assert "rubocop" not in result
    assert "php-cs-fixer" not in result
