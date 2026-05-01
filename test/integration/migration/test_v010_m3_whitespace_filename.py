"""Hardening tests for Phase 7a review findings.

#1 — M3 must handle filenames containing whitespace (word-split safety).
#7 — Secondary migration failure must propagate as non-zero exit from
     cmd_migrate; the "migration complete" banner must NOT appear.
"""

from __future__ import annotations

import json
import os
import textwrap
from pathlib import Path

from .helpers import (
    _write,
    _versioned_seed,
    build_synth_v010,
    run_migrate,
    PAYLOAD_VERSION,
)


def _read_events(edikt_home: Path) -> list[dict]:
    log = edikt_home / "events.jsonl"
    if not log.exists():
        return []
    return [json.loads(line) for line in log.read_text().splitlines() if line.strip()]


# ─── Finding #1: M3 whitespace-safe filename handling ────────────────────────


def test_v010_m3_handles_spaces_in_filename_unmodified(sandbox_home):
    """A flat command with a space in its name that matches the payload is
    removed cleanly — no word-split corruption, no partial deletion.

    'my plan.md' content matches the namespaced replacement so M3 should
    delete it (unmodified path). Verifies the file survives intact until
    deletion and is fully gone afterwards.
    """
    sb = sandbox_home

    # Stage a post-M1 v0.1.0 layout (M2 + M5 will also fire).
    build_synth_v010(sb)

    # Add a flat command with a space in its name. Set its content to match the
    # namespaced replacement so M3 takes the "unmodified — delete" path.
    spaced_flat = sb["claude_home"] / "commands" / "edikt" / "my plan.md"
    # The namespaced replacement for "my plan.md" would need to exist under
    # current/commands/edikt/<subdir>/my plan.md. We add it to the payload.
    namespaced = (
        sb["edikt_home"]
        / "versions"
        / PAYLOAD_VERSION
        / "commands"
        / "edikt"
        / "sdlc"
        / "my plan.md"
    )
    namespaced.parent.mkdir(parents=True, exist_ok=True)
    namespaced.write_text("# sdlc/my plan (v0.5.0)\n")
    spaced_flat.write_text("# sdlc/my plan (v0.5.0)\n")

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, (
        f"migrate failed unexpectedly\nstderr:\n{proc.stderr}\nstdout:\n{proc.stdout}"
    )

    # The spaced file must be removed (unmodified — sha matched).
    assert not spaced_flat.exists(), (
        "'my plan.md' should have been removed by M3 (unmodified match)"
    )


def test_v010_m3_handles_spaces_in_filename_modified(sandbox_home):
    """A flat command with a space in its name that was user-modified is
    preserved under custom/ — it must never be deleted or corrupted.

    This is the critical safety assertion: user-renamed/modified files with
    spaces must survive the migration intact.
    """
    sb = sandbox_home

    build_synth_v010(sb)

    spaced_flat = sb["claude_home"] / "commands" / "edikt" / "my plan.md"
    # The namespaced replacement exists but with different content.
    namespaced = (
        sb["edikt_home"]
        / "versions"
        / PAYLOAD_VERSION
        / "commands"
        / "edikt"
        / "sdlc"
        / "my plan.md"
    )
    namespaced.parent.mkdir(parents=True, exist_ok=True)
    namespaced.write_text("# sdlc/my plan (v0.5.0)\n")
    # User-modified content — sha will NOT match.
    spaced_flat.write_text("# My custom plan command — do not delete\nsome extra content\n")

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, (
        f"migrate failed unexpectedly\nstderr:\n{proc.stderr}\nstdout:\n{proc.stdout}"
    )

    # Original flat location must be gone.
    assert not spaced_flat.exists(), (
        "'my plan.md' should have been moved out of the flat commands dir"
    )
    # Preserved under custom/ with the exact content intact.
    custom_file = sb["edikt_home"] / "custom" / "my plan.md"
    assert custom_file.exists(), (
        "'my plan.md' should be preserved under custom/ (user-modified)"
    )
    assert "My custom plan command" in custom_file.read_text(), (
        "preserved file content must be intact"
    )


# ─── Finding #7: Secondary migration failure propagation ─────────────────────


def test_secondary_migration_failure_propagates_non_zero(sandbox_home):
    """When a secondary migration (M2) cannot create its backup directory,
    cmd_migrate must exit non-zero and the 'Migration complete.' banner must
    NOT appear.

    Injection: create the backups/ directory with mode 000 so that
    _ensure_secondary_backup_dir cannot create its migration-<ts>-<pid>
    subdirectory inside it. M2 calls _ensure_secondary_backup_dir first and
    the error propagates up through _run_secondary_migrations.
    """
    sb = sandbox_home

    # Build a v0.1.0 layout that will trigger M2 + M3 + M5.
    build_synth_v010(sb)

    # Pre-create the backups dir with mode 000 so mkdir -p inside it fails.
    backups_dir = sb["edikt_home"] / "backups"
    backups_dir.mkdir(parents=True, exist_ok=True)
    backups_dir.chmod(0o000)

    proc = run_migrate(sb, "--yes")

    # Restore permissions so cleanup can proceed.
    backups_dir.chmod(0o755)

    # Migration must exit non-zero.
    assert proc.returncode != 0, (
        f"expected non-zero exit from migrate when backup dir creation fails, got 0\n"
        f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
    )

    # The top-level 'Migration complete.' success banner must NOT appear.
    assert "Migration complete." not in proc.stdout, (
        f"'Migration complete.' banner must NOT appear on partial failure\n"
        f"stdout:\n{proc.stdout}"
    )

    # The migration_partial_failure event must be written to events.jsonl.
    # Note: the events.jsonl may not exist if backups_dir chmod also blocked it,
    # but edikt_home itself is still writable so events.jsonl should be writable.
    events = _read_events(sb["edikt_home"])
    event_names = [e.get("event") for e in events]
    assert "migration_partial_failure" in event_names, (
        f"expected migration_partial_failure event; got events: {event_names}\n"
        f"stdout:\n{proc.stdout}\nstderr:\n{proc.stderr}"
    )


def test_secondary_migration_reruns_after_fix(sandbox_home):
    """After a transient failure (backup dir unwritable), fixing the condition
    and re-running 'edikt migrate' must succeed.
    """
    sb = sandbox_home

    build_synth_v010(sb)

    # First run: fail by making the backups dir unwritable.
    backups_dir = sb["edikt_home"] / "backups"
    backups_dir.mkdir(parents=True, exist_ok=True)
    backups_dir.chmod(0o000)
    proc1 = run_migrate(sb, "--yes")
    backups_dir.chmod(0o755)
    assert proc1.returncode != 0, (
        f"first run should fail\nstdout:\n{proc1.stdout}\nstderr:\n{proc1.stderr}"
    )

    # Second run: condition fixed — should succeed.
    proc2 = run_migrate(sb, "--yes")
    assert proc2.returncode == 0, (
        f"second run should succeed after fixing backup dir\n"
        f"stderr:\n{proc2.stderr}\nstdout:\n{proc2.stdout}"
    )

    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    assert "paths:" in cfg
    assert "stack:" in cfg
    assert "gates:" in cfg
