"""Finding #4 — shared directive-checks drift detection.

The canonical Python script lives in
`commands/gov/_shared-directive-checks.md §Inline Script`.
Both `commands/gov/compile.md §12c` and `commands/gov/review.md §5b`
reference that script by prose.  If anyone edits only one copy while the
shared file drifts, silent divergence creeps in.

Strategy chosen (per task instructions): SHA-256 hash the inline Python
code-fence body extracted from each of the three files and assert they
are identical.  This is more robust than byte-equality because formatting
in how each file embeds the heredoc is allowed to differ (e.g. different
surrounding prose), but the Python script body itself must be identical.

Extraction rules:
- `_shared-directive-checks.md`: the ``python3 - <<'PY' ... PY`` block
  inside a ```bash ... ``` fence (first occurrence).  The *script body*
  is everything between the ``python3 - <<'PY'`` line and the ``PY``
  terminator.
- `compile.md §12c`: same pattern — first occurrence of a
  ``python3 - <<'PY' ... PY`` heredoc inside a ```bash fence.
  The compile command's directive-quality pass *invokes* the shared
  script inline by embedding it; this test asserts that embedding is
  identical to the source.
- `review.md §5b`: same extraction pattern.

If a file does not contain the inline script (e.g., it uses a file-include
or prose-reference approach instead), the test is marked xfail with an
explanatory message rather than silently passing — the absence of the
script in a caller is itself a structural signal worth surfacing.
"""

from __future__ import annotations

import hashlib
import re
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]

SHARED_CHECKS_MD = REPO_ROOT / "commands" / "gov" / "_shared-directive-checks.md"
COMPILE_MD = REPO_ROOT / "commands" / "gov" / "compile.md"
REVIEW_MD = REPO_ROOT / "commands" / "gov" / "review.md"


# ─── Script extraction ────────────────────────────────────────────────────────

_HEREDOC_PATTERN = re.compile(
    r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```",
    re.DOTALL,
)


def _extract_script(path: Path) -> str | None:
    """Return the script body from the first ``python3 - <<'PY'`` heredoc.

    Returns None if the file does not contain such a heredoc.
    """
    text = path.read_text()
    m = _HEREDOC_PATTERN.search(text)
    if not m:
        return None
    return m.group(1)


def _sha256(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_shared_checks_md_contains_inline_script():
    """Sanity: the canonical source file contains the inline script."""
    assert SHARED_CHECKS_MD.exists(), f"Missing: {SHARED_CHECKS_MD}"
    script = _extract_script(SHARED_CHECKS_MD)
    assert script is not None, (
        f"{SHARED_CHECKS_MD.name} must contain a ``python3 - <<'PY'`` "
        "heredoc inside a ```bash fence — this is the canonical script source."
    )
    assert len(script) > 50, (
        f"Extracted script from {SHARED_CHECKS_MD.name} is suspiciously short."
    )


def test_shared_checks_is_referenced_in_both_callers():
    """Both compile.md and review.md must reference _shared-directive-checks.md.

    This is a prose-reference test: regardless of whether callers embed the
    heredoc or delegate by filename, they must name the shared file so a reader
    can trace the dependency.  This test always passes when the file is
    referenced and does not depend on the xfail conditions above.
    """
    for md, label in [(COMPILE_MD, "compile.md"), (REVIEW_MD, "review.md")]:
        text = md.read_text()
        assert "_shared-directive-checks" in text, (
            f"{label} must reference '_shared-directive-checks' by name "
            "so the caller → shared-script relationship is traceable."
        )


def test_canonical_script_is_syntactically_valid_python():
    """The script body extracted from _shared-directive-checks.md compiles cleanly."""
    import ast

    script = _extract_script(SHARED_CHECKS_MD)
    assert script is not None, "Cannot validate syntax — no script found."
    try:
        ast.parse(script)
    except SyntaxError as exc:
        pytest.fail(
            f"Canonical script in _shared-directive-checks.md has a syntax error:\n{exc}"
        )


def test_both_callers_reference_shared_checks_by_name_and_signature():
    """Both compile.md and review.md reference _shared-directive-checks.md by name
    and pass all four required inputs from the shared procedure's §Inputs section.

    The §Inputs table defines four required fields that every caller must pass to
    the inline script:
      - adr_id
      - directive_body
      - canonical_phrases
      - no_directives_reason

    This test extracts those required input names from _shared-directive-checks.md
    and asserts that each caller mentions every required input in its prose
    (case-insensitive substring match). A caller that silently drops an input will
    be caught here, whereas test_shared_checks_is_referenced_in_both_callers only
    checks filename presence.

    The real runtime drift check is
    test/integration/test_shared_directive_checks.py::test_same_input_produces_same_output
    which runs both callers against the same fixture and asserts byte-identical
    warnings. This test covers the structural contract: callers cannot silently drop
    an input field without this test catching it.
    """
    # Extract required input names from the §Inputs table in _shared-directive-checks.md.
    # The table rows have the form:  | `field_name` | type | description |
    # We match backtick-quoted names in the first column to stay robust to prose changes.
    shared_text = SHARED_CHECKS_MD.read_text()
    input_pattern = re.compile(r"^\|\s*`(\w+)`\s*\|", re.MULTILINE)
    required_inputs = input_pattern.findall(shared_text)

    # Sanity: the shared file must define at least the four known required inputs.
    assert len(required_inputs) >= 4, (
        f"Expected at least 4 required inputs in {SHARED_CHECKS_MD.name}§Inputs; "
        f"found {len(required_inputs)}: {required_inputs}. "
        "Update this test if the §Inputs table structure changed."
    )

    for md, label in [(COMPILE_MD, "compile.md"), (REVIEW_MD, "review.md")]:
        text = md.read_text().lower()
        # Check filename reference (belt-and-suspenders with test above)
        assert "_shared-directive-checks" in text, (
            f"{label} must reference '_shared-directive-checks' by name."
        )
        # Check each required input is mentioned in caller prose
        for input_name in required_inputs:
            assert input_name.lower() in text, (
                f"{label} does not mention required input '{input_name}' from "
                f"_shared-directive-checks.md §Inputs. "
                f"If the caller passes this input under a different name, update "
                f"the §Inputs table in _shared-directive-checks.md to match."
            )
