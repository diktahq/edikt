"""Governance integrity — ADR and invariant sentinel block verification.

Every accepted ADR and active invariant must have a correctly compiled
[edikt:directives:start]: # sentinel block. Tests verify:

1. The sentinel block exists.
2. Required fields are present.
3. source_hash matches a SHA-256 of the artifact body (block excluded).
4. directives_hash matches a SHA-256 of the auto directives list.
5. The directives list is non-empty.

These tests run without ANTHROPIC_API_KEY — pure file parsing.

Why this matters: if a sentinel block is stale or hand-edited, the
governance compile will produce wrong directives. Claude will enforce
rules that were superseded or miss rules that were added. This is the
silent failure mode that invalidates the entire governance system.
"""

from __future__ import annotations

import hashlib
import re
from pathlib import Path
from typing import NamedTuple

import pytest
import yaml

REPO_ROOT = Path(__file__).resolve().parents[3]
DECISIONS_DIR = REPO_ROOT / "docs" / "architecture" / "decisions"
INVARIANTS_DIR = REPO_ROOT / "docs" / "architecture" / "invariants"

_BLOCK_RE = re.compile(
    r"\[edikt:directives:start\]: #\n(.*?)\n\[edikt:directives:end\]: #",
    re.DOTALL,
)


# ─── Helpers ─────────────────────────────────────────────────────────────────


class ArtifactInfo(NamedTuple):
    path: Path
    kind: str          # "adr" or "invariant"
    status: str        # "accepted" / "active" / "draft" / etc.
    body: str          # full file text
    block_yaml: str    # raw YAML inside sentinel, or ""
    block: dict        # parsed sentinel, or {}


def _parse_artifact(path: Path, kind: str) -> ArtifactInfo:
    text = path.read_text()
    status = ""
    for line in text.splitlines():
        if line.lower().startswith("**status:**"):
            status = line.split(":", 1)[1].strip().strip("*").lower()
            break
        if re.match(r"^status:\s*", line, re.IGNORECASE):
            status = line.split(":", 1)[1].strip().lower()
            break

    m = _BLOCK_RE.search(text)
    block_yaml = m.group(1) if m else ""
    block: dict = _parse_block(block_yaml)

    return ArtifactInfo(
        path=path,
        kind=kind,
        status=status,
        body=text,
        block_yaml=block_yaml,
        block=block,
    )


def _parse_block(block_yaml: str) -> dict:
    """Parse a sentinel block's YAML content robustly.

    yaml.safe_load fails on sentinel blocks because:
    - Directive strings contain '(ref: ADR-NNN)' — ': ' looks like a mapping
    - Directive strings contain backtick-quoted paths — ` can't start a token

    The sentinel format is well-defined and small. Parse it line-by-line
    with a simple state machine instead of relying on a full YAML parser.
    """
    if not block_yaml:
        return {}

    result: dict = {}
    current_key: str | None = None
    current_list: list | None = None

    for raw_line in block_yaml.splitlines():
        line = raw_line.rstrip()

        # List item continuation.
        if line.startswith("  - ") and current_list is not None:
            current_list.append(line[4:])
            continue

        # Empty list shorthand: key: [] or key: [value, ...]
        m_empty = re.match(r'^(\w[\w_-]*):\s*\[(.*)\]\s*$', line)
        if m_empty:
            key = m_empty.group(1)
            inner = m_empty.group(2).strip()
            result[key] = [i.strip() for i in inner.split(",")] if inner else []
            current_key = None
            current_list = None
            continue

        # Scalar key: value.
        m_scalar = re.match(r'^(\w[\w_-]*):\s+(.+)$', line)
        if m_scalar:
            key, value = m_scalar.group(1), m_scalar.group(2).strip().strip('"')
            result[key] = value
            current_key = None
            current_list = None
            continue

        # List header: key:\n  - ...
        m_list = re.match(r'^(\w[\w_-]*):\s*$', line)
        if m_list:
            key = m_list.group(1)
            result[key] = []
            current_key = key
            current_list = result[key]
            continue

    return result


def _body_without_block(text: str) -> str:
    """Strip the sentinel block (inclusive of markers) for hash computation.

    Per ADR-008: source_hash is SHA-256 of the body with the sentinel block
    excluded, normalized (CRLF→LF, trailing whitespace stripped per line).
    """
    stripped = _BLOCK_RE.sub("", text)
    lines = [line.rstrip() for line in stripped.replace("\r\n", "\n").splitlines()]
    while lines and not lines[-1]:
        lines.pop()
    return "\n".join(lines)


def _sha256(s: str) -> str:
    return hashlib.sha256(s.encode()).hexdigest()


def _accepted_adrs() -> list[ArtifactInfo]:
    artifacts = [_parse_artifact(p, "adr") for p in sorted(DECISIONS_DIR.glob("ADR-*.md"))]
    return [a for a in artifacts if "superseded" not in a.status]


def _active_invariants() -> list[ArtifactInfo]:
    return [
        _parse_artifact(p, "invariant")
        for p in sorted(INVARIANTS_DIR.glob("INV-*.md"))
    ]


def _all_enforceable() -> list[ArtifactInfo]:
    return _accepted_adrs() + _active_invariants()


# ─── Parametrized fixtures ────────────────────────────────────────────────────

@pytest.fixture(
    params=[a.path.name for a in _accepted_adrs()],
    ids=lambda n: n,
)
def accepted_adr(request: pytest.FixtureRequest) -> ArtifactInfo:
    return _parse_artifact(DECISIONS_DIR / request.param, "adr")


@pytest.fixture(
    params=[a.path.name for a in _active_invariants()],
    ids=lambda n: n,
)
def active_invariant(request: pytest.FixtureRequest) -> ArtifactInfo:
    return _parse_artifact(INVARIANTS_DIR / request.param, "invariant")


# ─── ADR tests ───────────────────────────────────────────────────────────────


def test_adr_has_sentinel_block(accepted_adr: ArtifactInfo) -> None:
    """Every accepted ADR must have a compiled sentinel block.

    Without a sentinel block there are no directives — the ADR exists in
    the repo but contributes nothing to governance enforcement.
    """
    assert accepted_adr.block_yaml, (
        f"{accepted_adr.path.name}: missing [edikt:directives:start]: # sentinel block. "
        "Run /edikt:adr:compile to generate it."
    )


def test_adr_sentinel_has_directives(accepted_adr: ArtifactInfo) -> None:
    """Accepted ADR sentinel must contain at least one auto directive."""
    if not accepted_adr.block_yaml:
        pytest.skip("no sentinel block — covered by test_adr_has_sentinel_block")
    directives = accepted_adr.block.get("directives") or []
    assert directives, (
        f"{accepted_adr.path.name}: sentinel block exists but directives: list is empty. "
        "Either compile produced nothing or the Decision section has no enforceable statements."
    )


def test_adr_sentinel_has_compiler_version(accepted_adr: ArtifactInfo) -> None:
    if not accepted_adr.block_yaml:
        pytest.skip("no sentinel block")
    if not accepted_adr.block.get("source_hash"):
        pytest.skip("pre-ADR-008 artifact — compiler_version not required")
    assert accepted_adr.block.get("compiler_version"), (
        f"{accepted_adr.path.name}: has source_hash but missing compiler_version. "
        "Run /edikt:adr:compile to refresh."
    )


def test_adr_source_hash_matches_body(accepted_adr: ArtifactInfo) -> None:
    """source_hash must match SHA-256 of the body with sentinel excluded.

    A mismatch means the ADR body was edited after the last compile.
    The directives may no longer reflect the current Decision section.
    Run /edikt:adr:compile to refresh.
    """
    if not accepted_adr.block_yaml:
        pytest.skip("no sentinel block")
    stored = accepted_adr.block.get("source_hash", "")
    if not stored:
        pytest.skip("source_hash not yet written — pre-ADR-008 artifact")
    if not re.match(r'^[0-9a-f]{64}$', stored):
        pytest.skip(f"source_hash is a placeholder ({stored!r}) — not yet compiled")

    body = _body_without_block(accepted_adr.body)
    computed = _sha256(body)
    assert stored == computed, (
        f"{accepted_adr.path.name}: source_hash mismatch.\n"
        f"  stored:   {stored}\n"
        f"  computed: {computed}\n"
        "The ADR body was edited after the last compile. "
        "Run /edikt:adr:compile to refresh the directives."
    )


def test_adr_directives_hash_matches(accepted_adr: ArtifactInfo) -> None:
    """directives_hash must match SHA-256 of the auto directives list.

    A mismatch means the directives list was hand-edited after compile.
    Per ADR-008, only manual_directives and suppressed_directives may be
    hand-edited — never the auto directives list.
    """
    if not accepted_adr.block_yaml:
        pytest.skip("no sentinel block")
    stored = accepted_adr.block.get("directives_hash", "")
    if not stored:
        pytest.skip("directives_hash not yet written — pre-ADR-008 artifact")
    if not re.match(r'^[0-9a-f]{64}$', stored):
        pytest.skip(f"directives_hash is a placeholder ({stored!r}) — not yet compiled")

    directives = accepted_adr.block.get("directives") or []
    computed = _sha256("\n".join(str(d) for d in directives))
    assert stored == computed, (
        f"{accepted_adr.path.name}: directives_hash mismatch.\n"
        f"  stored:   {stored}\n"
        f"  computed: {computed}\n"
        "The auto directives list was hand-edited. "
        "Move hand-edits to manual_directives: or re-run /edikt:adr:compile."
    )


# ─── Invariant tests ──────────────────────────────────────────────────────────


def test_invariant_has_sentinel_block(active_invariant: ArtifactInfo) -> None:
    assert active_invariant.block_yaml, (
        f"{active_invariant.path.name}: missing sentinel block. "
        "Run /edikt:invariant:compile to generate it."
    )


def test_invariant_sentinel_has_directives(active_invariant: ArtifactInfo) -> None:
    if not active_invariant.block_yaml:
        pytest.skip("no sentinel block")
    directives = active_invariant.block.get("directives") or []
    assert directives, (
        f"{active_invariant.path.name}: sentinel block has empty directives list."
    )


def test_invariant_directives_are_hard_constraints(active_invariant: ArtifactInfo) -> None:
    """Every invariant directive must use MUST or NEVER — no soft language.

    Invariants are non-negotiable. A directive that says 'should' or
    'prefer' in an invariant is a governance bug — it will not be enforced.
    """
    if not active_invariant.block_yaml:
        pytest.skip("no sentinel block")
    directives = active_invariant.block.get("directives") or []
    soft_words = {"should", "prefer", "consider", "try to", "ideally"}
    for directive in directives:
        text = str(directive).lower()
        violations = [w for w in soft_words if w in text]
        assert not violations, (
            f"{active_invariant.path.name}: invariant directive uses soft language "
            f"({violations!r}): {directive!r}\n"
            "Invariants require MUST or NEVER — rewrite with hard constraint language."
        )


def test_invariant_source_hash_matches_body(active_invariant: ArtifactInfo) -> None:
    if not active_invariant.block_yaml:
        pytest.skip("no sentinel block")
    stored = active_invariant.block.get("source_hash", "")
    if not stored:
        pytest.skip("source_hash not yet written")
    body = _body_without_block(active_invariant.body)
    computed = _sha256(body)
    assert stored == computed, (
        f"{active_invariant.path.name}: source_hash mismatch — body edited after last compile. "
        "Run /edikt:invariant:compile."
    )


# ─── Cross-artifact tests ─────────────────────────────────────────────────────


def test_no_adr_references_nonexistent_superseder() -> None:
    """An ADR that says 'Superseded by ADR-NNN' must point to an existing file."""
    existing = {p.name for p in DECISIONS_DIR.glob("ADR-*.md")}
    for adr in _accepted_adrs():
        for line in adr.body.splitlines():
            m = re.search(r"[Ss]uperseded by (ADR-\d+)", line)
            if m:
                ref_id = m.group(1)
                matches = [n for n in existing if ref_id in n]
                assert matches, (
                    f"{adr.path.name}: references '{ref_id}' as superseder "
                    f"but no matching ADR file found in {DECISIONS_DIR}."
                )


def test_all_ref_tags_point_to_known_artifacts() -> None:
    """Every (ref: ADR-NNN) or (ref: INV-NNN) in any directive must resolve."""
    adr_ids = {
        re.search(r"ADR-(\d+)", p.name).group(0)
        for p in DECISIONS_DIR.glob("ADR-*.md")
        if re.search(r"ADR-(\d+)", p.name)
    }
    inv_ids = {
        re.search(r"INV-(\d+)", p.name).group(0)
        for p in INVARIANTS_DIR.glob("INV-*.md")
        if re.search(r"INV-(\d+)", p.name)
    }
    known = adr_ids | inv_ids

    for artifact in _all_enforceable():
        directives = artifact.block.get("directives") or []
        for directive in directives:
            for ref in re.findall(r"\(ref: ((?:ADR|INV)-\d+)\)", str(directive)):
                assert ref in known, (
                    f"{artifact.path.name}: directive references '{ref}' "
                    f"but no matching file found. "
                    f"Known: {sorted(known)}"
                )
