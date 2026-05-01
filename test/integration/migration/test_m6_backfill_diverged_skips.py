"""Phase 11 — M6 backfill: diverged file is skipped.

Scenario:
  - Agent installed at v0.4.3 (edikt_version in frontmatter, no hash)
  - Installed file has been heavily user-edited — content differs from the
    re-synthesized template by > 15% of installed file size (Levenshtein)
  - After backfill attempt: file is UNCHANGED (no hash stamped)
  - events.jsonl contains a provenance_backfill_skipped entry with the
    "user customizations exceed safe-backfill threshold" reason
"""

from __future__ import annotations

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


# ── Fixture template (small enough that user edits easily exceed 15%) ────────

TEMPLATE_CONTENT = textwrap.dedent(
    """\
    ---
    name: diverged_agent
    description: "Agent used for divergence test"
    edikt_version: "0.4.3"
    ---

    Original template content.
    This text will be heavily modified in the installed file.
    """
)

# Installed content replaces the body with completely different text.
# Levenshtein distance from re-synth ≈ len(BODY_DIFF) which is well over
# 15% of the installed file size for this small template.
INSTALLED_CONTENT = textwrap.dedent(
    """\
    ---
    name: diverged_agent
    description: "Agent used for divergence test"
    edikt_version: "0.4.3"
    ---

    COMPLETELY DIFFERENT CONTENT ADDED BY USER.
    This section was rewritten entirely and bears no resemblance to the
    original template. Many paragraphs of custom instructions follow.

    ## Custom section 1

    - Custom rule A: do this
    - Custom rule B: do that
    - Custom rule C: do something else entirely

    ## Custom section 2

    User has added extensive domain-specific guidance that was never in
    the original template. The Levenshtein distance from the re-synthesized
    template will exceed the 15%% safe-backfill threshold.

    More custom content to push the divergence well past the threshold.
    Even more text here to ensure we are safely over the 15%% limit.
    """
)


@pytest.fixture
def sandbox(sandbox_home):
    sb = sandbox_home

    # Minimal versioned layout.
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
        'edikt_version: "0.4.3"\nbase: docs\n',
    )

    # Fixture template.
    _write(
        sb["edikt_home"] / "migration-fixtures" / "v0.4.3" / "templates" / "agents" / "diverged_agent.md",
        TEMPLATE_CONTENT,
    )

    # Installed agent — heavily diverged from template.
    agent_path = sb["claude_home"] / "agents" / "diverged_agent.md"
    _write(agent_path, INSTALLED_CONTENT)

    return {**sb, "agent_path": agent_path}


def run_doctor(sandbox: dict) -> subprocess.CompletedProcess:
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
        timeout=60,
    )


def test_diverged_file_is_unchanged(sandbox):
    """Heavily diverged file must not receive any provenance frontmatter."""
    original = sandbox["agent_path"].read_text()

    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    after = sandbox["agent_path"].read_text()
    assert after == original, "file was modified despite divergence"
    assert "edikt_template_hash:" not in after
    assert "edikt_template_version:" not in after


def test_diverged_emits_skipped_event(sandbox):
    """provenance_backfill_skipped event with threshold reason written."""
    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    events_path = sandbox["edikt_home"] / "events.jsonl"
    assert events_path.exists(), "events.jsonl not created"

    events = [json.loads(line) for line in events_path.read_text().splitlines() if line.strip()]
    skipped = [e for e in events if e.get("event") == "provenance_backfill_skipped"]
    assert skipped, "no provenance_backfill_skipped event found"

    ev = skipped[0]
    assert ev.get("agent") == "diverged_agent"
    reason = ev.get("reason", "")
    assert "threshold" in reason or "customization" in reason, (
        f"unexpected skip reason: {reason!r}"
    )


def test_no_captured_template_emits_skipped(sandbox):
    """Agent with a version for which no fixture exists emits skipped event."""
    # Replace the installed agent to reference a version we have no fixture for.
    _write(
        sandbox["agent_path"],
        textwrap.dedent(
            """\
            ---
            name: diverged_agent
            description: "No fixture version"
            edikt_version: "0.9.9"
            ---

            Content does not matter.
            """
        ),
    )

    proc = run_doctor(sandbox)
    assert proc.returncode == 0, proc.stderr + proc.stdout

    events_path = sandbox["edikt_home"] / "events.jsonl"
    events = [json.loads(line) for line in events_path.read_text().splitlines() if line.strip()]
    skipped = [e for e in events if e.get("event") == "provenance_backfill_skipped"]
    assert skipped, "no provenance_backfill_skipped event"
    reason = skipped[0].get("reason", "")
    assert "no captured template" in reason, f"unexpected reason: {reason!r}"
