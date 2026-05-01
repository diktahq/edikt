"""Phase 11 — M6 backfill: no version hint triggers user prompt.

Scenario A — user provides a valid version:
  - Agent has no edikt_version / version in frontmatter
  - Backfill prompts: "Which edikt version installed?"
  - User types "0.4.3" via stdin
  - Fixture exists for v0.4.3, installed content matches re-synth
  - Hash stamped, provenance_backfilled event written

Scenario B — user types "skip":
  - Prompt fires, user types "skip"
  - File unchanged, provenance_backfill_skipped event written

Scenario C — identical template bytes across two versions:
  - Template for v0.1.0 and v0.1.4 are byte-identical (same content)
  - Backfill prompts for disambiguation after detecting the duplicate
  - User picks v0.1.0 → edikt_template_version: "0.1.0" stamped
  - Hash is the same either way
"""

from __future__ import annotations

import hashlib
import json
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


TEMPLATE_CONTENT = textwrap.dedent(
    """\
    ---
    name: prompt_agent
    description: "Agent to test version prompt in M6 backfill"
    tools:
      - Read
      - Write
    maxTurns: 10
    ---

    You are a prompt agent used for testing.
    Your task is to verify that the version prompt fires correctly
    when no version hint is present in the frontmatter.

    ## Instructions

    - Follow all rules carefully
    - Emit events for every action
    - Never skip documentation steps
    """
)

# Installed content is the same as the template (near-match).
INSTALLED_CONTENT = TEMPLATE_CONTENT  # identical bytes → Levenshtein = 0


@pytest.fixture
def sandbox_base(sandbox_home):
    """Shared base setup: versioned layout + config + fixture."""
    sb = sandbox_home

    versions = sb["edikt_home"] / "versions" / "0.5.0"
    (versions / "hooks").mkdir(parents=True, exist_ok=True)
    (versions / "templates" / "agents").mkdir(parents=True, exist_ok=True)
    _write(versions / "VERSION", "0.5.0\n")

    cur = sb["edikt_home"] / "current"
    cur.symlink_to(Path("versions") / "0.5.0")
    (sb["edikt_home"] / "hooks").symlink_to(Path("current") / "hooks")
    (sb["edikt_home"] / "templates").symlink_to(Path("current") / "templates")

    _write(
        sb["edikt_home"] / "config.yaml",
        "edikt_version: \"0.5.0\"\nbase: docs\n",
    )

    # Fixture for v0.4.3.
    _write(
        sb["edikt_home"]
        / "migration-fixtures"
        / "v0.4.3"
        / "templates"
        / "agents"
        / "prompt_agent.md",
        TEMPLATE_CONTENT,
    )

    return sb


def run_doctor(sandbox: dict, stdin_input: str) -> subprocess.CompletedProcess:
    env = {
        **sandbox["env"],
        "EDIKT_MIGRATION_FIXTURES_BASE": str(
            sandbox["edikt_home"] / "migration-fixtures"
        ),
    }
    return subprocess.run(
        [str(EDIKT_BIN), "doctor", "--backfill-provenance"],
        env=env,
        capture_output=True,
        text=True,
        input=stdin_input,
        timeout=60,
    )


# ── Scenario A ───────────────────────────────────────────────────────────────

def test_prompt_fires_and_stamps_when_version_given(sandbox_base):
    """No version in frontmatter → prompt fires → user gives 0.4.3 → stamped."""
    sb = sandbox_base

    # Installed agent has NO edikt_version field.
    agent_path = sb["claude_home"] / "agents" / "prompt_agent.md"
    _write(agent_path, INSTALLED_CONTENT)

    # Provide "0.4.3" as stdin answer to the prompt.
    proc = run_doctor(sb, stdin_input="0.4.3\n")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    agent_text = agent_path.read_text()
    assert "edikt_template_hash:" in agent_text, "hash not stamped after prompt"
    assert "edikt_template_version:" in agent_text, "version not stamped after prompt"

    expected_hash = _md5(TEMPLATE_CONTENT)
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_hash:"):
            stamped = line.split(":", 1)[1].strip().strip('"')
            assert stamped == expected_hash, f"hash mismatch: {stamped} != {expected_hash}"
            break
    else:
        pytest.fail("edikt_template_hash not found")

    for line in agent_text.splitlines():
        if line.startswith("edikt_template_version:"):
            stamped_ver = line.split(":", 1)[1].strip().strip('"')
            assert stamped_ver == "0.4.3", f"version mismatch: {stamped_ver}"
            break
    else:
        pytest.fail("edikt_template_version not found")


def test_prompt_fires_and_emits_backfilled_event(sandbox_base):
    """provenance_backfilled event emitted after user provides version."""
    sb = sandbox_base
    agent_path = sb["claude_home"] / "agents" / "prompt_agent.md"
    _write(agent_path, INSTALLED_CONTENT)

    proc = run_doctor(sb, stdin_input="0.4.3\n")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    events_path = sb["edikt_home"] / "events.jsonl"
    assert events_path.exists(), "events.jsonl not created"
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    backfilled = [e for e in events if e.get("event") == "provenance_backfilled"]
    assert backfilled, "no provenance_backfilled event"
    assert backfilled[0].get("agent") == "prompt_agent"


# ── Scenario B ───────────────────────────────────────────────────────────────

def test_prompt_skip_input_leaves_file_unchanged(sandbox_base):
    """User types 'skip' at prompt → file unchanged + skipped event."""
    sb = sandbox_base
    agent_path = sb["claude_home"] / "agents" / "prompt_agent.md"
    _write(agent_path, INSTALLED_CONTENT)
    original = agent_path.read_text()

    proc = run_doctor(sb, stdin_input="skip\n")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    assert agent_path.read_text() == original, "file modified after 'skip'"
    assert "edikt_template_hash:" not in agent_path.read_text()

    events_path = sb["edikt_home"] / "events.jsonl"
    events = [json.loads(l) for l in events_path.read_text().splitlines() if l.strip()]
    skipped = [e for e in events if e.get("event") == "provenance_backfill_skipped"]
    assert skipped, "no provenance_backfill_skipped event"


# ── Scenario C: identical bytes across two versions ──────────────────────────

def test_identical_bytes_across_versions_prompts_disambiguation(sandbox_base):
    """Same template bytes at v0.1.0 and v0.1.4 → disambiguation prompt fires.

    The hash is the same for both versions. Whichever version the user picks,
    that value is written to edikt_template_version.
    """
    sb = sandbox_base

    # Add a second agent with fixtures in BOTH v0.1.0 and v0.1.4 (identical bytes).
    SHARED_TEMPLATE = textwrap.dedent(
        """\
        ---
        name: shared_agent
        description: "Identical template across v0.1.0 and v0.1.4"
        ---

        This content is identical across v0.1.0 and v0.1.4 templates.
        The backfill command must detect the duplicate and prompt the user
        to disambiguate which version to stamp in edikt_template_version.
        The hash value is the same regardless of which version is chosen.
        Additional content to make this template reasonably sized for testing.
        """
    )

    for ver in ("0.1.0", "0.1.4"):
        _write(
            sb["edikt_home"]
            / "migration-fixtures"
            / f"v{ver}"
            / "templates"
            / "agents"
            / "shared_agent.md",
            SHARED_TEMPLATE,
        )

    # Installed agent — identical to the shared template, no version hint.
    agent_path = sb["claude_home"] / "agents" / "shared_agent.md"
    _write(agent_path, SHARED_TEMPLATE)

    # User picks v0.1.0 (first prompt: which version installed; second: disambiguation)
    # The disambiguation prompt fires because both v0.1.0 and v0.1.4 share the same hash.
    # We provide: "0.1.0\n" for version hint, then "0.1.0\n" for disambiguation.
    proc = run_doctor(sb, stdin_input="0.1.0\n0.1.0\n")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    agent_text = agent_path.read_text()
    assert "edikt_template_hash:" in agent_text, "hash not stamped"
    assert "edikt_template_version:" in agent_text, "version not stamped"

    # Hash is the same regardless of version choice.
    expected_hash = _md5(SHARED_TEMPLATE)
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_hash:"):
            assert line.split(":", 1)[1].strip().strip('"') == expected_hash
            break

    # Version is 0.1.0 (user's pick).
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_version:"):
            stamped_ver = line.split(":", 1)[1].strip().strip('"')
            assert stamped_ver == "0.1.0", f"expected 0.1.0, got {stamped_ver!r}"
            break
    else:
        pytest.fail("edikt_template_version not found")


def test_identical_bytes_user_picks_other_version(sandbox_base):
    """Same template bytes — user picks the second version, that version is stamped."""
    sb = sandbox_base

    SHARED_TEMPLATE = textwrap.dedent(
        """\
        ---
        name: shared_agent2
        description: "Identical bytes — user picks second version"
        ---

        Identical content for disambiguation test (second variant).
        This template is byte-for-byte the same across v0.1.0 and v0.1.4.
        The user will pick v0.1.4 to confirm the version field is respected.
        More text to ensure template is large enough for stable thresholds.
        """
    )

    for ver in ("0.1.0", "0.1.4"):
        _write(
            sb["edikt_home"]
            / "migration-fixtures"
            / f"v{ver}"
            / "templates"
            / "agents"
            / "shared_agent2.md",
            SHARED_TEMPLATE,
        )

    agent_path = sb["claude_home"] / "agents" / "shared_agent2.md"
    _write(agent_path, SHARED_TEMPLATE)

    # User answers:
    #   version prompt → "0.1.0"  (initial version detection answer)
    #   disambiguation → "0.1.4"  (picks the other version)
    proc = run_doctor(sb, stdin_input="0.1.0\n0.1.4\n")
    assert proc.returncode == 0, proc.stderr + proc.stdout

    agent_text = agent_path.read_text()
    for line in agent_text.splitlines():
        if line.startswith("edikt_template_version:"):
            stamped_ver = line.split(":", 1)[1].strip().strip('"')
            assert stamped_ver == "0.1.4", f"expected 0.1.4, got {stamped_ver!r}"
            break
    else:
        pytest.fail("edikt_template_version not found")
