"""
Schema-completeness gate for /edikt:gov:compile §12a.

The gate is implemented as an inline Python script inside commands/gov/compile.md
§"Schema Completeness Gate". These tests extract that script and exercise it
directly (same pattern as test_compile_orphan_detection.py), so they:
  - run without SDK auth,
  - finish in milliseconds per case,
  - stay honest — they exercise the actual shipped script, not a test-local copy.

Cases:
  1. All blocks complete            → exit 0, no stdout
  2. One block missing source_hash  → exit 2, error names path + missing field
  3. Multiple blocks, multiple gaps → exit 2, all reported
  4. Empty input (no blocks)        → exit 0
  5. Legacy block (only content_hash, no v0.5.0+ fields) → exit 2
  6. Malformed JSON input           → exit 2 with diagnostic
  7. Singular vs plural pluralization in the error report
"""

from __future__ import annotations

import json
import os
import re
import subprocess
import sys
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]
COMPILE_MD = REPO_ROOT / "commands" / "gov" / "compile.md"


# ─── Script extraction ────────────────────────────────────────────────────────


def _extract_schema_script() -> str:
    """Extract the schema-completeness Python heredoc from compile.md §12a."""
    if not COMPILE_MD.exists():
        raise RuntimeError(f"commands/gov/compile.md not found at {COMPILE_MD}")

    content = COMPILE_MD.read_text()

    marker = "### Schema Completeness Gate"
    idx = content.find(marker)
    if idx == -1:
        raise RuntimeError(
            f"Could not find '{marker}' marker in compile.md — §12a may not be shipped yet."
        )

    window = content[idx:]
    m = re.search(r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```", window, flags=re.DOTALL)
    if not m:
        raise RuntimeError(
            "Could not extract schema-completeness Python script from compile.md §12a"
        )
    return m.group(1)


# ─── Runner helper ────────────────────────────────────────────────────────────


def _run(blocks) -> subprocess.CompletedProcess:
    """Run the schema-completeness script with EDIKT_BLOCKS_JSON set to `blocks`.

    `blocks` may be a list (json-encoded for the env var) or a raw string
    (passed verbatim, useful for malformed-JSON cases).
    """
    script = _extract_schema_script()
    env = {**os.environ}
    if isinstance(blocks, str):
        env["EDIKT_BLOCKS_JSON"] = blocks
    else:
        env["EDIKT_BLOCKS_JSON"] = json.dumps(blocks)
    return subprocess.run(
        [sys.executable, "-c", script],
        env=env,
        capture_output=True,
        text=True,
        timeout=10,
    )


# ─── Required field set under ADR-008 ─────────────────────────────────────────

REQUIRED_FIELDS = [
    "source_hash",
    "directives_hash",
    "compiler_version",
    "manual_directives",
    "suppressed_directives",
]

# A "complete" block carries these + the merge-formula content fields.
COMPLETE_BLOCK_FIELDS = [
    "paths",
    "scope",
    "directives",
    *REQUIRED_FIELDS,
]


# ─── Test cases ───────────────────────────────────────────────────────────────


def test_all_blocks_complete_passes():
    """Every block has all five ADR-008 fields → exit 0, no stdout."""
    blocks = [
        {"path": "docs/architecture/decisions/ADR-001.md", "fields": COMPLETE_BLOCK_FIELDS},
        {"path": "docs/architecture/invariants/INV-001.md", "fields": COMPLETE_BLOCK_FIELDS},
    ]
    result = _run(blocks)
    assert result.returncode == 0, f"expected exit 0, got {result.returncode}\nstderr: {result.stderr}"
    assert result.stdout == ""


def test_empty_input_passes():
    """No blocks to validate → exit 0."""
    result = _run([])
    assert result.returncode == 0, f"expected exit 0, got {result.returncode}\nstderr: {result.stderr}"


def test_unset_env_var_passes():
    """Missing env var → treated as no blocks → exit 0."""
    script = _extract_schema_script()
    env = {**os.environ}
    env.pop("EDIKT_BLOCKS_JSON", None)
    result = subprocess.run(
        [sys.executable, "-c", script],
        env=env,
        capture_output=True,
        text=True,
        timeout=10,
    )
    assert result.returncode == 0


def test_one_block_missing_source_hash_fails():
    """Block missing source_hash → exit 2, path + field reported."""
    blocks = [
        {
            "path": "docs/architecture/decisions/ADR-042.md",
            "fields": ["paths", "scope", "directives", "directives_hash",
                       "compiler_version", "manual_directives", "suppressed_directives"],
        },
    ]
    result = _run(blocks)
    assert result.returncode == 2, f"expected exit 2, got {result.returncode}\nstdout: {result.stdout}"
    assert "ADR-042.md" in result.stdout
    assert "source_hash" in result.stdout
    assert "1 sentinel block" in result.stdout  # singular


def test_multiple_blocks_multiple_gaps_all_reported():
    """All incomplete blocks are listed, each with their own missing-field set."""
    blocks = [
        {
            "path": "a.md",
            "fields": ["directives", "manual_directives", "suppressed_directives", "compiler_version"],
            # missing: source_hash, directives_hash
        },
        {
            "path": "b.md",
            "fields": ["directives", "source_hash", "directives_hash", "manual_directives", "suppressed_directives"],
            # missing: compiler_version
        },
        {
            "path": "c.md",
            "fields": COMPLETE_BLOCK_FIELDS,
            # missing: nothing
        },
    ]
    result = _run(blocks)
    assert result.returncode == 2
    assert "2 sentinel blocks" in result.stdout  # plural
    assert "a.md: missing [source_hash, directives_hash]" in result.stdout
    assert "b.md: missing [compiler_version]" in result.stdout
    assert "c.md" not in result.stdout  # complete blocks not reported


def test_legacy_v02x_block_with_only_content_hash_fails():
    """A legacy block carrying only content_hash (deprecated) is flagged as incomplete."""
    blocks = [
        {
            "path": "docs/architecture/decisions/ADR-001.md",
            "fields": ["paths", "scope", "directives", "content_hash"],
        },
    ]
    result = _run(blocks)
    assert result.returncode == 2
    # All five required fields should be in the missing list.
    for field in REQUIRED_FIELDS:
        assert field in result.stdout, f"expected {field!r} in error output"


def test_redirect_message_names_per_artifact_compile():
    """Error must redirect to per-artifact compile commands (the architectural fix)."""
    blocks = [
        {"path": "docs/architecture/decisions/ADR-001.md", "fields": []},
    ]
    result = _run(blocks)
    assert result.returncode == 2
    assert "/edikt:adr:compile" in result.stdout
    assert "/edikt:invariant:compile" in result.stdout
    assert "/edikt:guideline:compile" in result.stdout


def test_malformed_json_input_fails_with_diagnostic():
    """Bad JSON → exit 2, stderr names the parse error."""
    result = _run("{ this is not json }")
    assert result.returncode == 2
    assert "not valid JSON" in result.stderr


def test_blocks_must_be_list():
    """A JSON object instead of a list → exit 2."""
    result = _run('{"path": "a.md", "fields": []}')  # object, not array
    assert result.returncode == 2
    assert "must be a JSON array" in result.stderr


def test_block_entry_must_be_object():
    """Non-object block entry → exit 2."""
    result = _run('["not-an-object"]')
    assert result.returncode == 2
    assert "not a JSON object" in result.stderr


def test_fields_must_be_list():
    """`fields` not a list → exit 2."""
    result = _run('[{"path": "a.md", "fields": "source_hash"}]')
    assert result.returncode == 2
    assert "must be a list" in result.stderr


def test_required_field_set_matches_adr_008():
    """Sanity: hard-coded REQUIRED set in test must match ADR-008 directly.

    If this test starts failing, ADR-008 changed the schema and the gate
    + the test list need a coordinated update.
    """
    adr_path = REPO_ROOT / "docs" / "architecture" / "decisions" / "ADR-008-deterministic-compile-and-three-list-schema.md"
    if not adr_path.exists():
        pytest.skip("ADR-008 not present in this checkout")
    body = adr_path.read_text()
    for field in REQUIRED_FIELDS:
        assert field in body, f"REQUIRED_FIELDS lists {field!r} but ADR-008 body does not mention it"
