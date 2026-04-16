"""test_init_hash_deterministic.py — Phase 9: hash determinism contract.

Verifies that compute_hash() produces a stable, reproducible md5 hex digest
for a given file, and that different files (or different content) produce
different hashes. This is the contract SPEC-004 §7 requires: "hashing the
same source template on any machine produces the same hash."
"""

import hashlib
from pathlib import Path

import pytest

from .conftest import compute_hash, AGENTS_DIR


def test_hash_is_hex_string():
    path = AGENTS_DIR / "backend.md"
    h = compute_hash(path)
    assert isinstance(h, str)
    assert len(h) == 32
    assert all(c in "0123456789abcdef" for c in h)


def test_hash_is_idempotent():
    """Same file → same hash on repeated calls."""
    path = AGENTS_DIR / "backend.md"
    assert compute_hash(path) == compute_hash(path)


def test_different_files_have_different_hashes():
    """backend.md and qa.md differ in content → different hashes."""
    h_backend = compute_hash(AGENTS_DIR / "backend.md")
    h_qa = compute_hash(AGENTS_DIR / "qa.md")
    assert h_backend != h_qa


def test_hash_matches_hashlib_md5(tmp_path):
    """compute_hash must agree with hashlib.md5 on the same bytes."""
    content = b"known test content for hash verification\n"
    f = tmp_path / "sample.md"
    f.write_bytes(content)
    expected = hashlib.md5(content).hexdigest()
    assert compute_hash(f) == expected


def test_hash_before_substitution_differs_from_after(tmp_path):
    """Changing content changes the hash — substitution must NOT be applied before hashing."""
    template = b"Read docs/architecture/decisions for context.\n"
    after_sub = b"Read custom/adr for context.\n"

    f_before = tmp_path / "before.md"
    f_after = tmp_path / "after.md"
    f_before.write_bytes(template)
    f_after.write_bytes(after_sub)

    assert compute_hash(f_before) != compute_hash(f_after)


def test_all_stack_agent_templates_have_unique_hashes():
    """backend, qa, frontend, mobile must all have distinct hashes (all differ)."""
    agents = ["backend", "qa", "frontend", "mobile"]
    hashes = [compute_hash(AGENTS_DIR / f"{a}.md") for a in agents]
    assert len(set(hashes)) == len(hashes), "Duplicate hashes among stack agents"


def test_hash_unchanged_by_read_operation():
    """Reading the file to compute the hash must not alter the file."""
    path = AGENTS_DIR / "backend.md"
    original_bytes = path.read_bytes()
    compute_hash(path)
    assert path.read_bytes() == original_bytes
