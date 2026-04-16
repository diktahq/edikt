"""v0.3.0 → v0.5.0 — same surface as v0.2.0 but with the v0.3.0 marker."""

from __future__ import annotations

from .helpers import build_synth_v030, run_migrate


def test_v030_m5_and_m4(sandbox_home):
    sb = sandbox_home
    build_synth_v030(sb)

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr

    cfg = (sb["edikt_home"] / "config.yaml").read_text()
    assert "edikt_version: 0.3.0" in cfg
    assert "gates:" in cfg

    assert (sb["edikt_home"] / ".m4-pending").exists()


def test_v030_idempotent_after_compile_marker(sandbox_home):
    sb = sandbox_home
    build_synth_v030(sb)

    # Pretend Phase 7b's compile invocation has already updated governance.md
    # by injecting the v2 frontmatter key — M4 must NOT re-fire on the second run.
    gov = sb["claude_home"] / "rules" / "governance.md"
    gov.write_text("---\ncompile_schema_version: 2\n---\n" + gov.read_text())

    proc = run_migrate(sb, "--yes")
    assert proc.returncode == 0, proc.stderr
    assert not (sb["edikt_home"] / ".m4-pending").exists()
