"""v0.1.0 → v0.5.0 migration: M2 (sentinels) + M3 (flat commands) + M5 (config keys)."""

from __future__ import annotations

import json
from pathlib import Path

from conftest import build_synth_v010, run_migrate


def _read_events(edikt_home: Path) -> list[dict]:
    log = edikt_home / "events.jsonl"
    if not log.exists():
        return []
    return [json.loads(line) for line in log.read_text().splitlines() if line.strip()]


def test_v010_full_migration_chain(sandbox_home):
    sb = sandbox_home
    build_synth_v010(sb)

    # User-generated content under HOME — must be untouched after migrate.
    user_doc = sb["home"] / "project" / "docs" / "plans" / "PLAN-foo.md"
    user_doc.parent.mkdir(parents=True)
    user_doc.write_text("# my plan — DO NOT TOUCH\n")
    user_adr = sb["home"] / "project" / "docs" / "architecture" / "decisions" / "ADR-001.md"
    user_adr.parent.mkdir(parents=True)
    user_adr.write_text("# ADR-001\nStatus: Accepted\n")

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, f"migrate failed: stderr=\n{proc.stderr}\nstdout=\n{proc.stdout}"

    # M2: HTML sentinels rewritten.
    claudemd = (sb["edikt_home"] / "CLAUDE.md").read_text()
    assert "<!-- edikt:start -->" not in claudemd
    assert "<!-- edikt:end -->" not in claudemd
    assert "[edikt:start]: #" in claudemd
    assert "[edikt:end]: #" in claudemd
    # Content between markers preserved.
    assert "managed content goes here" in claudemd
    # Backup written.
    backups = list((sb["edikt_home"] / "backups").glob("migration-*"))
    assert backups, "expected migration backup dir"
    assert (backups[0] / "CLAUDE.md.pre-m2").exists()

    # M3: flat plan.md was unmodified — should be removed.
    flat_plan = sb["claude_home"] / "commands" / "edikt" / "plan.md"
    assert not flat_plan.exists(), "M3 should have removed the unmodified flat plan.md"

    # M5: config.yaml gained paths/stack/gates.
    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    assert "paths:" in cfg
    assert "stack:" in cfg
    assert "gates:" in cfg
    # Existing keys preserved.
    assert "edikt_version: 0.1.0" in cfg
    assert "base: docs" in cfg

    # M4: not detected (no governance.md present in this fixture).
    assert not (sb["edikt_home"] / ".m4-pending").exists()

    # User content untouched.
    assert user_doc.read_text() == "# my plan — DO NOT TOUCH\n"
    assert user_adr.read_text() == "# ADR-001\nStatus: Accepted\n"

    # Symlink chain intact.
    current = sb["edikt_home"] / "current"
    assert current.is_symlink()
    assert (sb["edikt_home"] / "hooks").is_symlink()
    assert (sb["edikt_home"] / "templates").is_symlink()

    # Events emitted for the steps that ran.
    events = _read_events(sb["edikt_home"])
    steps = {e.get("step") for e in events if e.get("event") == "migration_step_completed"}
    assert "M2" in steps
    assert "M3" in steps
    assert "M5" in steps


def test_v010_migration_idempotent(sandbox_home):
    sb = sandbox_home
    build_synth_v010(sb)

    proc1 = run_migrate(sb, "--yes")
    assert proc1.returncode == 0

    # Second run should be a no-op (each detection gate now fails).
    proc2 = run_migrate(sb, "--yes")
    assert proc2.returncode == 0
    assert "No migration needed" in proc2.stderr or "no signals detected" in proc2.stdout
