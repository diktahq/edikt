"""v0.2.0 → v0.5.0 — exercises M5 (gates only) and M4 stub."""

from __future__ import annotations

from .helpers import build_synth_v020, run_migrate


def test_v020_m5_gates_and_m4_pending(sandbox_home):
    sb = sandbox_home
    build_synth_v020(sb)

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr

    # M2 should NOT have run (no HTML sentinels in this fixture).
    claudemd = sb["edikt_home"] / "CLAUDE.md"
    assert not claudemd.exists()

    # M3 should NOT have run (no flat top-level commands seeded).
    flat_dir = sb["claude_home"] / "commands" / "edikt"
    assert list(flat_dir.glob("*.md")) == []

    # M5: only gates was missing → only gates added.
    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    assert "gates:" in cfg
    # Existing keys preserved unchanged.
    assert "edikt_version: 0.2.0" in cfg
    assert "stack: [go]" in cfg

    # M4: governance.md present without v2 sentinel → marker file written.
    pending = sb["edikt_home"] / ".m4-pending"
    assert pending.exists()


def test_v020_dry_run_describes_actions(sandbox_home):
    sb = sandbox_home
    build_synth_v020(sb)

    proc = run_migrate(sb, "--dry-run")
    assert proc.returncode == 0, proc.stderr
    out = proc.stdout + proc.stderr
    assert "M5" in out
    assert "M4" in out

    # Dry-run must NOT mutate disk.
    assert not (sb["edikt_home"] / ".m4-pending").exists()
    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    assert "gates:" not in cfg
