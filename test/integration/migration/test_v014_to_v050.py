"""v0.1.4 → v0.5.0 — same M2/M3/M5 chain as v0.1.0, but with a brainstorm-era config."""

from __future__ import annotations

from .helpers import build_synth_v014, run_migrate


def test_v014_full_migration_chain(sandbox_home):
    sb = sandbox_home
    build_synth_v014(sb)

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr

    # M2.
    claudemd = (sb["edikt_home"] / "CLAUDE.md").read_text()
    assert "[edikt:start]: #" in claudemd
    assert "<!-- edikt:start -->" not in claudemd

    # M3 — flat plan.md gone.
    flat_plan = sb["claude_home"] / "commands" / "edikt" / "plan.md"
    assert not flat_plan.exists()

    # M5.
    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    for key in ("paths:", "stack:", "gates:"):
        assert key in cfg, f"M5 missing key {key} in config.yaml after migrate"
    assert "edikt_version: 0.1.4" in cfg


def test_v014_user_modified_command_preserved(sandbox_home):
    sb = sandbox_home
    build_synth_v014(sb)
    # Overwrite the flat plan.md so its sha differs from the namespaced version.
    flat = sb["claude_home"] / "commands" / "edikt" / "plan.md"
    flat.write_text("# CUSTOM USER CONTENT — must not be deleted\n")

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr

    # Original file gone from the flat path.
    assert not flat.exists()
    # Preserved under custom/.
    custom = sb["edikt_home"] / "custom" / "plan.md"
    assert custom.exists()
    assert "CUSTOM USER CONTENT" in custom.read_text()
