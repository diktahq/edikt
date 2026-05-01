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
    with an indentation-aware state machine.

    Schema (ADR-008 + SPEC-005):
      Top level (indent 0)      — scalar | list | dict
      Lists                     — "  - <item>" (indent 2, dash continuation)
      Dicts (e.g. behavioral_signal) — indent 2 sub-keys, each scalar | list | dict
      Nested dict (e.g. refuse_edit_matching_frontmatter) — indent 4 scalar sub-keys

    SPEC-005 adds two optional top-level keys:
      canonical_phrases — list of strings
      behavioral_signal — dict containing refuse_to_write / refuse_tool / cite
                          (all lists of strings) and optionally
                          refuse_edit_matching_frontmatter (nested dict).

    Missing SPEC-005 fields default to [] and {} respectively. Never raise.
    """
    if not block_yaml:
        return {}

    result: dict = {}
    lines = block_yaml.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i].rstrip()

        # Skip blank lines
        if not line.strip():
            i += 1
            continue

        # Only top-level (indent 0) lines are consumed here. Nested content is
        # consumed by _consume_nested_block() below when a top-level key maps
        # to a dict/list.
        if line.startswith(" "):
            # Orphaned indented line at top level — skip (defensive).
            i += 1
            continue

        # Empty list shorthand: key: [] or key: [value, ...]
        m_empty = re.match(r'^(\w[\w_-]*):\s*\[(.*)\]\s*$', line)
        if m_empty:
            key = m_empty.group(1)
            inner = m_empty.group(2).strip()
            result[key] = [t.strip() for t in inner.split(",")] if inner else []
            i += 1
            continue

        # Empty dict shorthand: key: {}
        m_edict = re.match(r'^(\w[\w_-]*):\s*\{\s*\}\s*$', line)
        if m_edict:
            result[m_edict.group(1)] = {}
            i += 1
            continue

        # Scalar: key: value
        m_scalar = re.match(r'^(\w[\w_-]*):\s+(.+)$', line)
        if m_scalar:
            key, value = m_scalar.group(1), m_scalar.group(2).strip().strip('"')
            result[key] = value
            i += 1
            continue

        # List or dict header: key: (no value; nested content follows)
        m_header = re.match(r'^(\w[\w_-]*):\s*$', line)
        if m_header:
            key = m_header.group(1)
            nested, consumed = _consume_nested_block(lines, i + 1, base_indent=2)
            result[key] = nested
            i += 1 + consumed
            continue

        i += 1

    # SPEC-005 backward-compatibility defaults. Missing = empty.
    result.setdefault("canonical_phrases", [])
    result.setdefault("behavioral_signal", {})

    return result


def _consume_nested_block(
    lines: list[str],
    start: int,
    base_indent: int,
) -> tuple[list | dict, int]:
    """Consume indented content belonging to a parent key at the given indent.

    Returns (parsed_value, number_of_lines_consumed).

    Detects list vs dict shape from the first non-blank indented line:
      "<spaces>- " → list
      "<spaces>key:" → dict

    Recurses one level deeper for dict values that are themselves lists or dicts.
    """
    # Peek first non-blank line to determine shape
    idx = start
    while idx < len(lines) and not lines[idx].strip():
        idx += 1

    if idx == len(lines):
        return [], idx - start

    first = lines[idx]
    indent = len(first) - len(first.lstrip(" "))
    if indent < base_indent:
        return [], idx - start  # Nothing indented belongs here

    stripped = first.lstrip(" ")
    if stripped.startswith("- "):
        return _consume_list(lines, start, base_indent)
    return _consume_dict(lines, start, base_indent)


def _consume_list(lines: list[str], start: int, base_indent: int) -> tuple[list, int]:
    """Consume a list body indented at base_indent with '- ' items."""
    result: list = []
    prefix = " " * base_indent + "- "
    i = start
    while i < len(lines):
        raw = lines[i]
        line = raw.rstrip()
        if not line.strip():
            i += 1
            continue
        # Break on dedent
        if not (line.startswith(prefix) or line.startswith(" " * base_indent)):
            break
        if line.startswith(prefix):
            item = line[len(prefix):].strip().strip('"')
            result.append(item)
            i += 1
            continue
        # Hit a more-indented or non-list-item line within nesting → stop
        break
    return result, i - start


def _consume_dict(lines: list[str], start: int, base_indent: int) -> tuple[dict, int]:
    """Consume a dict body indented at base_indent with 'key: value' entries."""
    result: dict = {}
    indent_spaces = " " * base_indent
    i = start
    while i < len(lines):
        raw = lines[i]
        line = raw.rstrip()
        if not line.strip():
            i += 1
            continue
        # Break on dedent (anything less indented than our base)
        line_indent = len(raw) - len(raw.lstrip(" "))
        if line_indent < base_indent:
            break
        # Must start at our base indent to be a direct child
        if line_indent != base_indent:
            break

        content = line[base_indent:]

        # Empty list shorthand
        m_empty = re.match(r'^(\w[\w_-]*):\s*\[(.*)\]\s*$', content)
        if m_empty:
            key = m_empty.group(1)
            inner = m_empty.group(2).strip()
            result[key] = [t.strip() for t in inner.split(",")] if inner else []
            i += 1
            continue

        # Empty dict shorthand
        m_edict = re.match(r'^(\w[\w_-]*):\s*\{\s*\}\s*$', content)
        if m_edict:
            result[m_edict.group(1)] = {}
            i += 1
            continue

        # Scalar
        m_scalar = re.match(r'^(\w[\w_-]*):\s+(.+)$', content)
        if m_scalar:
            key, value = m_scalar.group(1), m_scalar.group(2).strip().strip('"')
            result[key] = value
            i += 1
            continue

        # Nested list or dict
        m_header = re.match(r'^(\w[\w_-]*):\s*$', content)
        if m_header:
            key = m_header.group(1)
            nested, consumed = _consume_nested_block(lines, i + 1, base_indent=base_indent + 2)
            result[key] = nested
            i += 1 + consumed
            continue

        # Unrecognized — break to avoid infinite loop
        break
    return result, i - start


# SPEC-005 Phase 5 — path-traversal rejection (AC-5.6)

_PATH_TRAVERSAL_PATTERNS = ("..", "~/")


def validate_behavioral_signal(signal: dict) -> list[str]:
    """Return a list of validation errors for a behavioral_signal dict.

    Empty list = valid. Non-empty = caller should treat as a parse error with
    one message per offending entry. Protects against path-traversal inputs
    that would let an ADR author weaponize the benchmark runner (SPEC-005
    security review, critical finding #2).
    """
    errors: list[str] = []
    refuse_to_write = signal.get("refuse_to_write") or []
    for entry in refuse_to_write:
        if not isinstance(entry, str):
            continue
        if entry.startswith("/"):
            errors.append(
                f"behavioral_signal.refuse_to_write entry {entry!r} uses an absolute path; "
                "substring matching only — paths must be relative substrings."
            )
            continue
        for pat in _PATH_TRAVERSAL_PATTERNS:
            if pat in entry:
                errors.append(
                    f"behavioral_signal.refuse_to_write entry {entry!r} contains {pat!r}; "
                    "path-traversal metacharacters are rejected."
                )
                break
    return errors


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
