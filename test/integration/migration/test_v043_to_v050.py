"""v0.4.3 → v0.5.0 — most-v0.5.0-like; only M4 should fire."""

from __future__ import annotations

from .helpers import build_synth_v043, run_migrate


def test_v043_only_m4(sandbox_home):
    sb = sandbox_home
    build_synth_v043(sb)

    cfg_before = (sb["edikt_home"] / "config.yaml").read_text()

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr

    # M2/M3/M5 no-ops.
    assert not (sb["edikt_home"] / "CLAUDE.md").exists()
    flat_dir = sb["claude_home"] / "commands" / "edikt"
    assert list(flat_dir.glob("*.md")) == []
    cfg_after = (sb["edikt_home"] / "config.yaml").read_text()
    assert cfg_after == cfg_before, "config.yaml unchanged"

    # M4 fires (governance.md present, no v2 sentinel).
    assert (sb["edikt_home"] / ".m4-pending").exists()
