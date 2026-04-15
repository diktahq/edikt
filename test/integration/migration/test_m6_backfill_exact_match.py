"""Phase 11 — M6 backfill: exact match stamps provenance frontmatter.

Scenario:
  - Agent installed at v0.4.3 (edikt_version in frontmatter, no hash)
  - Fixture template bytes match the installed file (no user edits)
  - Levenshtein distance == 0  →  near-match
  - After backfill: edikt_template_hash + edikt_template_version in frontmatter
  - events.jsonl contains a provenance_backfilled entry
"""

from __future__ import annotations

import hashlib
import json
import os
import subprocess
import textwrap
from pathlib import Path

import pytest

REPO_ROOT = Path(__file__).resolve().parents[3]
EDIKT_BIN = REPO_ROOT / "bin" / "edikt"


def _write(path: Path, content: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(content)


def _md5(content: str) -> str:
    return hashlib.md5(content.encode()).hexdigest()


# ── Fixture template content ─────────────────────────────────────────────────
# Must be large enough (≥ ~200 chars) that the difference introduced by
# `edikt_version:` in the installed frontmatter stays under the 15% threshold.

TEMPLATE_CONTENT = textwrap.dedent(
    """\
    ---
    name: test_agent
    description: "Test agent for Phase 11 M6 backfill — exact match"
    tools:
      - Read
      - Write
    maxTurns: 10
    edikt_version: "0.4.3"
    ---

    You are a test agent. Your role is to verify that provenance backfill
    works correctly when the installed file matches the re-synthesized template.

    ## Instructions

    - Read files before editing them
    - Write tests for every feature you implement
    - Document your reasoning before acting
    - Keep changes minimal and focused

    ## Constraints

    - Never delete files without explicit confirmation
    - Always validate inputs at system boundaries
    """
)


@pytest.fixture
def sandbox(sandbox_home, tmp_path):
    """Extend the standard migration sandbox with:
      - .claude/agents/test_agent.md  (installed, no hash, edikt_version=0.4.3)
      - edikt_home/migration-fixtures/v0.4.3/templates/agents/test_agent.md
      - edikt_home/config.yaml  (minimal — no custom paths/stack)
      - edikt_home/versions/0.5.0/  (required for current symlink)
    """
    sb = sandbox_home

    # Minimal versioned layout so doctor doesn't fail on symlink checks.
    versions = sb["edikt_home"] / "versions" / "0.5.0"
    (versions / "hooks").mkdir(parents=True, exist_ok=True)
    (versions / "templates" / "agents").mkdir(parents=True, exist_ok=True)
    _write(versions / "VERSION", "0.5.0\n")

    cur = sb["edikt_home"] / "current"
    cur.symlink_to(Path("versions") / "0.5.0")
    (sb["edikt_home"] / "hooks").symlink_to(Path("current") / "hooks")
    (sb["edikt_home"] / "templates").symlink_to(Path("current") / "templates")

    # Minimal config.yaml (no custom paths — substitution will be a no-op)
    _write(
        sb["edikt_home"] / "config.yaml",
        textwrap.dedent(
            """\
            edikt_version: "0.4.3"
            base: docs
            """
        ),
    )

    # Fixture template.
    fixture_path = (
        sb["edikt_home"]
        / "migration-fixtures"
        / "v0.4.3"
        / "templates"
        / "agents"
        / "test_agent.md"
    )
    _write(fixture_path, TEMPLATE_CONTENT)

    # Installed agent — same bytes as template (exact match), no provenance hash.
    agent_path = sb["claude_home"] / "agents" / "test_agent.md"
    _write(agent_path, TEMPLATE_CONTENT)

    return {**sb, "agent_path": agent_path, "fixture_path": fixture_path}


def run_doctor(sandbox: dict, *args: str, stdin_input: str = "") -> subprocess.CompletedProcess:
    env = {
        **sandbox["env"],
        "EDIKT_MIGRATION_FIXTURES_BASE": str(
            sandbox["edikt_home"] / "migration-fixtures"
        ),
    }
    return subprocess.run(
        [str(EDIKT_BIN), "doctor", "--backfill-provenance", *args],
        env=env,
        capture_output=True,
        text=True,
        input=stdin_input,
        timeout=60,
    )


def test_exact_match_stamps_frontmatter(sandbox):
    """Installed file matches re-synth → hash + version written to frontmatter."""
    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    agent_text = sandbox["agent_path"].read_text()
    assert "edikt_template_hash:" in agent_text, "hash not stamped"
    assert "edikt_template_version:" in agent_text, "version not stamped"

    # Hash must match md5 of raw template bytes.
    expected_hash = _md5(TEMPLATE_CONTENT)
    # Extract stamped hash from frontmatter.
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_hash:"):
            stamped_hash = line.split(":", 1)[1].strip().strip('"')
            assert stamped_hash == expected_hash, (
                f"hash mismatch: expected {expected_hash}, got {stamped_hash}"
            )
            break
    else:
        pytest.fail("edikt_template_hash line not found")

    # Version stamped is 0.4.3.
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_version:"):
            stamped_ver = line.split(":", 1)[1].strip().strip('"')
            assert stamped_ver == "0.4.3", f"version mismatch: {stamped_ver}"
            break
    else:
        pytest.fail("edikt_template_version line not found")


def test_exact_match_emits_provenance_backfilled_event(sandbox):
    """provenance_backfilled event written to events.jsonl after stamp."""
    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    events_path = sandbox["edikt_home"] / "events.jsonl"
    assert events_path.exists(), "events.jsonl not created"

    events = [json.loads(line) for line in events_path.read_text().splitlines() if line.strip()]
    backfilled = [e for e in events if e.get("event") == "provenance_backfilled"]
    assert backfilled, "no provenance_backfilled event found"
    ev = backfilled[0]
    assert ev.get("agent") == "test_agent"
    assert ev.get("version") == "0.4.3"


def test_dry_run_does_not_write(sandbox):
    """--dry-run lists the candidate but does not modify the file."""
    original = sandbox["agent_path"].read_text()
    proc = run_doctor(sandbox, "--dry-run")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    # File must be unchanged.
    assert sandbox["agent_path"].read_text() == original, "file was modified in dry-run"

    # Dry-run output should mention the candidate.
    combined = proc.stdout + proc.stderr
    assert "dry-run" in combined.lower() or "would stamp" in combined.lower()


def test_already_provenanced_skipped(sandbox):
    """Agent with existing edikt_template_hash is not touched."""
    # Pre-stamp the file with a dummy hash.
    original = sandbox["agent_path"].read_text()
    # Insert dummy hash into frontmatter.
    new_content = original.replace(
        "---\nname: test_agent",
        '---\nname: test_agent\nedikt_template_hash: "dummyhash"\nedikt_template_version: "0.4.3"',
    )
    sandbox["agent_path"].write_text(new_content)

    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    # File still has the dummy hash (not overwritten).
    result = sandbox["agent_path"].read_text()
    assert 'edikt_template_hash: "dummyhash"' in result
