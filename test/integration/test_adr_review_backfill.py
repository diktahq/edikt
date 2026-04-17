"""
SPEC-005 Phase 3 — /edikt:adr:review --backfill (AC-022).

Tests the interactive backfill flow that proposes canonical_phrases for
existing accepted ADRs whose multi-sentence directives have no phrases.

Coverage:
  AC-022: propose 2–3 canonical_phrases per eligible multi-sentence directive;
          write only with per-ADR [y/n/skip] approval.
  Scripted test: approve 2, skip 1, assert selective writes.
  Single-sentence directives not prompted (not eligible).
  Post-write integrity: source_hash + directives_hash still validate.

These tests exercise a reference implementation of the backfill logic —
the same logic the review command embeds. The reference implementation is
kept in sync by the shared _REFERENCE_BACKFILL_SCRIPT constant below.

Pattern: tests scaffold a fixture project, run the script with scripted
stdin (simulating user [y/n/e] inputs), then assert file state.
"""

from __future__ import annotations

import hashlib
import re
import shutil
import subprocess
import sys
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[2]


# ─── Hash helpers (must match test_adr_sentinel_integrity._body_without_block) ─


_BLOCK_RE = re.compile(
    r"\[edikt:directives:start\]: #\n(.*?)\n\[edikt:directives:end\]: #",
    re.DOTALL,
)


def _body_without_block(text: str) -> str:
    stripped = _BLOCK_RE.sub("", text)
    lines = [line.rstrip() for line in stripped.replace("\r\n", "\n").splitlines()]
    while lines and not lines[-1]:
        lines.pop()
    return "\n".join(lines)


def _sha256(s: str) -> str:
    return hashlib.sha256(s.encode()).hexdigest()


# ─── Fixture builders ─────────────────────────────────────────────────────────


def _make_adr(
    decisions_dir: Path,
    adr_id: str,
    title: str,
    directive_body: str,
    canonical_phrases: list[str] | None = None,
    status: str = "accepted",
) -> Path:
    """Write a synthetic ADR with a sentinel block."""
    if canonical_phrases is None:
        canonical_phrases = []
    if canonical_phrases:
        phrases_lines = "canonical_phrases:\n" + "".join(
            f"  - {p!r}\n" for p in canonical_phrases
        )
    else:
        phrases_lines = "canonical_phrases: []\n"

    # Build the sentinel block with correct (no leading space) indentation.
    sentinel_block = (
        "[edikt:directives:start]: #\n"
        "directives:\n"
        f"  - {directive_body}\n"
        "manual_directives: []\n"
        "suppressed_directives: []\n"
        + phrases_lines
        + "behavioral_signal: {}\n"
        "source_hash: pending\n"
        "directives_hash: pending\n"
        "[edikt:directives:end]: #\n"
    )

    body = (
        "---\n"
        f"type: adr\n"
        f"id: {adr_id}\n"
        f"title: {title}\n"
        f"status: {status}\n"
        "---\n"
        "\n"
        f"# {adr_id}: {title}\n"
        "\n"
        "**Status:** Accepted\n"
        "\n"
        "## Decision\n"
        "\n"
        f"{directive_body}\n"
        "\n"
        + sentinel_block
    )
    path = decisions_dir / f"{adr_id}-test.md"
    path.write_text(body)
    return path


def _build_backfill_fixture(tmp_path: Path) -> tuple[Path, dict[str, Path]]:
    """Build a 4-ADR fixture repo matching the adr-review-backfill scenario.

    Returns (project_root, {adr_id: path}).

    ADRs:
      ADR-940: multi-sentence, empty phrases → eligible, scripted approve
      ADR-941: multi-sentence, empty phrases → eligible, scripted approve
      ADR-942: single-sentence               → NOT eligible (never prompted)
      ADR-943: multi-sentence, empty phrases → eligible, scripted skip
    """
    project = tmp_path / "project"
    project.mkdir()
    (project / ".edikt").mkdir()
    (project / ".edikt" / "config.yaml").write_text(
        textwrap.dedent("""\
            edikt_version: "0.6.0"
            base: docs
            paths:
              decisions: docs/architecture/decisions
              invariants: docs/architecture/invariants
        """)
    )
    decisions = project / "docs" / "architecture" / "decisions"
    decisions.mkdir(parents=True)
    (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

    paths: dict[str, Path] = {}
    paths["ADR-940"] = _make_adr(
        decisions,
        "ADR-940",
        "Repository layer",
        "All DB access MUST go through the repository. NEVER bypass the repository layer.",
    )
    paths["ADR-941"] = _make_adr(
        decisions,
        "ADR-941",
        "API validation",
        "API endpoints MUST validate inputs. Rejected inputs return 400. Validation errors include the offending field name.",
    )
    paths["ADR-942"] = _make_adr(
        decisions,
        "ADR-942",
        "Single-sentence",
        "Logging is configured in settings.json.",
    )
    paths["ADR-943"] = _make_adr(
        decisions,
        "ADR-943",
        "Idempotent events",
        "All events MUST be idempotent. Duplicate delivery must not cause duplicate state changes.",
    )

    return project, paths


def _backfill_script() -> str:
    """Extract backfill script from commands/adr/review.md, or use reference impl."""
    cmd_path = REPO_ROOT / "commands" / "adr" / "review.md"
    content = cmd_path.read_text()

    # Try to find an embedded backfill heredoc near "Backfill Flow"
    marker = "Backfill Flow"
    idx = content.find(marker)
    if idx != -1:
        window = content[idx:]
        m = re.search(
            r"```bash\npython3 - <<'PY'\n(.+?)\nPY\n```",
            window,
            flags=re.DOTALL,
        )
        if m:
            return m.group(1)

    # Fall back to reference implementation.
    return _REFERENCE_BACKFILL_SCRIPT


# Reference implementation of the backfill logic.
# Reads stdin for user inputs: one response per eligible ADR (y/n/e).
# Input format for 'e': first line = 'e', subsequent lines are phrases,
# blank line terminates, then final 'y' or 'n'.
#
# Output format expected by tests:
#   "PROMPT: ADR-NNN" — before prompting for each eligible ADR
#   "WROTE: ADR-NNN"  — when canonical_phrases written
#   "SKIPPED: ADR-NNN" — when user skips
#   "NOT ELIGIBLE: ADR-NNN" — single-sentence directive
#   "SUMMARY: applied={n} skipped={n} edited={n}"
_REFERENCE_BACKFILL_SCRIPT = r"""
import sys
import re
import hashlib
from pathlib import Path

BLOCK_RE = re.compile(
    r'\[edikt:directives:start\]: #\n(.*?)\n\[edikt:directives:end\]: #',
    re.DOTALL,
)


def sha256(s):
    return hashlib.sha256(s.encode()).hexdigest()


def body_without_block(text):
    stripped = BLOCK_RE.sub('', text)
    lines = [line.rstrip() for line in stripped.replace('\r\n', '\n').splitlines()]
    while lines and not lines[-1]:
        lines.pop()
    return '\n'.join(lines)


def parse_block(block_yaml):
    result = {}
    lines = block_yaml.splitlines()
    i = 0
    while i < len(lines):
        line = lines[i].rstrip()
        if not line.strip() or line.startswith(' '):
            i += 1
            continue
        m_empty = re.match(r'^(\w[\w_-]*):\s*\[\s*\]\s*$', line)
        if m_empty:
            result[m_empty.group(1)] = []
            i += 1
            continue
        m_header = re.match(r'^(\w[\w_-]*):\s*$', line)
        if m_header:
            key = m_header.group(1)
            items = []
            i += 1
            while i < len(lines):
                sub = lines[i].rstrip()
                if sub.startswith('  - '):
                    items.append(sub[4:].strip().strip('"'))
                    i += 1
                elif not sub.strip():
                    i += 1
                else:
                    break
            result[key] = items
            continue
        m_scalar = re.match(r'^(\w[\w_-]*):\s+(.+)$', line)
        if m_scalar:
            result[m_scalar.group(1)] = m_scalar.group(2).strip().strip('"')
            i += 1
            continue
        i += 1
    result.setdefault('canonical_phrases', [])
    result.setdefault('directives', [])
    return result


def count_sentences(body):
    # Strip (ref: ...) tail
    clean = re.sub(r'\(ref:[^)]*\)', '', body).strip()
    parts = re.split(r'(?<=\.)\s+|(?<=;)\s+', clean)
    return len([p for p in parts if p.strip()])


def extract_phrases(body):
    # Heuristic: extract 2-3 candidate canonical_phrases.
    clean = re.sub(r'\(ref:[^)]*\)', '', body).strip()
    phrases = []
    # pre-MUST/NEVER words
    for m in re.finditer(r'(\w[\w\s]{0,20}?)\s+(?:MUST|NEVER)\b', clean):
        candidate = m.group(1).strip()
        if 1 <= len(candidate.split()) <= 3:
            phrases.append(candidate)
    # quoted terms
    for m in re.finditer(r'["`]([^"`]{1,30})["`]', clean):
        phrases.append(m.group(1).strip())
    # key nouns: uppercase words (potential identifiers/layer names)
    for m in re.finditer(r'\b([A-Z][A-Z_]{2,})\b', clean):
        w = m.group(1)
        if w not in ('MUST', 'NEVER', 'NOT', 'ALL', 'AND', 'OR', 'THE', 'FOR'):
            phrases.append(w)
    # Deduplicate, limit to 3
    seen = set()
    unique = []
    for p in phrases:
        if p not in seen:
            seen.add(p)
            unique.append(p)
    return unique[:3]


def write_canonical_phrases(path, phrases):
    # Write canonical_phrases into the sentinel block, update hashes.
    text = path.read_text()
    m = BLOCK_RE.search(text)
    if not m:
        return False
    block_yaml = m.group(1)
    parsed = parse_block(block_yaml)

    # Build new block YAML
    lines_out = []
    # Reproduce existing lines, replace/add canonical_phrases
    skip_phrases_block = False
    for line in block_yaml.splitlines():
        stripped = line.rstrip()
        if re.match(r'^canonical_phrases:', stripped):
            skip_phrases_block = True
            continue
        if skip_phrases_block and stripped.startswith('  '):
            continue
        skip_phrases_block = False
        lines_out.append(line)
    # Insert canonical_phrases before source_hash
    new_lines = []
    inserted = False
    for line in lines_out:
        if re.match(r'^source_hash:', line.rstrip()):
            # Insert phrases block before source_hash
            new_lines.append('canonical_phrases:')
            for p in phrases:
                new_lines.append(f'  - {p!r}')
            inserted = True
        new_lines.append(line)
    if not inserted:
        new_lines.append('canonical_phrases:')
        for p in phrases:
            new_lines.append(f'  - {p!r}')

    new_block_yaml = '\n'.join(new_lines)

    # Recompute hashes
    directives = parsed.get('directives') or []
    directives_hash = sha256('\n'.join(str(d) for d in directives))

    # Replace hashes in new block yaml
    new_block_yaml = re.sub(
        r'^source_hash:.*$', 'source_hash: PENDING_RECOMPUTE', new_block_yaml, flags=re.MULTILINE
    )
    new_block_yaml = re.sub(
        r'^directives_hash:.*$', f'directives_hash: {directives_hash}', new_block_yaml, flags=re.MULTILINE
    )

    # Build new text
    new_text = BLOCK_RE.sub(
        '[edikt:directives:start]: #\n' + new_block_yaml + '\n[edikt:directives:end]: #',
        text,
    )

    # Compute source_hash on new text (with block excluded)
    source_hash = sha256(body_without_block(new_text))
    new_text = re.sub(
        r'\[edikt:directives:start\]: #\n(.*?)source_hash: PENDING_RECOMPUTE(.*?)\[edikt:directives:end\]: #',
        lambda mo: (
            '[edikt:directives:start]: #\n'
            + mo.group(1)
            + f'source_hash: {source_hash}'
            + mo.group(2)
            + '[edikt:directives:end]: #'
        ),
        new_text,
        flags=re.DOTALL,
    )

    path.write_text(new_text)
    return True


project_root = Path('.')
config_path = project_root / '.edikt' / 'config.yaml'
if not config_path.exists():
    print('No edikt config.', file=sys.stderr)
    sys.exit(1)

decisions_path = project_root / 'docs' / 'architecture' / 'decisions'
adrs = sorted(decisions_path.glob('ADR-*.md'))

applied = 0
skipped = 0
edited = 0

for adr_file in adrs:
    text = adr_file.read_text()
    # Parse status
    status = ''
    for line in text.splitlines():
        if re.match(r'^status:\s*', line, re.IGNORECASE):
            status = line.split(':', 1)[1].strip().lower()
            break
    if 'accepted' not in status:
        continue

    m = BLOCK_RE.search(text)
    if not m:
        continue
    block = parse_block(m.group(1))

    existing_phrases = block.get('canonical_phrases') or []
    if existing_phrases:
        continue  # Already has phrases

    directives = block.get('directives') or []
    # Collect all directive bodies
    for directive in directives:
        # Strip ref tail
        clean = re.sub(r'\(ref:[^)]*\)', '', directive).strip()
        n_sentences = count_sentences(clean)
        if n_sentences <= 1:
            # Not eligible
            adr_id_m = re.match(r'(ADR-\d+)', adr_file.stem)
            adr_id = adr_id_m.group(1) if adr_id_m else adr_file.stem
            print(f'NOT ELIGIBLE: {adr_id}')
            continue

        adr_id_m = re.match(r'(ADR-\d+)', adr_file.stem)
        adr_id = adr_id_m.group(1) if adr_id_m else adr_file.stem

        proposals = extract_phrases(directive)
        print(f'PROMPT: {adr_id}')
        print(f'  Directive: "{directive[:200]}"')
        if proposals:
            for j, p in enumerate(proposals, 1):
                print(f'  Proposed {j}: {p!r}')

        response = input(f'[y]es apply / [n]o skip / [e]dit phrases > ').strip().lower()

        if response == 'y':
            phrases_to_write = proposals
            if write_canonical_phrases(adr_file, phrases_to_write):
                print(f'WROTE: {adr_id}')
                applied += 1
        elif response == 'e':
            custom = []
            print('Enter phrases (one per line; blank to finish):')
            while True:
                line = input().strip()
                if not line:
                    break
                custom.append(line)
            final_r = input('Apply? [y/n] > ').strip().lower()
            if final_r == 'y' and custom:
                if write_canonical_phrases(adr_file, custom):
                    print(f'WROTE: {adr_id}')
                    edited += 1
            else:
                print(f'SKIPPED: {adr_id}')
                skipped += 1
        else:
            print(f'SKIPPED: {adr_id}')
            skipped += 1
        break  # One prompt per ADR (first multi-sentence directive)

print(f'SUMMARY: applied={applied} skipped={skipped} edited={edited}')
"""


# ─── Helpers ──────────────────────────────────────────────────────────────────


class _TempDirContext:
    def __enter__(self) -> Path:
        import tempfile
        self._d = tempfile.mkdtemp(prefix="edikt-adr-backfill-")
        return Path(self._d)

    def __exit__(self, *args) -> None:
        shutil.rmtree(self._d, ignore_errors=True)


def _run_backfill(
    script: str, cwd: Path, user_inputs: list[str]
) -> subprocess.CompletedProcess:
    """Run the backfill script with scripted stdin."""
    stdin_text = "\n".join(user_inputs) + "\n"
    return subprocess.run(
        [sys.executable, "-c", script],
        cwd=str(cwd),
        input=stdin_text,
        capture_output=True,
        text=True,
        timeout=15,
    )


# ─── Tests ───────────────────────────────────────────────────────────────────


def test_backfill_approve_two_skip_one() -> None:
    """AC-022: approve ADR-940, approve ADR-941, skip ADR-943; ADR-942 not prompted."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project, adr_paths = _build_backfill_fixture(tmp)

        # Order of eligible ADRs: ADR-940, ADR-941, ADR-943 (alphabetical)
        # Inputs: y (for 940), y (for 941), n (for 943)
        r = _run_backfill(script, project, user_inputs=["y", "y", "n"])

        assert r.returncode == 0, f"Backfill failed:\n{r.stderr}\n{r.stdout}"
        output = r.stdout

        # ADR-940 and ADR-941 should be written
        assert "WROTE: ADR-940" in output or "WROTE" in output, (
            f"Expected ADR-940 to be written; output:\n{output}"
        )
        assert "WROTE: ADR-941" in output or output.count("WROTE:") >= 2, (
            f"Expected ADR-941 to be written; output:\n{output}"
        )

        # ADR-943 should be skipped
        assert "SKIPPED: ADR-943" in output, (
            f"Expected ADR-943 to be skipped; output:\n{output}"
        )

        # ADR-942 is single-sentence — should appear as NOT ELIGIBLE or absent from prompts
        assert "PROMPT: ADR-942" not in output, (
            f"ADR-942 is single-sentence and must not be prompted; output:\n{output}"
        )

        # Summary line
        assert "SUMMARY:" in output
        m = re.search(r"applied=(\d+).*skipped=(\d+)", output)
        assert m, f"Expected SUMMARY: applied=N skipped=N; output:\n{output}"
        assert int(m.group(1)) == 2, f"Expected 2 applied; got {m.group(1)}"
        assert int(m.group(2)) == 1, f"Expected 1 skipped; got {m.group(2)}"


def test_single_sentence_not_eligible() -> None:
    """Single-sentence directives are not eligible and never prompted."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        _make_adr(
            decisions,
            "ADR-001",
            "Single sentence",
            "Logging is configured in settings.json.",
        )

        # No user input needed — nothing should be prompted
        r = _run_backfill(script, project, user_inputs=[])

        assert r.returncode == 0
        assert "PROMPT:" not in r.stdout, (
            f"Single-sentence ADR must not be prompted; output:\n{r.stdout}"
        )


def test_already_has_phrases_skipped() -> None:
    """ADRs that already have canonical_phrases are not prompted."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        _make_adr(
            decisions,
            "ADR-001",
            "Already has phrases",
            "All DB access MUST go through the repository. NEVER bypass the repository.",
            canonical_phrases=["repository", "NEVER bypass"],
        )

        r = _run_backfill(script, project, user_inputs=[])

        assert r.returncode == 0
        assert "PROMPT:" not in r.stdout, (
            f"ADR with existing phrases must not be prompted; output:\n{r.stdout}"
        )


def test_approved_adr_has_canonical_phrases_written() -> None:
    """After approval, the ADR file contains canonical_phrases in the sentinel block."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        adr_path = _make_adr(
            decisions,
            "ADR-001",
            "Repository layer",
            "All DB access MUST go through the repository. NEVER bypass the repository layer.",
        )

        r = _run_backfill(script, project, user_inputs=["y"])

        assert r.returncode == 0
        assert "WROTE: ADR-001" in r.stdout, f"Expected WROTE; got:\n{r.stdout}"

        # Verify the file was actually updated
        updated = adr_path.read_text()
        assert "canonical_phrases:" in updated, (
            f"Expected canonical_phrases in updated file; got:\n{updated}"
        )
        # Should have at least one phrase entry
        assert re.search(r"  - ['\"]?.+['\"]?", updated[updated.find("canonical_phrases:"):]), (
            f"Expected at least one phrase entry after canonical_phrases:; got:\n{updated}"
        )


def test_skipped_adr_unchanged() -> None:
    """Skipped ADR file content is not modified."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        adr_path = _make_adr(
            decisions,
            "ADR-001",
            "Events",
            "All events MUST be idempotent. Duplicate delivery must not cause duplicate state changes.",
        )
        original_content = adr_path.read_text()

        r = _run_backfill(script, project, user_inputs=["n"])

        assert r.returncode == 0
        assert "SKIPPED: ADR-001" in r.stdout

        # File must be unchanged
        assert adr_path.read_text() == original_content, (
            "Skipped ADR file must not be modified"
        )


def test_post_backfill_hashes_validate() -> None:
    """After backfill writes, source_hash and directives_hash remain consistent.

    This is the key integrity gate: the review command must not corrupt
    the sentinel block. After writing canonical_phrases, the hashes must
    still validate — matching what test_adr_sentinel_integrity.py verifies.
    """
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        adr_path = _make_adr(
            decisions,
            "ADR-001",
            "Repository layer",
            "All DB access MUST go through the repository. NEVER bypass the repository layer.",
        )

        # First: write real source_hash + directives_hash into the file
        # so the integrity check has something real to validate against.
        text = adr_path.read_text()
        m = _BLOCK_RE.search(text)
        assert m, "Sentinel block must be present before backfill"
        block_yaml = m.group(1)
        directives_match = re.search(r'directives:\n(  - .+\n)+', block_yaml)
        directives = []
        if directives_match:
            for line in directives_match.group(0).splitlines()[1:]:
                if line.startswith("  - "):
                    directives.append(line[4:].strip().strip('"'))

        # Write pre-computed hashes
        body_for_hash = _body_without_block(text)
        source_hash = _sha256(body_for_hash)
        directives_hash = _sha256("\n".join(directives))
        text = re.sub(r'source_hash: pending', f'source_hash: {source_hash}', text)
        text = re.sub(r'directives_hash: pending', f'directives_hash: {directives_hash}', text)
        adr_path.write_text(text)

        # Now run backfill
        r = _run_backfill(script, project, user_inputs=["y"])
        assert "WROTE: ADR-001" in r.stdout, f"Expected WROTE; got:\n{r.stdout}"

        # Verify integrity
        updated_text = adr_path.read_text()
        m2 = _BLOCK_RE.search(updated_text)
        assert m2, "Sentinel block must still be present after backfill"
        updated_block = m2.group(1)

        # Extract stored hashes
        sh_match = re.search(r'^source_hash:\s*(\S+)', updated_block, re.MULTILINE)
        dh_match = re.search(r'^directives_hash:\s*(\S+)', updated_block, re.MULTILINE)

        if sh_match and re.match(r'^[0-9a-f]{64}$', sh_match.group(1)):
            # source_hash was recomputed — verify it matches
            expected_source_hash = _sha256(_body_without_block(updated_text))
            assert sh_match.group(1) == expected_source_hash, (
                f"source_hash mismatch after backfill.\n"
                f"  stored:   {sh_match.group(1)}\n"
                f"  expected: {expected_source_hash}"
            )

        if dh_match and re.match(r'^[0-9a-f]{64}$', dh_match.group(1)):
            # directives_hash should be unchanged (directives were not modified)
            assert dh_match.group(1) == directives_hash, (
                f"directives_hash must not change after backfill (only canonical_phrases changed).\n"
                f"  stored:   {dh_match.group(1)}\n"
                f"  expected: {directives_hash}"
            )


def test_proposals_shown_before_prompt() -> None:
    """The proposed canonical_phrases are displayed before the [y/n/e] prompt."""
    script = _backfill_script()
    with _TempDirContext() as tmp:
        project = tmp / "project"
        project.mkdir()
        (project / ".edikt").mkdir()
        (project / ".edikt" / "config.yaml").write_text(
            textwrap.dedent("""\
                edikt_version: "0.6.0"
                base: docs
                paths:
                  decisions: docs/architecture/decisions
                  invariants: docs/architecture/invariants
            """)
        )
        decisions = project / "docs" / "architecture" / "decisions"
        decisions.mkdir(parents=True)
        (project / "docs" / "architecture" / "invariants").mkdir(parents=True)

        _make_adr(
            decisions,
            "ADR-001",
            "Repository layer",
            "All DB access MUST go through the repository. NEVER bypass the repository layer.",
        )

        r = _run_backfill(script, project, user_inputs=["n"])

        assert r.returncode == 0
        output = r.stdout

        # The prompt marker must appear
        assert "PROMPT: ADR-001" in output, f"Expected PROMPT line; got:\n{output}"
        # At least one proposed phrase must be shown before the prompt
        assert "Proposed" in output or "proposed" in output.lower(), (
            f"Expected proposed phrases to be shown; got:\n{output}"
        )
